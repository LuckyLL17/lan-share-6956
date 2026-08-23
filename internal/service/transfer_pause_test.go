package service_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"lan-share/config"
	"lan-share/internal/db"
	"lan-share/internal/handler"
	"lan-share/internal/model"
	"lan-share/internal/repository"
	"lan-share/internal/service"
)

// The scenario crosses internal/handler/transfer_handler.go,
// internal/service/transfer_service.go, internal/repository/transfer_repo.go,
// and internal/model/transfer.go.
func TestTransferPauseKeepsPausedState(t *testing.T) {
	root := t.TempDir()
	database, err := db.Open(filepath.Join(root, "pause.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cfg := &config.Config{
		Device:  config.DeviceConfig{Name: "pause-test"},
		Network: config.NetworkConfig{FileServerPort: 8765},
		Storage: config.StorageConfig{DownloadDir: root},
	}
	repo := repository.NewTransferRepository(database)
	svc := service.NewTransferService(cfg, repo)
	transferHandler := handler.NewTransferHandler(svc)
	router := gin.New()
	router.POST("/api/v1/transfers/:id/pause", transferHandler.Pause)
	router.GET("/api/v1/transfers/:id/progress", transferHandler.Progress)

	initial := filepath.Join(root, "partial.bin")
	if err := os.WriteFile(initial, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	remote := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("d")); err != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(started)
		<-release
		_, _ = w.Write([]byte("efg"))
	}))
	remote.Listener, err = net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	remote.Start()
	defer remote.Close()
	remotePort := remote.Listener.Addr().(*net.TCPAddr).Port

	transfer := &model.Transfer{
		Direction:        model.TransferDirectionDownload,
		PeerIP:           "127.0.0.1",
		PeerPort:         remotePort,
		RemotePath:       "large.bin",
		LocalPath:        initial,
		FileName:         "large.bin",
		FileSize:         7,
		BytesTransferred: 3,
		Status:           model.TransferStatusQueued,
	}
	if err := repo.Create(context.Background(), transfer); err != nil {
		t.Fatal(err)
	}

	if err := svc.Start(context.Background(), transfer.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("transfer did not reach the pausable read")
	}

	pauseReq := httptest.NewRequest(http.MethodPost, "/api/v1/transfers/"+itoa(transfer.ID)+"/pause", nil)
	pauseRec := httptest.NewRecorder()
	router.ServeHTTP(pauseRec, pauseReq)
	if pauseRec.Code != http.StatusOK {
		t.Fatalf("pause status = %d, body=%s", pauseRec.Code, pauseRec.Body.String())
	}
	close(release)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := repo.Get(context.Background(), transfer.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if current != nil && current.Status != model.TransferStatusRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	current, err := repo.Get(context.Background(), transfer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current == nil {
		t.Fatal("transfer record disappeared")
	}
	if current.Status != model.TransferStatusPaused {
		t.Fatalf("paused transfer changed to %s", current.Status)
	}
	if current.BytesTransferred < 3 {
		t.Fatalf("pause lost progress: got %d bytes", current.BytesTransferred)
	}

	progressReq := httptest.NewRequest(http.MethodGet, "/api/v1/transfers/"+itoa(transfer.ID)+"/progress", nil)
	progressRec := httptest.NewRecorder()
	router.ServeHTTP(progressRec, progressReq)
	if progressRec.Code != http.StatusOK || !strings.Contains(progressRec.Body.String(), `"status":"paused"`) {
		t.Fatalf("progress endpoint did not preserve pause: status=%d body=%s", progressRec.Code, progressRec.Body.String())
	}
}

func TestTransferRealFailureReported(t *testing.T) {
	root := t.TempDir()
	database, err := db.Open(filepath.Join(root, "failure.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cfg := &config.Config{
		Device:  config.DeviceConfig{Name: "failure-test"},
		Network: config.NetworkConfig{FileServerPort: 8765},
		Storage: config.StorageConfig{DownloadDir: root},
	}
	repo := repository.NewTransferRepository(database)
	svc := service.NewTransferService(cfg, repo)

	remote := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "remote unavailable", http.StatusBadGateway)
	}))
	remote.Listener, err = net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	remote.Start()
	defer remote.Close()
	remotePort := remote.Listener.Addr().(*net.TCPAddr).Port

	transfer := &model.Transfer{
		Direction:  model.TransferDirectionDownload,
		PeerIP:     "127.0.0.1",
		PeerPort:   remotePort,
		RemotePath: "missing.bin",
		LocalPath:  filepath.Join(root, "missing.bin"),
		FileName:   "missing.bin",
		FileSize:   10,
		Status:     model.TransferStatusQueued,
	}
	if err := repo.Create(context.Background(), transfer); err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background(), transfer.ID); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, getErr := repo.Get(context.Background(), transfer.ID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if current != nil && current.Status == model.TransferStatusFailed {
			if !strings.Contains(current.ErrMsg, "download status 502") {
				t.Fatalf("failure reason was not preserved: %q", current.ErrMsg)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("real transfer failure was not reported")
}

func itoa(id int64) string {
	const digits = "0123456789"
	if id == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for id > 0 {
		buf = append(buf, digits[id%10])
		id /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
