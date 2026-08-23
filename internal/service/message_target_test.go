package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"lan-share/config"
	"lan-share/internal/model"
	"lan-share/internal/network"
	"lan-share/internal/repository"
)

func TestTargetedUDPMessageDoesNotPolluteLocalInbox(t *testing.T) {
	database, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "messages.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`
		CREATE TABLE messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_ip TEXT NOT NULL,
			from_name TEXT NOT NULL DEFAULT '',
			to_ip TEXT DEFAULT '',
			content TEXT NOT NULL,
			read INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL
		);
	`); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{Device: config.DeviceConfig{Name: "local-device"}}
	svc := NewDiscoverService(
		cfg,
		network.NewUDPDiscovery(0, cfg.Device.Name),
		repository.NewDeviceRepository(database),
		repository.NewMessageRepository(database),
	)

	message := model.Message{
		FromIP:   "198.51.100.10",
		FromName: "remote-device",
		ToIP:     "203.0.113.77",
		Content:  "not for this device",
	}
	svc.handleMessagePacket(network.MessagePacket{
		FromIP:   message.FromIP,
		FromName: message.FromName,
		ToIP:     message.ToIP,
		Content:  message.Content,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	messages, err := svc.msgRepo.List(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	unread, err := svc.msgRepo.CountUnread(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 || unread != 0 {
		t.Fatalf("targeted message persisted: messages=%d unread=%d", len(messages), unread)
	}
}
