package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"lan-share/config"
	"lan-share/internal/db"
	"lan-share/internal/network"
	"lan-share/internal/repository"
)

func TestStartupFailureCleanupDoesNotHang(t *testing.T) {
	udp := network.NewUDPDiscovery(0, "startup-test")
	assertReturns(t, "UDP close", func() { udp.Close() })

	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := &config.Config{
		Device:  config.DeviceConfig{Name: "startup-test"},
		Network: config.NetworkConfig{UDPDiscoverPort: 0, FileServerPort: 8765},
	}
	svc := NewDiscoverService(
		cfg,
		network.NewUDPDiscovery(0, cfg.Device.Name),
		repository.NewDeviceRepository(database),
		repository.NewMessageRepository(database),
	)
	_ = context.Background()
	assertReturns(t, "discover service stop", svc.Stop)
}

func assertReturns(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("%s hung during startup cleanup", name)
	}
}
