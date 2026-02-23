package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeAuthManager struct {
	enabled  bool
	username string
	ok       bool
}

func (f fakeAuthManager) Enabled() bool {
	return f.enabled
}

func (f fakeAuthManager) AuthenticateRequest(*http.Request) (string, bool) {
	return f.username, f.ok
}

func TestRequireAPIAuthDisabled(t *testing.T) {
	t.Parallel()

	mw := RequireAPIAuth(fakeAuthManager{enabled: false})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/downloads", nil))
	if res.Code != http.StatusNoContent {
		t.Fatalf("status=%d", res.Code)
	}
}

func TestRequireAPIAuthUnauthorized(t *testing.T) {
	t.Parallel()

	mw := RequireAPIAuth(fakeAuthManager{enabled: true, ok: false})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/downloads", nil))
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", res.Code)
	}
}

func TestRequireAPIAuthAuthorizedSetsUser(t *testing.T) {
	t.Parallel()

	mw := RequireAPIAuth(fakeAuthManager{enabled: true, ok: true, username: "admin"})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := AuthUserFromContext(r.Context()); got != "admin" {
			t.Fatalf("user=%q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/downloads", nil))
	if res.Code != http.StatusNoContent {
		t.Fatalf("status=%d", res.Code)
	}
}
