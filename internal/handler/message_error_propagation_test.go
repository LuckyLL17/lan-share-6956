package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"lan-share/config"
	"lan-share/internal/db"
	"lan-share/internal/network"
	"lan-share/internal/repository"
	"lan-share/internal/service"
)

// The request path crosses internal/handler/message_handler.go,
// internal/service/discover_service.go, and internal/network/udp.go.
func TestMessageSendFailureReachesHTTPResponse(t *testing.T) {
	root := t.TempDir()
	database, err := db.Open(filepath.Join(root, "messages.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cfg := &config.Config{
		Device:  config.DeviceConfig{Name: "message-test"},
		Network: config.NetworkConfig{UDPDiscoverPort: 18765},
	}
	udp := network.NewUDPDiscovery(cfg.Network.UDPDiscoverPort, cfg.Device.Name)
	discoverSvc := service.NewDiscoverService(
		cfg,
		udp,
		repository.NewDeviceRepository(database),
		repository.NewMessageRepository(database),
	)
	messageHandler := NewMessageHandler(repository.NewMessageRepository(database), discoverSvc)
	router := gin.New()
	router.POST("/api/v1/messages", messageHandler.Create)

	payload, err := json.Marshal(map[string]string{
		"to_ip":   "192.0.2.1",
		"content": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code < http.StatusBadRequest {
		t.Fatalf("unreachable UDP target returned success: status=%d body=%s", rec.Code, rec.Body.String())
	}
}
