package ai

import (
	"context"
	"testing"
	"time"

	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/testutil"
)

func TestAIIndexAndSearchRanking(t *testing.T) {
	r := testutil.NewSQLiteRepo(t)
	now := time.Now()
	_ = r.UpsertMediaItem(context.Background(), domain.MediaItem{ID: "m1", Title: "Film science fiction", Kind: domain.MediaVideo, Path: "/tmp/a.mp4", Tags: []string{"espace"}, CreatedAt: now, UpdatedAt: now})
	_ = r.UpsertMediaItem(context.Background(), domain.MediaItem{ID: "m2", Title: "Documentaire cuisine", Kind: domain.MediaVideo, Path: "/tmp/b.mp4", Tags: []string{"cuisine"}, CreatedAt: now, UpdatedAt: now})

	svc := NewService(r, FakeEmbeddingProvider{}, FakeLLMProvider{})
	count, err := svc.Index(context.Background())
	if err != nil {
		t.Fatalf("index error: %v", err)
	}
	if count != 2 {
		t.Fatalf("indexed count=%d", count)
	}
	results, err := svc.Search(context.Background(), "science espace", 2)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("empty search result")
	}
	if results[0].MediaID != "m1" {
		t.Fatalf("unexpected best result: %+v", results[0])
	}
}

func TestAIAskModes(t *testing.T) {
	r := testutil.NewSQLiteRepo(t)
	now := time.Now()
	_ = r.UpsertMediaItem(context.Background(), domain.MediaItem{ID: "m1", Title: "Série robotique", Kind: domain.MediaVideo, Path: "/tmp/a.mp4", CreatedAt: now, UpdatedAt: now})
	svc := NewService(r, FakeEmbeddingProvider{}, FakeLLMProvider{})
	_, _ = svc.Index(context.Background())

	searchOnly, err := svc.Ask(context.Background(), "robot", true)
	if err != nil {
		t.Fatalf("ask searchOnly error: %v", err)
	}
	if searchOnly.Answer == "" {
		t.Fatalf("expected answer in searchOnly")
	}

	answer, err := svc.Ask(context.Background(), "robot", false)
	if err != nil {
		t.Fatalf("ask error: %v", err)
	}
	if answer.Answer == "" {
		t.Fatalf("expected generated answer")
	}
}

func TestAIReportTracksUsage(t *testing.T) {
	r := testutil.NewSQLiteRepo(t)
	now := time.Now()
	_ = r.UpsertMediaItem(context.Background(), domain.MediaItem{ID: "m1", Title: "Film robotique", Kind: domain.MediaVideo, Path: "/tmp/a.mp4", CreatedAt: now, UpdatedAt: now})
	svc := NewService(r, FakeEmbeddingProvider{}, FakeLLMProvider{})

	if _, err := svc.Index(context.Background()); err != nil {
		t.Fatalf("index error: %v", err)
	}
	if _, err := svc.Search(context.Background(), "robot", 3); err != nil {
		t.Fatalf("search error: %v", err)
	}
	if _, err := svc.Ask(context.Background(), "robot", true); err != nil {
		t.Fatalf("ask search only error: %v", err)
	}
	if _, err := svc.Ask(context.Background(), "robot", false); err != nil {
		t.Fatalf("ask error: %v", err)
	}

	report := svc.Report()
	if report.EmbeddingProvider != "fake" || report.LLMProvider != "fake" {
		t.Fatalf("unexpected providers: %+v", report)
	}
	if report.IndexRuns != 1 || report.LastIndexedCount != 1 {
		t.Fatalf("unexpected index metrics: %+v", report)
	}
	if report.SearchRuns != 3 {
		t.Fatalf("unexpected search runs: %d", report.SearchRuns)
	}
	if report.AskRuns != 2 || report.AskSearchOnlyRuns != 1 {
		t.Fatalf("unexpected ask metrics: %+v", report)
	}
	if report.EmbeddingCalls != 4 {
		t.Fatalf("unexpected embedding calls: %d", report.EmbeddingCalls)
	}
	if report.EmbeddingTokensEstimated <= 0 {
		t.Fatalf("expected embedding token usage > 0")
	}
	if report.LLMPromptTokensEstimated <= 0 || report.LLMCompletionTokensEstimated <= 0 {
		t.Fatalf("expected llm token usage > 0")
	}
}

func TestAIIndexDedupesSameDocumentAcrossMediaAndDownloads(t *testing.T) {
	r := testutil.NewSQLiteRepo(t)
	now := time.Now()
	const sharedPath = "/tmp/shared/movie.mkv"

	_ = r.UpsertMediaItem(context.Background(), domain.MediaItem{
		ID:        "m1",
		Title:     "Film doublon",
		Kind:      domain.MediaVideo,
		Path:      sharedPath,
		Tags:      []string{"action"},
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = r.CreateDownload(context.Background(), domain.DownloadJob{
		ID:              "d1",
		FileName:        "movie.mkv",
		SourceLink:      "https://example.test/movie.mkv",
		DestinationPath: sharedPath,
		Status:          domain.DownloadCompleted,
		CreatedAt:       now,
		UpdatedAt:       now,
	})

	svc := NewService(r, FakeEmbeddingProvider{}, FakeLLMProvider{})
	count, err := svc.Index(context.Background())
	if err != nil {
		t.Fatalf("index error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected deduped indexed count=1 got=%d", count)
	}

	results, err := svc.Search(context.Background(), "film action", 10)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected deduped result count=1 got=%d results=%+v", len(results), results)
	}
}
