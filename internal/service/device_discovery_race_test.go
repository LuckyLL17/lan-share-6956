package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"lan-share/config"
	"lan-share/internal/db"
	"lan-share/internal/network"
	"lan-share/internal/repository"
)

// This exercises internal/handler/device_handler.go ->
// internal/service/discover_service.go -> internal/network/udp.go.
func TestConcurrentDeviceDiscoveryCache(t *testing.T) {
	root := t.TempDir()
	database, err := db.Open(filepath.Join(root, "devices.db"))
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Device:  config.DeviceConfig{Name: "cache-test"},
		Network: config.NetworkConfig{UDPDiscoverPort: 18765, FileServerPort: 8765},
	}
	devRepo := repository.NewDeviceRepository(database)
	msgRepo := repository.NewMessageRepository(database)
	udp := network.NewUDPDiscovery(cfg.Network.UDPDiscoverPort, cfg.Device.Name)
	svc := NewDiscoverService(cfg, udp, devRepo, msgRepo)

	const writers = 8
	const readers = 16
	const devicesPerWriter = 4
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for w := 0; w < writers; w++ {
		w := w
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < devicesPerWriter; i++ {
				svc.handleDiscoveryPacket(network.DiscoverPacket{
					Type: "discover",
					Name: "peer-" + strconv.Itoa(w) + "-" + strconv.Itoa(i),
					IP:   "10.0." + strconv.Itoa(w%250) + "." + strconv.Itoa(i+1),
					Port: 8765,
					OS:   "test",
				})
			}
		}()
	}
	for i := 0; i < readers; i++ {
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < devicesPerWriter; j++ {
				if _, err := svc.ListDevices(context.Background()); err != nil {
					t.Errorf("device list failed: %v", err)
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	list, err := svc.ListDevices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := writers * devicesPerWriter
	if len(list) != want {
		data, _ := json.Marshal(list)
		t.Fatalf("device cache contains %d entries, want %d: %s", len(list), want, data)
	}
}
