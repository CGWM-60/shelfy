package fileserver

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/infra/storage"
	"cgwm/shelfy/internal/testutil"
)

func TestListAndResolve(t *testing.T) {
	r := testutil.NewSQLiteRepo(t)
	filePath := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(filePath, []byte("sample"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	now := time.Now()
	if err := r.UpsertMediaItem(context.Background(), domain.MediaItem{ID: "m1", Title: "Sample", Kind: domain.MediaVideo, Path: filePath, MimeType: "video/mp4", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("upsert media: %v", err)
	}

	svc := NewService(r, storage.LocalFS{})
	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 entry got %d", len(list))
	}
	resolved, err := svc.Resolve(context.Background(), "m1")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.MimeType != "video/mp4" {
		t.Fatalf("mime=%s", resolved.MimeType)
	}
}

func TestAuthorized(t *testing.T) {
	req := httptest.NewRequest("GET", "/files/", nil)
	if Authorized(req, "", "") != true {
		t.Fatalf("expected public access")
	}
	if Authorized(req, "user", "pass") {
		t.Fatalf("expected unauthorized")
	}
	req.SetBasicAuth("user", "pass")
	if !Authorized(req, "user", "pass") {
		t.Fatalf("expected authorized")
	}
}
