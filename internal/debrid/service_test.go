package debrid_test

import (
	"context"
	"testing"

	"cgwm/shelfy/internal/debrid"
	fakeprovider "cgwm/shelfy/internal/debrid/providers/fake"
	"cgwm/shelfy/internal/testutil"
)

func setupService(t *testing.T) *debrid.Service {
	t.Helper()
	r := testutil.NewSQLiteRepo(t)
	registry := debrid.NewRegistry()
	registry.Register(fakeprovider.New())
	svc := debrid.NewService(r, registry)
	_, err := svc.CreateAccount(context.Background(), "fake", "Fake account", "", true, true)
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return svc
}

func TestAuthDeviceFlowFake(t *testing.T) {
	svc := setupService(t)
	session, err := svc.StartAuth(context.Background(), 1)
	if err != nil {
		t.Fatalf("start auth: %v", err)
	}
	if session.UserCode == "" || session.VerificationURI == "" {
		t.Fatalf("unexpected session: %+v", session)
	}
	done, err := svc.PollAuth(context.Background(), 1, session)
	if err != nil {
		t.Fatalf("poll 1: %v", err)
	}
	if done {
		t.Fatalf("expected pending on first poll")
	}
	done, err = svc.PollAuth(context.Background(), 1, session)
	if err != nil {
		t.Fatalf("poll 2: %v", err)
	}
	if !done {
		t.Fatalf("expected done on second poll")
	}
	status, err := svc.Status(context.Background(), 1)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status != "ok" {
		t.Fatalf("status=%s", status)
	}
}

func TestAuthPasswordFlowFake(t *testing.T) {
	svc := setupService(t)
	providers := svc.ListProviders()
	if len(providers) == 0 || !providers[0].SupportsPassword {
		t.Fatalf("expected fake provider to support password flow")
	}
	if err := svc.AuthWithPassword(context.Background(), 1, "demo@example.com", "secret"); err != nil {
		t.Fatalf("password auth: %v", err)
	}
	status, err := svc.Status(context.Background(), 1)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status != "ok" {
		t.Fatalf("status=%s", status)
	}
}

func TestResolveLink_Direct(t *testing.T) {
	svc := setupService(t)
	items, err := svc.ResolveLink(context.Background(), "fake:direct:https://files.test/a.bin", debrid.ResolveOptions{UseDebrid: true})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(items) != 1 || items[0].DirectLink != "https://files.test/a.bin" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestResolveLink_Folder(t *testing.T) {
	svc := setupService(t)
	items, err := svc.ResolveLink(context.Background(), "fake:folder:3:https://files.test", debrid.ResolveOptions{UseDebrid: true})
	if err != nil {
		t.Fatalf("resolve folder: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 files got %d", len(items))
	}
}

func TestResolveLink_TransientRetry(t *testing.T) {
	svc := setupService(t)
	items, err := svc.ResolveLink(context.Background(), "fake:retry:https://files.test/retry.bin", debrid.ResolveOptions{UseDebrid: true})
	if err != nil {
		t.Fatalf("resolve retry: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected single item")
	}
}

func TestResolveLink_NoActiveAccount(t *testing.T) {
	r := testutil.NewSQLiteRepo(t)
	registry := debrid.NewRegistry()
	registry.Register(fakeprovider.New())
	svc := debrid.NewService(r, registry)

	if _, err := svc.ResolveLink(context.Background(), "fake:direct:https://files.test/a.bin", debrid.ResolveOptions{UseDebrid: true}); err == nil {
		t.Fatalf("expected error")
	}
}
