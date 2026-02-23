package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"cgwm/shelfy/internal/ai"
	"cgwm/shelfy/internal/api"
	"cgwm/shelfy/internal/api/handlers"
	"cgwm/shelfy/internal/auth"
	"cgwm/shelfy/internal/config"
	"cgwm/shelfy/internal/debrid"
	"cgwm/shelfy/internal/debrid/providers/debridlink"
	"cgwm/shelfy/internal/debrid/providers/fake"
	"cgwm/shelfy/internal/dlna"
	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/downloader"
	"cgwm/shelfy/internal/events"
	"cgwm/shelfy/internal/fileserver"
	"cgwm/shelfy/internal/infra/httpclient"
	"cgwm/shelfy/internal/infra/logger"
	"cgwm/shelfy/internal/infra/storage"
	"cgwm/shelfy/internal/media"
	"cgwm/shelfy/internal/repo"
	"cgwm/shelfy/internal/service"
	"cgwm/shelfy/internal/smb"
	"github.com/rs/zerolog"
)

func main() {
	cfg := config.Load()
	log := logger.NewJSONLogger(nil)

	db, err := repo.OpenDB(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("open db")
	}
	if err := repo.AutoMigrate(db); err != nil {
		log.Fatal().Err(err).Msg("migrate db")
	}

	r := repo.NewGormRepository(db)
	bus := events.NewBus()
	httpc := httpclient.New(cfg.RequestTimeout)
	downloadHTTP := httpclient.NewStreaming(cfg.RequestTimeout)
	engine := downloader.NewEngine(r, downloadHTTP, storage.LocalFS{}, bus, log, cfg.StoragePath)
	engine.Configure(cfg.DownloadMaxConcurrent, cfg.DownloadAutoResume)
	if err := engine.Recover(context.Background()); err != nil {
		log.Error().Err(err).Msg("recover downloads")
	}

	registry := debrid.NewRegistry()
	registry.Register(fake.New())
	registry.Register(debridlink.New(httpc, debridlink.Config{
		APIBaseURL:        cfg.DebridLinkAPIBase,
		OAuthClientID:     cfg.DebridLinkOAuthClient,
		OAuthClientSecret: cfg.DebridLinkOAuthSecret,
		OAuthScope:        cfg.DebridLinkOAuthScope,
	}))
	debridSvc := debrid.NewService(r, registry)

	scanner := media.NewScanner(r, media.OSWalker{}, bus)
	mediaSvc := media.NewService(r, scanner)
	fileSvc := fileserver.NewService(r, storage.LocalFS{})
	dlnaSvc := dlna.NewService(nil, cfg.DLNAEnabled)
	smbBackend := smb.NewSambaBackend(nil, nil, cfg.SMBBinary, cfg.SMBStatusBinary, filepath.Join(os.TempDir(), "shelfy"))
	smbSvc := smb.NewService(smbBackend, cfg.SMBEnabled, smb.Config{ShareName: cfg.SMBShareName, SharePath: cfg.SMBSharePath})

	var embed ai.EmbeddingProvider = ai.FakeEmbeddingProvider{}
	var llm ai.LLMProvider = ai.FakeLLMProvider{}
	if cfg.AIEnabled && cfg.MistralAPIKey != "" {
		embed = ai.NewMistralEmbeddingProvider(httpc, cfg.MistralAPIKey, cfg.MistralEmbedModel)
		llm = ai.NewMistralLLMProvider(httpc, cfg.MistralAPIKey, cfg.MistralChatModel)
	}
	aiSvc := ai.NewService(r, embed, llm)
	aiSvc.SetDefaultTopK(cfg.AITopK)

	defaultSettings := domain.AppSettings{
		DownloadMaxConcurrent: cfg.DownloadMaxConcurrent,
		DownloadAutoResume:    cfg.DownloadAutoResume,
		DownloadsPath:         cfg.StoragePath,
		LibraryPaths:          cfg.MediaPaths,
		AIEnabled:             cfg.AIEnabled,
		AITopK:                cfg.AITopK,
		Theme:                 "clair",
		VisibleColumns:        []string{"nom", "etat", "progression", "vitesse", "eta"},
		FileServerAuthEnabled: cfg.FileServerAuthUser != "" || cfg.FileServerAuthPass != "",
		FileServerAuthUser:    cfg.FileServerAuthUser,
		DLNAEnabled:           cfg.DLNAEnabled,
		SMBEnabled:            cfg.SMBEnabled,
		SMBShareName:          cfg.SMBShareName,
		SMBSharePath:          cfg.SMBSharePath,
	}
	settingsSvc := service.NewSettingsService(r, defaultSettings)
	if err := smbSvc.ApplySettings(context.Background(), cfg.SMBEnabled, smb.Config{ShareName: cfg.SMBShareName, SharePath: cfg.SMBSharePath}); err != nil {
		log.Warn().Err(err).Msg("smb start failed")
	}
	engine.SetOnCompletedHook(func(ctx context.Context, job domain.DownloadJob) error {
		if strings.TrimSpace(job.DestinationPath) == "" {
			return nil
		}
		settings, err := settingsSvc.Get(ctx)
		if err != nil {
			return err
		}
		destAbs, err := filepath.Abs(filepath.Clean(job.DestinationPath))
		if err != nil {
			return err
		}
		for _, root := range settings.LibraryPaths {
			if strings.TrimSpace(root) == "" {
				continue
			}
			rootAbs, err := filepath.Abs(filepath.Clean(root))
			if err != nil {
				continue
			}
			if destAbs == rootAbs || strings.HasPrefix(destAbs, rootAbs+string(filepath.Separator)) {
				_, scanErr := mediaSvc.Scan(ctx, root)
				return scanErr
			}
		}
		return nil
	})

	appSvc := &service.App{
		Downloads: service.NewDownloadService(r, engine, debridSvc, bus),
		Debrid:    debridSvc,
		Media:     mediaSvc,
		AI:        aiSvc,
		Files:     fileSvc,
		Settings:  settingsSvc,
		DLNA:      dlnaSvc,
		SMB:       smbSvc,
	}
	authManager, err := auth.NewManager(auth.Config{
		Enabled:    cfg.AuthEnabled,
		Username:   cfg.AuthUser,
		Password:   cfg.AuthPass,
		Secret:     cfg.AuthSessionSecret,
		SessionTTL: cfg.AuthSessionTTL,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("auth config")
	}
	mediaRoot := cfg.StoragePath
	if len(cfg.MediaPaths) > 0 {
		mediaRoot = cfg.MediaPaths[0]
	}
	h := handlers.New(appSvc, bus, mediaRoot, storage.LocalFS{}, cfg.FileServerAuthUser, cfg.FileServerAuthPass, authManager)
	router := api.NewRouter(h, log)

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: router}
	dlnaBaseURL := strings.TrimSpace(cfg.DLNABaseURL)
	if dlnaBaseURL == "" {
		dlnaBaseURL = inferDLNABaseURL(cfg.HTTPAddr)
	}
	dlnaLocation := strings.TrimRight(dlnaBaseURL, "/") + "/dlna/device.xml"
	ssdpResponder := dlna.NewSSDPResponder(dlnaSvc, dlnaLocation)
	ssdpResponder.SetIdentity("uuid:shelfy", "shelfy/1.0 UPnP/1.0 DLNA/1.5")
	ssdpCtx, ssdpCancel := context.WithCancel(context.Background())
	defer ssdpCancel()
	go func() {
		if err := ssdpResponder.Run(ssdpCtx); err != nil {
			log.Warn().Err(err).Msg("dlna ssdp responder stopped")
		}
	}()
	go func() {
		logAPIBootSummary(log, cfg, dlnaBaseURL)
		log.Info().Str("addr", cfg.HTTPAddr).Msg("api listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("listen")
		}
	}()

	<-signalCtx()
	ssdpCancel()
	_ = server.Close()
}

