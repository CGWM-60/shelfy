package smb

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeBackend struct {
	running     bool
	clients     []Client
	nilClients  bool
	startErr    error
	clientsErr  error
	startCalls  int
	stopCalls   int
	statusCalls int
}

func (f *fakeBackend) Name() string { return "fake" }

func (f *fakeBackend) Start(context.Context, Config) error {
	f.startCalls++
	if f.startErr != nil {
		return f.startErr
	}
	f.running = true
	return nil
}

func (f *fakeBackend) Stop(context.Context) error {
	f.stopCalls++
	f.running = false
	return nil
}

func (f *fakeBackend) Running(context.Context) (bool, error) {
	f.statusCalls++
	return f.running, nil
}

func (f *fakeBackend) Clients(context.Context) ([]Client, error) {
	if f.clientsErr != nil {
		return nil, f.clientsErr
	}
	if f.nilClients {
		return nil, nil
	}
	out := make([]Client, len(f.clients))
	copy(out, f.clients)
	return out, nil
}

func TestServiceApplySettingsLifecycle(t *testing.T) {
	now := time.Date(2026, 2, 23, 11, 0, 0, 0, time.UTC)
	backend := &fakeBackend{clients: []Client{{Username: "guest", Machine: "VLC", Address: "192.168.1.22"}}}
	svc := NewServiceWithClock(backend, false, Config{ShareName: "shelfy", SharePath: "/tmp/media"}, fakeClock{now: now})

	if err := svc.ApplySettings(context.Background(), true, Config{ShareName: "shelfy", SharePath: "/tmp/media"}); err != nil {
		t.Fatalf("enable smb: %v", err)
	}
	status := svc.Snapshot()
	if !status.Enabled || !status.Running {
		t.Fatalf("expected enabled/running status: %+v", status)
	}
	if len(status.Clients) != 1 {
		t.Fatalf("expected one client")
	}

	if err := svc.ApplySettings(context.Background(), false, Config{ShareName: "shelfy", SharePath: "/tmp/media"}); err != nil {
		t.Fatalf("disable smb: %v", err)
	}
	status = svc.Snapshot()
	if status.Running {
		t.Fatalf("expected stopped status")
	}
}

func TestServiceApplySettingsBackendError(t *testing.T) {
	backend := &fakeBackend{startErr: errors.New("cannot start")}
	svc := NewService(backend, false, Config{ShareName: "shelfy", SharePath: "/tmp/media"})
	if err := svc.ApplySettings(context.Background(), true, Config{ShareName: "shelfy", SharePath: "/tmp/media"}); err == nil {
		t.Fatalf("expected start error")
	}
	if svc.Snapshot().LastError == "" {
		t.Fatalf("expected last error")
	}
}

func TestServiceSnapshotNormalizesNilClients(t *testing.T) {
	backend := &fakeBackend{nilClients: true}
	svc := NewService(backend, false, Config{ShareName: "shelfy", SharePath: "/tmp/media"})
	if err := svc.ApplySettings(context.Background(), true, Config{ShareName: "shelfy", SharePath: "/tmp/media"}); err != nil {
		t.Fatalf("enable smb: %v", err)
	}
	status := svc.Snapshot()
	if status.Clients == nil {
		t.Fatalf("expected non-nil clients slice")
	}
	if len(status.Clients) != 0 {
		t.Fatalf("expected no clients")
	}
}

func TestServiceApplySettingsDoesNotFailWhenClientsListingFails(t *testing.T) {
	backend := &fakeBackend{clientsErr: errors.New("exec: smbstatus not found")}
	svc := NewService(backend, false, Config{ShareName: "shelfy", SharePath: "/tmp/media"})
	if err := svc.ApplySettings(context.Background(), true, Config{ShareName: "shelfy", SharePath: "/tmp/media"}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	status := svc.Snapshot()
	if !status.Enabled || !status.Running {
		t.Fatalf("expected enabled and running")
	}
	if status.LastError == "" {
		t.Fatalf("expected last error to be set")
	}
	if status.Clients == nil || len(status.Clients) != 0 {
		t.Fatalf("expected empty clients list")
	}
}
