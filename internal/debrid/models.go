package debrid

import "time"

type AuthType string

const (
	AuthTypeNone         AuthType = "none"
	AuthTypeAPIKey       AuthType = "api_key"
	AuthTypeOAuth2Device AuthType = "oauth2_device"
	AuthTypeOAuth2Pass   AuthType = "oauth2_password"
)

type Account struct {
	ID           uint
	Provider     string
	Label        string
	IsActive     bool
	IsDefault    bool
	AuthType     AuthType
	AccessToken  string
	RefreshToken string
	TokenExpiry  *time.Time
	APIKey       string
}

type Token struct {
	AccessToken  string
	RefreshToken string
	Expiry       *time.Time
}

type LinkInfo struct {
	Link string
}

type FileInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

type FolderInfo struct {
	Name  string     `json:"name"`
	Files []FileInfo `json:"files"`
}

type DebridResult struct {
	Provider  string      `json:"provider"`
	Type      string      `json:"type"`
	DirectURL string      `json:"directUrl,omitempty"`
	File      *FileInfo   `json:"file,omitempty"`
	Folder    *FolderInfo `json:"folder,omitempty"`
}

type AuthSession struct {
	SessionID       string `json:"sessionId"`
	DeviceCode      string `json:"deviceCode,omitempty"`
	VerificationURI string `json:"verificationUri,omitempty"`
	UserCode        string `json:"userCode,omitempty"`
	IntervalSec     int    `json:"intervalSec"`
	ExpiresIn       int    `json:"expiresIn,omitempty"`
}

type UnrestrictOptions struct {
	Force    bool
	Password string
	Account  Account
}
