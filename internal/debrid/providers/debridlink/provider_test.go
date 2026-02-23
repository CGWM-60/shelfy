package debridlink

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cgwm/shelfy/internal/debrid"
)

func TestDeviceFlowAndUnrestrict(t *testing.T) {
	pollCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/oauth/device/code":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method %s", r.Method)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"device_code":      "dev-1",
				"user_code":        "ABCD-1234",
				"verification_url": "https://debrid-link.com/device",
				"interval":         3,
				"expires_in":       900,
			})
		case "/api/oauth/token":
			pollCount++
			if pollCount == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  "access-token",
				"refresh_token": "refresh-token",
				"expires_in":    3600,
			})
		case "/api/v2/account/infos":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				t.Fatalf("missing bearer")
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "value": map[string]string{"username": "demo"}})
		case "/api/v2/downloader/add":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"value": map[string]interface{}{
					"id":          "x1",
					"name":        "one.bin",
					"downloadUrl": "https://files.test/one.bin",
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	provider := New(server.Client(), Config{APIBaseURL: server.URL, OAuthClientID: "client-id"})
	session, err := provider.StartAuth(context.Background())
	if err != nil {
		t.Fatalf("start auth: %v", err)
	}
	if session.DeviceCode == "" || session.UserCode == "" {
		t.Fatalf("bad session: %+v", session)
	}

	_, done, err := provider.PollAuth(context.Background(), session)
	if err != nil {
		t.Fatalf("poll1: %v", err)
	}
	if done {
		t.Fatalf("expected pending")
	}
	token, done, err := provider.PollAuth(context.Background(), session)
	if err != nil {
		t.Fatalf("poll2: %v", err)
	}
	if !done || token.AccessToken == "" {
		t.Fatalf("expected token done=%v token=%+v", done, token)
	}
	if err := provider.ValidateToken(context.Background(), token); err != nil {
		t.Fatalf("validate token: %v", err)
	}

	result, err := provider.Unrestrict(context.Background(), "https://host/file", debrid.UnrestrictOptions{Account: debrid.Account{AccessToken: token.AccessToken}})
	if err != nil {
		t.Fatalf("unrestrict: %v", err)
	}
	if result.Type != "file" || result.DirectURL == "" {
		t.Fatalf("bad result: %+v", result)
	}
}

func TestUnrestrictFolder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/downloader/add" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": true,
				"value": []map[string]interface{}{
					{"id": "f1", "name": "file1.bin", "downloadUrl": "https://files.test/file1.bin"},
					{"id": "f2", "name": "file2.bin", "downloadUrl": "https://files.test/file2.bin"},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	provider := New(server.Client(), Config{APIBaseURL: server.URL, OAuthClientID: "client-id"})
	result, err := provider.Unrestrict(context.Background(), "https://host/folder", debrid.UnrestrictOptions{Account: debrid.Account{AccessToken: "token"}})
	if err != nil {
		t.Fatalf("unrestrict folder: %v", err)
	}
	if result.Type != "folder" || result.Folder == nil || len(result.Folder.Files) != 2 {
		t.Fatalf("bad folder result: %+v", result)
	}
}

func TestUnrestrictWithPassword(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/downloader/add" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["url"] != "https://host/file" {
			t.Fatalf("url=%s", body["url"])
		}
		if body["password"] != "folder-pass" {
			t.Fatalf("password=%s", body["password"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"value": map[string]interface{}{
				"id":          "x1",
				"name":        "one.bin",
				"downloadUrl": "https://files.test/one.bin",
			},
		})
	}))
	defer server.Close()

	provider := New(server.Client(), Config{APIBaseURL: server.URL, OAuthClientID: "client-id"})
	_, err := provider.Unrestrict(context.Background(), "https://host/file", debrid.UnrestrictOptions{
		Password: "folder-pass",
		Account:  debrid.Account{AccessToken: "token"},
	})
	if err != nil {
		t.Fatalf("unrestrict with password: %v", err)
	}
}

func TestPasswordAuthFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/oauth/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("grant_type") != "password" {
			t.Fatalf("grant type=%s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_id") != "client-id" {
			t.Fatalf("client_id=%s", r.Form.Get("client_id"))
		}
		if r.Form.Get("client_secret") != "client-secret" {
			t.Fatalf("client_secret=%s", r.Form.Get("client_secret"))
		}
		if r.Form.Get("username") != "demo@example.com" || r.Form.Get("password") != "secret" {
			t.Fatalf("bad credentials")
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "pwd-access",
			"refresh_token": "pwd-refresh",
			"expires_in":    1800,
		})
	}))
	defer server.Close()

	provider := New(server.Client(), Config{APIBaseURL: server.URL, OAuthClientID: "client-id", OAuthClientSecret: "client-secret"})
	token, err := provider.AuthWithPassword(context.Background(), "demo@example.com", "secret")
	if err != nil {
		t.Fatalf("password auth: %v", err)
	}
	if token.AccessToken != "pwd-access" {
		t.Fatalf("unexpected token: %+v", token)
	}
}
