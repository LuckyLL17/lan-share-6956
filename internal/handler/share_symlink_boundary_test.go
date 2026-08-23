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

func TestShareSymlinkCannotEscapeRoot(t *testing.T) {
	root := t.TempDir()
	shareRoot := filepath.Join(root, "share")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(shareRoot, "shortcut")); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := service.NewShareService(repository.NewShareRepository(database))
	if _, err := svc.Create(context.Background(), service.CreateShareInput{
		Alias: "docs", Path: shareRoot, Permission: model.SharePermissionReadWrite,
	}); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	fileHandler := NewFileHandler(svc, service.NewPreviewService())
	r.GET("/files/:alias/*path", fileHandler.Browse)
	r.POST("/api/v1/uploads/:alias", fileHandler.Upload)

	readReq := httptest.NewRequest(http.MethodGet, "/files/docs/shortcut/secret.txt", nil)
	readRec := httptest.NewRecorder()
	r.ServeHTTP(readRec, readReq)
	if readRec.Code == http.StatusOK {
		t.Fatalf("symlink outside shared root was served: %s", readRec.Body.String())
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "escaped.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("escaped"))
	_ = writer.WriteField("path", "shortcut/escaped.txt")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	uploadReq := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/docs", &body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRec := httptest.NewRecorder()
	r.ServeHTTP(uploadRec, uploadReq)
	if uploadRec.Code == http.StatusOK {
		t.Fatalf("symlink outside shared root was accepted: %s", uploadRec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(outside, "escaped.txt")); err == nil {
		t.Fatal("upload created a file outside shared root")
	}
}
