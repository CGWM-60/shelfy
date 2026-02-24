package repo

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"cgwm/shelfy/internal/domain"
	"gorm.io/gorm"
)

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&DownloadJobModel{},
		&DebridAccountModel{},
		&DebridMappingModel{},
		&MediaItemModel{},
		&MediaProgressModel{},
		&AIChunkModel{},
		&AppSettingsModel{},
	)
}

func (r *GormRepository) CreateDownload(ctx context.Context, job domain.DownloadJob) error {
	model := toDownloadModel(job)
	return r.db.WithContext(ctx).Create(&model).Error
}

func (r *GormRepository) UpdateDownload(ctx context.Context, job domain.DownloadJob) error {
	model := toDownloadModel(job)
	return r.db.WithContext(ctx).Model(&DownloadJobModel{}).Where("id = ?", job.ID).Updates(model).Error
}

func (r *GormRepository) GetDownload(ctx context.Context, id string) (domain.DownloadJob, error) {
	var model DownloadJobModel
	if err := r.db.WithContext(ctx).First(&model, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.DownloadJob{}, ErrNotFound
		}
		return domain.DownloadJob{}, err
	}
	return fromDownloadModel(model), nil
}

func (r *GormRepository) ListDownloads(ctx context.Context) ([]domain.DownloadJob, error) {
	var models []DownloadJobModel
	if err := r.db.WithContext(ctx).Order("created_at desc").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.DownloadJob, 0, len(models))
	for _, m := range models {
		out = append(out, fromDownloadModel(m))
	}
	return out, nil
}

func (r *GormRepository) DeleteDownload(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&DownloadJobModel{}, "id = ?", id).Error
}

func (r *GormRepository) CreateDebridAccount(ctx context.Context, account domain.DebridAccount) (domain.DebridAccount, error) {
	m := toDebridModel(account)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return domain.DebridAccount{}, err
	}
	return fromDebridModel(m), nil
}

func (r *GormRepository) UpdateDebridAccount(ctx context.Context, account domain.DebridAccount) (domain.DebridAccount, error) {
	m := toDebridModel(account)
	if err := r.db.WithContext(ctx).Model(&DebridAccountModel{}).Where("id = ?", account.ID).Updates(m).Error; err != nil {
		return domain.DebridAccount{}, err
	}
	return r.GetDebridAccount(ctx, account.ID)
}

func (r *GormRepository) GetDebridAccount(ctx context.Context, id uint) (domain.DebridAccount, error) {
	var m DebridAccountModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.DebridAccount{}, ErrNotFound
		}
		return domain.DebridAccount{}, err
	}
	return fromDebridModel(m), nil
}

func (r *GormRepository) ListDebridAccounts(ctx context.Context) ([]domain.DebridAccount, error) {
	var models []DebridAccountModel
	if err := r.db.WithContext(ctx).Order("created_at asc").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.DebridAccount, 0, len(models))
	for _, m := range models {
		out = append(out, fromDebridModel(m))
	}
	return out, nil
}

func (r *GormRepository) DeleteDebridAccount(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&DebridAccountModel{}, "id = ?", id).Error
}

func (r *GormRepository) DeactivateDebridDefaults(ctx context.Context, provider string) error {
	return r.db.WithContext(ctx).Model(&DebridAccountModel{}).Where("provider = ?", provider).Update("is_default", false).Error
}

func (r *GormRepository) CreateDebridMapping(ctx context.Context, jobID, source, direct, provider string) error {
	m := DebridMappingModel{JobID: jobID, SourceLink: source, DirectLink: direct, Provider: provider}
	return r.db.WithContext(ctx).Create(&m).Error
}

func (r *GormRepository) UpsertMediaItem(ctx context.Context, item domain.MediaItem) error {
	m := toMediaModel(item)
	// Keep scan idempotent by path and clean historical duplicates.
	if err := r.db.WithContext(ctx).
		Where("path = ? AND id <> ?", m.Path, m.ID).
		Delete(&MediaItemModel{}).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Save(&m).Error
}

