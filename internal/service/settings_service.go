package service

import (
	"context"

	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/repo"
)

type SettingsService struct {
	repo     repo.Repository
	defaults domain.AppSettings
}

func NewSettingsService(r repo.Repository, defaults domain.AppSettings) *SettingsService {
	return &SettingsService{repo: r, defaults: defaults}
}

func (s *SettingsService) Get(ctx context.Context) (domain.AppSettings, error) {
	stored, err := s.repo.GetAppSettings(ctx)
	if err == nil {
		return mergeSettings(s.defaults, stored), nil
	}
	if err == repo.ErrNotFound {
		return s.defaults, nil
	}
	return domain.AppSettings{}, err
}

func (s *SettingsService) Save(ctx context.Context, input domain.AppSettings) (domain.AppSettings, error) {
	normalized := mergeSettings(s.defaults, input)
	if err := s.repo.SaveAppSettings(ctx, normalized); err != nil {
		return domain.AppSettings{}, err
	}
	return normalized, nil
}

func mergeSettings(base, override domain.AppSettings) domain.AppSettings {
	out := base
	if override.DownloadMaxConcurrent > 0 {
		out.DownloadMaxConcurrent = override.DownloadMaxConcurrent
	}
	if override.DownloadsPath != "" {
		out.DownloadsPath = override.DownloadsPath
	}
	if len(override.LibraryPaths) > 0 {
		out.LibraryPaths = override.LibraryPaths
	}
	if override.GlobalRateLimitKB > 0 {
		out.GlobalRateLimitKB = override.GlobalRateLimitKB
	}
	if override.AITopK > 0 {
		out.AITopK = override.AITopK
	}
	if override.Theme != "" {
		out.Theme = override.Theme
	}
	if len(override.VisibleColumns) > 0 {
		out.VisibleColumns = override.VisibleColumns
	}
	if override.FileServerAuthUser != "" {
		out.FileServerAuthUser = override.FileServerAuthUser
	}
	if override.SMBShareName != "" {
		out.SMBShareName = override.SMBShareName
	}
	if override.SMBSharePath != "" {
		out.SMBSharePath = override.SMBSharePath
	}

	out.DownloadAutoResume = override.DownloadAutoResume
	out.AIEnabled = override.AIEnabled
	out.FileServerAuthEnabled = override.FileServerAuthEnabled
	out.DLNAEnabled = override.DLNAEnabled
	out.SMBEnabled = override.SMBEnabled
	return out
}
