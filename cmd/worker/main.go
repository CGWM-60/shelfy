package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"cgwm/shelfy/internal/config"
	"cgwm/shelfy/internal/debrid"
	"cgwm/shelfy/internal/debrid/providers/fake"
	"cgwm/shelfy/internal/downloader"
	"cgwm/shelfy/internal/events"
	"cgwm/shelfy/internal/infra/httpclient"
	"cgwm/shelfy/internal/infra/logger"
	"cgwm/shelfy/internal/infra/storage"
	"cgwm/shelfy/internal/repo"
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
	engine := downloader.NewEngine(r, httpclient.NewStreaming(cfg.RequestTimeout), storage.LocalFS{}, events.NewBus(), log, cfg.StoragePath)
	engine.Configure(cfg.DownloadMaxConcurrent, cfg.DownloadAutoResume)
	if err := engine.Recover(context.Background()); err != nil {
		log.Error().Err(err).Msg("recover")
	}

	registry := debrid.NewRegistry()
	registry.Register(fake.New())
	_ = debrid.NewService(r, registry)

	log.Info().
		Str("service", "worker").
		Str("env", cfg.Env).
		Str("storage_path", cfg.StoragePath).
		Strs("media_paths", cfg.MediaPaths).
		Int("download_max_concurrent", cfg.DownloadMaxConcurrent).
		Bool("download_auto_resume", cfg.DownloadAutoResume).
		Bool("download_auto_group", cfg.DownloadAutoGroup).
		Bool("ai_enabled", cfg.AIEnabled).
		Bool("dlna_enabled", cfg.DLNAEnabled).
		Bool("smb_enabled", cfg.SMBEnabled).
		Msg("worker startup configuration")
	log.Info().Msg("worker ready")
	<-waitSignalContext()
	log.Info().Msg("worker stopped")
}

func waitSignalContext() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	return ch
}
