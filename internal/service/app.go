package service

import (
	"cgwm/shelfy/internal/ai"
	"cgwm/shelfy/internal/debrid"
	"cgwm/shelfy/internal/dlna"
	"cgwm/shelfy/internal/fileserver"
	"cgwm/shelfy/internal/media"
	"cgwm/shelfy/internal/smb"
)

type App struct {
	Downloads *DownloadService
	Debrid    *debrid.Service
	Media     *media.Service
	AI        *ai.Service
	Files     *fileserver.Service
	Settings  *SettingsService
	DLNA      *dlna.Service
	SMB       *smb.Service
}
