package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"lan-share/internal/db"
	"lan-share/internal/model"
	"lan-share/internal/repository"
)

func TestShareServiceCreateAndListItems(t *testing.T) {
	root := t.TempDir()
	shareRoot := filepath.Join(root, "share")
	if err := os.MkdirAll(shareRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shareRoot, "notes.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewShareService(repository.NewShareRepository(database))
	share, err := svc.Create(context.Background(), CreateShareInput{
		Alias: "docs", Path: shareRoot, Permission: model.SharePermissionReadOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	if share.Alias != "docs" || !share.Enabled {
		t.Fatalf("unexpected share: %+v", share)
	}
	items, err := svc.ListItems(context.Background(), "docs", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "notes.txt" {
		t.Fatalf("unexpected items: %+v", items)
	}
	if _, _, err := svc.ResolveFile(context.Background(), "docs", "../outside"); err != nil {
		t.Fatalf("clean relative path should remain inside share: %v", err)
	}
}
