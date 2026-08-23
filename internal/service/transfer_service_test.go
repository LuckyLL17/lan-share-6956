package service

import (
	"context"
	"database/sql"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lan-share/config"
	"lan-share/internal/repository"

	_ "modernc.org/sqlite"
)

// newTestService constructs an isolated TransferService backed by a
// per-test in-memory SQLite database.
func newTestService(t *testing.T, downloadDir string) (*TransferService, *repository.TransferRepository) {
	t.Helper()
	dsn := "file::memory:?cache=shared"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`PRAGMA journal_mode(WAL); PRAGMA foreign_keys(1);`); err != nil {
		t.Fatalf("set pragmas: %v", err)
	}
	if err := migrateTransfers(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewTransferRepository(database)
	cfg := &config.Config{
		Storage: config.StorageConfig{DownloadDir: downloadDir},
		Network: config.NetworkConfig{FileServerPort: 0},
	}
	return NewTransferService(cfg, repo), repo
}

// migrateTransfers creates only the transfers table for test isolation.
func migrateTransfers(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS transfers (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			direction         TEXT NOT NULL,
			peer_device_id     TEXT NOT NULL DEFAULT '',
			peer_ip           TEXT NOT NULL,
			peer_port         INTEGER NOT NULL DEFAULT 8765,
			share_alias       TEXT DEFAULT '',
			remote_path       TEXT DEFAULT '',
			local_path        TEXT DEFAULT '',
			file_name         TEXT NOT NULL,
			file_size         INTEGER NOT NULL DEFAULT 0,
			bytes_transferred INTEGER NOT NULL DEFAULT 0,
			status            TEXT NOT NULL DEFAULT 'queued',
			err_msg           TEXT DEFAULT '',
			created_at        DATETIME NOT NULL,
			updated_at        DATETIME NOT NULL,
			finished_at       DATETIME
		);`)
	return err
}

// slowHandler streams a fixed payload byte-by-byte with a small delay so
// callers can pause mid-transfer.
func slowHandler(size int, chunkDelay time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", itoa(size))
		flusher, _ := w.(http.Flusher)
		buf := make([]byte, 1)
		for i := 0; i < size; i++ {
			buf[0] = 'a'
			if _, err := w.Write(buf); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(chunkDelay)
		}
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// TestTransferPauseKeepsPausedState reproduces the regression where pausing a
// large in-flight transfer ended up marked as failed while progress was
// preserved but no longer resumable. After the fix, the paused transfer
// must remain paused with bytes_transferred preserved and resumable.
func TestTransferPauseKeepsPausedState(t *testing.T) {
	dir := t.TempDir()

	// 2 KiB payload at 1 byte / 5ms → ~10s total. Plenty of time to pause.
	srv := httptest.NewServer(slowHandler(2048, 5*time.Millisecond))
	defer srv.Close()

	host, port := hostPort(t, srv.URL)
	svc, repo := newTestService(t, dir)
	ctx := context.Background()

	localPath := filepath.Join(dir, "big.bin")
	tr, err := svc.Create(ctx, CreateTransferInput{
		Direction:  "download",
		PeerIP:     host,
		PeerPort:   port,
		ShareAlias: "any",
		RemotePath: "big.bin",
		FileName:   "big.bin",
		FileSize:   2048,
		LocalPath:  localPath,
		StartNow:   true,
	})
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	if !waitForProgress(t, repo, tr.ID, 1, 3*time.Second) {
		t.Fatalf("transfer never started")
	}

	if err := svc.Pause(ctx, tr.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}

	// Allow run() to observe cancellation and exit.
	time.Sleep(200 * time.Millisecond)

	current, err := repo.Get(ctx, tr.ID)
	if err != nil || current == nil {
		t.Fatalf("get after pause: %v", err)
	}
	if current.Status != "paused" {
		t.Fatalf("paused transfer changed to failed: status=%q err=%q bytes=%d",
			current.Status, current.ErrMsg, current.BytesTransferred)
	}
	if current.BytesTransferred == 0 {
		t.Fatalf("paused transfer lost progress: bytes=%d", current.BytesTransferred)
	}
	if !current.IsResumable() {
		t.Fatalf("paused transfer not resumable: bytes=%d/%d",
			current.BytesTransferred, current.FileSize)
	}
}

// TestTransferRealErrorReportedAsFailed ensures genuine network failures are
// still surfaced as failed (the fix must not swallow real errors).
func TestTransferRealErrorReportedAsFailed(t *testing.T) {
	dir := t.TempDir()

	svc, repo := newTestService(t, dir)
	ctx := context.Background()

	// Allocate a port and immediately release it to force connection refused.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().(*net.TCPAddr)
	_ = l.Close()

	tr, err := svc.Create(ctx, CreateTransferInput{
		Direction:  "download",
		PeerIP:     "127.0.0.1",
		PeerPort:   addr.Port,
		ShareAlias: "any",
		RemotePath: "x.bin",
		FileName:   "x.bin",
		FileSize:   16,
		LocalPath:  filepath.Join(dir, "x.bin"),
		StartNow:   true,
	})
	if err != nil {
		t.Fatalf("create transfer: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cur, err := repo.Get(ctx, tr.ID)
		if err != nil || cur == nil {
			t.Fatalf("get: %v", err)
		}
		if cur.Status == "failed" {
			if strings.TrimSpace(cur.ErrMsg) == "" {
				t.Fatalf("failed transfer missing err_msg")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("transfer did not reach failed status")
}

func waitForProgress(t *testing.T, repo *repository.TransferRepository, id int64, minBytes int64, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cur, err := repo.Get(context.Background(), id)
		if err == nil && cur != nil && cur.BytesTransferred >= minBytes {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func hostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	u := strings.TrimPrefix(rawURL, "http://")
	host, portStr, ok := strings.Cut(u, ":")
	if !ok {
		t.Fatalf("bad srv.URL %q", rawURL)
	}
	var port int
	for _, c := range portStr {
		if c < '0' || c > '9' {
			t.Fatalf("bad port in %q", rawURL)
		}
		port = port*10 + int(c-'0')
	}
	return host, port
}
