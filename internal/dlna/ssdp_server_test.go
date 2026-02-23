package dlna

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

type responderPacketConn struct {
	reads     [][]byte
	readErr   error
	writes    [][]byte
	writeAddr []string
	closed    bool
}

func (c *responderPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	if len(c.reads) == 0 {
		if c.readErr != nil {
			return 0, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 5), Port: 1900}, c.readErr
		}
		return 0, &net.UDPAddr{IP: net.IPv4(192, 168, 1, 5), Port: 1900}, timeoutErr{}
	}
	next := c.reads[0]
	c.reads = c.reads[1:]
	copy(p, next)
	return len(next), &net.UDPAddr{IP: net.IPv4(192, 168, 1, 22), Port: 43210}, nil
}

func (c *responderPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	cp := make([]byte, len(p))
	copy(cp, p)
	c.writes = append(c.writes, cp)
	c.writeAddr = append(c.writeAddr, addr.String())
	return len(p), nil
}

func (c *responderPacketConn) SetDeadline(time.Time) error { return nil }
func (c *responderPacketConn) Close() error {
	c.closed = true
	return nil
}

type responderFactory struct {
	conn PacketConn
	err  error
}

func (f responderFactory) ListenPacket(string, string) (PacketConn, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.conn, nil
}

func TestSSDPResponderRespondsToMSearchWhenEnabled(t *testing.T) {
	service := NewService(nil, true)
	conn := &responderPacketConn{reads: [][]byte{[]byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: ssdp:all\r\nUSER-AGENT: VLC\r\n\r\n")}}
	responder := NewSSDPResponderWithFactory(responderFactory{conn: conn}, service, "http://127.0.0.1:8080/dlna/device.xml")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()
	if err := responder.Run(ctx); err != nil {
		t.Fatalf("run responder: %v", err)
	}
	if len(conn.writes) == 0 {
		t.Fatalf("expected SSDP responses")
	}
	foundResponse := false
	for _, raw := range conn.writes {
		if strings.Contains(string(raw), "HTTP/1.1 200 OK") {
			foundResponse = true
			break
		}
	}
	if !foundResponse {
		t.Fatalf("expected M-SEARCH response, got: %s", string(conn.writes[0]))
	}
	snapshot := service.Snapshot()
	if len(snapshot.Devices) == 0 {
		t.Fatalf("expected requester observed in snapshot")
	}
}

func TestSSDPResponderNoResponseWhenDisabled(t *testing.T) {
	service := NewService(nil, false)
	conn := &responderPacketConn{reads: [][]byte{[]byte("M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: ssdp:all\r\n\r\n")}}
	responder := NewSSDPResponderWithFactory(responderFactory{conn: conn}, service, "http://127.0.0.1:8080/dlna/device.xml")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(40 * time.Millisecond)
		cancel()
	}()
	if err := responder.Run(ctx); err != nil {
		t.Fatalf("run responder: %v", err)
	}
	if len(conn.writes) != 0 {
		t.Fatalf("expected no responses while disabled")
	}
}

func TestSSDPResponderListenError(t *testing.T) {
	responder := NewSSDPResponderWithFactory(responderFactory{err: errors.New("listen boom")}, NewService(nil, true), "http://127.0.0.1:8080/dlna/device.xml")
	if err := responder.Run(context.Background()); err == nil {
		t.Fatalf("expected listen error")
	}
}

func TestResolveTargetsCanonical(t *testing.T) {
	targets := resolveTargets("urn:schemas-upnp-org:device:MediaServer:1")
	if len(targets) != 1 || targets[0] != "urn:schemas-upnp-org:device:MediaServer:1" {
		t.Fatalf("unexpected canonical target: %v", targets)
	}
	all := resolveTargets("ssdp:all")
	foundUUID := false
	for _, target := range all {
		if target == "uuid:shelfy" {
			foundUUID = true
			break
		}
	}
	if !foundUUID {
		t.Fatalf("expected uuid target in ssdp:all: %v", all)
	}
}
