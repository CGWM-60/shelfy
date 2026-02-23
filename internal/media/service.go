package media

import (
	"context"

	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/repo"
)

type Service struct {
	repo    repo.Repository
	scanner *Scanner
}

func NewService(r repo.Repository, scanner *Scanner) *Service {
	return &Service{repo: r, scanner: scanner}
}

func (s *Service) Scan(ctx context.Context, root string) (int, error) {
	return s.scanner.Scan(ctx, root)
}

func (s *Service) List(ctx context.Context, query, kind string) ([]domain.MediaItem, error) {
	return s.repo.ListMedia(ctx, query, kind)
}

func (s *Service) Get(ctx context.Context, id string) (domain.MediaItem, error) {
	return s.repo.GetMedia(ctx, id)
}

func (s *Service) SaveProgress(ctx context.Context, mediaID string, positionMs int64) error {
	return s.repo.SaveMediaProgress(ctx, domain.MediaProgress{MediaID: mediaID, PositionMs: positionMs})
}

func (s *Service) GetProgress(ctx context.Context, mediaID string) (domain.MediaProgress, error) {
	return s.repo.GetMediaProgress(ctx, mediaID)
}
