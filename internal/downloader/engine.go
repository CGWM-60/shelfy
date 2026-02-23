package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/events"
	"cgwm/shelfy/internal/infra/httpclient"
	"cgwm/shelfy/internal/infra/storage"
	"cgwm/shelfy/internal/repo"
	"github.com/rs/zerolog"
)

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type Engine struct {
	repo       repo.Repository
	httpClient httpclient.Client
	fs         storage.FileSystem
	bus        *events.Bus
	log        zerolog.Logger
	basePath   string
	clock      Clock

	maxConcurrent int
	autoResume    bool
	slots         chan struct{}

	mu      sync.Mutex
	cancels map[string]context.CancelFunc

	onCompleted func(context.Context, domain.DownloadJob) error
}

func NewEngine(r repo.Repository, client httpclient.Client, fs storage.FileSystem, bus *events.Bus, log zerolog.Logger, basePath string) *Engine {
	maxConcurrent := 3
	return &Engine{
		repo:          r,
		httpClient:    client,
		fs:            fs,
		bus:           bus,
		log:           log,
		basePath:      basePath,
		clock:         realClock{},
		maxConcurrent: maxConcurrent,
		autoResume:    true,
		slots:         make(chan struct{}, maxConcurrent),
		cancels:       map[string]context.CancelFunc{},
	}
}

func (e *Engine) Configure(maxConcurrent int, autoResume bool) {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.maxConcurrent = maxConcurrent
	e.autoResume = autoResume
	e.slots = make(chan struct{}, maxConcurrent)
}

func (e *Engine) SetOnCompletedHook(hook func(context.Context, domain.DownloadJob) error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onCompleted = hook
}

func (e *Engine) Recover(ctx context.Context) error {
	jobs, err := e.repo.ListDownloads(ctx)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.Status == domain.DownloadRunning {
			j.Status = domain.DownloadQueued
			j.UpdatedAt = e.clock.Now()
			if err := e.repo.UpdateDownload(ctx, j); err != nil {
				return err
			}
			if e.autoResume {
				if err := e.Start(ctx, j.ID); err != nil {
					e.log.Warn().Err(err).Str("id", j.ID).Msg("auto-resume failed")
				}
			}
		}
	}
	return nil
}

func (e *Engine) Start(ctx context.Context, id string) error {
	job, err := e.repo.GetDownload(ctx, id)
	if err != nil {
		return err
	}
	// Idempotent start: if already running, report success.
	if job.Status == domain.DownloadRunning {
		return nil
	}
	if job.Status != domain.DownloadQueued && job.Status != domain.DownloadPaused && job.Status != domain.DownloadFailed {
		return fmt.Errorf("cannot start status %s", job.Status)
	}
	job.Status = domain.DownloadQueued
	job.ErrorMessage = ""
	job.UpdatedAt = e.clock.Now()
	if err := e.repo.UpdateDownload(ctx, job); err != nil {
		return err
	}
	e.bus.Publish(events.Event{Type: "download_queued", At: e.clock.Now(), Payload: map[string]string{"id": id}})

	e.mu.Lock()
	if cancel, ok := e.cancels[id]; ok {
		cancel()
	}
	runCtx, cancel := context.WithCancel(context.Background())
	e.cancels[id] = cancel
	e.mu.Unlock()

	go e.runWithSlot(runCtx, id)
	return nil
}

func (e *Engine) runWithSlot(ctx context.Context, id string) {
	select {
	case <-ctx.Done():
		return
	case e.slots <- struct{}{}:
		defer func() { <-e.slots }()
	}
	job, err := e.repo.GetDownload(context.Background(), id)
	if err != nil {
		e.log.Error().Err(err).Str("id", id).Msg("get job before run failed")
		return
	}
	job.Status = domain.DownloadRunning
	job.UpdatedAt = e.clock.Now()
	if err := e.repo.UpdateDownload(context.Background(), job); err != nil {
		e.log.Error().Err(err).Str("id", id).Msg("set running failed")
		return
	}
	e.bus.Publish(events.Event{Type: "download_started", At: e.clock.Now(), Payload: map[string]string{"id": id}})
	e.run(ctx, id)
}

func (e *Engine) Pause(ctx context.Context, id string) error {
	job, err := e.repo.GetDownload(ctx, id)
	if err != nil {
		return err
	}
	if job.Status != domain.DownloadRunning && job.Status != domain.DownloadQueued {
		return nil
	}
	job.Status = domain.DownloadPaused
	job.UpdatedAt = e.clock.Now()
	if err := e.repo.UpdateDownload(ctx, job); err != nil {
		return err
	}
	e.stop(id)
	e.bus.Publish(events.Event{Type: "download_paused", At: e.clock.Now(), Payload: map[string]string{"id": id}})
	return nil
}

func (e *Engine) Resume(ctx context.Context, id string) error {
	return e.Start(ctx, id)
}

func (e *Engine) Cancel(ctx context.Context, id string) error {
	job, err := e.repo.GetDownload(ctx, id)
	if err != nil {
		return err
	}
	if job.Status == domain.DownloadCompleted || job.Status == domain.DownloadCanceled {
		return nil
	}
	job.Status = domain.DownloadCanceled
	job.UpdatedAt = e.clock.Now()
	if err := e.repo.UpdateDownload(ctx, job); err != nil {
		return err
	}
	e.stop(id)
	e.bus.Publish(events.Event{Type: "download_canceled", At: e.clock.Now(), Payload: map[string]string{"id": id}})
	return nil
}

func (e *Engine) stop(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cancel, ok := e.cancels[id]; ok {
		cancel()
		delete(e.cancels, id)
	}
}

