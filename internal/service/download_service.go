package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"cgwm/shelfy/internal/debrid"
	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/downloader"
	"cgwm/shelfy/internal/events"
	"cgwm/shelfy/internal/repo"
	"github.com/google/uuid"
)

type DownloadService struct {
	repo             repo.Repository
	engine           *downloader.Engine
	debrid           *debrid.Service
	bus              *events.Bus
	defaultAutoGroup bool
}

func NewDownloadService(r repo.Repository, engine *downloader.Engine, debridSvc *debrid.Service, bus *events.Bus, defaultAutoGroup ...bool) *DownloadService {
	autoGroup := true
	if len(defaultAutoGroup) > 0 {
		autoGroup = defaultAutoGroup[0]
	}
	return &DownloadService{
		repo:             r,
		engine:           engine,
		debrid:           debridSvc,
		bus:              bus,
		defaultAutoGroup: autoGroup,
	}
}

func (s *DownloadService) Add(ctx context.Context, req domain.DownloadAddRequest) ([]domain.DownloadJob, error) {
	links := flattenLinks(req.Links)
	if len(links) == 0 {
		return nil, errors.New("at least one link is required")
	}
	created := make([]domain.DownloadJob, 0)
	seenKeys, err := s.loadDownloadDuplicateKeys(ctx)
	if err != nil {
		return nil, err
	}
	autoGroupEnabled := s.resolveAutoGroupEnabled(ctx)
	accountID := req.AccountID
	if req.DebridAccountID > 0 {
		accountID = req.DebridAccountID
	}
	for _, link := range links {
		resolved, err := s.debrid.ResolveLink(ctx, link, debrid.ResolveOptions{
			UseDebrid: req.UseDebrid,
			Provider:  req.Provider,
			AccountID: accountID,
			Password:  req.DebridPassword,
		})
		if err != nil {
			s.bus.Publish(events.Event{Type: "debrid_failed", At: time.Now(), Payload: map[string]interface{}{"source": link, "error": err.Error()}})
			return nil, err
		}
		if req.UseDebrid {
			s.bus.Publish(events.Event{Type: "debrid_started", At: time.Now(), Payload: map[string]interface{}{"source": link}})
		}
		groupFolder := ""
		if autoGroupEnabled {
			groupFolder = inferDownloadGroupFolder(link, resolved)
		}
		for _, item := range resolved {
			job := domain.DownloadJob{
				ID:         uuid.NewString(),
				SourceLink: item.SourceLink,
				DirectLink: item.DirectLink,
				FileName:   item.FileName,
				Status:     domain.DownloadQueued,
				Priority:   req.Priority,
				MaxRetries: 3,
				UseDebrid:  req.UseDebrid,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			if job.FileName == "" {
				job.FileName = deriveName(item.DirectLink)
			}
			if req.DestinationDir != "" {
				targetDir := req.DestinationDir
				if groupFolder != "" {
					targetDir = path.Join(targetDir, groupFolder)
				}
				job.DestinationPath = path.Join(targetDir, job.FileName)
			}
			jobKeys := duplicateKeysForJob(job)
			if hasDuplicateKey(seenKeys, jobKeys) {
				s.bus.Publish(events.Event{
					Type: "download_duplicate_skipped",
					At:   time.Now(),
					Payload: map[string]interface{}{
						"source":      job.SourceLink,
						"direct":      job.DirectLink,
						"destination": job.DestinationPath,
					},
				})
				continue
			}
			if err := s.repo.CreateDownload(ctx, job); err != nil {
				return nil, err
			}
			addDuplicateKeys(seenKeys, jobKeys)
			if req.UseDebrid {
				_ = s.repo.CreateDebridMapping(ctx, job.ID, item.SourceLink, item.DirectLink, item.Provider)
				s.bus.Publish(events.Event{Type: "debrid_done", At: time.Now(), Payload: map[string]interface{}{"jobId": job.ID, "source": item.SourceLink, "direct": item.DirectLink}})
			}
			created = append(created, job)
		}
	}
	return created, nil
}

func (s *DownloadService) resolveAutoGroupEnabled(ctx context.Context) bool {
	settings, err := s.repo.GetAppSettings(ctx)
	if err != nil {
		return s.defaultAutoGroup
	}
	return settings.DownloadAutoGroup
}

func (s *DownloadService) List(ctx context.Context) ([]domain.DownloadJob, error) {
	return s.repo.ListDownloads(ctx)
}

func (s *DownloadService) Start(ctx context.Context, id string) error {
	return s.engine.Start(ctx, id)
}

func (s *DownloadService) Pause(ctx context.Context, id string) error {
	return s.engine.Pause(ctx, id)
}

func (s *DownloadService) Resume(ctx context.Context, id string) error {
	return s.engine.Resume(ctx, id)
}

func (s *DownloadService) Delete(ctx context.Context, id string) error {
	if err := s.engine.Cancel(ctx, id); err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			return fmt.Errorf("cancel job: %w", err)
		}
	}
	return s.repo.DeleteDownload(ctx, id)
}

