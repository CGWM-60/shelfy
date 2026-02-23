package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/events"
	"cgwm/shelfy/internal/infra/httpclient"
	"cgwm/shelfy/internal/infra/logger"
	"cgwm/shelfy/internal/infra/storage"
	"cgwm/shelfy/internal/testutil"
)

func TestEngineRetryThenSuccess(t *testing.T) {
	repo := testutil.NewSQLiteRepo(t)
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&calls, 1)
		if c == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte("hello world"))
	}))
	defer ts.Close()

	job := domain.DownloadJob{
		ID:         "job-1",
		SourceLink: ts.URL + "/file.bin",
		Status:     domain.DownloadQueued,
		MaxRetries: 2,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := repo.CreateDownload(context.Background(), job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	engine := NewEngine(repo, httpclient.New(5*time.Second), storage.LocalFS{}, events.NewBus(), logger.NewJSONLogger(nil), filepath.Join(t.TempDir(), "downloads"))
	if err := engine.Start(context.Background(), job.ID); err != nil {
		t.Fatalf("start: %v", err)
	}

	waitForStatus(t, repo, job.ID, domain.DownloadCompleted)
	updated, err := repo.GetDownload(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updated.Retries == 0 {
		t.Fatalf("expected retries > 0")
	}
}

func TestEngineConcurrencyLimit(t *testing.T) {
	repo := testutil.NewSQLiteRepo(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 10; i++ {
			_, _ = w.Write([]byte("chunk"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(25 * time.Millisecond)
		}
	}))
	defer server.Close()

	now := time.Now()
	job1 := domain.DownloadJob{ID: "j1", SourceLink: server.URL + "/a.bin", Status: domain.DownloadQueued, MaxRetries: 1, CreatedAt: now, UpdatedAt: now}
	job2 := domain.DownloadJob{ID: "j2", SourceLink: server.URL + "/b.bin", Status: domain.DownloadQueued, MaxRetries: 1, CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateDownload(context.Background(), job1); err != nil {
		t.Fatalf("create job1: %v", err)
	}
	if err := repo.CreateDownload(context.Background(), job2); err != nil {
		t.Fatalf("create job2: %v", err)
	}

	engine := NewEngine(repo, httpclient.New(5*time.Second), storage.LocalFS{}, events.NewBus(), logger.NewJSONLogger(nil), filepath.Join(t.TempDir(), "downloads"))
	engine.Configure(1, false)
	if err := engine.Start(context.Background(), "j1"); err != nil {
		t.Fatalf("start j1: %v", err)
	}
	if err := engine.Start(context.Background(), "j2"); err != nil {
		t.Fatalf("start j2: %v", err)
	}

	time.Sleep(80 * time.Millisecond)
	current1, _ := repo.GetDownload(context.Background(), "j1")
	current2, _ := repo.GetDownload(context.Background(), "j2")
	runningCount := 0
	if current1.Status == domain.DownloadRunning {
		runningCount++
	}
	if current2.Status == domain.DownloadRunning {
		runningCount++
	}
	if runningCount > 1 {
		t.Fatalf("expected at most one running, got j1=%s j2=%s", current1.Status, current2.Status)
	}

	waitForStatus(t, repo, "j1", domain.DownloadCompleted)
	waitForStatus(t, repo, "j2", domain.DownloadCompleted)
}

func TestEngineStartRunningIsNoop(t *testing.T) {
	repo := testutil.NewSQLiteRepo(t)
	now := time.Now()
	job := domain.DownloadJob{
		ID:         "running-1",
		SourceLink: "https://example.test/file.bin",
		Status:     domain.DownloadRunning,
		MaxRetries: 1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := repo.CreateDownload(context.Background(), job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	engine := NewEngine(repo, httpclient.New(5*time.Second), storage.LocalFS{}, events.NewBus(), logger.NewJSONLogger(nil), filepath.Join(t.TempDir(), "downloads"))
	if err := engine.Start(context.Background(), job.ID); err != nil {
		t.Fatalf("start running should be noop, got error: %v", err)
	}

	updated, err := repo.GetDownload(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if updated.Status != domain.DownloadRunning {
		t.Fatalf("expected running status unchanged, got %s", updated.Status)
	}
}

func TestEngineCallsCompletedHook(t *testing.T) {
	repo := testutil.NewSQLiteRepo(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer ts.Close()

	now := time.Now()
	job := domain.DownloadJob{
		ID:         "hook-1",
		SourceLink: ts.URL + "/file.bin",
		Status:     domain.DownloadQueued,
		MaxRetries: 1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := repo.CreateDownload(context.Background(), job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	engine := NewEngine(repo, httpclient.New(5*time.Second), storage.LocalFS{}, events.NewBus(), logger.NewJSONLogger(nil), filepath.Join(t.TempDir(), "downloads"))
	var (
		mu     sync.Mutex
		called int
		gotID  string
	)
	engine.SetOnCompletedHook(func(ctx context.Context, completed domain.DownloadJob) error {
		mu.Lock()
		defer mu.Unlock()
		called++
		gotID = completed.ID
		return nil
	})
	if err := engine.Start(context.Background(), job.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForStatus(t, repo, job.ID, domain.DownloadCompleted)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		ok := called > 0
		id := gotID
		mu.Unlock()
		if ok {
			if id != job.ID {
				t.Fatalf("unexpected hook id: %s", id)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("expected completed hook to be called")
}

func TestEngineRangeFallbackRestartsFileWhenRangeIgnored(t *testing.T) {
	repo := testutil.NewSQLiteRepo(t)
	const body = "NEW-DATA-FROM-SERVER"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intentionally ignore Range and always return 200.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	downloadDir := filepath.Join(t.TempDir(), "downloads")
	dest := filepath.Join(downloadDir, "file.bin")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(dest, []byte("OLD-PARTIAL"), 0o644); err != nil {
		t.Fatalf("write old partial: %v", err)
	}

	job := domain.DownloadJob{
		ID:              "range-fallback",
		SourceLink:      ts.URL + "/file.bin",
		FileName:        "file.bin",
		DestinationPath: dest,
		Status:          domain.DownloadQueued,
		MaxRetries:      1,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := repo.CreateDownload(context.Background(), job); err != nil {
		t.Fatalf("create job: %v", err)
	}

	engine := NewEngine(repo, httpclient.New(5*time.Second), storage.LocalFS{}, events.NewBus(), logger.NewJSONLogger(nil), downloadDir)
	if err := engine.Start(context.Background(), job.ID); err != nil {
		t.Fatalf("start: %v", err)
	}

	waitForStatus(t, repo, job.ID, domain.DownloadCompleted)
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read final file: %v", err)
	}
	if string(got) != body {
		t.Fatalf("unexpected final file content: %q", string(got))
	}
}

func TestParseContentRangeTotal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		raw   string
		total int64
		ok    bool
	}{
		{name: "partial format", raw: "bytes 10-19/100", total: 100, ok: true},
		{name: "star total", raw: "bytes */1000", total: 1000, ok: true},
		{name: "invalid", raw: "invalid", total: 0, ok: false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseContentRangeTotal(tc.raw)
			if ok != tc.ok || got != tc.total {
				t.Fatalf("parseContentRangeTotal(%q) = (%d, %v), want (%d, %v)", tc.raw, got, ok, tc.total, tc.ok)
			}
		})
	}
}

func waitForStatus(t *testing.T, repo interface {
	GetDownload(context.Context, string) (domain.DownloadJob, error)
}, id string, status domain.DownloadStatus) {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		job, err := repo.GetDownload(context.Background(), id)
		if err == nil && job.Status == status {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	job, _ := repo.GetDownload(context.Background(), id)
	t.Fatalf("timeout waiting status=%s got=%s", status, job.Status)
}
