package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

const defaultCookieName = "shelfy_session"

var ErrInvalidConfig = errors.New("invalid auth config")

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

type Config struct {
	Enabled    bool
	Username   string
	Password   string
	Secret     string
	CookieName string
	SessionTTL time.Duration
	Clock      Clock
}

type Manager struct {
	enabled    bool
	username   string
	password   string
	secret     []byte
	cookieName string
	sessionTTL time.Duration
	clock      Clock
}

type tokenPayload struct {
	Username string `json:"u"`
	Expiry   int64  `json:"e"`
}

func NewManager(cfg Config) (*Manager, error) {
	if !cfg.Enabled {
		return &Manager{enabled: false}, nil
	}
	if strings.TrimSpace(cfg.Username) == "" || strings.TrimSpace(cfg.Password) == "" || strings.TrimSpace(cfg.Secret) == "" {
		return nil, ErrInvalidConfig
	}
	cookieName := strings.TrimSpace(cfg.CookieName)
	if cookieName == "" {
		cookieName = defaultCookieName
	}
	ttl := cfg.SessionTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	clock := cfg.Clock
	if clock == nil {
		clock = realClock{}
	}
	return &Manager{
		enabled:    true,
		username:   cfg.Username,
		password:   cfg.Password,
		secret:     []byte(cfg.Secret),
		cookieName: cookieName,
		sessionTTL: ttl,
		clock:      clock,
	}, nil
}

func (m *Manager) Enabled() bool {
	if m == nil {
		return false
	}
	return m.enabled
}

func (m *Manager) CookieName() string {
	if m == nil || m.cookieName == "" {
		return defaultCookieName
	}
	return m.cookieName
}

func (m *Manager) CheckCredentials(username, password string) bool {
	if !m.Enabled() {
		return true
	}
	return secureEqual(m.username, username) && secureEqual(m.password, password)
}

func (m *Manager) AuthenticateRequest(r *http.Request) (string, bool) {
	if !m.Enabled() {
		return "", true
	}
	cookie, err := r.Cookie(m.CookieName())
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", false
	}
	username, ok := m.ValidateToken(cookie.Value)
	return username, ok
}

func (m *Manager) ValidateToken(token string) (string, bool) {
	if !m.Enabled() {
		return "", true
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return "", false
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	signatureRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	expectedSignature := sign(m.secret, payloadRaw)
	if !hmac.Equal(signatureRaw, expectedSignature) {
		return "", false
	}
	var payload tokenPayload
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		return "", false
	}
	if strings.TrimSpace(payload.Username) == "" {
		return "", false
	}
	if payload.Expiry <= m.clock.Now().Unix() {
		return "", false
	}
	return payload.Username, true
}

func (m *Manager) NewSessionCookie(username string, secure bool) (*http.Cookie, error) {
	if !m.Enabled() {
		return nil, nil
	}
	payload := tokenPayload{
		Username: username,
		Expiry:   m.clock.Now().Add(m.sessionTTL).Unix(),
	}
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	signature := sign(m.secret, payloadRaw)
	token := base64.RawURLEncoding.EncodeToString(payloadRaw) + "." + base64.RawURLEncoding.EncodeToString(signature)
	return &http.Cookie{
		Name:     m.CookieName(),
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(m.sessionTTL.Seconds()),
	}, nil
}

func (m *Manager) ExpiredCookie(secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     m.CookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
}

func sign(secret, payload []byte) []byte {
	h := hmac.New(sha256.New, secret)
	_, _ = h.Write(payload)
	return h.Sum(nil)
}

func secureEqual(expected, provided string) bool {
	expectedBytes := []byte(expected)
	providedBytes := []byte(provided)
	if len(expectedBytes) == 0 || len(providedBytes) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare(expectedBytes, providedBytes) == 1
}
