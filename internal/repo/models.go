package repo

import "time"

type DownloadJobModel struct {
	ID              string `gorm:"primaryKey"`
	SourceLink      string
	DirectLink      string
	FileName        string
	DestinationPath string
	Status          string
	Priority        int
	SizeBytes       int64
	DownloadedBytes int64
	SpeedBytes      int64
	ETASeconds      int64
	Retries         int
	MaxRetries      int
	UseDebrid       bool
	ErrorMessage    string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type DebridAccountModel struct {
	ID           uint `gorm:"primaryKey"`
	Provider     string
	Label        string
	IsActive     bool
	IsDefault    bool
	AuthType     string
	AccessToken  string
	RefreshToken string
	TokenExpiry  *time.Time
	APIKeyMasked string
	APIKeyCipher string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type DebridMappingModel struct {
	ID         uint `gorm:"primaryKey"`
	JobID      string
	SourceLink string
	DirectLink string
	Provider   string
	CreatedAt  time.Time
}

type MediaItemModel struct {
	ID         string `gorm:"primaryKey"`
	Title      string
	Kind       string
	Path       string
	MimeType   string
	SeriesName string
	Season     int
	Episode    int
	Tags       string
	DurationMs int64
	SizeBytes  int64
	ModifiedAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type MediaProgressModel struct {
	ID         uint `gorm:"primaryKey"`
	MediaID    string
	PositionMs int64
	UpdatedAt  time.Time
}

type AIChunkModel struct {
	ID        string `gorm:"primaryKey"`
	MediaID   string
	Content   string
	Embedding string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type AppSettingsModel struct {
	ID        uint `gorm:"primaryKey"`
	Payload   string
	UpdatedAt time.Time
}
