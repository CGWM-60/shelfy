package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"cgwm/shelfy/internal/debrid"
	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/downloader"
	"cgwm/shelfy/internal/events"
	"cgwm/shelfy/internal/repo"
	"github.com/google/uuid"
)

type DownloadService struct {
	repo   repo.Repository
	engine *downloader.Engine
	debrid *debrid.Service
	bus    *events.Bus
}

func NewDownloadService(r repo.Repository, engine *downloader.Engine, debridSvc *debrid.Service, bus *events.Bus) *DownloadService {
	return &DownloadService{repo: r, engine: engine, debrid: debridSvc, bus: bus}
}

func (s *DownloadService) Add(ctx context.Context, req domain.DownloadAddRequest) ([]domain.DownloadJob, error) {
	links := flattenLinks(req.Links)
	if len(links) == 0 {
		return nil, errors.New("at least one link is required")
	}
	created := make([]domain.DownloadJob, 0)
	accountID := req.AccountID
	if req.DebridAccountID > 0 {
		accountID = req.DebridAccountID
	}
	for _, link := range links {
		resolved, err := s.debrid.ResolveLink(ctx, link, debrid.ResolveOptions{
			UseDebrid: req.UseDebrid,
			Provider:  req.Provider,
			AccountID: accountID,
			Password:  req.DebridPassword,
		})
		if err != nil {
			s.bus.Publish(events.Event{Type: "debrid_failed", At: time.Now(), Payload: map[string]interface{}{"source": link, "error": err.Error()}})
			return nil, err
		}
		if req.UseDebrid {
			s.bus.Publish(events.Event{Type: "debrid_started", At: time.Now(), Payload: map[string]interface{}{"source": link}})
		}
		for _, item := range resolved {
			job := domain.DownloadJob{
				ID:         uuid.NewString(),
				SourceLink: item.SourceLink,
				DirectLink: item.DirectLink,
				FileName:   item.FileName,
				Status:     domain.DownloadQueued,
				Priority:   req.Priority,
				MaxRetries: 3,
				UseDebrid:  req.UseDebrid,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			if job.FileName == "" {
				job.FileName = deriveName(item.DirectLink)
			}
			if req.DestinationDir != "" {
				job.DestinationPath = path.Join(req.DestinationDir, job.FileName)
			}
			if err := s.repo.CreateDownload(ctx, job); err != nil {
				return nil, err
			}
			if req.UseDebrid {
				_ = s.repo.CreateDebridMapping(ctx, job.ID, item.SourceLink, item.DirectLink, item.Provider)
				s.bus.Publish(events.Event{Type: "debrid_done", At: time.Now(), Payload: map[string]interface{}{"jobId": job.ID, "source": item.SourceLink, "direct": item.DirectLink}})
			}
			created = append(created, job)
		}
	}
	return created, nil
}

func (s *DownloadService) List(ctx context.Context) ([]domain.DownloadJob, error) {
	return s.repo.ListDownloads(ctx)
}

func (s *DownloadService) Start(ctx context.Context, id string) error {
	return s.engine.Start(ctx, id)
}

func (s *DownloadService) Pause(ctx context.Context, id string) error {
	return s.engine.Pause(ctx, id)
}

func (s *DownloadService) Resume(ctx context.Context, id string) error {
	return s.engine.Resume(ctx, id)
}

func (s *DownloadService) Delete(ctx context.Context, id string) error {
	if err := s.engine.Cancel(ctx, id); err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			return fmt.Errorf("cancel job: %w", err)
		}
	}
	return s.repo.DeleteDownload(ctx, id)
}

func flattenLinks(raw []string) []string {
	out := make([]string, 0)
	for _, candidate := range raw {
		for _, link := range downloader.ParseLinks(candidate) {
			out = append(out, link)
		}
	}
	return out
}

func deriveName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "download.bin"
	}
	name := path.Base(u.Path)
	if name == "" || name == "." || strings.TrimSpace(name) == "" {
		return "download.bin"
	}
	return name
}
