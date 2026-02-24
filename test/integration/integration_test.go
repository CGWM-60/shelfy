package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"cgwm/shelfy/internal/ai"
	"cgwm/shelfy/internal/api"
	"cgwm/shelfy/internal/api/handlers"
	"cgwm/shelfy/internal/debrid"
	fakeprovider "cgwm/shelfy/internal/debrid/providers/fake"
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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type env struct {
	repo      *repo.GormRepository
	bus       *events.Bus
	engine    *downloader.Engine
	downloads *service.DownloadService
	debridSvc *debrid.Service
	mediaSvc  *media.Service
	router    http.Handler
	storage   string
}

func setupEnv(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	dbPath := filepath.Join(root, "integration.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := repo.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := repo.NewGormRepository(db)
	bus := events.NewBus()
	engine := downloader.NewEngine(r, httpclient.NewStreaming(10*time.Second), storage.LocalFS{}, bus, logger.NewJSONLogger(nil), filepath.Join(root, "downloads"))
	registry := debrid.NewRegistry()
	registry.Register(fakeprovider.New())
	debridSvc := debrid.NewService(r, registry)
	scanner := media.NewScanner(r, media.OSWalker{}, bus)
	mediaSvc := media.NewService(r, scanner)
	aiSvc := ai.NewService(r, ai.FakeEmbeddingProvider{}, ai.FakeLLMProvider{})
	fileSvc := fileserver.NewService(r, storage.LocalFS{})
	settingsSvc := service.NewSettingsService(r, domain.AppSettings{
		DownloadMaxConcurrent: 3,
		DownloadAutoResume:    true,
		DownloadsPath:         filepath.Join(root, "downloads"),
		LibraryPaths:          []string{filepath.Join(root, "media")},
		AIEnabled:             true,
		AITopK:                5,
		Theme:                 "clair",
		VisibleColumns:        []string{"nom"},
	})
	app := &service.App{
		Downloads: service.NewDownloadService(r, engine, debridSvc, bus),
		Debrid:    debridSvc,
		Media:     mediaSvc,
		AI:        aiSvc,
		Files:     fileSvc,
		Settings:  settingsSvc,
	}
	router := api.NewRouter(handlers.New(app, bus, filepath.Join(root, "media"), storage.LocalFS{}, "", "", nil), logger.NewJSONLogger(nil))
	return &env{repo: r, bus: bus, engine: engine, downloads: app.Downloads, debridSvc: debridSvc, mediaSvc: mediaSvc, router: router, storage: filepath.Join(root, "downloads")}
}

func TestDownloadPauseResumeRestart(t *testing.T) {
	env := setupEnv(t)
	data := bytes.Repeat([]byte("a"), 2*1024*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := 0
		if rng := r.Header.Get("Range"); strings.HasPrefix(rng, "bytes=") {
			fmt.Sscanf(strings.TrimPrefix(strings.TrimSuffix(rng, "-"), "bytes="), "%d", &start)
			w.WriteHeader(http.StatusPartialContent)
		}
		chunk := 64 * 1024
		for i := start; i < len(data); i += chunk {
			end := i + chunk
			if end > len(data) {
				end = len(data)
			}
			_, _ = w.Write(data[i:end])
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(8 * time.Millisecond)
		}
	}))
	defer server.Close()

	jobs, err := env.downloads.Add(context.Background(), domain.DownloadAddRequest{Links: []string{server.URL + "/big.bin"}})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	jobID := jobs[0].ID
	if err := env.downloads.Start(context.Background(), jobID); err != nil {
		t.Fatalf("start: %v", err)
	}

	waitForDownloaded(t, env.repo, jobID, 128*1024)
	if err := env.downloads.Pause(context.Background(), jobID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	paused := waitForStatus(t, env.repo, jobID, domain.DownloadPaused)
	if paused.DownloadedBytes == 0 {
		t.Fatalf("expected partial bytes")
	}

	newEngine := downloader.NewEngine(env.repo, httpclient.NewStreaming(10*time.Second), storage.LocalFS{}, env.bus, logger.NewJSONLogger(nil), env.storage)
	if err := newEngine.Recover(context.Background()); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if err := newEngine.Resume(context.Background(), jobID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	completed := waitForStatus(t, env.repo, jobID, domain.DownloadCompleted)
	if completed.DownloadedBytes < int64(len(data)) {
		t.Fatalf("expected full download got=%d", completed.DownloadedBytes)
	}
}

func TestDebridFakeFolderRetryAndSSE(t *testing.T) {
	env := setupEnv(t)
	if _, err := env.debridSvc.CreateAccount(context.Background(), "fake", "Fake", "", true, true); err != nil {
		t.Fatalf("create account: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		env.router.ServeHTTP(rec, req)
		close(done)
	}()
	time.Sleep(40 * time.Millisecond)

	_, err := env.downloads.Add(context.Background(), domain.DownloadAddRequest{UseDebrid: true, Links: []string{
		"fake:direct:https://example.test/a.bin",
		"fake:folder:2:https://example.test",
		"fake:retry:https://example.test/retry.bin",
	}})
	if err != nil {
		t.Fatalf("add with debrid: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done

	jobs, err := env.repo.ListDownloads(context.Background())
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs) != 4 {
		t.Fatalf("expected 4 jobs got %d", len(jobs))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: debrid_done") {
		t.Fatalf("expected debrid_done SSE, body=%s", body)
	}
}

func TestMediaScanFixture(t *testing.T) {
	env := setupEnv(t)
	fixture := filepath.Join("..", "fixtures", "media")
	count, err := env.mediaSvc.Scan(context.Background(), fixture)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if count < 5 {
		t.Fatalf("expected at least 5 media items, got %d", count)
	}
	items, _ := env.mediaSvc.List(context.Background(), "", "")
	if len(items) < 5 {
		t.Fatalf("expected items")
	}
	types := map[domain.MediaKind]bool{}
	for _, item := range items {
		types[item.Kind] = true
	}
	if !types[domain.MediaVideo] || !types[domain.MediaAudio] || !types[domain.MediaImage] || !types[domain.MediaPDF] {
		t.Fatalf("expected video/audio/image/pdf kinds, got=%v", types)
	}
}

func TestMediaScanFixtureIsIdempotent(t *testing.T) {
	env := setupEnv(t)
	fixture := filepath.Join("..", "fixtures", "media")
	firstCount, err := env.mediaSvc.Scan(context.Background(), fixture)
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if firstCount < 5 {
		t.Fatalf("expected at least 5 scanned files, got %d", firstCount)
	}
	itemsAfterFirst, err := env.mediaSvc.List(context.Background(), "", "")
	if err != nil {
		t.Fatalf("list after first scan: %v", err)
	}
	secondCount, err := env.mediaSvc.Scan(context.Background(), fixture)
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}
	if secondCount != firstCount {
		t.Fatalf("expected same scanned files count, first=%d second=%d", firstCount, secondCount)
	}
	itemsAfterSecond, err := env.mediaSvc.List(context.Background(), "", "")
	if err != nil {
		t.Fatalf("list after second scan: %v", err)
	}
	if len(itemsAfterSecond) != len(itemsAfterFirst) {
		t.Fatalf("scan should be idempotent, first list=%d second list=%d", len(itemsAfterFirst), len(itemsAfterSecond))
	}
}

func TestMediaScanAutoIndexesAI(t *testing.T) {
	env := setupEnv(t)
	mediaRoot := t.TempDir()
	mediaFile := filepath.Join(mediaRoot, "my-movie.mp4")
	if err := os.WriteFile(mediaFile, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write media file: %v", err)
	}

	scanRes := postJSON(t, env.router, "/api/media/scan", map[string]string{"path": mediaRoot})
	if scanRes.Code != http.StatusOK {
		t.Fatalf("scan status=%d body=%s", scanRes.Code, scanRes.Body.String())
	}

	searchRes := postJSON(t, env.router, "/api/ai/search", map[string]interface{}{"query": "my movie", "limit": 5})
	if searchRes.Code != http.StatusOK {
		t.Fatalf("ai search status=%d body=%s", searchRes.Code, searchRes.Body.String())
	}
	var payload struct {
		Data []domain.AISearchResult `json:"data"`
	}
	if err := json.Unmarshal(searchRes.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode ai search: %v body=%s", err, searchRes.Body.String())
	}
	if len(payload.Data) == 0 {
		t.Fatalf("expected automatic ai indexing to return search results")
	}
}

func TestStreamRangeHeadersAndBody(t *testing.T) {
	env := setupEnv(t)
	mediaFile := filepath.Join(t.TempDir(), "sample.mp4")
	content := []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if err := os.WriteFile(mediaFile, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	item := domain.MediaItem{ID: "m-stream", Title: "Sample", Kind: domain.MediaVideo, Path: mediaFile, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := env.repo.UpsertMediaItem(context.Background(), item); err != nil {
		t.Fatalf("upsert media: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/stream/m-stream", nil)
	req.Header.Set("Range", "bytes=0-3")
	res := httptest.NewRecorder()
	env.router.ServeHTTP(res, req)
	if res.Code != http.StatusPartialContent {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if got := res.Header().Get("Content-Range"); got != "bytes 0-3/36" {
		t.Fatalf("content-range=%s", got)
	}
	if res.Body.String() != "0123" {
		t.Fatalf("body=%q", res.Body.String())
	}
}

func TestFileServerListAndStream(t *testing.T) {
	env := setupEnv(t)
	mediaFile := filepath.Join(t.TempDir(), "sample.mp3")
	content := []byte("abcdefghijklmnopqrstuvwxyz")
	if err := os.WriteFile(mediaFile, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	item := domain.MediaItem{ID: "m-file", Title: "Sample Audio", Kind: domain.MediaAudio, Path: mediaFile, MimeType: "audio/mpeg", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := env.repo.UpsertMediaItem(context.Background(), item); err != nil {
		t.Fatalf("upsert media: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/files/", nil)
	listRes := httptest.NewRecorder()
	env.router.ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listRes.Code, listRes.Body.String())
	}
	if !strings.Contains(listRes.Body.String(), "m-file") {
		t.Fatalf("expected file in listing body=%s", listRes.Body.String())
	}

	streamReq := httptest.NewRequest(http.MethodGet, "/files/m-file/stream", nil)
	streamReq.Header.Set("Range", "bytes=0-2")
	streamRes := httptest.NewRecorder()
	env.router.ServeHTTP(streamRes, streamReq)
	if streamRes.Code != http.StatusPartialContent {
		t.Fatalf("stream status=%d body=%s", streamRes.Code, streamRes.Body.String())
	}
	if streamRes.Body.String() != "abc" {
		t.Fatalf("stream body=%q", streamRes.Body.String())
	}
}

func waitForDownloaded(t *testing.T, r *repo.GormRepository, id string, min int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := r.GetDownload(context.Background(), id)
		if err == nil && job.DownloadedBytes >= min {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	job, _ := r.GetDownload(context.Background(), id)
	t.Fatalf("downloaded bytes timeout got=%d", job.DownloadedBytes)
}

func waitForStatus(t *testing.T, r *repo.GormRepository, id string, status domain.DownloadStatus) domain.DownloadJob {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		job, err := r.GetDownload(context.Background(), id)
		if err == nil && job.Status == status {
			return job
		}
		time.Sleep(40 * time.Millisecond)
	}
	job, _ := r.GetDownload(context.Background(), id)
	t.Fatalf("timeout status=%s got=%s", status, job.Status)
	return job
}

func postJSON(t *testing.T, router http.Handler, path string, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	return res
}

func readBody(res *httptest.ResponseRecorder) string {
	b, _ := io.ReadAll(res.Body)
	return string(b)
}

var _ = sync.Mutex{}
