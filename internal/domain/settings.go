package domain

type AppSettings struct {
	DownloadMaxConcurrent int      `json:"downloadMaxConcurrent"`
	DownloadAutoResume    bool     `json:"downloadAutoResume"`
	DownloadsPath         string   `json:"downloadsPath"`
	LibraryPaths          []string `json:"libraryPaths"`
	GlobalRateLimitKB     int      `json:"globalRateLimitKB"`
	AIEnabled             bool     `json:"aiEnabled"`
	AITopK                int      `json:"aiTopK"`
	Theme                 string   `json:"theme"`
	VisibleColumns        []string `json:"visibleColumns"`
	FileServerAuthEnabled bool     `json:"fileServerAuthEnabled"`
	FileServerAuthUser    string   `json:"fileServerAuthUser"`
	DLNAEnabled           bool     `json:"dlnaEnabled"`
	SMBEnabled            bool     `json:"smbEnabled"`
	SMBShareName          string   `json:"smbShareName"`
	SMBSharePath          string   `json:"smbSharePath"`
}
