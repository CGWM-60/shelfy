package dlna

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

type fakePacketConn struct {
	responses [][]byte
	writes    int
	deadline  time.Time
	closed    bool
}

func (f *fakePacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	if len(f.responses) == 0 {
		return 0, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1900}, timeoutErr{}
	}
	next := f.responses[0]
	f.responses = f.responses[1:]
	copy(p, next)
	return len(next), &net.UDPAddr{IP: net.IPv4(192, 168, 1, 10), Port: 1900}, nil
}

func (f *fakePacketConn) WriteTo([]byte, net.Addr) (int, error) {
	f.writes++
	return 1, nil
}

func (f *fakePacketConn) SetDeadline(t time.Time) error {
	f.deadline = t
	return nil
}

func (f *fakePacketConn) Close() error {
	f.closed = true
	return nil
}

type fakeFactory struct {
	conn *fakePacketConn
	err  error
}

func (f fakeFactory) ListenPacket(string, string) (PacketConn, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.conn, nil
}

func TestParseSSDPResponse(t *testing.T) {
	payload := []byte("HTTP/1.1 200 OK\r\nST: urn:schemas-upnp-org:device:MediaRenderer:1\r\nUSN: uuid:abcd\r\nSERVER: Linux/5.0 UPnP/1.1 DLNADOC/1.50\r\nLOCATION: http://192.168.1.12:8200/rootDesc.xml\r\n\r\n")
	device, ok := parseSSDPResponse(payload, "192.168.1.12:1900")
	if !ok {
		t.Fatalf("expected parsed device")
	}
	if device.USN != "uuid:abcd" || device.Location == "" {
		t.Fatalf("unexpected device %+v", device)
	}
}

func TestDiscoverReadsDevicesFromPacketConn(t *testing.T) {
	conn := &fakePacketConn{responses: [][]byte{
		[]byte("HTTP/1.1 200 OK\r\nST: upnp:rootdevice\r\nUSN: uuid:one\r\nSERVER: DLNA/1.5\r\nLOCATION: http://host1/device.xml\r\n\r\n"),
		[]byte("HTTP/1.1 200 OK\r\nST: upnp:rootdevice\r\nUSN: uuid:one\r\nSERVER: DLNA/1.5\r\nLOCATION: http://host1/device.xml\r\n\r\n"),
		[]byte("HTTP/1.1 200 OK\r\nST: urn:schemas-upnp-org:device:MediaRenderer:1\r\nUSN: uuid:two\r\nSERVER: DLNA/1.5\r\nLOCATION: http://host2/device.xml\r\n\r\n"),
	}}
	discovery := NewSSDPDiscoveryWithFactory(fakeFactory{conn: conn}, 50*time.Millisecond)
	devices, err := discovery.Discover(context.Background())
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if conn.writes != 1 {
		t.Fatalf("expected one multicast write, got=%d", conn.writes)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 deduped devices, got=%d", len(devices))
	}
	if !conn.closed {
		t.Fatalf("connection should be closed")
	}
}

func TestDiscoverFactoryError(t *testing.T) {
	discovery := NewSSDPDiscoveryWithFactory(fakeFactory{err: errors.New("boom")}, time.Second)
	if _, err := discovery.Discover(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}
