package service

import (
	"context"
	"strings"
	"testing"

	"cgwm/shelfy/internal/debrid"
	fakeprovider "cgwm/shelfy/internal/debrid/providers/fake"
	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/events"
	"cgwm/shelfy/internal/testutil"
)

func TestDownloadServiceAdd_AutoGroupsSeriesInDestinationDir(t *testing.T) {
	r := testutil.NewSQLiteRepo(t)
	debridSvc := debrid.NewService(r, debrid.NewRegistry())
	svc := NewDownloadService(r, nil, debridSvc, events.NewBus())

	created, err := svc.Add(context.Background(), domain.DownloadAddRequest{
		Links: []string{
			"https://cdn.test/Naruto.S01E01.mkv",
			"https://cdn.test/Naruto.S01E02.mkv",
		},
		DestinationDir: "/downloads",
		UseDebrid:      false,
	})
	if err != nil {
		t.Fatalf("add downloads: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 created jobs, got=%d", len(created))
	}
	for _, job := range created {
		if !strings.HasPrefix(job.DestinationPath, "/downloads/Naruto/") {
			t.Fatalf("expected grouped destination path, got=%q", job.DestinationPath)
		}
	}
}

func TestDownloadServiceAdd_UsesDebridFolderNameForGrouping(t *testing.T) {
	r := testutil.NewSQLiteRepo(t)
	registry := debrid.NewRegistry()
	registry.Register(fakeprovider.New())
	debridSvc := debrid.NewService(r, registry)
	if _, err := debridSvc.CreateAccount(context.Background(), "fake", "Fake account", "", true, true); err != nil {
		t.Fatalf("create account: %v", err)
	}

	svc := NewDownloadService(r, nil, debridSvc, events.NewBus())
	created, err := svc.Add(context.Background(), domain.DownloadAddRequest{
		Links:          []string{"fake:folder:2:https://files.test"},
		UseDebrid:      true,
		DestinationDir: "/downloads",
	})
	if err != nil {
		t.Fatalf("add debrid folder: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 jobs from folder, got=%d", len(created))
	}
	for _, job := range created {
		if !strings.HasPrefix(job.DestinationPath, "/downloads/fake-folder/") {
			t.Fatalf("expected debrid folder grouping, got=%q", job.DestinationPath)
		}
	}
}

func TestDownloadServiceAdd_DedupesWithinSameRequest(t *testing.T) {
	r := testutil.NewSQLiteRepo(t)
	debridSvc := debrid.NewService(r, debrid.NewRegistry())
	svc := NewDownloadService(r, nil, debridSvc, events.NewBus())

	created, err := svc.Add(context.Background(), domain.DownloadAddRequest{
		Links: []string{
			"https://cdn.test/One.bin",
			"https://cdn.test/One.bin",
		},
		DestinationDir: "/downloads",
		UseDebrid:      false,
	})
	if err != nil {
		t.Fatalf("add downloads: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 created job after dedupe, got=%d", len(created))
	}
}

func TestDownloadServiceAdd_DedupesAgainstExistingJobs(t *testing.T) {
	r := testutil.NewSQLiteRepo(t)
	debridSvc := debrid.NewService(r, debrid.NewRegistry())
	svc := NewDownloadService(r, nil, debridSvc, events.NewBus())

	first, err := svc.Add(context.Background(), domain.DownloadAddRequest{
		Links:          []string{"https://cdn.test/existing.bin"},
		DestinationDir: "/downloads",
		UseDebrid:      false,
	})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("expected first add to create 1 job, got=%d", len(first))
	}

	second, err := svc.Add(context.Background(), domain.DownloadAddRequest{
		Links:          []string{"https://cdn.test/existing.bin"},
		DestinationDir: "/downloads",
		UseDebrid:      false,
	})
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("expected duplicate add to create 0 job, got=%d", len(second))
	}

	all, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 persisted job, got=%d", len(all))
	}
}