func (r *GormRepository) GetMedia(ctx context.Context, id string) (domain.MediaItem, error) {
	var m MediaItemModel
	if err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.MediaItem{}, ErrNotFound
		}
		return domain.MediaItem{}, err
	}
	return fromMediaModel(m), nil
}

func (r *GormRepository) DeleteMediaByID(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&MediaItemModel{}, "id = ?", id).Error
}

func (r *GormRepository) ListMedia(ctx context.Context, query, kind string) ([]domain.MediaItem, error) {
	q := r.db.WithContext(ctx).Model(&MediaItemModel{})
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if query != "" {
		q = q.Where("lower(title) LIKE ?", "%"+strings.ToLower(query)+"%")
	}
	var models []MediaItemModel
	if err := q.Order("title asc").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]domain.MediaItem, 0, len(models))
	for _, m := range models {
		out = append(out, fromMediaModel(m))
	}
	return out, nil
}

func (r *GormRepository) SaveMediaProgress(ctx context.Context, p domain.MediaProgress) error {
	var m MediaProgressModel
	err := r.db.WithContext(ctx).First(&m, "media_id = ?", p.MediaID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		m = MediaProgressModel{MediaID: p.MediaID, PositionMs: p.PositionMs}
		return r.db.WithContext(ctx).Create(&m).Error
	}
	if err != nil {
		return err
	}
	m.PositionMs = p.PositionMs
	return r.db.WithContext(ctx).Save(&m).Error
}

func (r *GormRepository) GetMediaProgress(ctx context.Context, mediaID string) (domain.MediaProgress, error) {
	var m MediaProgressModel
	if err := r.db.WithContext(ctx).First(&m, "media_id = ?", mediaID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.MediaProgress{}, ErrNotFound
		}
		return domain.MediaProgress{}, err
	}
	return domain.MediaProgress{MediaID: m.MediaID, PositionMs: m.PositionMs}, nil
}

func (r *GormRepository) UpsertAIChunk(ctx context.Context, id, mediaID, content, embedding string) error {
	m := AIChunkModel{ID: id, MediaID: mediaID, Content: content, Embedding: embedding}
	return r.db.WithContext(ctx).Save(&m).Error
}

func (r *GormRepository) ListAIChunks(ctx context.Context) ([]AIChunkModel, error) {
	var out []AIChunkModel
	if err := r.db.WithContext(ctx).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *GormRepository) DeleteAIChunksByMedia(ctx context.Context, mediaID string) error {
	return r.db.WithContext(ctx).Delete(&AIChunkModel{}, "media_id = ?", mediaID).Error
}

func (r *GormRepository) DeleteAllAIChunks(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("1 = 1").Delete(&AIChunkModel{}).Error
}

func (r *GormRepository) GetAppSettings(ctx context.Context) (domain.AppSettings, error) {
	var m AppSettingsModel
	if err := r.db.WithContext(ctx).First(&m, "id = 1").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.AppSettings{}, ErrNotFound
		}
		return domain.AppSettings{}, err
	}
	var settings domain.AppSettings
	if err := json.Unmarshal([]byte(m.Payload), &settings); err != nil {
		return domain.AppSettings{}, err
	}
	return settings, nil
}

func (r *GormRepository) SaveAppSettings(ctx context.Context, settings domain.AppSettings) error {
	payload, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	m := AppSettingsModel{ID: 1, Payload: string(payload)}
	return r.db.WithContext(ctx).Save(&m).Error
}

func toDownloadModel(job domain.DownloadJob) DownloadJobModel {
	return DownloadJobModel{
		ID:              job.ID,
		SourceLink:      job.SourceLink,
		DirectLink:      job.DirectLink,
		FileName:        job.FileName,
		DestinationPath: job.DestinationPath,
		Status:          string(job.Status),
		Priority:        job.Priority,
		SizeBytes:       job.SizeBytes,
		DownloadedBytes: job.DownloadedBytes,
		SpeedBytes:      job.SpeedBytes,
		ETASeconds:      job.ETASeconds,
		Retries:         job.Retries,
		MaxRetries:      job.MaxRetries,
		UseDebrid:       job.UseDebrid,
		ErrorMessage:    job.ErrorMessage,
		CreatedAt:       job.CreatedAt,
		UpdatedAt:       job.UpdatedAt,
	}
}

