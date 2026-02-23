package fileserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"cgwm/shelfy/internal/infra/storage"
	"cgwm/shelfy/internal/testutil"
)

func TestManagedOperationsLifecycle(t *testing.T) {
	root := t.TempDir()
	r := testutil.NewSQLiteRepo(t)
	svc := NewService(r, storage.LocalFS{})

	dirPath := filepath.Join(root, "nested")
	if err := svc.CreateManagedDir(context.Background(), []string{root}, dirPath); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if _, err := os.Stat(dirPath); err != nil {
		t.Fatalf("stat created dir: %v", err)
	}

	filePath := filepath.Join(dirPath, "a.txt")
	if err := svc.WriteManagedFile(context.Background(), []string{root}, filePath, []byte("hello")); err != nil {
		t.Fatalf("write file: %v", err)
	}
	content, err := svc.ReadManagedFile(context.Background(), []string{root}, filePath, 1024)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if content != "hello" {
		t.Fatalf("unexpected content: %q", content)
	}

	targetPath := filepath.Join(root, "moved", "renamed.txt")
	if err := svc.MoveManagedPath(context.Background(), []string{root}, filePath, targetPath); err != nil {
		t.Fatalf("move file: %v", err)
	}
	if _, err := os.Stat(targetPath); err != nil {
		t.Fatalf("stat moved file: %v", err)
	}

	if err := svc.DeleteManagedPath(context.Background(), []string{root}, filepath.Join(root, "moved"), false); err == nil {
		t.Fatalf("expected recursive requirement for folder")
	}
	if err := svc.DeleteManagedPath(context.Background(), []string{root}, filepath.Join(root, "moved"), true); err != nil {
		t.Fatalf("delete folder recursive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "moved")); !os.IsNotExist(err) {
		t.Fatalf("expected moved folder removed, err=%v", err)
	}
}

func TestManagedOperationsRejectOutsideRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	r := testutil.NewSQLiteRepo(t)
	svc := NewService(r, storage.LocalFS{})

	if err := svc.WriteManagedFile(context.Background(), []string{root}, filepath.Join(outside, "x.txt"), []byte("x")); err == nil {
		t.Fatalf("expected path guard error")
	}
}

func TestManagedOperationsAcceptRelativePathFromRoot(t *testing.T) {
	root := t.TempDir()
	r := testutil.NewSQLiteRepo(t)
	svc := NewService(r, storage.LocalFS{})

	if err := svc.WriteManagedFile(context.Background(), []string{root}, filepath.Join("docs", "note.txt"), []byte("hello")); err != nil {
		t.Fatalf("expected relative path accepted: %v", err)
	}
	target := filepath.Join(root, "docs", "note.txt")
	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read relative target: %v", err)
	}
	if string(content) != "hello" {
		t.Fatalf("unexpected content: %q", string(content))
	}
}

func TestListManagedPathAcceptsRelativeCurrentPathMatchingRoot(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "data", "downloads")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "sample.bin"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	r := testutil.NewSQLiteRepo(t)
	svc := NewService(r, storage.LocalFS{})
	listing, err := svc.ListManagedPath(context.Background(), []string{root}, filepath.Join("data", "downloads"))
	if err != nil {
		t.Fatalf("list with relative current path: %v", err)
	}
	expectedRoot := root
	if resolved, resolveErr := filepath.EvalSymlinks(root); resolveErr == nil {
		expectedRoot = resolved
	}
	if listing.Path != expectedRoot {
		t.Fatalf("unexpected listing path: got=%q want=%q", listing.Path, expectedRoot)
	}
	if len(listing.Items) != 1 || listing.Items[0].Name != "sample.bin" {
		t.Fatalf("unexpected listing items: %+v", listing.Items)
	}
}
