package fileserver

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/infra/storage"
	"cgwm/shelfy/internal/repo"
)

type Entry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	MimeType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes"`
}

type Service struct {
	repo    repo.Repository
	fs      storage.FileSystem
	manager ManageFS
}

func NewService(r repo.Repository, fs storage.FileSystem) *Service {
	return &Service{repo: r, fs: fs, manager: localManageFS{}}
}

func (s *Service) List(ctx context.Context) ([]Entry, error) {
	media, err := s.repo.ListMedia(ctx, "", "")
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(media))
	for _, m := range media {
		size := m.SizeBytes
		if size == 0 {
			if info, statErr := s.fs.Stat(m.Path); statErr == nil {
				size = info.Size()
			}
		}
		mimeType := m.MimeType
		if mimeType == "" {
			mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(m.Path)))
		}
		entries = append(entries, Entry{
			ID:        m.ID,
			Name:      m.Title,
			Kind:      string(m.Kind),
			Path:      m.Path,
			MimeType:  mimeType,
			SizeBytes: size,
		})
	}
	return entries, nil
}

func (s *Service) Resolve(ctx context.Context, id string) (Entry, error) {
	item, err := s.repo.GetMedia(ctx, id)
	if err != nil {
		return Entry{}, err
	}
	size := item.SizeBytes
	if size == 0 {
		if info, statErr := s.fs.Stat(item.Path); statErr == nil {
			size = info.Size()
		}
	}
	mimeType := item.MimeType
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(item.Path)))
	}
	return Entry{ID: item.ID, Name: item.Title, Kind: string(item.Kind), Path: item.Path, MimeType: mimeType, SizeBytes: size}, nil
}

func BuildDownloadFilename(entry Entry) string {
	ext := filepath.Ext(entry.Path)
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		name = "file"
	}
	if ext == "" {
		return name
	}
	if strings.HasSuffix(strings.ToLower(name), strings.ToLower(ext)) {
		return name
	}
	return fmt.Sprintf("%s%s", name, ext)
}

var _ = domain.MediaItem{}