func flattenLinks(raw []string) []string {
	out := make([]string, 0)
	for _, candidate := range raw {
		for _, link := range downloader.ParseLinks(candidate) {
			out = append(out, link)
		}
	}
	return out
}

func deriveName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "download.bin"
	}
	name := path.Base(u.Path)
	if name == "" || name == "." || strings.TrimSpace(name) == "" {
		return "download.bin"
	}
	return name
}

func inferDownloadGroupFolder(sourceLink string, resolved []debrid.ResolvedItem) string {
	if len(resolved) == 0 {
		return ""
	}
	for _, item := range resolved {
		if sanitized := sanitizeFolderName(item.GroupName); sanitized != "" {
			return sanitized
		}
	}
	for _, item := range resolved {
		name := item.FileName
		if strings.TrimSpace(name) == "" {
			name = deriveName(item.DirectLink)
		}
		if inferred := inferCollectionFromFileName(name); inferred != "" {
			return inferred
		}
	}
	if len(resolved) > 1 {
		if inferred := inferCollectionFromLink(sourceLink); inferred != "" {
			return inferred
		}
	}
	return ""
}

var episodeSuffixPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)[\s._-]+s\d{1,2}e\d{1,3}.*$`),
	regexp.MustCompile(`(?i)[\s._-]+\d{1,2}x\d{1,3}.*$`),
	regexp.MustCompile(`(?i)[\s._-]+(ep|episode|chap|chapter|ch|tome|vol)\s*\d+.*$`),
	regexp.MustCompile(`(?i)[\s._-]+\d{2,4}$`),
}

func inferCollectionFromFileName(fileName string) string {
	base := strings.TrimSpace(fileName)
	if base == "" {
		return ""
	}
	ext := path.Ext(base)
	name := strings.TrimSpace(strings.TrimSuffix(base, ext))
	if name == "" {
		return ""
	}
	candidate := name
	for _, re := range episodeSuffixPatterns {
		candidate = strings.TrimSpace(re.ReplaceAllString(candidate, ""))
	}
	if candidate == "" || strings.EqualFold(candidate, name) {
		return ""
	}
	return sanitizeFolderName(candidate)
}

func inferCollectionFromLink(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) == 0 {
		return ""
	}
	last := strings.TrimSpace(segments[len(segments)-1])
	if last == "" {
		return ""
	}
	name := strings.TrimSuffix(last, path.Ext(last))
	return sanitizeFolderName(name)
}

func sanitizeFolderName(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "", "?", "", "\"", "'", "<", "", ">", "", "|", "-")
	value = replacer.Replace(value)
	value = strings.TrimSpace(value)
	if value == "" || value == "." || value == ".." {
		return ""
	}
	return value
}

func (s *DownloadService) loadDownloadDuplicateKeys(ctx context.Context) (map[string]struct{}, error) {
	jobs, err := s.repo.ListDownloads(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(jobs)*3)
	for _, job := range jobs {
		if job.Status == domain.DownloadCanceled {
			continue
		}
		addDuplicateKeys(seen, duplicateKeysForJob(job))
	}
	return seen, nil
}

func duplicateKeysForJob(job domain.DownloadJob) []string {
	keys := make([]string, 0, 3)
	if dst := normalizePathForDuplicate(job.DestinationPath); dst != "" {
		keys = append(keys, "dst:"+dst)
	}
	direct := normalizeLinkForDuplicate(job.DirectLink)
	source := normalizeLinkForDuplicate(job.SourceLink)
	if direct != "" {
		keys = append(keys, "direct:"+direct)
	}
	if len(keys) == 0 {
		if source != "" {
			keys = append(keys, "source:"+source)
		}
	}
	if source != "" && source == direct {
		keys = append(keys, "source:"+source)
	}
	return keys
}

func normalizePathForDuplicate(raw string) string {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return ""
	}
	abs, err := filepath.Abs(filepath.Clean(cleaned))
	if err != nil {
		return strings.ToLower(filepath.Clean(cleaned))
	}
	return strings.ToLower(abs)
}

func normalizeLinkForDuplicate(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return strings.ToLower(trimmed)
	}
	parsed.Fragment = ""
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path != "" {
		parsed.Path = path.Clean(parsed.Path)
	}
	return strings.ToLower(parsed.String())
}

func hasDuplicateKey(seen map[string]struct{}, keys []string) bool {
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			return true
		}
	}
	return false
}

func addDuplicateKeys(seen map[string]struct{}, keys []string) {
	for _, key := range keys {
		seen[key] = struct{}{}
	}
}
