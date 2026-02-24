package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"cgwm/shelfy/internal/ai"
	"cgwm/shelfy/internal/api"
	"cgwm/shelfy/internal/api/handlers"
	"cgwm/shelfy/internal/auth"
	"cgwm/shelfy/internal/debrid"
	fakeprovider "cgwm/shelfy/internal/debrid/providers/fake"
	"cgwm/shelfy/internal/dlna"
	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/downloader"
	"cgwm/shelfy/internal/events"
	"cgwm/shelfy/internal/fileserver"
	"cgwm/shelfy/internal/infra/httpclient"
	"cgwm/shelfy/internal/infra/logger"
	"cgwm/shelfy/internal/infra/storage"
	"cgwm/shelfy/internal/media"
	"cgwm/shelfy/internal/service"
	"cgwm/shelfy/internal/smb"
	"cgwm/shelfy/internal/testutil"
)

func newRouter(t *testing.T, mediaDir string) http.Handler {
	return newRouterWithFileAuth(t, mediaDir, "", "")
}

func newRouterWithFileAuth(t *testing.T, mediaDir, user, pass string) http.Handler {
	t.Helper()
	r := testutil.NewSQLiteRepo(t)
	bus := events.NewBus()
	engine := downloader.NewEngine(r, httpclient.New(5*time.Second), storage.LocalFS{}, bus, logger.NewJSONLogger(nil), filepath.Join(t.TempDir(), "downloads"))

	registry := debrid.NewRegistry()
	registry.Register(fakeprovider.New())
	debridSvc := debrid.NewService(r, registry)

	scanner := media.NewScanner(r, media.OSWalker{}, bus)
	mediaSvc := media.NewService(r, scanner)
	aiSvc := ai.NewService(r, ai.FakeEmbeddingProvider{}, ai.FakeLLMProvider{})
	fileSvc := fileserver.NewService(r, storage.LocalFS{})
	dlnaSvc := dlna.NewService(&fakeDLNADiscoverer{}, false)
	smbSvc := smb.NewService(&fakeSMBBackend{}, false, smb.Config{ShareName: "shelfy", SharePath: t.TempDir()})
	settingsSvc := service.NewSettingsService(r, domain.AppSettings{DownloadMaxConcurrent: 3, DownloadAutoResume: true, DownloadsPath: t.TempDir(), LibraryPaths: []string{mediaDir}, AIEnabled: true, AITopK: 5, Theme: "clair", VisibleColumns: []string{"nom"}, DLNAEnabled: false, SMBEnabled: false, SMBShareName: "shelfy", SMBSharePath: t.TempDir()})

	app := &service.App{
		Downloads: service.NewDownloadService(r, engine, debridSvc, bus),
		Debrid:    debridSvc,
		Media:     mediaSvc,
		AI:        aiSvc,
		Files:     fileSvc,
		Settings:  settingsSvc,
		DLNA:      dlnaSvc,
		SMB:       smbSvc,
	}
	return api.NewRouter(handlers.New(app, bus, mediaDir, storage.LocalFS{}, user, pass, nil), logger.NewJSONLogger(nil))
}

func newRouterWithAPIAuth(t *testing.T, mediaDir string) http.Handler {
	t.Helper()
	r := testutil.NewSQLiteRepo(t)
	bus := events.NewBus()
	engine := downloader.NewEngine(r, httpclient.New(5*time.Second), storage.LocalFS{}, bus, logger.NewJSONLogger(nil), filepath.Join(t.TempDir(), "downloads"))

	registry := debrid.NewRegistry()
	registry.Register(fakeprovider.New())
	debridSvc := debrid.NewService(r, registry)
	scanner := media.NewScanner(r, media.OSWalker{}, bus)
	mediaSvc := media.NewService(r, scanner)
	aiSvc := ai.NewService(r, ai.FakeEmbeddingProvider{}, ai.FakeLLMProvider{})
	fileSvc := fileserver.NewService(r, storage.LocalFS{})
	dlnaSvc := dlna.NewService(&fakeDLNADiscoverer{}, false)
	smbSvc := smb.NewService(&fakeSMBBackend{}, false, smb.Config{ShareName: "shelfy", SharePath: t.TempDir()})
	settingsSvc := service.NewSettingsService(r, domain.AppSettings{DownloadMaxConcurrent: 3, DownloadAutoResume: true, DownloadsPath: t.TempDir(), LibraryPaths: []string{mediaDir}, AIEnabled: true, AITopK: 5, Theme: "clair", VisibleColumns: []string{"nom"}, DLNAEnabled: false, SMBEnabled: false, SMBShareName: "shelfy", SMBSharePath: t.TempDir()})

	app := &service.App{
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
		Enabled:    true,
		Username:   "admin",
		Password:   "secret",
		Secret:     "session-secret",
		SessionTTL: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("new auth manager: %v", err)
	}
	return api.NewRouter(handlers.New(app, bus, mediaDir, storage.LocalFS{}, "", "", authManager), logger.NewJSONLogger(nil))
}

type fakeDLNADiscoverer struct{}

func (fakeDLNADiscoverer) Discover(context.Context) ([]dlna.Device, error) {
	return []dlna.Device{
		{
			USN:      "uuid:tv-1",
			ST:       "urn:schemas-upnp-org:device:MediaRenderer:1",
			Server:   "DLNA/1.5",
			Location: "http://192.168.1.20:8200/device.xml",
			Address:  "192.168.1.20:1900",
		},
	}, nil
}

type fakeSMBBackend struct{}

