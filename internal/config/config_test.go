package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPriorityEnvOverFile(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_DSN", "test.db")
	t.Setenv("CONFIG_FILE", filepath.Join(t.TempDir(), "config.json"))

	content := `{"httpAddr":":7070","downloadMaxConcurrent":2,"aiTopK":7}`
	if err := os.WriteFile(os.Getenv("CONFIG_FILE"), []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := Load()
	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("env must override file, got %s", cfg.HTTPAddr)
	}
	if cfg.DownloadMaxConcurrent != 2 {
		t.Fatalf("file value not applied")
	}
	if cfg.AITopK != 7 {
		t.Fatalf("file value not applied")
	}
}

func TestLoadAuthEnv(t *testing.T) {
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_USER", "admin")
	t.Setenv("AUTH_PASS", "secret")
	t.Setenv("AUTH_SESSION_SECRET", "session-secret")
	t.Setenv("AUTH_SESSION_TTL_HOURS", "48")
	t.Setenv("CONFIG_FILE", filepath.Join(t.TempDir(), "missing.json"))

	cfg := Load()
	if !cfg.AuthEnabled {
		t.Fatalf("expected auth enabled")
	}
	if cfg.AuthUser != "admin" {
		t.Fatalf("unexpected auth user: %q", cfg.AuthUser)
	}
	if cfg.AuthPass != "secret" {
		t.Fatalf("unexpected auth pass")
	}
	if cfg.AuthSessionSecret != "session-secret" {
		t.Fatalf("unexpected auth session secret")
	}
	if cfg.AuthSessionTTL.Hours() != 48 {
		t.Fatalf("unexpected auth session ttl: %s", cfg.AuthSessionTTL)
	}
}
