package media

import (
	"context"
	"io/fs"
	"mime"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/events"
	"cgwm/shelfy/internal/repo"
	"github.com/google/uuid"
)

type Walker interface {
	WalkDir(root string, fn fs.WalkDirFunc) error
}

type OSWalker struct{}

func (OSWalker) WalkDir(root string, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, fn)
}

type Scanner struct {
	repo   repo.Repository
	walker Walker
	bus    *events.Bus
}

func NewScanner(r repo.Repository, walker Walker, bus *events.Bus) *Scanner {
	if walker == nil {
		walker = OSWalker{}
	}
	return &Scanner{repo: r, walker: walker, bus: bus}
}

var episodeRegex = regexp.MustCompile(`(?i)s(\d{1,2})e(\d{1,2})`)

func (s *Scanner) Scan(ctx context.Context, root string) (int, error) {
	count := 0
	root = canonicalMediaPath(root)
	seen := make(map[string]struct{})

	err := s.walker.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		canonicalPath := canonicalMediaPath(path)
		item, ok := parseMedia(canonicalPath)
		if !ok {
			return nil
		}
		item.ID = uuid.NewSHA1(uuid.NameSpaceURL, []byte(canonicalPath)).String()
		now := time.Now()
		item.CreatedAt = now
		item.UpdatedAt = now
		if info, statErr := d.Info(); statErr == nil {
			item.SizeBytes = info.Size()
			item.ModifiedAt = info.ModTime()
		}
		if err := s.repo.UpsertMediaItem(ctx, item); err != nil {
			return err
		}
		seen[item.Path] = struct{}{}
		count++
		s.bus.Publish(events.Event{Type: "media_scanned", At: now, Payload: map[string]interface{}{"title": item.Title, "path": item.Path}})
		return nil
	})
	if err != nil {
		return count, err
	}

	// Remove stale rows for this root (deleted/moved files).
	all, err := s.repo.ListMedia(ctx, "", "")
	if err != nil {
		return count, err
	}
	for _, item := range all {
		if !isWithinRoot(filepath.Clean(item.Path), root) {
			continue
		}
		if _, ok := seen[item.Path]; ok {
			continue
		}
		if err := s.repo.DeleteMediaByID(ctx, item.ID); err != nil {
			return count, err
		}
	}
	return count, nil
}

func parseMedia(filePath string) (domain.MediaItem, bool) {
	kind := detectKind(filePath)
	if kind == domain.MediaOther {
		return domain.MediaItem{}, false
	}
	base := filepath.Base(filePath)
	title := strings.TrimSuffix(base, filepath.Ext(base))
	item := domain.MediaItem{
		Title:    title,
		Kind:     kind,
		Path:     filePath,
		MimeType: mime.TypeByExtension(strings.ToLower(filepath.Ext(filePath))),
	}

	match := episodeRegex.FindStringSubmatch(title)
	if len(match) == 3 {
		item.SeriesName = inferSeriesName(title)
		item.Season = atoi(match[1])
		item.Episode = atoi(match[2])
	}
	return item, true
}

func inferSeriesName(title string) string {
	match := episodeRegex.FindStringIndex(title)
	if match == nil {
		return title
	}
	return strings.Trim(strings.ReplaceAll(title[:match[0]], ".", " "), " -_")
}

func atoi(v string) int {
	n := 0
	for _, ch := range v {
		n = n*10 + int(ch-'0')
	}
	return n
}

func detectKind(filePath string) domain.MediaKind {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".mp4", ".mkv", ".webm", ".avi", ".mov":
		return domain.MediaVideo
	case ".mp3", ".flac", ".wav", ".aac", ".m4a":
		return domain.MediaAudio
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return domain.MediaImage
	case ".pdf":
		return domain.MediaPDF
	default:
		return domain.MediaOther
	}
}

func isWithinRoot(filePath, root string) bool {
	if filePath == root {
		return true
	}
	prefix := root + string(filepath.Separator)
	return strings.HasPrefix(filePath, prefix)
}

func canonicalMediaPath(raw string) string {
	cleaned := filepath.Clean(strings.TrimSpace(raw))
	if cleaned == "." || cleaned == "" {
		return cleaned
	}
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return cleaned
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil && strings.TrimSpace(resolved) != "" {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(abs)
}
