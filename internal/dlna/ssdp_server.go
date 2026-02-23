package dlna

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

type SSDPResponder struct {
	factory      PacketConnFactory
	service      *Service
	location     string
	serverHeader string
	usn          string
	now          func() time.Time
}

func NewSSDPResponder(service *Service, location string) *SSDPResponder {
	return NewSSDPResponderWithFactory(netPacketConnFactory{}, service, location)
}

func NewSSDPResponderWithFactory(factory PacketConnFactory, service *Service, location string) *SSDPResponder {
	if factory == nil {
		factory = netPacketConnFactory{}
	}
	return &SSDPResponder{
		factory:      factory,
		service:      service,
		location:     strings.TrimSpace(location),
		serverHeader: "Shelfy/1.0 UPnP/1.0 DLNA/1.5",
		usn:          "uuid:shelfy-dlna",
		now:          time.Now,
	}
}

func (s *SSDPResponder) SetIdentity(usn, serverHeader string) {
	if strings.TrimSpace(usn) != "" {
		s.usn = strings.TrimSpace(usn)
	}
	if strings.TrimSpace(serverHeader) != "" {
		s.serverHeader = strings.TrimSpace(serverHeader)
	}
}

func (s *SSDPResponder) Run(ctx context.Context) error {
	if s.service == nil || strings.TrimSpace(s.location) == "" {
		return nil
	}
	s.service.ConfigureSSDP(":1900", s.location, s.usn, s.serverHeader)
	conn, err := s.factory.ListenPacket("udp4", ":1900")
	if err != nil {
		return err
	}
	defer conn.Close()
	multicastAddr, err := net.ResolveUDPAddr("udp4", defaultSSDPAddress)
	if err != nil {
		return err
	}
	s.sendNotify(conn, multicastAddr, "ssdp:alive")
	announceTicker := time.NewTicker(10 * time.Second)
	defer announceTicker.Stop()
	defer s.sendNotify(conn, multicastAddr, "ssdp:byebye")

	buf := make([]byte, 8192)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-announceTicker.C:
			if s.service.Enabled() {
				s.sendNotify(conn, multicastAddr, "ssdp:alive")
			}
		default:
		}

		if err := conn.SetDeadline(time.Now().Add(1 * time.Second)); err != nil {
			return err
		}
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			if isTimeout(err) {
				continue
			}
			return err
		}

		if !s.service.Enabled() {
			continue
		}
		raw := string(buf[:n])
		headers := parseHeaders(raw)
		if !isMSearch(raw, headers) {
			continue
		}
		s.service.ObserveClient(addr.String(), headers["user-agent"], "ssdp-msearch")
		s.service.RecordMSearch(addr.String(), headers["st"], headers["user-agent"])
		for _, st := range resolveTargets(headers["st"]) {
			response := buildSSDPResponse(st, s.location, s.serverHeader, s.usn, s.now())
			if _, err := conn.WriteTo([]byte(response), addr); err == nil {
				s.service.RecordSSDPResponse(addr.String(), st)
			}
		}
	}
}

func (s *SSDPResponder) sendNotify(conn PacketConn, multicastAddr net.Addr, nts string) {
	if !s.service.Enabled() {
		return
	}
	for _, nt := range resolveTargets("ssdp:all") {
		msg := buildSSDPNotify(nt, nts, s.location, s.serverHeader, s.usn, s.now())
		if _, err := conn.WriteTo([]byte(msg), multicastAddr); err == nil {
			s.service.RecordSSDPNotify(nts, nt)
		}
	}
}

func isMSearch(raw string, headers map[string]string) bool {
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(raw)), "M-SEARCH") {
		return false
	}
	man := strings.ToLower(headers["man"])
	return strings.Contains(man, "ssdp:discover")
}

func resolveTargets(st string) []string {
	normalized := strings.TrimSpace(strings.ToLower(st))
	if normalized == "" || normalized == "ssdp:all" {
		return []string{
			"upnp:rootdevice",
			"urn:schemas-upnp-org:device:MediaServer:1",
			"urn:schemas-upnp-org:service:ContentDirectory:1",
			"uuid:shelfy",
		}
	}
	switch normalized {
	case "upnp:rootdevice":
		return []string{"upnp:rootdevice"}
	case "urn:schemas-upnp-org:device:mediaserver:1":
		return []string{"urn:schemas-upnp-org:device:MediaServer:1"}
	case "urn:schemas-upnp-org:service:contentdirectory:1":
		return []string{"urn:schemas-upnp-org:service:ContentDirectory:1"}
	default:
		return []string{strings.TrimSpace(st)}
	}
}

func buildSSDPResponse(st, location, serverHeader, usn string, now time.Time) string {
	if strings.TrimSpace(usn) == "" {
		usn = "uuid:shelfy-dlna"
	}
	if !strings.HasPrefix(strings.ToLower(usn), "uuid:") {
		usn = "uuid:" + usn
	}
	if strings.EqualFold(strings.TrimSpace(st), "uuid:shelfy") {
		st = "uuid:shelfy"
	}
	fullUSN := usn
	switch strings.ToLower(strings.TrimSpace(st)) {
	case "upnp:rootdevice", "urn:schemas-upnp-org:device:mediaserver:1", "urn:schemas-upnp-org:service:contentdirectory:1":
		fullUSN = fmt.Sprintf("%s::%s", usn, st)
	}
	return fmt.Sprintf("HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=1800\r\nDATE: %s\r\nEXT:\r\nLOCATION: %s\r\nSERVER: %s\r\nST: %s\r\nUSN: %s\r\nCONTENT-LENGTH: 0\r\n\r\n",
		now.UTC().Format(time.RFC1123),
		strings.TrimSpace(location),
		strings.TrimSpace(serverHeader),
		st,
		fullUSN,
	)
}

func buildSSDPNotify(nt, nts, location, serverHeader, usn string, now time.Time) string {
	if strings.TrimSpace(usn) == "" {
		usn = "uuid:shelfy"
	}
	if !strings.HasPrefix(strings.ToLower(usn), "uuid:") {
		usn = "uuid:" + usn
	}
	if strings.EqualFold(strings.TrimSpace(nt), "uuid:shelfy") {
		nt = "uuid:shelfy"
	}
	fullUSN := usn
	switch strings.ToLower(strings.TrimSpace(nt)) {
	case "upnp:rootdevice", "urn:schemas-upnp-org:device:mediaserver:1", "urn:schemas-upnp-org:service:contentdirectory:1":
		fullUSN = fmt.Sprintf("%s::%s", usn, nt)
	}
	return fmt.Sprintf("NOTIFY * HTTP/1.1\r\nHOST: %s\r\nCACHE-CONTROL: max-age=1800\r\nLOCATION: %s\r\nNT: %s\r\nNTS: %s\r\nSERVER: %s\r\nUSN: %s\r\n\r\n",
		defaultSSDPAddress,
		strings.TrimSpace(location),
		nt,
		nts,
		strings.TrimSpace(serverHeader),
		fullUSN,
	)
}
