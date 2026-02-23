package fileserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ManageFS interface {
	Stat(name string) (fs.FileInfo, error)
	ReadDir(name string) ([]os.DirEntry, error)
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(name string, data []byte, perm os.FileMode) error
	ReadFile(name string) ([]byte, error)
	Rename(oldPath, newPath string) error
	Remove(name string) error
	RemoveAll(path string) error
}

type localManageFS struct{}

func (localManageFS) Stat(name string) (fs.FileInfo, error)        { return os.Stat(name) }
func (localManageFS) ReadDir(name string) ([]os.DirEntry, error)   { return os.ReadDir(name) }
func (localManageFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (localManageFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	return os.WriteFile(name, data, perm)
}
func (localManageFS) ReadFile(name string) ([]byte, error) { return os.ReadFile(name) }
func (localManageFS) Rename(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }
func (localManageFS) Remove(name string) error             { return os.Remove(name) }
func (localManageFS) RemoveAll(path string) error          { return os.RemoveAll(path) }

type ManagedEntry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	IsDir      bool      `json:"isDir"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type PathListing struct {
	Path  string         `json:"path"`
	Items []ManagedEntry `json:"items"`
}

func (s *Service) ListManagedPath(ctx context.Context, roots []string, currentPath string) (PathListing, error) {
	_ = ctx
	normalizedRoots, err := normalizeRoots(roots)
	if err != nil {
		return PathListing{}, err
	}
	if len(normalizedRoots) == 0 {
		return PathListing{}, errors.New("no configured roots")
	}

	pathToList := normalizedRoots[0]
	if strings.TrimSpace(currentPath) != "" {
		pathToList, err = ensureWithinRoots(currentPath, normalizedRoots)
		if err != nil {
			return PathListing{}, err
		}
	}

	info, err := s.manager.Stat(pathToList)
	if err != nil {
		return PathListing{}, err
	}
	if !info.IsDir() {
		return PathListing{}, errors.New("path must be a directory")
	}

	entries, err := s.manager.ReadDir(pathToList)
	if err != nil {
		return PathListing{}, err
	}
	out := make([]ManagedEntry, 0, len(entries))
	for _, entry := range entries {
		childPath := filepath.Clean(filepath.Join(pathToList, entry.Name()))
		childInfo, statErr := entry.Info()
		if statErr != nil {
			continue
		}
		out = append(out, ManagedEntry{
			Name:       entry.Name(),
			Path:       childPath,
			IsDir:      entry.IsDir(),
			SizeBytes:  childInfo.Size(),
			ModifiedAt: childInfo.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return PathListing{Path: pathToList, Items: out}, nil
}

func (s *Service) ReadManagedFile(ctx context.Context, roots []string, filePath string, maxBytes int64) (string, error) {
	_ = ctx
	normalizedRoots, err := normalizeRoots(roots)
	if err != nil {
		return "", err
	}
	absPath, err := ensureWithinRoots(filePath, normalizedRoots)
	if err != nil {
		return "", err
	}
	info, err := s.manager.Stat(absPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("path is a directory")
	}
	if maxBytes > 0 && info.Size() > maxBytes {
		return "", fmt.Errorf("file too large to edit (%d bytes)", info.Size())
	}
	content, err := s.manager.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func (s *Service) CreateManagedDir(ctx context.Context, roots []string, dirPath string) error {
	_ = ctx
	normalizedRoots, err := normalizeRoots(roots)
	if err != nil {
		return err
	}
	absPath, err := ensureWithinRoots(dirPath, normalizedRoots)
	if err != nil {
		return err
	}
	return s.manager.MkdirAll(absPath, 0o755)
}

func (s *Service) WriteManagedFile(ctx context.Context, roots []string, filePath string, content []byte) error {
	_ = ctx
	normalizedRoots, err := normalizeRoots(roots)
	if err != nil {
		return err
	}
	absPath, err := ensureWithinRoots(filePath, normalizedRoots)
	if err != nil {
		return err
	}
	return s.manager.WriteFile(absPath, content, 0o644)
}

func (s *Service) MoveManagedPath(ctx context.Context, roots []string, fromPath, toPath string) error {
	_ = ctx
	normalizedRoots, err := normalizeRoots(roots)
	if err != nil {
		return err
	}
	fromAbs, err := ensureWithinRoots(fromPath, normalizedRoots)
	if err != nil {
		return err
	}
	toAbs, err := ensureWithinRoots(toPath, normalizedRoots)
	if err != nil {
		return err
	}
	if err := s.manager.MkdirAll(filepath.Dir(toAbs), 0o755); err != nil {
		return err
	}
	return s.manager.Rename(fromAbs, toAbs)
}

func (s *Service) DeleteManagedPath(ctx context.Context, roots []string, targetPath string, recursive bool) error {
	_ = ctx
	normalizedRoots, err := normalizeRoots(roots)
	if err != nil {
		return err
	}
	absPath, err := ensureWithinRoots(targetPath, normalizedRoots)
	if err != nil {
		return err
	}
	info, err := s.manager.Stat(absPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if !recursive {
			return errors.New("directory deletion requires recursive=true")
		}
		return s.manager.RemoveAll(absPath)
	}
	return s.manager.Remove(absPath)
}

func normalizeRoots(roots []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		cleaned := strings.TrimSpace(root)
		if cleaned == "" {
			continue
		}
		abs, err := stablePath(cleaned)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	sort.Strings(out)
	return out, nil
}

func ensureWithinRoots(target string, roots []string) (string, error) {
	if len(roots) == 0 {
		return "", errors.New("no configured roots")
	}
	cleaned := strings.TrimSpace(target)
	if cleaned == "" {
		return "", errors.New("empty path")
	}
	// 1) Try direct resolution first (absolute, or relative to current working dir).
	abs, err := stablePath(cleaned)
	if err == nil {
		if withinRoots(abs, roots) {
			return abs, nil
		}
	}

	// 2) If still relative and not within roots, resolve relative to the first allowed root.
	if !filepath.IsAbs(cleaned) {
		joined := filepath.Join(roots[0], cleaned)
		absJoined, joinErr := stablePath(joined)
		if joinErr != nil {
			return "", joinErr
		}
		if withinRoots(absJoined, roots) {
			return absJoined, nil
		}
	}

	return "", errors.New("path outside allowed roots")
}

func withinRoots(abs string, roots []string) bool {
	for _, root := range roots {
		if abs == root {
			return true
		}
		prefix := root + string(filepath.Separator)
		if strings.HasPrefix(abs, prefix) {
			return true
		}
	}
	return false
}

func stablePath(raw string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return "", err
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		return resolved, nil
	}
	// If the full path does not exist yet, resolve the nearest existing parent
	// (handles /var -> /private/var symlink divergence on macOS temp paths).
	missing := make([]string, 0, 4)
	cursor := abs
	for {
		if _, statErr := os.Stat(cursor); statErr == nil {
			if resolvedCursor, resolveErr := filepath.EvalSymlinks(cursor); resolveErr == nil {
				parts := append([]string{resolvedCursor}, missing...)
				return filepath.Join(parts...), nil
			}
			break
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			break
		}
		missing = append([]string{filepath.Base(cursor)}, missing...)
		cursor = parent
	}
	return abs, nil
}