func (fakeSMBBackend) Name() string { return "fake-smb" }
func (fakeSMBBackend) Start(context.Context, smb.Config) error {
	return nil
}
func (fakeSMBBackend) Stop(context.Context) error { return nil }
func (fakeSMBBackend) Running(context.Context) (bool, error) {
	return true, nil
}
func (fakeSMBBackend) Clients(context.Context) ([]smb.Client, error) {
	return []smb.Client{{Username: "guest", Machine: "VLC", Address: "192.168.1.33"}}, nil
}

func TestPostDownloadsValidation(t *testing.T) {
	router := newRouter(t, t.TempDir())
	req := httptest.NewRequest(http.MethodPost, "/api/downloads", bytes.NewBufferString("{"))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", res.Code)
	}
}

func TestPostDownloadsCreated(t *testing.T) {
	router := newRouter(t, t.TempDir())
	payload := map[string]interface{}{"links": []string{"https://example.test/file.bin"}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/downloads", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
}

func TestPostDownloadsDedupesAndAutoIndexesAI(t *testing.T) {
	router := newRouter(t, t.TempDir())
	payload := map[string]interface{}{
		"links": []string{
			"https://example.test/file.bin",
			"https://example.test/file.bin",
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/downloads", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var created struct {
		Data []domain.DownloadJob `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created jobs: %v body=%s", err, res.Body.String())
	}
	if len(created.Data) != 1 {
		t.Fatalf("expected deduped created jobs=1 got=%d body=%s", len(created.Data), res.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/downloads", nil)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	var listed struct {
		Data []domain.DownloadJob `json:"data"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode listed jobs: %v body=%s", err, listRes.Body.String())
	}
	if len(listed.Data) != 1 {
		t.Fatalf("expected persisted jobs=1 got=%d", len(listed.Data))
	}

	reportReq := httptest.NewRequest(http.MethodGet, "/api/ai/report", nil)
	reportRes := httptest.NewRecorder()
	router.ServeHTTP(reportRes, reportReq)
	if reportRes.Code != http.StatusOK {
		t.Fatalf("report status=%d body=%s", reportRes.Code, reportRes.Body.String())
	}
	var report struct {
		Data ai.Report `json:"data"`
	}
	if err := json.Unmarshal(reportRes.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v body=%s", err, reportRes.Body.String())
	}
	if report.Data.IndexRuns == 0 {
		t.Fatalf("expected auto ai indexing on downloads add, report=%+v", report.Data)
	}
}

func TestPostDownloadsWithDestinationDir(t *testing.T) {
	router := newRouter(t, t.TempDir())
	payload := map[string]interface{}{
		"links":          []string{"https://example.test/file.bin"},
		"destinationDir": "/tmp/custom-downloads",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/downloads", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "/tmp/custom-downloads") {
		t.Fatalf("expected destination path in response, body=%s", res.Body.String())
	}
}

func TestGetDownloadsKeepsQueueOrderForMultiAdd(t *testing.T) {
	router := newRouter(t, t.TempDir())

	first := "https://example.test/first.bin"
	second := "https://example.test/second.bin"
	payload := map[string]interface{}{
		"links": []string{first, second},
	}
	body, _ := json.Marshal(payload)

	createReq := httptest.NewRequest(http.MethodPost, "/api/downloads", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRes.Code, createRes.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/downloads", nil)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRes.Code, listRes.Body.String())
	}

	var decoded struct {
		Data []domain.DownloadJob `json:"data"`
	}
	if err := json.Unmarshal(listRes.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode list: %v body=%s", err, listRes.Body.String())
	}
	if len(decoded.Data) < 2 {
		t.Fatalf("expected at least 2 downloads, got=%d body=%s", len(decoded.Data), listRes.Body.String())
	}
	if decoded.Data[0].SourceLink != first || decoded.Data[1].SourceLink != second {
		t.Fatalf("unexpected queue order: got=[%s, %s] want=[%s, %s]", decoded.Data[0].SourceLink, decoded.Data[1].SourceLink, first, second)
	}
}

func TestDebridAccountEndpoints(t *testing.T) {
	router := newRouter(t, t.TempDir())

	getProviders := httptest.NewRequest(http.MethodGet, "/api/debrid/providers", nil)
	providersRes := httptest.NewRecorder()
	router.ServeHTTP(providersRes, getProviders)
	if providersRes.Code != http.StatusOK {
		t.Fatalf("providers status=%d", providersRes.Code)
	}

	createBody, _ := json.Marshal(map[string]interface{}{
		"provider":  "fake",
		"label":     "Mon compte",
		"isActive":  true,
		"isDefault": true,
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/debrid/accounts", bytes.NewReader(createBody))
	createRes := httptest.NewRecorder()
	router.ServeHTTP(createRes, createReq)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("create account status=%d body=%s", createRes.Code, createRes.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/debrid/accounts", nil)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list accounts status=%d", listRes.Code)
	}

	authStartReq := httptest.NewRequest(http.MethodPost, "/api/debrid/accounts/1/auth/start", nil)
	authStartRes := httptest.NewRecorder()
	router.ServeHTTP(authStartRes, authStartReq)
	if authStartRes.Code != http.StatusOK {
		t.Fatalf("auth start status=%d body=%s", authStartRes.Code, authStartRes.Body.String())
	}

	var authPayload struct {
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(authStartRes.Body.Bytes(), &authPayload)
	pollBody, _ := json.Marshal(authPayload.Data)
	authPollReq := httptest.NewRequest(http.MethodPost, "/api/debrid/accounts/1/auth/poll", bytes.NewReader(pollBody))
	authPollRes := httptest.NewRecorder()
	router.ServeHTTP(authPollRes, authPollReq)
	if authPollRes.Code != http.StatusOK {
		t.Fatalf("auth poll status=%d body=%s", authPollRes.Code, authPollRes.Body.String())
	}
	authPollRes = httptest.NewRecorder()
	authPollReq = httptest.NewRequest(http.MethodPost, "/api/debrid/accounts/1/auth/poll", bytes.NewReader(pollBody))
	router.ServeHTTP(authPollRes, authPollReq)
	if authPollRes.Code != http.StatusOK {
		t.Fatalf("auth poll 2 status=%d body=%s", authPollRes.Code, authPollRes.Body.String())
	}

	authPassBody, _ := json.Marshal(map[string]string{"username": "demo@example.com", "password": "secret"})
	authPassReq := httptest.NewRequest(http.MethodPost, "/api/debrid/accounts/1/auth/password", bytes.NewReader(authPassBody))
	authPassRes := httptest.NewRecorder()
	router.ServeHTTP(authPassRes, authPassReq)
	if authPassRes.Code != http.StatusOK {
		t.Fatalf("auth password status=%d body=%s", authPassRes.Code, authPassRes.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/debrid/accounts/1/status", nil)
	statusRes := httptest.NewRecorder()
	router.ServeHTTP(statusRes, statusReq)
	if statusRes.Code != http.StatusOK {
		t.Fatalf("status endpoint=%d body=%s", statusRes.Code, statusRes.Body.String())
	}
}

func TestAIStatus(t *testing.T) {
	router := newRouter(t, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/api/ai/status", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d", res.Code)
	}
}

func TestAPIAuthLoginFlow(t *testing.T) {
	router := newRouterWithAPIAuth(t, t.TempDir())

	statusReq := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusRes := httptest.NewRecorder()
	router.ServeHTTP(statusRes, statusReq)
	if statusRes.Code != http.StatusOK {
		t.Fatalf("status auth=%d body=%s", statusRes.Code, statusRes.Body.String())
	}
	if !strings.Contains(statusRes.Body.String(), `"enabled":true`) {
		t.Fatalf("unexpected auth status body=%s", statusRes.Body.String())
	}

	protectedReq := httptest.NewRequest(http.MethodGet, "/api/downloads", nil)
	protectedRes := httptest.NewRecorder()
	router.ServeHTTP(protectedRes, protectedReq)
	if protectedRes.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got=%d body=%s", protectedRes.Code, protectedRes.Body.String())
	}

	badLoginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "wrong"})
	badLoginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(badLoginBody))
	badLoginRes := httptest.NewRecorder()
	router.ServeHTTP(badLoginRes, badLoginReq)
	if badLoginRes.Code != http.StatusUnauthorized {
		t.Fatalf("expected bad login unauthorized, got=%d", badLoginRes.Code)
	}

	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "secret"})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRes := httptest.NewRecorder()
	router.ServeHTTP(loginRes, loginReq)
	if loginRes.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", loginRes.Code, loginRes.Body.String())
	}
	cookies := loginRes.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie")
	}

	protectedWithCookieReq := httptest.NewRequest(http.MethodGet, "/api/downloads", nil)
	protectedWithCookieReq.AddCookie(cookies[0])
	protectedWithCookieRes := httptest.NewRecorder()
	router.ServeHTTP(protectedWithCookieRes, protectedWithCookieReq)
	if protectedWithCookieRes.Code != http.StatusOK {
		t.Fatalf("expected protected access with cookie, got=%d body=%s", protectedWithCookieRes.Code, protectedWithCookieRes.Body.String())
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(cookies[0])
	logoutRes := httptest.NewRecorder()
	router.ServeHTTP(logoutRes, logoutReq)
	if logoutRes.Code != http.StatusOK {
		t.Fatalf("logout status=%d body=%s", logoutRes.Code, logoutRes.Body.String())
	}
	logoutCookies := logoutRes.Result().Cookies()
	if len(logoutCookies) == 0 || logoutCookies[0].MaxAge != -1 {
		t.Fatalf("expected expired cookie on logout")
	}
}

