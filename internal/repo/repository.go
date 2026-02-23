package repo

import (
	"context"

	"cgwm/shelfy/internal/domain"
)

type Repository interface {
	CreateDownload(ctx context.Context, job domain.DownloadJob) error
	UpdateDownload(ctx context.Context, job domain.DownloadJob) error
	GetDownload(ctx context.Context, id string) (domain.DownloadJob, error)
	ListDownloads(ctx context.Context) ([]domain.DownloadJob, error)
	DeleteDownload(ctx context.Context, id string) error

	CreateDebridAccount(ctx context.Context, account domain.DebridAccount) (domain.DebridAccount, error)
	UpdateDebridAccount(ctx context.Context, account domain.DebridAccount) (domain.DebridAccount, error)
	GetDebridAccount(ctx context.Context, id uint) (domain.DebridAccount, error)
	ListDebridAccounts(ctx context.Context) ([]domain.DebridAccount, error)
	DeleteDebridAccount(ctx context.Context, id uint) error
	DeactivateDebridDefaults(ctx context.Context, provider string) error

	CreateDebridMapping(ctx context.Context, jobID, source, direct, provider string) error

	UpsertMediaItem(ctx context.Context, item domain.MediaItem) error
	DeleteMediaByID(ctx context.Context, id string) error
	GetMedia(ctx context.Context, id string) (domain.MediaItem, error)
	ListMedia(ctx context.Context, query string, kind string) ([]domain.MediaItem, error)
	SaveMediaProgress(ctx context.Context, p domain.MediaProgress) error
	GetMediaProgress(ctx context.Context, mediaID string) (domain.MediaProgress, error)

	UpsertAIChunk(ctx context.Context, id, mediaID, content, embedding string) error
	ListAIChunks(ctx context.Context) ([]AIChunkModel, error)
	DeleteAIChunksByMedia(ctx context.Context, mediaID string) error

	GetAppSettings(ctx context.Context) (domain.AppSettings, error)
	SaveAppSettings(ctx context.Context, settings domain.AppSettings) error
}
