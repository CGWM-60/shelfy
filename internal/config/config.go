package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env            string
	ConfigFilePath string
	HTTPAddr       string
	DBDriver       string
	DBDSN          string
	StoragePath    string
	MediaPaths     []string
	RequestTimeout time.Duration

	DownloadMaxConcurrent int
	DownloadAutoResume    bool

	MistralAPIKey     string
	MistralEmbedModel string
	MistralChatModel  string
	AIEnabled         bool
	AITopK            int

	DebridLinkAPIBase     string
	DebridLinkOAuthClient string
	DebridLinkOAuthSecret string
	DebridLinkOAuthScope  string

	FileServerAuthUser string
	FileServerAuthPass string
	FileServerEnabled  bool
	WebDAVEnabled      bool
	DLNAEnabled        bool
	DLNABaseURL        string
	SMBEnabled         bool
	SMBShareName       string
	SMBSharePath       string
	SMBBinary          string
	SMBStatusBinary    string
	AuthEnabled        bool
	AuthUser           string
	AuthPass           string
	AuthSessionSecret  string
	AuthSessionTTL     time.Duration
}

type fileConfig struct {
	Env            string   `json:"env"`
	HTTPAddr       string   `json:"httpAddr"`
	DBDriver       string   `json:"dbDriver"`
	DBDSN          string   `json:"dbDsn"`
	StoragePath    string   `json:"storagePath"`
	MediaPaths     []string `json:"mediaPaths"`
	RequestTimeout int      `json:"requestTimeoutSeconds"`

	DownloadMaxConcurrent int  `json:"downloadMaxConcurrent"`
	DownloadAutoResume    bool `json:"downloadAutoResume"`

	MistralAPIKey     string `json:"mistralApiKey"`
	MistralEmbedModel string `json:"mistralEmbedModel"`
	MistralChatModel  string `json:"mistralChatModel"`
	AIEnabled         *bool  `json:"aiEnabled"`
	AITopK            int    `json:"aiTopK"`

	DebridLinkAPIBase     string `json:"debridLinkApiBase"`
	DebridLinkOAuthClient string `json:"debridLinkOAuthClient"`
	DebridLinkOAuthSecret string `json:"debridLinkOAuthSecret"`
	DebridLinkOAuthScope  string `json:"debridLinkOAuthScope"`

	FileServerAuthUser string `json:"fileServerAuthUser"`
	FileServerAuthPass string `json:"fileServerAuthPass"`
	FileServerEnabled  *bool  `json:"fileServerEnabled"`
	WebDAVEnabled      *bool  `json:"webDavEnabled"`
	DLNAEnabled        *bool  `json:"dlnaEnabled"`
	DLNABaseURL        string `json:"dlnaBaseUrl"`
	SMBEnabled         *bool  `json:"smbEnabled"`
	SMBShareName       string `json:"smbShareName"`
	SMBSharePath       string `json:"smbSharePath"`
	SMBBinary          string `json:"smbBinary"`
	SMBStatusBinary    string `json:"smbStatusBinary"`
	AuthEnabled        *bool  `json:"authEnabled"`
	AuthUser           string `json:"authUser"`
	AuthPass           string `json:"authPass"`
	AuthSessionSecret  string `json:"authSessionSecret"`
	AuthSessionTTL     int    `json:"authSessionTtlHours"`
}

func Load() Config {
	cfg := defaults()
	cfg.ConfigFilePath = getOrDefault("CONFIG_FILE", "config.json")

	applyFile(&cfg)
	applyEnv(&cfg)
	return cfg
}

func defaults() Config {
	return Config{
		Env:                   "dev",
		HTTPAddr:              ":8080",
		DBDriver:              "sqlite",
		DBDSN:                 "shelfy.db",
		StoragePath:           "./data/downloads",
		MediaPaths:            []string{"./data/media"},
		RequestTimeout:        30 * time.Second,
		DownloadMaxConcurrent: 3,
		DownloadAutoResume:    true,
		MistralEmbedModel:     "mistral-embed",
		MistralChatModel:      "mistral-small-latest",
		AIEnabled:             true,
		AITopK:                5,
		DebridLinkAPIBase:     "https://debrid-link.com",
		DebridLinkOAuthScope:  "get.account get.post.delete.downloader get.files get.post.stream",
		FileServerEnabled:     true,
		WebDAVEnabled:         false,
		DLNAEnabled:           false,
		SMBEnabled:            false,
		SMBShareName:          "shelfy",
		SMBBinary:             "smbd",
		SMBStatusBinary:       "smbstatus",
		AuthEnabled:           false,
		AuthSessionTTL:        24 * time.Hour,
	}
}