func TestAIReport(t *testing.T) {
	router := newRouter(t, t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/api/ai/report", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}

	var body struct {
		Data struct {
			EmbeddingProvider string `json:"embeddingProvider"`
			LLMProvider       string `json:"llmProvider"`
			UptimeSec         int64  `json:"uptimeSec"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if body.Data.EmbeddingProvider != "fake" || body.Data.LLMProvider != "fake" {
		t.Fatalf("unexpected providers: %+v", body.Data)
	}
	if body.Data.UptimeSec < 0 {
		t.Fatalf("invalid uptime: %d", body.Data.UptimeSec)
	}
}

func TestMediaScanAutoIndexesAI(t *testing.T) {
	mediaDir := t.TempDir()
	filePath := filepath.Join(mediaDir, "movie.mp4")
	if err := os.WriteFile(filePath, []byte("movie"), 0o644); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	router := newRouter(t, mediaDir)

	scanBody, _ := json.Marshal(map[string]string{"path": mediaDir})
	scanReq := httptest.NewRequest(http.MethodPost, "/api/media/scan", bytes.NewReader(scanBody))
	scanReq.Header.Set("Content-Type", "application/json")
	scanRes := httptest.NewRecorder()
	router.ServeHTTP(scanRes, scanReq)
	if scanRes.Code != http.StatusOK {
		t.Fatalf("scan status=%d body=%s", scanRes.Code, scanRes.Body.String())
	}

	reportReq := httptest.NewRequest(http.MethodGet, "/api/ai/report", nil)
	reportRes := httptest.NewRecorder()
	router.ServeHTTP(reportRes, reportReq)
	if reportRes.Code != http.StatusOK {
		t.Fatalf("report status=%d body=%s", reportRes.Code, reportRes.Body.String())
	}
	var report struct {
		Data ai.Report `json:"data"`
	}
	if err := json.Unmarshal(reportRes.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode report: %v body=%s", err, reportRes.Body.String())
	}
	if report.Data.IndexRuns == 0 || report.Data.LastIndexedCount == 0 {
		t.Fatalf("expected auto ai indexing on media scan, report=%+v", report.Data)
	}
}

func TestSettingsEndpoints(t *testing.T) {
	router := newRouter(t, t.TempDir())
	getReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	getRes := httptest.NewRecorder()
	router.ServeHTTP(getRes, getReq)
	if getRes.Code != http.StatusOK {
		t.Fatalf("get status=%d", getRes.Code)
	}

	putBody, _ := json.Marshal(map[string]interface{}{"downloadMaxConcurrent": 2, "downloadAutoResume": false, "downloadsPath": "/tmp", "libraryPaths": []string{"/tmp/lib"}, "aiEnabled": true, "aiTopK": 4, "theme": "sombre", "visibleColumns": []string{"nom"}, "fileServerAuthEnabled": false})
	putReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(putBody))
	putRes := httptest.NewRecorder()
	router.ServeHTTP(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", putRes.Code, putRes.Body.String())
	}
}

func TestSystemStorageEndpoint(t *testing.T) {
	mediaDir := t.TempDir()
	router := newRouter(t, mediaDir)

	downloadBody, _ := json.Marshal(map[string]interface{}{
		"links":          []string{"https://example.test/file.bin"},
		"destinationDir": mediaDir,
	})
	downloadReq := httptest.NewRequest(http.MethodPost, "/api/downloads", bytes.NewReader(downloadBody))
	downloadRes := httptest.NewRecorder()
	router.ServeHTTP(downloadRes, downloadReq)
	if downloadRes.Code != http.StatusCreated {
		t.Fatalf("create download status=%d body=%s", downloadRes.Code, downloadRes.Body.String())
	}

	storageReq := httptest.NewRequest(http.MethodGet, "/api/system/storage", nil)
	storageRes := httptest.NewRecorder()
	router.ServeHTTP(storageRes, storageReq)
	if storageRes.Code != http.StatusOK {
		t.Fatalf("storage status=%d body=%s", storageRes.Code, storageRes.Body.String())
	}

	var payload struct {
		Data struct {
			Roots []struct {
				Path           string `json:"path"`
				Exists         bool   `json:"exists"`
				KnownUsedBytes int64  `json:"knownUsedBytes"`
			} `json:"roots"`
			Totals struct {
				KnownUsedBytes int64 `json:"knownUsedBytes"`
			} `json:"totals"`
		} `json:"data"`
	}
	if err := json.Unmarshal(storageRes.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode storage payload: %v body=%s", err, storageRes.Body.String())
	}
	if len(payload.Data.Roots) == 0 {
		t.Fatalf("expected at least one storage root")
	}
	if payload.Data.Totals.KnownUsedBytes < 0 {
		t.Fatalf("invalid known used bytes total=%d", payload.Data.Totals.KnownUsedBytes)
	}
}

func TestSystemSpeedtestEndpoint(t *testing.T) {
	mediaDir := t.TempDir()
	router := newRouter(t, mediaDir)

	reqBody, _ := json.Marshal(map[string]int{"sizeMB": 1})
	speedReq := httptest.NewRequest(http.MethodPost, "/api/system/speedtest", bytes.NewReader(reqBody))
	speedRes := httptest.NewRecorder()
	router.ServeHTTP(speedRes, speedReq)
	if speedRes.Code != http.StatusOK {
		t.Fatalf("speedtest status=%d body=%s", speedRes.Code, speedRes.Body.String())
	}

	var payload struct {
		Data struct {
			Path       string  `json:"path"`
			SampleMB   int     `json:"sampleMB"`
			WriteMBps  float64 `json:"writeMBps"`
			ReadMBps   float64 `json:"readMBps"`
			DurationMs int64   `json:"durationMs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(speedRes.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode speedtest payload: %v body=%s", err, speedRes.Body.String())
	}
	if payload.Data.SampleMB != 1 {
		t.Fatalf("expected sampleMB=1 got=%d", payload.Data.SampleMB)
	}
	if payload.Data.Path == "" {
		t.Fatalf("expected speedtest path in response")
	}
	if payload.Data.WriteMBps < 0 || payload.Data.ReadMBps < 0 {
		t.Fatalf("invalid throughput write=%.4f read=%.4f", payload.Data.WriteMBps, payload.Data.ReadMBps)
	}
}

func TestDLNASettingsAndScanEndpoints(t *testing.T) {
	router := newRouter(t, t.TempDir())

	initialReq := httptest.NewRequest(http.MethodGet, "/api/dlna/devices", nil)
	initialRes := httptest.NewRecorder()
	router.ServeHTTP(initialRes, initialReq)
	if initialRes.Code != http.StatusOK {
		t.Fatalf("initial dlna status=%d body=%s", initialRes.Code, initialRes.Body.String())
	}
	if !strings.Contains(initialRes.Body.String(), "\"enabled\":false") {
		t.Fatalf("expected disabled snapshot, body=%s", initialRes.Body.String())
	}

	putBody, _ := json.Marshal(map[string]interface{}{
		"downloadMaxConcurrent": 2,
		"downloadAutoResume":    true,
		"downloadsPath":         "/tmp",
		"libraryPaths":          []string{"/tmp/lib"},
		"aiEnabled":             true,
		"aiTopK":                4,
		"theme":                 "sombre",
		"visibleColumns":        []string{"nom"},
		"fileServerAuthEnabled": false,
		"dlnaEnabled":           true,
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(putBody))
	putRes := httptest.NewRecorder()
	router.ServeHTTP(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", putRes.Code, putRes.Body.String())
	}

	scanReq := httptest.NewRequest(http.MethodPost, "/api/dlna/scan", nil)
	scanRes := httptest.NewRecorder()
	router.ServeHTTP(scanRes, scanReq)
	if scanRes.Code != http.StatusOK {
		t.Fatalf("scan status=%d body=%s", scanRes.Code, scanRes.Body.String())
	}
	if !strings.Contains(scanRes.Body.String(), "\"uuid:tv-1\"") {
		t.Fatalf("expected fake device in scan result, body=%s", scanRes.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/dlna/devices", nil)
	listRes := httptest.NewRecorder()
	router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRes.Code, listRes.Body.String())
	}
	if !strings.Contains(listRes.Body.String(), "\"enabled\":true") {
		t.Fatalf("expected enabled snapshot after settings update, body=%s", listRes.Body.String())
	}
	if !strings.Contains(listRes.Body.String(), "\"uuid:tv-1\"") {
		t.Fatalf("expected fake device in list result, body=%s", listRes.Body.String())
	}

	debugReq := httptest.NewRequest(http.MethodGet, "/api/dlna/debug", nil)
	debugRes := httptest.NewRecorder()
	router.ServeHTTP(debugRes, debugReq)
	if debugRes.Code != http.StatusOK {
		t.Fatalf("debug status=%d body=%s", debugRes.Code, debugRes.Body.String())
	}
	if !strings.Contains(debugRes.Body.String(), "\"enabled\":true") {
		t.Fatalf("expected enabled debug snapshot, body=%s", debugRes.Body.String())
	}
}

func TestSMBStatusAndRefreshEndpoints(t *testing.T) {
	router := newRouter(t, t.TempDir())
	putBody, _ := json.Marshal(map[string]interface{}{
		"downloadMaxConcurrent": 2,
		"downloadAutoResume":    true,
		"downloadsPath":         "/tmp",
		"libraryPaths":          []string{"/tmp/lib"},
		"aiEnabled":             true,
		"aiTopK":                4,
		"theme":                 "clair",
		"visibleColumns":        []string{"nom"},
		"fileServerAuthEnabled": false,
		"smbEnabled":            true,
		"smbShareName":          "shelfy",
		"smbSharePath":          "/tmp/lib",
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(putBody))
	putRes := httptest.NewRecorder()
	router.ServeHTTP(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("put smb settings status=%d body=%s", putRes.Code, putRes.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/smb/status", nil)
	statusRes := httptest.NewRecorder()
	router.ServeHTTP(statusRes, statusReq)
	if statusRes.Code != http.StatusOK {
		t.Fatalf("smb status=%d body=%s", statusRes.Code, statusRes.Body.String())
	}
	if !strings.Contains(statusRes.Body.String(), "\"running\":true") {
		t.Fatalf("expected running smb backend, body=%s", statusRes.Body.String())
	}
	if !strings.Contains(statusRes.Body.String(), "\"guest\"") {
		t.Fatalf("expected smb clients, body=%s", statusRes.Body.String())
	}

	refreshReq := httptest.NewRequest(http.MethodPost, "/api/smb/refresh", nil)
	refreshRes := httptest.NewRecorder()
	router.ServeHTTP(refreshRes, refreshReq)
	if refreshRes.Code != http.StatusOK {
		t.Fatalf("smb refresh=%d body=%s", refreshRes.Code, refreshRes.Body.String())
	}
}

func TestDLNAServerEndpoints(t *testing.T) {
	mediaDir := t.TempDir()
	filePath := filepath.Join(mediaDir, "movie.mp4")
	if err := os.WriteFile(filePath, []byte("0123456789abcdef"), 0o644); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	router := newRouter(t, mediaDir)

	_ = postJSON(router, "/api/media/scan", map[string]string{"path": mediaDir})
	putBody, _ := json.Marshal(map[string]interface{}{
		"downloadMaxConcurrent": 3,
		"downloadAutoResume":    true,
		"downloadsPath":         mediaDir,
		"libraryPaths":          []string{mediaDir},
		"aiEnabled":             true,
		"aiTopK":                5,
		"theme":                 "clair",
		"visibleColumns":        []string{"nom"},
		"dlnaEnabled":           true,
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(putBody))
	putRes := httptest.NewRecorder()
	router.ServeHTTP(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("enable dlna status=%d body=%s", putRes.Code, putRes.Body.String())
	}

	descReq := httptest.NewRequest(http.MethodGet, "/dlna/device.xml", nil)
	descReq.Host = "127.0.0.1:8080"
	descRes := httptest.NewRecorder()
	router.ServeHTTP(descRes, descReq)
	if descRes.Code != http.StatusOK {
		t.Fatalf("device.xml status=%d body=%s", descRes.Code, descRes.Body.String())
	}
	if !strings.Contains(descRes.Body.String(), "MediaServer:1") {
		t.Fatalf("unexpected device description: %s", descRes.Body.String())
	}

	scpdReq := httptest.NewRequest(http.MethodGet, "/dlna/scpd/content_directory.xml", nil)
	scpdRes := httptest.NewRecorder()
	router.ServeHTTP(scpdRes, scpdReq)
	if scpdRes.Code != http.StatusOK {
		t.Fatalf("scpd status=%d body=%s", scpdRes.Code, scpdRes.Body.String())
	}
	if !strings.Contains(scpdRes.Body.String(), "<name>Browse</name>") {
		t.Fatalf("expected Browse action in scpd")
	}
	connSCPDReq := httptest.NewRequest(http.MethodGet, "/dlna/scpd/connection_manager.xml", nil)
	connSCPDRes := httptest.NewRecorder()
	router.ServeHTTP(connSCPDRes, connSCPDReq)
	if connSCPDRes.Code != http.StatusOK {
		t.Fatalf("connection manager scpd status=%d body=%s", connSCPDRes.Code, connSCPDRes.Body.String())
	}
	if !strings.Contains(connSCPDRes.Body.String(), "<name>GetProtocolInfo</name>") {
		t.Fatalf("expected GetProtocolInfo action in connection manager scpd")
	}

	soapPayload := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">
      <ObjectID>0</ObjectID>
      <BrowseFlag>BrowseDirectChildren</BrowseFlag>
      <Filter>*</Filter>
      <StartingIndex>0</StartingIndex>
      <RequestedCount>10</RequestedCount>
      <SortCriteria></SortCriteria>
    </u:Browse>
  </s:Body>
</s:Envelope>`
	controlReq := httptest.NewRequest(http.MethodPost, "/dlna/control/content_directory", strings.NewReader(soapPayload))
	controlReq.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)
	controlReq.Host = "127.0.0.1:8080"
	controlRes := httptest.NewRecorder()
	router.ServeHTTP(controlRes, controlReq)
	if controlRes.Code != http.StatusOK {
		t.Fatalf("control status=%d body=%s", controlRes.Code, controlRes.Body.String())
	}
	if !strings.Contains(controlRes.Body.String(), "/dlna/media/") {
		t.Fatalf("expected media URL in browse response: %s", controlRes.Body.String())
	}

	connControlReq := httptest.NewRequest(http.MethodPost, "/dlna/control/connection_manager", strings.NewReader(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/"><s:Body><u:GetProtocolInfo xmlns:u="urn:schemas-upnp-org:service:ConnectionManager:1"/></s:Body></s:Envelope>`))
	connControlReq.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:ConnectionManager:1#GetProtocolInfo"`)
	connControlRes := httptest.NewRecorder()
	router.ServeHTTP(connControlRes, connControlReq)
	if connControlRes.Code != http.StatusOK {
		t.Fatalf("connection manager control status=%d body=%s", connControlRes.Code, connControlRes.Body.String())
	}
	if !strings.Contains(connControlRes.Body.String(), "<u:GetProtocolInfoResponse") {
		t.Fatalf("expected protocol info response: %s", connControlRes.Body.String())
	}

	mediaListRes := httptest.NewRecorder()
	router.ServeHTTP(mediaListRes, httptest.NewRequest(http.MethodGet, "/api/media", nil))
	if mediaListRes.Code != http.StatusOK {
		t.Fatalf("list media status=%d body=%s", mediaListRes.Code, mediaListRes.Body.String())
	}
	var mediaPayload struct {
		Data []domain.MediaItem `json:"data"`
	}
	if err := json.Unmarshal(mediaListRes.Body.Bytes(), &mediaPayload); err != nil {
		t.Fatalf("decode media list: %v", err)
	}
	if len(mediaPayload.Data) == 0 {
		t.Fatalf("expected at least one media entry")
	}
	mediaID := mediaPayload.Data[0].ID

	streamReq := httptest.NewRequest(http.MethodGet, "/dlna/media/"+mediaID, nil)
	streamReq.Header.Set("Range", "bytes=0-3")
	streamRes := httptest.NewRecorder()
	router.ServeHTTP(streamRes, streamReq)
	if streamRes.Code != http.StatusPartialContent {
		t.Fatalf("dlna stream status=%d body=%s", streamRes.Code, streamRes.Body.String())
	}
	if streamRes.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("expected range headers")
	}

	devicesReq := httptest.NewRequest(http.MethodGet, "/api/dlna/devices", nil)
	devicesRes := httptest.NewRecorder()
	router.ServeHTTP(devicesRes, devicesReq)
	if devicesRes.Code != http.StatusOK {
		t.Fatalf("devices status=%d body=%s", devicesRes.Code, devicesRes.Body.String())
	}
	if !strings.Contains(devicesRes.Body.String(), "client:") {
		t.Fatalf("expected connected client entries, body=%s", devicesRes.Body.String())
	}
}

func TestDLNABrowseFoldersAndSubfolders(t *testing.T) {
	mediaDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(mediaDir, "Films", "Action"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "Films", "Action", "movie.mp4"), []byte("0123456789"), 0o644); err != nil {
		t.Fatalf("write media file: %v", err)
	}
	router := newRouter(t, mediaDir)

	_ = postJSON(router, "/api/media/scan", map[string]string{"path": mediaDir})
	putBody, _ := json.Marshal(map[string]interface{}{
		"downloadMaxConcurrent": 3,
		"downloadAutoResume":    true,
		"downloadsPath":         mediaDir,
		"libraryPaths":          []string{mediaDir},
		"aiEnabled":             true,
		"aiTopK":                5,
		"theme":                 "clair",
		"visibleColumns":        []string{"nom"},
		"dlnaEnabled":           true,
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(putBody))
	putRes := httptest.NewRecorder()
	router.ServeHTTP(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("enable dlna status=%d body=%s", putRes.Code, putRes.Body.String())
	}

	rootBrowse := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">
      <ObjectID>0</ObjectID>
      <BrowseFlag>BrowseDirectChildren</BrowseFlag>
      <Filter>*</Filter>
      <StartingIndex>0</StartingIndex>
      <RequestedCount>100</RequestedCount>
      <SortCriteria></SortCriteria>
    </u:Browse>
  </s:Body>
</s:Envelope>`
	rootReq := httptest.NewRequest(http.MethodPost, "/dlna/control/content_directory", strings.NewReader(rootBrowse))
	rootReq.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)
	rootReq.Host = "127.0.0.1:8080"
	rootRes := httptest.NewRecorder()
	router.ServeHTTP(rootRes, rootReq)
	if rootRes.Code != http.StatusOK {
		t.Fatalf("root browse status=%d body=%s", rootRes.Code, rootRes.Body.String())
	}
	rootBody := html.UnescapeString(rootRes.Body.String())
	if !strings.Contains(rootBody, "<container") || !strings.Contains(rootBody, "Films") {
		t.Fatalf("expected folder container in root browse: %s", rootRes.Body.String())
	}
	filmsID := extractContainerIDByTitle(t, rootBody, "Films")
	if filmsID == "" {
		t.Fatalf("expected Films container id")
	}

	subBrowse := strings.ReplaceAll(rootBrowse, "<ObjectID>0</ObjectID>", "<ObjectID>"+filmsID+"</ObjectID>")
	subReq := httptest.NewRequest(http.MethodPost, "/dlna/control/content_directory", strings.NewReader(subBrowse))
	subReq.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)
	subReq.Host = "127.0.0.1:8080"
	subRes := httptest.NewRecorder()
	router.ServeHTTP(subRes, subReq)
	if subRes.Code != http.StatusOK {
		t.Fatalf("sub browse status=%d body=%s", subRes.Code, subRes.Body.String())
	}
	subBody := html.UnescapeString(subRes.Body.String())
	if !strings.Contains(subBody, "<container") || !strings.Contains(subBody, "Action") {
		t.Fatalf("expected subfolder container: %s", subRes.Body.String())
	}
	actionID := extractContainerIDByTitle(t, subBody, "Action")
	if actionID == "" {
		t.Fatalf("expected Action container id")
	}

	leafBrowse := strings.ReplaceAll(rootBrowse, "<ObjectID>0</ObjectID>", "<ObjectID>"+actionID+"</ObjectID>")
	leafReq := httptest.NewRequest(http.MethodPost, "/dlna/control/content_directory", strings.NewReader(leafBrowse))
	leafReq.Header.Set("SOAPACTION", `"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`)
	leafReq.Host = "127.0.0.1:8080"
	leafRes := httptest.NewRecorder()
	router.ServeHTTP(leafRes, leafReq)
	if leafRes.Code != http.StatusOK {
		t.Fatalf("leaf browse status=%d body=%s", leafRes.Code, leafRes.Body.String())
	}
	leafBody := html.UnescapeString(leafRes.Body.String())
	if !strings.Contains(leafBody, "<item") || !strings.Contains(leafBody, "/dlna/media/") {
		t.Fatalf("expected media item in leaf browse: %s", leafRes.Body.String())
	}
}

func extractContainerIDByTitle(t *testing.T, body, title string) string {
	t.Helper()
	pattern := `<container id="([^"]+)"[^>]*><dc:title>` + regexp.QuoteMeta(title) + `</dc:title>`
	re := regexp.MustCompile(pattern)
	match := re.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func TestFSManagementAndLibraryRescan(t *testing.T) {
	mediaDir := t.TempDir()
	router := newRouter(t, mediaDir)

	rootsReq := httptest.NewRequest(http.MethodGet, "/api/fs/roots", nil)
	rootsRes := httptest.NewRecorder()
	router.ServeHTTP(rootsRes, rootsReq)
	if rootsRes.Code != http.StatusOK {
		t.Fatalf("roots status=%d body=%s", rootsRes.Code, rootsRes.Body.String())
	}

	createDirBody, _ := json.Marshal(map[string]string{"path": filepath.Join(mediaDir, "folder")})
	createDirReq := httptest.NewRequest(http.MethodPost, "/api/fs/mkdir", bytes.NewReader(createDirBody))
	createDirRes := httptest.NewRecorder()
	router.ServeHTTP(createDirRes, createDirReq)
	if createDirRes.Code != http.StatusNoContent {
		t.Fatalf("mkdir status=%d body=%s", createDirRes.Code, createDirRes.Body.String())
	}

	fileA := filepath.Join(mediaDir, "folder", "a.mp4")
	putFileBody, _ := json.Marshal(map[string]string{"path": fileA, "content": "video-a"})
	putFileReq := httptest.NewRequest(http.MethodPut, "/api/fs/file", bytes.NewReader(putFileBody))
	putFileRes := httptest.NewRecorder()
	router.ServeHTTP(putFileRes, putFileReq)
	if putFileRes.Code != http.StatusNoContent {
		t.Fatalf("put file status=%d body=%s", putFileRes.Code, putFileRes.Body.String())
	}

	getFileReq := httptest.NewRequest(http.MethodGet, "/api/fs/file?path="+url.QueryEscape(fileA), nil)
	getFileRes := httptest.NewRecorder()
	router.ServeHTTP(getFileRes, getFileReq)
	if getFileRes.Code != http.StatusOK {
		t.Fatalf("get file status=%d body=%s", getFileRes.Code, getFileRes.Body.String())
	}

	mediaRes := httptest.NewRecorder()
	router.ServeHTTP(mediaRes, httptest.NewRequest(http.MethodGet, "/api/media", nil))
	if mediaRes.Code != http.StatusOK {
		t.Fatalf("media list status=%d body=%s", mediaRes.Code, mediaRes.Body.String())
	}
	if !bytes.Contains(mediaRes.Body.Bytes(), []byte("a")) {
		t.Fatalf("expected media list to contain file a, body=%s", mediaRes.Body.String())
	}

	fileB := filepath.Join(mediaDir, "folder", "b.mp4")
	moveBody, _ := json.Marshal(map[string]string{"fromPath": fileA, "toPath": fileB})
	moveReq := httptest.NewRequest(http.MethodPatch, "/api/fs/move", bytes.NewReader(moveBody))
	moveRes := httptest.NewRecorder()
	router.ServeHTTP(moveRes, moveReq)
	if moveRes.Code != http.StatusNoContent {
		t.Fatalf("move status=%d body=%s", moveRes.Code, moveRes.Body.String())
	}

	mediaRes = httptest.NewRecorder()
	router.ServeHTTP(mediaRes, httptest.NewRequest(http.MethodGet, "/api/media", nil))
	if mediaRes.Code != http.StatusOK {
		t.Fatalf("media list status after move=%d body=%s", mediaRes.Code, mediaRes.Body.String())
	}
	body := mediaRes.Body.String()
	if !strings.Contains(body, "b") || strings.Contains(body, "\"title\":\"a\"") {
		t.Fatalf("unexpected media list after move: %s", body)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/fs/item?path="+url.QueryEscape(filepath.Join(mediaDir, "folder"))+"&recursive=true", nil)
	deleteRes := httptest.NewRecorder()
	router.ServeHTTP(deleteRes, deleteReq)
	if deleteRes.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleteRes.Code, deleteRes.Body.String())
	}

	mediaRes = httptest.NewRecorder()
	router.ServeHTTP(mediaRes, httptest.NewRequest(http.MethodGet, "/api/media", nil))
	if mediaRes.Code != http.StatusOK {
		t.Fatalf("media list status after delete=%d body=%s", mediaRes.Code, mediaRes.Body.String())
	}
	if strings.Contains(mediaRes.Body.String(), "\"title\":\"b\"") {
		t.Fatalf("expected b removed after recursive delete, body=%s", mediaRes.Body.String())
	}
}

func TestFSRootsNormalizeAndDedupeRelativePaths(t *testing.T) {
	workspace := t.TempDir()
	prevWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("chdir workspace: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prevWD) })

	downloadsRel := filepath.Join("data", "downloads")
	mediaRel := filepath.Join("data", "media")
	if err := os.MkdirAll(downloadsRel, 0o755); err != nil {
		t.Fatalf("mkdir downloads: %v", err)
	}
	if err := os.MkdirAll(mediaRel, 0o755); err != nil {
		t.Fatalf("mkdir media: %v", err)
	}

	router := newRouter(t, filepath.Join(workspace, mediaRel))
	putBody, _ := json.Marshal(map[string]interface{}{
		"downloadMaxConcurrent": 2,
		"downloadAutoResume":    true,
		"downloadsPath":         downloadsRel,
		"libraryPaths":          []string{downloadsRel, mediaRel},
		"aiEnabled":             true,
		"aiTopK":                5,
		"theme":                 "clair",
		"visibleColumns":        []string{"nom"},
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(putBody))
	putRes := httptest.NewRecorder()
	router.ServeHTTP(putRes, putReq)
	if putRes.Code != http.StatusOK {
		t.Fatalf("put settings status=%d body=%s", putRes.Code, putRes.Body.String())
	}

	getSettingsReq := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	getSettingsRes := httptest.NewRecorder()
	router.ServeHTTP(getSettingsRes, getSettingsReq)
	if getSettingsRes.Code != http.StatusOK {
		t.Fatalf("get settings status=%d body=%s", getSettingsRes.Code, getSettingsRes.Body.String())
	}
	var settingsPayload struct {
		Data domain.AppSettings `json:"data"`
	}
	if err := json.Unmarshal(getSettingsRes.Body.Bytes(), &settingsPayload); err != nil {
		t.Fatalf("decode settings: %v body=%s", err, getSettingsRes.Body.String())
	}
	currentSettings := settingsPayload.Data
	if currentSettings.DownloadsPath != downloadsRel || len(currentSettings.LibraryPaths) != 2 {
		t.Fatalf("unexpected settings after save: %+v", currentSettings)
	}

	rootsReq := httptest.NewRequest(http.MethodGet, "/api/fs/roots", nil)
	rootsRes := httptest.NewRecorder()
	router.ServeHTTP(rootsRes, rootsReq)
	if rootsRes.Code != http.StatusOK {
		t.Fatalf("roots status=%d body=%s", rootsRes.Code, rootsRes.Body.String())
	}
	var payload struct {
		Data struct {
			Roots []string `json:"roots"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rootsRes.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode roots: %v", err)
	}
	expectedDownloads, _ := filepath.Abs(downloadsRel)
	expectedMedia, _ := filepath.Abs(mediaRel)
	if resolved, err := filepath.EvalSymlinks(expectedDownloads); err == nil {
		expectedDownloads = resolved
	}
	if resolved, err := filepath.EvalSymlinks(expectedMedia); err == nil {
		expectedMedia = resolved
	}
	roots := payload.Data.Roots
	if len(roots) != 2 {
		t.Fatalf("expected 2 deduped roots, got=%d roots=%v body=%s", len(roots), roots, rootsRes.Body.String())
	}
	if !contains(roots, expectedDownloads) || !contains(roots, expectedMedia) {
		t.Fatalf("unexpected normalized roots=%v expected downloads=%q media=%q", roots, expectedDownloads, expectedMedia)
	}
}

func TestFilesEndpoint(t *testing.T) {
	mediaDir := t.TempDir()
	filePath := filepath.Join(mediaDir, "image.png")
	if err := os.WriteFile(filePath, []byte("png"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	router := newRouter(t, mediaDir)
	_ = postJSON(router, "/api/media/scan", map[string]string{"path": mediaDir})

	filesReq := httptest.NewRequest(http.MethodGet, "/files/", nil)
	filesRes := httptest.NewRecorder()
	router.ServeHTTP(filesRes, filesReq)
	if filesRes.Code != http.StatusOK {
		t.Fatalf("files status=%d body=%s", filesRes.Code, filesRes.Body.String())
	}
}

func TestFilesEndpointAuth(t *testing.T) {
	router := newRouterWithFileAuth(t, t.TempDir(), "vlc", "secret")
	req := httptest.NewRequest(http.MethodGet, "/files/", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d", res.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/files/", nil)
	req.SetBasicAuth("vlc", "secret")
	res = httptest.NewRecorder()
	router.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", res.Code)
	}
}

func postJSON(router http.Handler, path string, payload interface{}) *httptest.ResponseRecorder {
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

var _ = context.Background
