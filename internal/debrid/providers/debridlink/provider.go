package debridlink

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cgwm/shelfy/internal/debrid"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type Config struct {
	APIBaseURL        string
	OAuthClientID     string
	OAuthClientSecret string
	OAuthScope        string
}

type Provider struct {
	client HTTPClient
	cfg    Config
}

func New(client HTTPClient, cfg Config) *Provider {
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://debrid-link.com"
	}
	cfg.APIBaseURL = strings.TrimRight(cfg.APIBaseURL, "/")
	if cfg.OAuthScope == "" {
		cfg.OAuthScope = "get.account get.post.delete.downloader get.files get.post.stream"
	}
	return &Provider{client: client, cfg: cfg}
}

func (p *Provider) Name() string {
	return "debridlink"
}

func (p *Provider) AuthType() debrid.AuthType {
	return debrid.AuthTypeOAuth2Device
}

func (p *Provider) StartAuth(ctx context.Context) (debrid.AuthSession, error) {
	if p.client == nil {
		return debrid.AuthSession{}, errors.New("missing HTTP client")
	}
	if p.cfg.OAuthClientID == "" {
		return debrid.AuthSession{}, errors.New("DEBRIDLINK_OAUTH_CLIENT_ID missing")
	}
	form := url.Values{}
	form.Set("client_id", p.cfg.OAuthClientID)
	form.Set("scope", p.cfg.OAuthScope)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.APIBaseURL+"/api/oauth/device/code", strings.NewReader(form.Encode()))
	if err != nil {
		return debrid.AuthSession{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return debrid.AuthSession{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return debrid.AuthSession{}, fmt.Errorf("debridlink device code status %d: %s", resp.StatusCode, string(body))
	}
	var payload struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURL string `json:"verification_url"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return debrid.AuthSession{}, err
	}
	if payload.DeviceCode == "" {
		return debrid.AuthSession{}, errors.New("debridlink missing device_code")
	}
	verification := payload.VerificationURL
	if verification == "" {
		verification = strings.TrimRight(p.cfg.APIBaseURL, "/") + "/device"
	}
	return debrid.AuthSession{
		SessionID:       payload.DeviceCode,
		DeviceCode:      payload.DeviceCode,
		UserCode:        payload.UserCode,
		VerificationURI: verification,
		IntervalSec:     payload.Interval,
		ExpiresIn:       payload.ExpiresIn,
	}, nil
}

func (p *Provider) PollAuth(ctx context.Context, session debrid.AuthSession) (debrid.Token, bool, error) {
	if p.client == nil {
		return debrid.Token{}, false, errors.New("missing HTTP client")
	}
	if p.cfg.OAuthClientID == "" {
		return debrid.Token{}, false, errors.New("DEBRIDLINK_OAUTH_CLIENT_ID missing")
	}
	deviceCode := session.DeviceCode
	if deviceCode == "" {
		deviceCode = session.SessionID
	}
	if deviceCode == "" {
		return debrid.Token{}, false, errors.New("missing device code")
	}

	firstTry := func(grantType string) (*http.Response, error) {
		form := url.Values{}
		setOAuthClient(form, p.cfg.OAuthClientID, p.cfg.OAuthClientSecret)
		form.Set("code", deviceCode)
		form.Set("grant_type", grantType)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.APIBaseURL+"/api/oauth/token", strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return p.client.Do(req)
	}

	resp, err := firstTry("http://oauth.net/grant_type/device/1.0")
	if err != nil {
		return debrid.Token{}, false, err
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var oauthErr struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &oauthErr)
		if oauthErr.Error == "unsupported_grant_type" {
			resp, err = firstTry("urn:ietf:params:oauth:grant-type:device_code")
			if err != nil {
				return debrid.Token{}, false, err
			}
		} else {
			return parsePollResponse(resp.StatusCode, body)
		}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return parsePollResponse(resp.StatusCode, body)
}

func (p *Provider) AuthWithPassword(ctx context.Context, username, password string) (debrid.Token, error) {
	if p.client == nil {
		return debrid.Token{}, errors.New("missing HTTP client")
	}
	if p.cfg.OAuthClientID == "" {
		return debrid.Token{}, errors.New("DEBRIDLINK_OAUTH_CLIENT_ID missing")
	}
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return debrid.Token{}, errors.New("username and password are required")
	}

	form := url.Values{}
	setOAuthClient(form, p.cfg.OAuthClientID, p.cfg.OAuthClientSecret)
	form.Set("grant_type", "password")
	form.Set("username", username)
	form.Set("password", password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.APIBaseURL+"/api/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return debrid.Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return debrid.Token{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return debrid.Token{}, fmt.Errorf("debridlink oauth password error: %s", strings.TrimSpace(string(body)))
	}
	token, err := parseOAuthToken(body)
	if err != nil {
		return debrid.Token{}, err
	}
	return token, nil
}

func parsePollResponse(status int, body []byte) (debrid.Token, bool, error) {
	if status >= 300 {
		var oauthErr struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &oauthErr)
		switch oauthErr.Error {
		case "authorization_pending", "slow_down":
			return debrid.Token{}, false, nil
		default:
			return debrid.Token{}, false, fmt.Errorf("debridlink oauth error: %s", strings.TrimSpace(string(body)))
		}
	}
	token, err := parseOAuthToken(body)
	if err != nil {
		return debrid.Token{}, false, err
	}
	return token, true, nil
}

func parseOAuthToken(body []byte) (debrid.Token, error) {
	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return debrid.Token{}, err
	}
	if payload.AccessToken == "" {
		return debrid.Token{}, errors.New("debridlink token missing access_token")
	}
	var expiry *time.Time
	if payload.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
		expiry = &t
	}
	return debrid.Token{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, Expiry: expiry}, nil
}

func setOAuthClient(form url.Values, clientID, clientSecret string) {
	form.Set("client_id", clientID)
	if strings.TrimSpace(clientSecret) != "" {
		form.Set("client_secret", clientSecret)
	}
}

func (p *Provider) ValidateToken(ctx context.Context, token debrid.Token) error {
	bearer := strings.TrimSpace(token.AccessToken)
	if bearer == "" {
		return errors.New("missing access token")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.APIBaseURL+"/api/v2/account/infos", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("debridlink validate token status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (p *Provider) Unrestrict(ctx context.Context, link string, opts debrid.UnrestrictOptions) (debrid.DebridResult, error) {
	if p.client == nil {
		return debrid.DebridResult{}, errors.New("missing HTTP client")
	}
	token := strings.TrimSpace(opts.Account.AccessToken)
	if token == "" {
		token = strings.TrimSpace(opts.Account.APIKey)
	}
	if token == "" {
		return debrid.DebridResult{}, errors.New("debridlink account token/api key missing")
	}

	requestBody := map[string]string{"url": link}
	if strings.TrimSpace(opts.Password) != "" {
		requestBody["password"] = opts.Password
	}
	payload, _ := json.Marshal(requestBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.APIBaseURL+"/api/v2/downloader/add", bytes.NewReader(payload))
	if err != nil {
		return debrid.DebridResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return debrid.DebridResult{}, err
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return debrid.DebridResult{}, fmt.Errorf("debridlink add status %d: %s", resp.StatusCode, string(responseBody))
	}

	var envelope struct {
		Success bool            `json:"success"`
		Value   json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return debrid.DebridResult{}, err
	}
	if len(envelope.Value) == 0 {
		return debrid.DebridResult{}, errors.New("debridlink empty value")
	}
	if envelope.Value[0] == '{' {
		file, err := parseDLItem(envelope.Value)
		if err != nil {
			return debrid.DebridResult{}, err
		}
		return debrid.DebridResult{Provider: p.Name(), Type: "file", DirectURL: file.URL, File: &file}, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(envelope.Value, &items); err != nil {
		return debrid.DebridResult{}, err
	}
	files := make([]debrid.FileInfo, 0, len(items))
	for _, raw := range items {
		file, err := parseDLItem(raw)
		if err != nil {
			continue
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return debrid.DebridResult{}, errors.New("debridlink folder empty")
	}
	return debrid.DebridResult{Provider: p.Name(), Type: "folder", Folder: &debrid.FolderInfo{Name: "debridlink-folder", Files: files}}, nil
}

func parseDLItem(raw json.RawMessage) (debrid.FileInfo, error) {
	var item struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DownloadURL string `json:"downloadUrl"`
		URL         string `json:"url"`
		Size        int64  `json:"size"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return debrid.FileInfo{}, err
	}
	direct := item.DownloadURL
	if direct == "" {
		direct = item.URL
	}
	if direct == "" {
		return debrid.FileInfo{}, errors.New("missing download URL")
	}
	name := item.Name
	if name == "" {
		name = "file.bin"
	}
	return debrid.FileInfo{ID: item.ID, Name: name, URL: direct, Size: item.Size}, nil
}

func (p *Provider) ListFolder(ctx context.Context, link string) (debrid.FolderInfo, error) {
	result, err := p.Unrestrict(ctx, link, debrid.UnrestrictOptions{})
	if err != nil {
		return debrid.FolderInfo{}, err
	}
	if result.Folder == nil {
		return debrid.FolderInfo{}, errors.New("not a folder")
	}
	return *result.Folder, nil
}

func (p *Provider) GetStreamURL(ctx context.Context, fileID string) (string, error) {
	return "", errors.New("stream URL not supported")
}
