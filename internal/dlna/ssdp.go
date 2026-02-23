package dlna

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	defaultSSDPAddress = "239.255.255.250:1900"
)

type PacketConn interface {
	ReadFrom(p []byte) (n int, addr net.Addr, err error)
	WriteTo(p []byte, addr net.Addr) (n int, err error)
	SetDeadline(t time.Time) error
	Close() error
}

type PacketConnFactory interface {
	ListenPacket(network, address string) (PacketConn, error)
}

type netPacketConnFactory struct{}

func (netPacketConnFactory) ListenPacket(network, address string) (PacketConn, error) {
	if shouldUseSSDPMulticastListen(network, address) {
		maddr, err := net.ResolveUDPAddr("udp4", defaultSSDPAddress)
		if err != nil {
			return nil, err
		}
		conn, err := net.ListenMulticastUDP("udp4", nil, maddr)
		if err != nil {
			return nil, err
		}
		_ = conn.SetReadBuffer(1 << 20)
		return conn, nil
	}
	return net.ListenPacket(network, address)
}

func shouldUseSSDPMulticastListen(network, address string) bool {
	if strings.ToLower(strings.TrimSpace(network)) != "udp4" {
		return false
	}
	raw := strings.TrimSpace(address)
	if raw == ":1900" || raw == "0.0.0.0:1900" {
		return true
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return false
	}
	host = strings.TrimSpace(host)
	return port == "1900" && (host == "" || host == "0.0.0.0")
}

type SSDPDiscovery struct {
	factory PacketConnFactory
	target  string
	timeout time.Duration
	mxSec   int
}

func NewSSDPDiscovery(timeout time.Duration) *SSDPDiscovery {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &SSDPDiscovery{
		factory: netPacketConnFactory{},
		target:  defaultSSDPAddress,
		timeout: timeout,
		mxSec:   1,
	}
}

func NewSSDPDiscoveryWithFactory(factory PacketConnFactory, timeout time.Duration) *SSDPDiscovery {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if factory == nil {
		factory = netPacketConnFactory{}
	}
	return &SSDPDiscovery{
		factory: factory,
		target:  defaultSSDPAddress,
		timeout: timeout,
		mxSec:   1,
	}
}

func (d *SSDPDiscovery) Discover(ctx context.Context) ([]Device, error) {
	conn, err := d.factory.ListenPacket("udp4", ":0")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	addr, err := net.ResolveUDPAddr("udp4", d.target)
	if err != nil {
		return nil, err
	}

	request := fmt.Sprintf("M-SEARCH * HTTP/1.1\r\nHOST: %s\r\nMAN: \"ssdp:discover\"\r\nMX: %d\r\nST: ssdp:all\r\n\r\n", d.target, d.mxSec)
	if _, err := conn.WriteTo([]byte(request), addr); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(d.timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}

	devices := make([]Device, 0)
	seen := map[string]struct{}{}
	buf := make([]byte, 8192)
	for {
		if err := ctx.Err(); err != nil {
			break
		}
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			if isTimeout(err) {
				break
			}
			return devices, err
		}
		device, ok := parseSSDPResponse(buf[:n], from.String())
		if !ok {
			continue
		}
		key := deviceKey(device)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		device.LastSeenAt = time.Now()
		devices = append(devices, device)
	}
	return devices, nil
}

func parseSSDPResponse(payload []byte, sourceAddr string) (Device, bool) {
	text := string(payload)
	if !strings.HasPrefix(strings.ToUpper(text), "HTTP/1.1 200") {
		return Device{}, false
	}
	headers := parseHeaders(text)
	location := headers["location"]
	st := headers["st"]
	server := headers["server"]
	usn := headers["usn"]
	if location == "" && usn == "" {
		return Device{}, false
	}
	if !looksLikeDLNA(st, server, usn) {
		return Device{}, false
	}
	return Device{
		USN:      usn,
		ST:       st,
		Server:   server,
		Location: location,
		Address:  sourceAddr,
	}, true
}

func parseHeaders(raw string) map[string]string {
	headers := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		headers[key] = val
	}
	return headers
}

func looksLikeDLNA(st, server, usn string) bool {
	combined := strings.ToLower(st + " " + server + " " + usn)
	return strings.Contains(combined, "upnp") || strings.Contains(combined, "dlna") || strings.Contains(combined, "schemas-upnp-org")
}

func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}
