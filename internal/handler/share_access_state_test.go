package handler

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"lan-share/internal/db"
	"lan-share/internal/model"
	"lan-share/internal/repository"
	"lan-share/internal/service"
)

func TestDisabledShareStopsAllAccess(t *testing.T) {
	root := t.TempDir()
	shareRoot := filepath.Join(root, "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shareRoot, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	database, err := db.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	enabled := false
	svc := service.NewShareService(repository.NewShareRepository(database))
	if _, err := svc.Create(context.Background(), service.CreateShareInput{
		Alias: "docs", Path: shareRoot, Permission: model.SharePermissionReadWrite, Enabled: &enabled,
	}); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	fileHandler := NewFileHandler(svc, service.NewPreviewService())
	r.GET("/files/:alias/*path", fileHandler.Browse)
	r.POST("/api/v1/uploads/:alias", fileHandler.Upload)

	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/files/docs/", nil))
	if listRec.Code == http.StatusOK {
		t.Fatalf("disabled share remained browsable: %s", listRec.Body.String())
	}

	readRec := httptest.NewRecorder()
	r.ServeHTTP(readRec, httptest.NewRequest(http.MethodGet, "/files/docs/secret.txt", nil))
	if readRec.Code == http.StatusOK {
		t.Fatalf("disabled share remained readable: %s", readRec.Body.String())
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "new.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("new"))
	_ = writer.WriteField("path", "new.txt")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/docs", &body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRec := httptest.NewRecorder()
	r.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code == http.StatusOK {
		t.Fatalf("disabled share remained writable: %s", uploadRec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(shareRoot, "new.txt")); err == nil {
		t.Fatal("disabled share accepted an upload")
	}
}
