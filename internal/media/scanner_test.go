package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cgwm/shelfy/internal/events"
	"cgwm/shelfy/internal/testutil"
)

func TestScanPrunesDeletedFiles(t *testing.T) {
	r := testutil.NewSQLiteRepo(t)
	root := t.TempDir()
	filePath := filepath.Join(root, "movie.mp4")
	if err := os.WriteFile(filePath, []byte("movie"), 0o644); err != nil {
		t.Fatalf("write media file: %v", err)
	}

	scanner := NewScanner(r, OSWalker{}, events.NewBus())
	count, err := scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 file on first scan, got %d", count)
	}
	items, err := r.ListMedia(context.Background(), "", "")
	if err != nil {
		t.Fatalf("list media after first scan: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 media row, got %d", len(items))
	}

	if err := os.Remove(filePath); err != nil {
		t.Fatalf("remove media file: %v", err)
	}
	count, err = scanner.Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 scanned file after deletion, got %d", count)
	}
	items, err = r.ListMedia(context.Background(), "", "")
	if err != nil {
		t.Fatalf("list media after second scan: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected media rows pruned, got %d", len(items))
	}
}
