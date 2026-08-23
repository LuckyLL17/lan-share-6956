package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"lan-share/config"
	"lan-share/internal/network"
	"lan-share/internal/repository"

	_ "modernc.org/sqlite"
)

// testCfg 构造一份测试用配置，使用随机端口避免冲突。
func testCfg(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		Device:  config.DeviceConfig{Name: "test-device", AutoReceive: true},
		Server:  config.ServerConfig{Host: "127.0.0.1", Port: 0, MaxUploadMB: 1},
		Network: config.NetworkConfig{UDPDiscoverPort: 0, FileServerPort: 0, DiscoverInterval: 5, DeviceTTL: 15},
		Storage: config.StorageConfig{},
	}
}

// testDB 打开一个临时 SQLite 数据库并执行迁移。
// 避开 db.Open 的 sync.Once，便于每个测试独立隔离。
func testDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	d, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := migrateForTest(d); err != nil {
		d.Close()
		t.Fatalf("migrate: %v", err)
	}
	return d, func() { _ = d.Close(); _ = os.Remove(path) }
}

func migrateForTest(d *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS devices (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			ip           TEXT NOT NULL,
			port         INTEGER NOT NULL DEFAULT 8765,
			status       TEXT NOT NULL DEFAULT 'online',
			first_seen   DATETIME NOT NULL,
			last_seen    DATETIME NOT NULL,
			os           TEXT DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS messages (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			from_ip     TEXT NOT NULL,
			from_name   TEXT NOT NULL DEFAULT '',
			to_ip       TEXT DEFAULT '',
			content     TEXT NOT NULL,
			read        INTEGER NOT NULL DEFAULT 0,
			created_at  DATETIME NOT NULL
		);`,
	}
	for _, s := range stmts {
		if _, err := d.Exec(s); err != nil {
			return fmt.Errorf("migrate stmt failed: %s: %w", s, err)
		}
	}
	return nil
}

// TestDiscoverService_StopWithoutStart 验证未启动场景：Start 从未调用，Stop 不应阻塞。
// 回归 bug：Stop 会卡在 <-s.done，因为 broadcastLoop 从未启动。
func TestDiscoverService_StopWithoutStart(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	devRepo := repository.NewDeviceRepository(db)
	msgRepo := repository.NewMessageRepository(db)
	udp := network.NewUDPDiscovery(0, "test")
	svc := NewDiscoverService(testCfg(t), udp, devRepo, msgRepo)

	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("DiscoverService.Stop hung when Start was never called")
	}
}

// TestDiscoverService_StopAfterStartFailure 验证启动失败清理场景。
// Start 因 UDP 端口冲突失败后，Stop 不应阻塞。
func TestDiscoverService_StopAfterStartFailure(t *testing.T) {
	// 先占用一个 UDP 端口，使后续 Listen 失败。
	occupier := network.NewUDPDiscovery(0, "occupier")
	if err := occupier.Listen(); err != nil {
		t.Fatalf("occupier listen: %v", err)
	}
	defer occupier.Close()
	port := occupier.LocalPort()

	db, cleanup := testDB(t)
	defer cleanup()
	devRepo := repository.NewDeviceRepository(db)
	msgRepo := repository.NewMessageRepository(db)
	udp := network.NewUDPDiscovery(port, "test")
	svc := NewDiscoverService(testCfg(t), udp, devRepo, msgRepo)

	if err := svc.Start(context.Background()); err == nil {
		svc.Stop()
		t.Fatalf("expected Start failure on occupied UDP port %d", port)
	}
	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("DiscoverService.Stop hung during startup cleanup")
	}
}

// TestDiscoverService_NormalLifecycle 验证正常运行场景：Start 成功后 Stop 在有限时间结束。
func TestDiscoverService_NormalLifecycle(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	devRepo := repository.NewDeviceRepository(db)
	msgRepo := repository.NewMessageRepository(db)
	udp := network.NewUDPDiscovery(0, "test")
	svc := NewDiscoverService(testCfg(t), udp, devRepo, msgRepo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("DiscoverService.Stop hung after successful Start")
	}
}

// TestDiscoverService_StopIdempotent 验证 Stop 多次调用安全。
func TestDiscoverService_StopIdempotent(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	devRepo := repository.NewDeviceRepository(db)
	msgRepo := repository.NewMessageRepository(db)
	udp := network.NewUDPDiscovery(0, "test")
	svc := NewDiscoverService(testCfg(t), udp, devRepo, msgRepo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	for i := 0; i < 3; i++ {
		svc.Stop()
	}
}
