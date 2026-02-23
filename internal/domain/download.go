package domain

import "time"

type DownloadStatus string

const (
	DownloadQueued    DownloadStatus = "queued"
	DownloadRunning   DownloadStatus = "running"
	DownloadPaused    DownloadStatus = "paused"
	DownloadCompleted DownloadStatus = "completed"
	DownloadFailed    DownloadStatus = "failed"
	DownloadCanceled  DownloadStatus = "canceled"
)

type DownloadJob struct {
	ID              string         `json:"id"`
	SourceLink      string         `json:"sourceLink"`
	DirectLink      string         `json:"directLink"`
	FileName        string         `json:"fileName"`
	DestinationPath string         `json:"destinationPath"`
	Status          DownloadStatus `json:"status"`
	Priority        int            `json:"priority"`
	SizeBytes       int64          `json:"sizeBytes"`
	DownloadedBytes int64          `json:"downloadedBytes"`
	SpeedBytes      int64          `json:"speedBytes"`
	ETASeconds      int64          `json:"etaSeconds"`
	Retries         int            `json:"retries"`
	MaxRetries      int            `json:"maxRetries"`
	UseDebrid       bool           `json:"useDebrid"`
	ErrorMessage    string         `json:"errorMessage"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

type DownloadAddRequest struct {
	Links           []string `json:"links"`
	UseDebrid       bool     `json:"useDebrid"`
	Provider        string   `json:"provider,omitempty"`
	DebridPassword  string   `json:"debridPassword,omitempty"`
	AccountID       uint     `json:"accountId,omitempty"`
	DebridAccountID uint     `json:"debridAccountId,omitempty"`
	Priority        int      `json:"priority"`
	DestinationDir  string   `json:"destinationDir,omitempty"`
	MaxParallel     int      `json:"maxParallel,omitempty"`
	RateLimitKB     int      `json:"rateLimitKB,omitempty"`
}

func CanTransition(from, to DownloadStatus) bool {
	switch from {
	case DownloadQueued:
		return to == DownloadRunning || to == DownloadCanceled
	case DownloadRunning:
		return to == DownloadPaused || to == DownloadCompleted || to == DownloadFailed || to == DownloadCanceled
	case DownloadPaused:
		return to == DownloadRunning || to == DownloadCanceled
	case DownloadFailed:
		return to == DownloadQueued || to == DownloadCanceled
	case DownloadCompleted, DownloadCanceled:
		return false
	default:
		return false
	}
}
