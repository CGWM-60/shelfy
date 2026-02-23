package domain

import "time"

type MediaKind string

const (
	MediaVideo MediaKind = "video"
	MediaAudio MediaKind = "audio"
	MediaImage MediaKind = "image"
	MediaPDF   MediaKind = "pdf"
	MediaOther MediaKind = "other"
)

type MediaItem struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Kind       MediaKind `json:"kind"`
	Path       string    `json:"path"`
	SeriesName string    `json:"seriesName,omitempty"`
	Season     int       `json:"season,omitempty"`
	Episode    int       `json:"episode,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	DurationMs int64     `json:"durationMs"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
	MimeType   string    `json:"mimeType"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type MediaProgress struct {
	MediaID    string `json:"mediaId"`
	PositionMs int64  `json:"positionMs"`
}
