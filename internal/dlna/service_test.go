package dlna

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeDiscoverer struct {
	devices []Device
	err     error
	calls   int
}

func (f *fakeDiscoverer) Discover(context.Context) ([]Device, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := make([]Device, len(f.devices))
	copy(out, f.devices)
	return out, nil
}

func TestServiceScanLifecycle(t *testing.T) {
	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	discovery := &fakeDiscoverer{devices: []Device{{USN: "uuid:tv-1", ST: "urn:schemas-upnp-org:device:MediaRenderer:1", Server: "DLNA/1.5", Location: "http://tv.local/device.xml"}}}
	svc := NewServiceWithClock(discovery, false, fakeClock{now: now})

	if _, err := svc.Scan(context.Background()); !errors.Is(err, ErrDisabled) {
		t.Fatalf("expected ErrDisabled, got=%v", err)
	}
	if discovery.calls != 0 {
		t.Fatalf("discover should not be called when disabled")
	}

	svc.SetEnabled(true)
	devices, err := svc.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected one device, got=%d", len(devices))
	}
	if devices[0].USN != "uuid:tv-1" {
		t.Fatalf("unexpected device: %+v", devices[0])
	}

	snapshot := svc.Snapshot()
	if !snapshot.Enabled {
		t.Fatalf("expected enabled snapshot")
	}
	if snapshot.LastScan.IsZero() {
		t.Fatalf("expected last scan timestamp")
	}

	svc.SetEnabled(false)
	snapshot = svc.Snapshot()
	if len(snapshot.Devices) != 0 {
		t.Fatalf("expected devices cleared on disable")
	}
}

func TestServiceScanKeepsCachedDevicesOnTransientError(t *testing.T) {
	now := time.Date(2026, 2, 23, 9, 0, 0, 0, time.UTC)
	discovery := &fakeDiscoverer{devices: []Device{{USN: "uuid:tv-1", ST: "upnp", Server: "DLNA/1.5", Location: "http://tv.local/device.xml"}}}
	svc := NewServiceWithClock(discovery, true, fakeClock{now: now})

	if _, err := svc.Scan(context.Background()); err != nil {
		t.Fatalf("first scan: %v", err)
	}
	discovery.err = errors.New("network down")
	devices, err := svc.Scan(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(devices) != 1 {
		t.Fatalf("expected cached device still returned")
	}
	if svc.Snapshot().LastError == "" {
		t.Fatalf("expected lastError")
	}
}

func TestObserveClient(t *testing.T) {
	svc := NewService(nil, true)
	svc.ObserveClient("192.168.1.50:50123", "VLC/3.0", "dlna-media")
	snapshot := svc.Snapshot()
	if len(snapshot.Devices) != 1 {
		t.Fatalf("expected one observed client, got=%d", len(snapshot.Devices))
	}
	if snapshot.Devices[0].USN != "client:192.168.1.50" {
		t.Fatalf("unexpected usn=%q", snapshot.Devices[0].USN)
	}
	if snapshot.Devices[0].ST != "dlna-media" {
		t.Fatalf("unexpected source=%q", snapshot.Devices[0].ST)
	}
}

func TestDebugSnapshotCounters(t *testing.T) {
	now := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)
	svc := NewServiceWithClock(nil, true, fakeClock{now: now})
	svc.ConfigureSSDP(":1900", "http://192.168.1.50:8080/dlna/device.xml", "uuid:shelfy", "shelfy/1.0 UPnP/1.0 DLNA/1.5")
	svc.RecordMSearch("192.168.1.60:51515", "ssdp:all", "VLC")
	svc.RecordSSDPResponse("192.168.1.60:51515", "upnp:rootdevice")
	svc.RecordSSDPNotify("ssdp:alive", "upnp:rootdevice")
	svc.RecordSSDPNotify("ssdp:byebye", "upnp:rootdevice")

	debug := svc.DebugSnapshot()
	if debug.BindAddress != ":1900" || debug.Location == "" || debug.USN != "uuid:shelfy" {
		t.Fatalf("unexpected debug ssdp config: %+v", debug)
	}
	if debug.MSearchCount != 1 || debug.ResponseCount != 1 {
		t.Fatalf("unexpected counters: %+v", debug)
	}
	if debug.NotifyAlive != 1 || debug.NotifyByebye != 1 {
		t.Fatalf("unexpected notify counters: %+v", debug)
	}
	if len(debug.RecentEvents) < 4 {
		t.Fatalf("expected debug events, got=%d", len(debug.RecentEvents))
	}
}