func fromDownloadModel(m DownloadJobModel) domain.DownloadJob {
	return domain.DownloadJob{
		ID:              m.ID,
		SourceLink:      m.SourceLink,
		DirectLink:      m.DirectLink,
		FileName:        m.FileName,
		DestinationPath: m.DestinationPath,
		Status:          domain.DownloadStatus(m.Status),
		Priority:        m.Priority,
		SizeBytes:       m.SizeBytes,
		DownloadedBytes: m.DownloadedBytes,
		SpeedBytes:      m.SpeedBytes,
		ETASeconds:      m.ETASeconds,
		Retries:         m.Retries,
		MaxRetries:      m.MaxRetries,
		UseDebrid:       m.UseDebrid,
		ErrorMessage:    m.ErrorMessage,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

func toDebridModel(a domain.DebridAccount) DebridAccountModel {
	return DebridAccountModel{
		ID:           a.ID,
		Provider:     a.Provider,
		Label:        a.Label,
		IsActive:     a.IsActive,
		IsDefault:    a.IsDefault,
		AuthType:     string(a.AuthType),
		AccessToken:  a.AccessToken,
		RefreshToken: a.RefreshToken,
		TokenExpiry:  a.TokenExpiry,
		APIKeyMasked: maskAPIKey(a.APIKey),
		APIKeyCipher: obfuscate(a.APIKey),
		CreatedAt:    a.CreatedAt,
		UpdatedAt:    a.UpdatedAt,
	}
}

func fromDebridModel(m DebridAccountModel) domain.DebridAccount {
	return domain.DebridAccount{
		ID:           m.ID,
		Provider:     m.Provider,
		Label:        m.Label,
		IsActive:     m.IsActive,
		IsDefault:    m.IsDefault,
		AuthType:     domain.DebridAuthType(m.AuthType),
		AccessToken:  m.AccessToken,
		RefreshToken: m.RefreshToken,
		TokenExpiry:  m.TokenExpiry,
		APIKey:       deobfuscate(m.APIKeyCipher),
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

func toMediaModel(m domain.MediaItem) MediaItemModel {
	return MediaItemModel{
		ID:         m.ID,
		Title:      m.Title,
		Kind:       string(m.Kind),
		Path:       m.Path,
		MimeType:   m.MimeType,
		SeriesName: m.SeriesName,
		Season:     m.Season,
		Episode:    m.Episode,
		Tags:       strings.Join(m.Tags, ","),
		DurationMs: m.DurationMs,
		SizeBytes:  m.SizeBytes,
		ModifiedAt: m.ModifiedAt,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}

func fromMediaModel(m MediaItemModel) domain.MediaItem {
	var tags []string
	if m.Tags != "" {
		tags = strings.Split(m.Tags, ",")
	}
	return domain.MediaItem{
		ID:         m.ID,
		Title:      m.Title,
		Kind:       domain.MediaKind(m.Kind),
		Path:       m.Path,
		MimeType:   m.MimeType,
		SeriesName: m.SeriesName,
		Season:     m.Season,
		Episode:    m.Episode,
		Tags:       tags,
		DurationMs: m.DurationMs,
		SizeBytes:  m.SizeBytes,
		ModifiedAt: m.ModifiedAt,
		CreatedAt:  m.CreatedAt,
		UpdatedAt:  m.UpdatedAt,
	}
}

func maskAPIKey(apiKey string) string {
	if len(apiKey) < 4 {
		return "****"
	}
	return "****" + apiKey[len(apiKey)-4:]
}

func obfuscate(v string) string {
	if v == "" {
		return ""
	}
	runes := []rune(v)
	for i := range runes {
		runes[i] ^= 7
	}
	return string(runes)
}

func deobfuscate(v string) string {
	return obfuscate(v)
}
