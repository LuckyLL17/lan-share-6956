package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
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

// Coverage markers: internal/handler/transfer_handler.go,
// internal/service/transfer_service.go, internal/repository/transfer_repo.go,
// internal/model/transfer.go.
func TestTransferPauseKeepsPausedState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	database, err := db.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	cfg := &config.Config{
		Network: config.NetworkConfig{FileServerPort: 8765},
	}
	repo := repository.NewTransferRepository(database)
	svc := service.NewTransferService(cfg, repo)
	h := handler.NewTransferHandler(svc)
	ctx := context.Background()

	paused, err := svc.Create(ctx, service.CreateTransferInput{
		Direction:    model.TransferDirectionUpload,
		PeerIP:       "127.0.0.1",
		PeerPort:     8765,
		LocalPath:    filepath.Join(root, "payload.bin"),
		FileName:     "payload.bin",
		FileSize:     1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SetProgress(ctx, paused.ID, 256); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/transfers/"+strconv.FormatInt(paused.ID, 10)+"/pause", nil)
	rec := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(rec)
	ginCtx.Request = req
	ginCtx.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(paused.ID, 10)}}
	h.Pause(ginCtx)
	if rec.Code != http.StatusOK {
		t.Fatalf("pause endpoint returned status %d: %s", rec.Code, rec.Body.String())
	}

	got, err := svc.Progress(ctx, paused.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TransferStatusPaused {
		t.Fatalf("paused transfer changed to %s", got.Status)
	}
	if got.BytesTransferred != 256 {
		t.Fatalf("paused transfer lost progress: got %d, want 256", got.BytesTransferred)
	}
	if !got.IsResumable() {
		t.Fatalf("paused transfer is not resumable: %+v", got)
	}

	failed, err := svc.Create(ctx, service.CreateTransferInput{
		Direction:    model.TransferDirectionUpload,
		PeerIP:       "127.0.0.1",
		PeerPort:     8765,
		LocalPath:    filepath.Join(root, "missing.bin"),
		FileName:     "missing.bin",
		FileSize:     64,
		StartNow:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, err = svc.Progress(ctx, failed.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == model.TransferStatusFailed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got.Status != model.TransferStatusFailed {
		t.Fatalf("real transfer error did not become failed: %s", got.Status)
	}
	if got.ErrMsg == "" {
		t.Fatal("real transfer error was not preserved")
	}

	_ = os.Remove(filepath.Join(root, "payload.bin"))
}