func applyFile(cfg *Config) {
	if cfg.ConfigFilePath == "" {
		return
	}
	content, err := os.ReadFile(cfg.ConfigFilePath)
	if err != nil {
		return
	}
	var raw fileConfig
	if err := json.Unmarshal(content, &raw); err != nil {
		return
	}
	if raw.Env != "" {
		cfg.Env = raw.Env
	}
	if raw.HTTPAddr != "" {
		cfg.HTTPAddr = raw.HTTPAddr
	}
	if raw.DBDriver != "" {
		cfg.DBDriver = raw.DBDriver
	}
	if raw.DBDSN != "" {
		cfg.DBDSN = raw.DBDSN
	}
	if raw.StoragePath != "" {
		cfg.StoragePath = raw.StoragePath
	}
	if len(raw.MediaPaths) > 0 {
		cfg.MediaPaths = raw.MediaPaths
	}
	if raw.RequestTimeout > 0 {
		cfg.RequestTimeout = time.Duration(raw.RequestTimeout) * time.Second
	}
	if raw.DownloadMaxConcurrent > 0 {
		cfg.DownloadMaxConcurrent = raw.DownloadMaxConcurrent
	}
	cfg.DownloadAutoResume = raw.DownloadAutoResume

	if raw.MistralAPIKey != "" {
		cfg.MistralAPIKey = raw.MistralAPIKey
	}
	if raw.MistralEmbedModel != "" {
		cfg.MistralEmbedModel = raw.MistralEmbedModel
	}
	if raw.MistralChatModel != "" {
		cfg.MistralChatModel = raw.MistralChatModel
	}
	if raw.AIEnabled != nil {
		cfg.AIEnabled = *raw.AIEnabled
	}
	if raw.AITopK > 0 {
		cfg.AITopK = raw.AITopK
	}

	if raw.DebridLinkAPIBase != "" {
		cfg.DebridLinkAPIBase = raw.DebridLinkAPIBase
	}
	if raw.DebridLinkOAuthClient != "" {
		cfg.DebridLinkOAuthClient = raw.DebridLinkOAuthClient
	}
	if raw.DebridLinkOAuthSecret != "" {
		cfg.DebridLinkOAuthSecret = raw.DebridLinkOAuthSecret
	}
	if raw.DebridLinkOAuthScope != "" {
		cfg.DebridLinkOAuthScope = raw.DebridLinkOAuthScope
	}
	if raw.FileServerAuthUser != "" {
		cfg.FileServerAuthUser = raw.FileServerAuthUser
	}
	if raw.FileServerAuthPass != "" {
		cfg.FileServerAuthPass = raw.FileServerAuthPass
	}
	if raw.FileServerEnabled != nil {
		cfg.FileServerEnabled = *raw.FileServerEnabled
	}
	if raw.WebDAVEnabled != nil {
		cfg.WebDAVEnabled = *raw.WebDAVEnabled
	}
	if raw.DLNAEnabled != nil {
		cfg.DLNAEnabled = *raw.DLNAEnabled
	}
	if raw.DLNABaseURL != "" {
		cfg.DLNABaseURL = raw.DLNABaseURL
	}
	if raw.SMBEnabled != nil {
		cfg.SMBEnabled = *raw.SMBEnabled
	}
	if raw.SMBShareName != "" {
		cfg.SMBShareName = raw.SMBShareName
	}
	if raw.SMBSharePath != "" {
		cfg.SMBSharePath = raw.SMBSharePath
	}
	if raw.SMBBinary != "" {
		cfg.SMBBinary = raw.SMBBinary
	}
	if raw.SMBStatusBinary != "" {
		cfg.SMBStatusBinary = raw.SMBStatusBinary
	}
	if raw.AuthEnabled != nil {
		cfg.AuthEnabled = *raw.AuthEnabled
	}
	if raw.AuthUser != "" {
		cfg.AuthUser = raw.AuthUser
	}
	if raw.AuthPass != "" {
		cfg.AuthPass = raw.AuthPass
	}
	if raw.AuthSessionSecret != "" {
		cfg.AuthSessionSecret = raw.AuthSessionSecret
	}
	if raw.AuthSessionTTL > 0 {
		cfg.AuthSessionTTL = time.Duration(raw.AuthSessionTTL) * time.Hour
	}
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("APP_ENV"); v != "" {
		cfg.Env = v
	}
	if v := os.Getenv("HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if v := os.Getenv("DB_DRIVER"); v != "" {
		cfg.DBDriver = v
	}
	if v := os.Getenv("DB_DSN"); v != "" {
		cfg.DBDSN = v
	}
	if v := os.Getenv("STORAGE_PATH"); v != "" {
		cfg.StoragePath = v
	}
	if v := os.Getenv("MEDIA_PATH"); v != "" {
		cfg.MediaPaths = []string{v}
	}
	if v := os.Getenv("MEDIA_PATHS"); v != "" {
		cfg.MediaPaths = splitCSV(v)
	}
	if parsed, ok := parsePositiveInt(os.Getenv("HTTP_TIMEOUT_SECONDS")); ok {
		cfg.RequestTimeout = time.Duration(parsed) * time.Second
	}
	if parsed, ok := parsePositiveInt(os.Getenv("DOWNLOAD_MAX_CONCURRENT")); ok {
		cfg.DownloadMaxConcurrent = parsed
	}
	if parsed, ok := parseBool(os.Getenv("DOWNLOAD_AUTO_RESUME")); ok {
		cfg.DownloadAutoResume = parsed
	}

	if v := os.Getenv("MISTRAL_API_KEY"); v != "" {
		cfg.MistralAPIKey = v
	}
	if v := os.Getenv("MISTRAL_EMBED_MODEL"); v != "" {
		cfg.MistralEmbedModel = v
	}
	if v := os.Getenv("MISTRAL_CHAT_MODEL"); v != "" {
		cfg.MistralChatModel = v
	}
	if parsed, ok := parseBool(os.Getenv("AI_ENABLED")); ok {
		cfg.AIEnabled = parsed
	}
	if parsed, ok := parsePositiveInt(os.Getenv("AI_TOP_K")); ok {
		cfg.AITopK = parsed
	}

	if v := os.Getenv("DEBRIDLINK_API_BASE"); v != "" {
		cfg.DebridLinkAPIBase = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("DEBRIDLINK_OAUTH_CLIENT_ID"); v != "" {
		cfg.DebridLinkOAuthClient = v
	}
	if v := os.Getenv("DEBRIDLINK_OAUTH_CLIENT_SECRET"); v != "" {
		cfg.DebridLinkOAuthSecret = v
	}
	if v := os.Getenv("DEBRIDLINK_OAUTH_SCOPE"); v != "" {
		cfg.DebridLinkOAuthScope = v
	}
	if v := os.Getenv("FILESERVER_AUTH_USER"); v != "" {
		cfg.FileServerAuthUser = v
	}
	if v := os.Getenv("FILESERVER_AUTH_PASS"); v != "" {
		cfg.FileServerAuthPass = v
	}
	if parsed, ok := parseBool(os.Getenv("FILESERVER_ENABLED")); ok {
		cfg.FileServerEnabled = parsed
	}
	if parsed, ok := parseBool(os.Getenv("WEBDAV_ENABLED")); ok {
		cfg.WebDAVEnabled = parsed
	}
	if parsed, ok := parseBool(os.Getenv("DLNA_ENABLED")); ok {
		cfg.DLNAEnabled = parsed
	}
	if v := os.Getenv("DLNA_BASE_URL"); v != "" {
		cfg.DLNABaseURL = v
	}
	if parsed, ok := parseBool(os.Getenv("SMB_ENABLED")); ok {
		cfg.SMBEnabled = parsed
	}
	if v := os.Getenv("SMB_SHARE_NAME"); v != "" {
		cfg.SMBShareName = v
	}
	if v := os.Getenv("SMB_SHARE_PATH"); v != "" {
		cfg.SMBSharePath = v
	}
	if v := os.Getenv("SMB_BINARY"); v != "" {
		cfg.SMBBinary = v
	}
	if v := os.Getenv("SMB_STATUS_BINARY"); v != "" {
		cfg.SMBStatusBinary = v
	}
	if parsed, ok := parseBool(os.Getenv("AUTH_ENABLED")); ok {
		cfg.AuthEnabled = parsed
	}
	if v := os.Getenv("AUTH_USER"); v != "" {
		cfg.AuthUser = v
	}
	if v := os.Getenv("AUTH_PASS"); v != "" {
		cfg.AuthPass = v
	}
	if v := os.Getenv("AUTH_SESSION_SECRET"); v != "" {
		cfg.AuthSessionSecret = v
	}
	if parsed, ok := parsePositiveInt(os.Getenv("AUTH_SESSION_TTL_HOURS")); ok {
		cfg.AuthSessionTTL = time.Duration(parsed) * time.Hour
	}

	if !filepath.IsAbs(cfg.StoragePath) {
		cfg.StoragePath = filepath.Clean(cfg.StoragePath)
	}
	for i := range cfg.MediaPaths {
		cfg.MediaPaths[i] = filepath.Clean(cfg.MediaPaths[i])
	}
	if cfg.SMBSharePath == "" {
		cfg.SMBSharePath = cfg.StoragePath
	}
	if !filepath.IsAbs(cfg.SMBSharePath) {
		cfg.SMBSharePath = filepath.Clean(cfg.SMBSharePath)
	}
}

func getOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parsePositiveInt(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func parseBool(raw string) (bool, bool) {
	if raw == "" {
		return false, false
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false, false
	}
	return parsed, true
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return []string{"./data/media"}
	}
	return out
}
