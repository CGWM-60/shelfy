package auth

import (
	"net/http/httptest"
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (c fakeClock) Now() time.Time {
	return c.now
}

func TestNewManagerRejectsInvalidConfigWhenEnabled(t *testing.T) {
	t.Parallel()

	_, err := NewManager(Config{Enabled: true, Username: "admin", Password: "", Secret: "secret"})
	if err == nil {
		t.Fatalf("expected invalid config error")
	}
}

func TestManagerSessionLifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)
	manager, err := NewManager(Config{
		Enabled:    true,
		Username:   "admin",
		Password:   "password",
		Secret:     "top-secret",
		SessionTTL: 2 * time.Hour,
		Clock:      fakeClock{now: now},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if !manager.CheckCredentials("admin", "password") {
		t.Fatalf("expected valid credentials")
	}
	if manager.CheckCredentials("admin", "wrong") {
		t.Fatalf("unexpected credentials accepted")
	}

	cookie, err := manager.NewSessionCookie("admin", false)
	if err != nil {
		t.Fatalf("new session cookie: %v", err)
	}
	req := httptest.NewRequest("GET", "/api/downloads", nil)
	req.AddCookie(cookie)
	username, ok := manager.AuthenticateRequest(req)
	if !ok || username != "admin" {
		t.Fatalf("expected authenticated request, ok=%v username=%q", ok, username)
	}

	tampered := *cookie
	tampered.Value = cookie.Value + "corrupt"
	reqTampered := httptest.NewRequest("GET", "/api/downloads", nil)
	reqTampered.AddCookie(&tampered)
	if _, ok := manager.AuthenticateRequest(reqTampered); ok {
		t.Fatalf("tampered cookie should be rejected")
	}
}

func TestManagerRejectsExpiredToken(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)
	manager, err := NewManager(Config{
		Enabled:    true,
		Username:   "admin",
		Password:   "password",
		Secret:     "top-secret",
		SessionTTL: time.Minute,
		Clock:      fakeClock{now: now},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	cookie, err := manager.NewSessionCookie("admin", false)
	if err != nil {
		t.Fatalf("new session cookie: %v", err)
	}

	expiredManager, err := NewManager(Config{
		Enabled:    true,
		Username:   "admin",
		Password:   "password",
		Secret:     "top-secret",
		SessionTTL: time.Minute,
		Clock:      fakeClock{now: now.Add(2 * time.Minute)},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	req := httptest.NewRequest("GET", "/api/downloads", nil)
	req.AddCookie(cookie)
	if _, ok := expiredManager.AuthenticateRequest(req); ok {
		t.Fatalf("expired cookie should be rejected")
	}
}
