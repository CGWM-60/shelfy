package dlna

import (
	"context"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrDisabled = errors.New("dlna disabled")

type Device struct {
	USN        string    `json:"usn"`
	ST         string    `json:"st"`
	Server     string    `json:"server"`
	Location   string    `json:"location"`
	Address    string    `json:"address"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

type Snapshot struct {
	Enabled   bool      `json:"enabled"`
	Devices   []Device  `json:"devices"`
	LastScan  time.Time `json:"lastScan"`
	LastError string    `json:"lastError,omitempty"`
}

type DebugEvent struct {
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	RemoteAddr string    `json:"remoteAddr,omitempty"`
	ST         string    `json:"st,omitempty"`
	UserAgent  string    `json:"userAgent,omitempty"`
	Note       string    `json:"note,omitempty"`
}

type DebugSnapshot struct {
	Enabled       bool         `json:"enabled"`
	BindAddress   string       `json:"bindAddress"`
	Location      string       `json:"location"`
	USN           string       `json:"usn"`
	ServerHeader  string       `json:"serverHeader"`
	LastScan      time.Time    `json:"lastScan"`
	LastError     string       `json:"lastError,omitempty"`
	MSearchCount  int64        `json:"msearchCount"`
	ResponseCount int64        `json:"responseCount"`
	NotifyAlive   int64        `json:"notifyAliveCount"`
	NotifyByebye  int64        `json:"notifyByebyeCount"`
	RecentEvents  []DebugEvent `json:"recentEvents"`
}

type Discoverer interface {
	Discover(context.Context) ([]Device, error)
}

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type Service struct {
	discoverer Discoverer
	clock      Clock

	mu        sync.RWMutex
	enabled   bool
	devices   map[string]Device
	lastScan  time.Time
	lastError string
	debug     debugState
}

type debugState struct {
	bindAddress       string
	location          string
	usn               string
	serverHeader      string
	msearchCount      int64
	responseCount     int64
	notifyAliveCount  int64
	notifyByebyeCount int64
	recentEvents      []DebugEvent
}

func NewService(discoverer Discoverer, enabled bool) *Service {
	return &Service{
		discoverer: discoverer,
		clock:      systemClock{},
		enabled:    enabled,
		devices:    map[string]Device{},
		debug:      debugState{recentEvents: []DebugEvent{}},
	}
}

func NewServiceWithClock(discoverer Discoverer, enabled bool, clock Clock) *Service {
	if clock == nil {
		clock = systemClock{}
	}
	return &Service{
		discoverer: discoverer,
		clock:      clock,
		enabled:    enabled,
		devices:    map[string]Device{},
		debug:      debugState{recentEvents: []DebugEvent{}},
	}
}

func (s *Service) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

func (s *Service) SetEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	if !enabled {
		s.devices = map[string]Device{}
		s.lastError = ""
	}
}

func (s *Service) Scan(ctx context.Context) ([]Device, error) {
	s.mu.RLock()
	enabled := s.enabled
	discoverer := s.discoverer
	s.mu.RUnlock()
	if !enabled {
		return nil, ErrDisabled
	}
	now := s.clock.Now()
	if discoverer == nil {
		s.mu.Lock()
		s.lastScan = now
		s.lastError = ""
		defer s.mu.Unlock()
		return s.sortedLocked(), nil
	}
	found, err := discoverer.Discover(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastScan = now
	if err != nil {
		s.lastError = err.Error()
		return s.sortedLocked(), err
	}
	s.lastError = ""
	for _, device := range found {
		key := deviceKey(device)
		if key == "" {
			continue
		}
		if device.LastSeenAt.IsZero() {
			device.LastSeenAt = now
		}
		s.devices[key] = device
	}
	return s.sortedLocked(), nil
}

func (s *Service) ObserveClient(remoteAddr, userAgent, source string) {
	s.mu.RLock()
	enabled := s.enabled
	s.mu.RUnlock()
	if !enabled {
		return
	}
	host := strings.TrimSpace(remoteAddr)
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return
	}
	now := s.clock.Now()
	device := Device{
		USN:        "client:" + host,
		ST:         strings.TrimSpace(source),
		Server:     strings.TrimSpace(userAgent),
		Address:    host,
		LastSeenAt: now,
	}
	if device.ST == "" {
		device.ST = "client"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices[deviceKey(device)] = device
}

func (s *Service) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		Enabled:   s.enabled,
		Devices:   s.sortedLocked(),
		LastScan:  s.lastScan,
		LastError: s.lastError,
	}
}

func (s *Service) ConfigureSSDP(bindAddress, location, usn, serverHeader string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.debug.bindAddress = strings.TrimSpace(bindAddress)
	s.debug.location = strings.TrimSpace(location)
	s.debug.usn = strings.TrimSpace(usn)
	s.debug.serverHeader = strings.TrimSpace(serverHeader)
}

func (s *Service) RecordMSearch(remoteAddr, st, userAgent string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.debug.msearchCount++
	s.appendDebugEventLocked(DebugEvent{
		At:         s.clock.Now(),
		Kind:       "msearch_received",
		RemoteAddr: strings.TrimSpace(remoteAddr),
		ST:         strings.TrimSpace(st),
		UserAgent:  strings.TrimSpace(userAgent),
	})
}

func (s *Service) RecordSSDPResponse(remoteAddr, st string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.debug.responseCount++
	s.appendDebugEventLocked(DebugEvent{
		At:         s.clock.Now(),
		Kind:       "response_sent",
		RemoteAddr: strings.TrimSpace(remoteAddr),
		ST:         strings.TrimSpace(st),
	})
}

func (s *Service) RecordSSDPNotify(nts, st string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch strings.ToLower(strings.TrimSpace(nts)) {
	case "ssdp:alive":
		s.debug.notifyAliveCount++
	case "ssdp:byebye":
		s.debug.notifyByebyeCount++
	}
	s.appendDebugEventLocked(DebugEvent{
		At:   s.clock.Now(),
		Kind: "notify_sent",
		ST:   strings.TrimSpace(st),
		Note: strings.TrimSpace(nts),
	})
}

func (s *Service) DebugSnapshot() DebugSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := DebugSnapshot{
		Enabled:       s.enabled,
		BindAddress:   s.debug.bindAddress,
		Location:      s.debug.location,
		USN:           s.debug.usn,
		ServerHeader:  s.debug.serverHeader,
		LastScan:      s.lastScan,
		LastError:     s.lastError,
		MSearchCount:  s.debug.msearchCount,
		ResponseCount: s.debug.responseCount,
		NotifyAlive:   s.debug.notifyAliveCount,
		NotifyByebye:  s.debug.notifyByebyeCount,
		RecentEvents:  append([]DebugEvent{}, s.debug.recentEvents...),
	}
	return out
}

func (s *Service) appendDebugEventLocked(event DebugEvent) {
	const maxEvents = 50
	s.debug.recentEvents = append(s.debug.recentEvents, event)
	if len(s.debug.recentEvents) > maxEvents {
		s.debug.recentEvents = append([]DebugEvent{}, s.debug.recentEvents[len(s.debug.recentEvents)-maxEvents:]...)
	}
}

func (s *Service) sortedLocked() []Device {
	out := make([]Device, 0, len(s.devices))
	for _, device := range s.devices {
		out = append(out, device)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastSeenAt.Equal(out[j].LastSeenAt) {
			return strings.ToLower(out[i].USN) < strings.ToLower(out[j].USN)
		}
		return out[i].LastSeenAt.After(out[j].LastSeenAt)
	})
	return out
}

func deviceKey(device Device) string {
	if strings.TrimSpace(device.USN) != "" {
		return strings.ToLower(strings.TrimSpace(device.USN))
	}
	if strings.TrimSpace(device.Location) != "" {
		return strings.ToLower(strings.TrimSpace(device.Location))
	}
	if strings.TrimSpace(device.Address) != "" {
		return strings.ToLower(strings.TrimSpace(device.Address))
	}
	return ""
}
