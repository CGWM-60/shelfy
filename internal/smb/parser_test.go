package smb

import "testing"

func TestParseSambaStatusJSON(t *testing.T) {
	raw := []byte(`{"sessions":[{"username":"guest","machine":"VLC","ip":"192.168.1.8","start":"2026-02-23T10:00:00Z"}]}`)
	clients, err := parseSambaStatusJSON(raw)
	if err != nil {
		t.Fatalf("parse status json: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("expected one client")
	}
	if clients[0].Address != "192.168.1.8" || clients[0].Username != "guest" {
		t.Fatalf("unexpected client: %+v", clients[0])
	}
}