func (e *Engine) run(ctx context.Context, id string) {
	defer e.stop(id)
	job, err := e.repo.GetDownload(context.Background(), id)
	if err != nil {
		e.log.Error().Err(err).Str("id", id).Msg("get job failed")
		return
	}

	for attempt := job.Retries; attempt <= job.MaxRetries; attempt++ {
		if err := e.downloadOnce(ctx, &job); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			job.Retries = attempt + 1
			job.ErrorMessage = err.Error()
			job.UpdatedAt = e.clock.Now()
			_ = e.repo.UpdateDownload(context.Background(), job)
			e.bus.Publish(events.Event{Type: "download_retry", At: e.clock.Now(), Payload: map[string]interface{}{"id": id, "retry": job.Retries, "error": err.Error()}})
			if job.Retries > job.MaxRetries {
				job.Status = domain.DownloadFailed
				_ = e.repo.UpdateDownload(context.Background(), job)
				e.bus.Publish(events.Event{Type: "download_failed", At: e.clock.Now(), Payload: map[string]interface{}{"id": id, "error": err.Error(), "errorMessage": err.Error()}})
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(Backoff(job.Retries)):
			}
			continue
		}
		job.Status = domain.DownloadCompleted
		job.UpdatedAt = e.clock.Now()
		_ = e.repo.UpdateDownload(context.Background(), job)
		e.bus.Publish(events.Event{Type: "download_completed", At: e.clock.Now(), Payload: map[string]string{"id": id}})
		e.mu.Lock()
		hook := e.onCompleted
		e.mu.Unlock()
		if hook != nil {
			if err := hook(context.Background(), job); err != nil {
				e.log.Warn().Err(err).Str("id", id).Msg("download completed hook failed")
			}
		}
		return
	}
}

func (e *Engine) downloadOnce(ctx context.Context, job *domain.DownloadJob) error {
	if err := e.fs.MkdirAll(e.basePath); err != nil {
		return err
	}
	urlToDownload := job.DirectLink
	if urlToDownload == "" {
		urlToDownload = job.SourceLink
	}
	name := job.FileName
	if name == "" {
		name = deriveFileName(urlToDownload)
		job.FileName = name
	}
	if job.DestinationPath == "" {
		job.DestinationPath = path.Join(e.basePath, name)
	}

	currentSize := int64(0)
	if info, err := e.fs.Stat(job.DestinationPath); err == nil {
		currentSize = info.Size()
	}
	requestedResume := currentSize > 0

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlToDownload, nil)
	if err != nil {
		return err
	}
	if requestedResume {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", currentSize))
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("http status %d", resp.StatusCode)
	}

	resumeAccepted := requestedResume && resp.StatusCode == http.StatusPartialContent
	if requestedResume && !resumeAccepted {
		// Server ignored Range; restart cleanly to avoid appending duplicate data.
		currentSize = 0
	}

	openFlags := os.O_CREATE | os.O_WRONLY
	if !resumeAccepted {
		openFlags |= os.O_TRUNC
	}
	file, err := e.fs.OpenFile(job.DestinationPath, openFlags, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	if resumeAccepted {
		if _, err := file.Seek(currentSize, io.SeekStart); err != nil {
			return err
		}
	}

	total := int64(0)
	if resp.ContentLength >= 0 {
		if resumeAccepted {
			total = currentSize + resp.ContentLength
		} else {
			total = resp.ContentLength
		}
	}
	if headerTotal, ok := parseContentRangeTotal(resp.Header.Get("Content-Range")); ok {
		total = headerTotal
	}

	buf := make([]byte, 256*1024)
	start := e.clock.Now()
	lastTick := start
	lastBytes := currentSize
	written := currentSize

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
			job.DownloadedBytes = written
			job.SizeBytes = total

			now := e.clock.Now()
			if now.Sub(lastTick) >= 200*time.Millisecond {
				delta := written - lastBytes
				secs := now.Sub(lastTick).Seconds()
				if secs > 0 {
					job.SpeedBytes = int64(float64(delta) / secs)
				}
				if job.SpeedBytes > 0 && total > 0 {
					remaining := total - written
					if remaining < 0 {
						remaining = 0
					}
					job.ETASeconds = remaining / job.SpeedBytes
				}
				job.UpdatedAt = now
				_ = e.repo.UpdateDownload(context.Background(), *job)
				e.bus.Publish(events.Event{Type: "download_progress", At: now, Payload: map[string]interface{}{"id": job.ID, "downloadedBytes": written, "sizeBytes": total, "speedBytes": job.SpeedBytes, "etaSeconds": job.ETASeconds}})
				lastTick = now
				lastBytes = written
			}
		}
		if errors.Is(readErr, io.EOF) {
			job.DownloadedBytes = written
			job.SizeBytes = total
			job.SpeedBytes = int64(float64(written-currentSize) / max(1, e.clock.Now().Sub(start).Seconds()))
			job.ETASeconds = 0
			job.UpdatedAt = e.clock.Now()
			_ = e.repo.UpdateDownload(context.Background(), *job)
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func deriveFileName(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "download.bin"
	}
	name := path.Base(u.Path)
	if name == "." || name == "/" || name == "" {
		return "download.bin"
	}
	if strings.Contains(name, "?") {
		name = strings.Split(name, "?")[0]
	}
	return name
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func parseContentRangeTotal(raw string) (int64, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, false
	}
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return 0, false
	}
	totalPart := strings.TrimSpace(parts[1])
	if totalPart == "" || totalPart == "*" {
		return 0, false
	}
	total, err := strconv.ParseInt(totalPart, 10, 64)
	if err != nil || total < 0 {
		return 0, false
	}
	return total, true
}
