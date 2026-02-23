package domain

import "time"

type DebridAuthType string

const (
	DebridAuthNone         DebridAuthType = "none"
	DebridAuthAPIKey       DebridAuthType = "api_key"
	DebridAuthOAuth2Device DebridAuthType = "oauth2_device"
	DebridAuthOAuth2Pass   DebridAuthType = "oauth2_password"
)

type DebridAccount struct {
	ID           uint           `json:"id"`
	Provider     string         `json:"provider"`
	Label        string         `json:"label"`
	IsActive     bool           `json:"isActive"`
	IsDefault    bool           `json:"isDefault"`
	AuthType     DebridAuthType `json:"authType"`
	AccessToken  string         `json:"accessToken,omitempty"`
	RefreshToken string         `json:"refreshToken,omitempty"`
	TokenExpiry  *time.Time     `json:"tokenExpiry,omitempty"`
	APIKey       string         `json:"apiKey,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

type DebridAuthSession struct {
	SessionID       string `json:"sessionId"`
	VerificationURI string `json:"verificationUri,omitempty"`
	UserCode        string `json:"userCode,omitempty"`
	IntervalSec     int    `json:"intervalSec"`
}

type DebridToken struct {
	AccessToken  string
	RefreshToken string
	Expiry       *time.Time
}

type DebridFileInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type DebridFolderInfo struct {
	Name  string           `json:"name"`
	Files []DebridFileInfo `json:"files"`
}

type DebridResult struct {
	Type      string            `json:"type"`
	DirectURL string            `json:"directUrl,omitempty"`
	File      *DebridFileInfo   `json:"file,omitempty"`
	Folder    *DebridFolderInfo `json:"folder,omitempty"`
	Provider  string            `json:"provider"`
}