func signalCtx() <-chan os.Signal {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	return c
}

func inferDLNABaseURL(httpAddr string) string {
	host, port, err := net.SplitHostPort(httpAddr)
	if err != nil {
		trimmed := strings.TrimSpace(httpAddr)
		if strings.HasPrefix(trimmed, ":") {
			port = strings.TrimPrefix(trimmed, ":")
			host = ""
		} else {
			host = trimmed
		}
	}
	if port == "" {
		port = "8080"
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		if guessed := firstLocalIPv4(); guessed != "" {
			host = guessed
		} else {
			host = "127.0.0.1"
		}
	}
	if p, err := strconv.Atoi(port); err == nil && p == 80 {
		return "http://" + host
	}
	return "http://" + net.JoinHostPort(host, port)
}

func firstLocalIPv4() string {
	type candidate struct {
		ip    string
		score int
	}
	best := candidate{}

	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(iface.Name))
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			v4 := ip.To4()
			if v4 == nil {
				continue
			}
			score := 0
			if iface.Flags&net.FlagBroadcast != 0 {
				score += 10
			}
			if iface.Flags&net.FlagPointToPoint == 0 {
				score += 5
			}
			if strings.HasPrefix(name, "en") || strings.HasPrefix(name, "eth") || strings.HasPrefix(name, "wlan") {
				score += 30
			}
			if isPrivateIPv4(v4) {
				score += 20
			}
			if v4[0] == 169 && v4[1] == 254 {
				score -= 100
			}
			if strings.Contains(name, "utun") || strings.Contains(name, "bridge") || strings.Contains(name, "docker") || strings.Contains(name, "vbox") || strings.Contains(name, "vmnet") || strings.Contains(name, "tailscale") || strings.Contains(name, "awdl") || strings.Contains(name, "llw") {
				score -= 50
			}
			if score > best.score {
				best = candidate{ip: v4.String(), score: score}
			}
		}
	}
	return best.ip
}

