package api

import (
	"net/http"

	"cgwm/shelfy/internal/api/handlers"
	"cgwm/shelfy/internal/api/middlewares"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

func NewRouter(h *handlers.Handler, log zerolog.Logger) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(middlewares.CorrelationID)
	r.Use(middlewares.Logging(log))

	r.Route("/api", func(r chi.Router) {
		r.Get("/health", h.GetHealth)
		r.Get("/auth/status", h.GetAuthStatus)
		r.Post("/auth/login", h.PostAuthLogin)
		r.Post("/auth/logout", h.PostAuthLogout)

		r.Group(func(r chi.Router) {
			r.Use(middlewares.RequireAPIAuth(h.AuthManager()))

			r.Post("/downloads", h.PostDownloads)
			r.Get("/downloads", h.GetDownloads)
			r.Post("/downloads/{id}/start", h.PostDownloadStart)
			r.Post("/downloads/{id}/pause", h.PostDownloadPause)
			r.Post("/downloads/{id}/resume", h.PostDownloadResume)
			r.Delete("/downloads/{id}", h.DeleteDownload)
			r.Get("/events", h.GetEvents)

			r.Get("/debrid/providers", h.GetDebridProviders)
			r.Post("/debrid/accounts", h.PostDebridAccount)
			r.Get("/debrid/accounts", h.GetDebridAccounts)
			r.Patch("/debrid/accounts/{id}", h.PatchDebridAccount)
			r.Delete("/debrid/accounts/{id}", h.DeleteDebridAccount)
			r.Post("/debrid/accounts/{id}/auth/start", h.PostDebridAuthStart)
			r.Post("/debrid/accounts/{id}/auth/poll", h.PostDebridAuthPoll)
			r.Post("/debrid/accounts/{id}/auth/password", h.PostDebridAuthPassword)
			r.Get("/debrid/accounts/{id}/status", h.GetDebridStatus)

			r.Post("/media/scan", h.PostMediaScan)
			r.Get("/media", h.GetMedia)
			r.Get("/media/{id}", h.GetMediaByID)
			r.Get("/stream/{id}", h.GetStream)
			r.Get("/subtitles/{id}", h.GetSubtitles)
			r.Post("/media/{id}/progress", h.PostMediaProgress)

			r.Post("/ai/index", h.PostAIIndex)
			r.Post("/ai/search", h.PostAISearch)
			r.Post("/ai/ask", h.PostAIAsk)
			r.Get("/ai/status", h.GetAIStatus)
			r.Get("/ai/report", h.GetAIReport)

			r.Get("/settings", h.GetSettings)
			r.Put("/settings", h.PutSettings)
			r.Get("/system/storage", h.GetSystemStorage)
			r.Post("/system/speedtest", h.PostSystemSpeedtest)
			r.Get("/dlna/devices", h.GetDLNADevices)
			r.Post("/dlna/scan", h.PostDLNAScan)
			r.Get("/dlna/debug", h.GetDLNADebug)
			r.Get("/smb/status", h.GetSMBStatus)
			r.Post("/smb/refresh", h.PostSMBRefresh)

			r.Get("/fs/roots", h.GetFSRoots)
			r.Get("/fs/list", h.GetFSList)
			r.Get("/fs/file", h.GetFSFile)
			r.Post("/fs/mkdir", h.PostFSMkdir)
			r.Put("/fs/file", h.PutFSFile)
			r.Patch("/fs/move", h.PatchFSMove)
			r.Delete("/fs/item", h.DeleteFSItem)
		})
	})

	r.Get("/files/", h.GetFiles)
	r.Get("/files/{id}/stream", h.GetFileStream)
	r.Get("/files/{id}/download", h.GetFileDownload)
	r.Get("/dlna/device.xml", h.GetDLNADeviceDescription)
	r.Get("/dlna/scpd/content_directory.xml", h.GetDLNAContentDirectorySCPD)
	r.Get("/dlna/scpd/connection_manager.xml", h.GetDLNAConnectionManagerSCPD)
	r.Post("/dlna/control/content_directory", h.PostDLNAContentDirectoryControl)
	r.Post("/dlna/control/connection_manager", h.PostDLNAConnectionManagerControl)
	r.Handle("/dlna/event/content_directory", http.HandlerFunc(h.HandleDLNAEvent))
	r.Handle("/dlna/event/connection_manager", http.HandlerFunc(h.HandleDLNAEvent))
	r.Get("/dlna/media/{id}", h.GetDLNAMedia)
	r.Get("/upnp/device.xml", h.GetDLNADeviceDescription)
	r.Get("/upnp/scpd/content_directory.xml", h.GetDLNAContentDirectorySCPD)
	r.Get("/upnp/scpd/connection_manager.xml", h.GetDLNAConnectionManagerSCPD)
	r.Post("/upnp/control/content_directory", h.PostDLNAContentDirectoryControl)
	r.Post("/upnp/control/connection_manager", h.PostDLNAConnectionManagerControl)
	r.Handle("/upnp/event/content_directory", http.HandlerFunc(h.HandleDLNAEvent))
	r.Handle("/upnp/event/connection_manager", http.HandlerFunc(h.HandleDLNAEvent))
	r.Get("/upnp/media/{id}", h.GetDLNAMedia)

	return r
}