func isPrivateIPv4(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	switch {
	case v4[0] == 10:
		return true
	case v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31:
		return true
	case v4[0] == 192 && v4[1] == 168:
		return true
	default:
		return false
	}
}

func logAPIBootSummary(log zerolog.Logger, cfg config.Config, dlnaBaseURL string) {
	apiBaseURL := inferDLNABaseURL(cfg.HTTPAddr)
	uiBaseURL := guessUIBaseURL(dlnaBaseURL, apiBaseURL)
	dlnaDeviceURL := strings.TrimRight(dlnaBaseURL, "/") + "/dlna/device.xml"
	healthURL := strings.TrimRight(apiBaseURL, "/") + "/api/health"

	log.Info().
		Str("service", "api").
		Str("env", cfg.Env).
		Str("http_addr", cfg.HTTPAddr).
		Str("api_url", apiBaseURL).
		Str("ui_url", uiBaseURL).
		Str("health_url", healthURL).
		Str("dlna_device_url", dlnaDeviceURL).
		Str("db_driver", cfg.DBDriver).
		Str("db_dsn", redactedDSN(cfg.DBDriver, cfg.DBDSN)).
		Str("storage_path", cfg.StoragePath).
		Strs("media_paths", cfg.MediaPaths).
		Int("download_max_concurrent", cfg.DownloadMaxConcurrent).
		Bool("download_auto_resume", cfg.DownloadAutoResume).
		Bool("ai_enabled", cfg.AIEnabled).
		Bool("auth_enabled", cfg.AuthEnabled).
		Bool("dlna_enabled", cfg.DLNAEnabled).
		Bool("smb_enabled", cfg.SMBEnabled).
		Msg("startup configuration")

	log.Info().
		Str("ui", uiBaseURL).
		Str("api", apiBaseURL).
		Str("health", healthURL).
		Str("dlna", dlnaDeviceURL).
		Msg("startup endpoints")
	log.Info().Msg(fmt.Sprintf("BOOT_ENDPOINTS ui=%s api=%s health=%s dlna=%s", uiBaseURL, apiBaseURL, healthURL, dlnaDeviceURL))
}

func guessUIBaseURL(dlnaBaseURL, apiBaseURL string) string {
	if trimmed := strings.TrimSpace(dlnaBaseURL); trimmed != "" {
		return strings.TrimRight(trimmed, "/")
	}
	u, err := url.Parse(strings.TrimSpace(apiBaseURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return apiBaseURL
	}
	host := u.Host
	if currentHost, currentPort, splitErr := net.SplitHostPort(u.Host); splitErr == nil {
		host = currentHost
		if currentPort == "8080" {
			return strings.TrimRight(apiBaseURL, "/")
		}
	}
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}
	u.Host = net.JoinHostPort(host, "8080")
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/")
}

func redactedDSN(driver, dsn string) string {
	trimmed := strings.TrimSpace(dsn)
	if trimmed == "" {
		return trimmed
	}
	driverName := strings.ToLower(strings.TrimSpace(driver))
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") || strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err == nil && parsed.User != nil {
			if _, hasPassword := parsed.User.Password(); hasPassword {
				parsed.User = url.UserPassword(parsed.User.Username(), "***")
				return parsed.String()
			}
		}
		return trimmed
	}
	if driverName == "mysql" || driverName == "mariadb" {
		at := strings.Index(trimmed, "@")
		if at > 0 {
			credentials := trimmed[:at]
			if colon := strings.Index(credentials, ":"); colon >= 0 {
				return credentials[:colon] + ":***" + trimmed[at:]
			}
		}
	}
	return trimmed
}
