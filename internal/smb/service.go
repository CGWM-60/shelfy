package smb

import (
	"context"
	"strings"
	"sync"
	"time"
)

type Client struct {
	Username    string    `json:"username"`
	Machine     string    `json:"machine"`
	Address     string    `json:"address"`
	ConnectedAt time.Time `json:"connectedAt"`
}

type Status struct {
	Enabled   bool      `json:"enabled"`
	Running   bool      `json:"running"`
	Backend   string    `json:"backend"`
	ShareName string    `json:"shareName"`
	SharePath string    `json:"sharePath"`
	LastError string    `json:"lastError,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
	Clients   []Client  `json:"clients"`
}

type Config struct {
	ShareName string
	SharePath string
}

type Backend interface {
	Name() string
	Start(context.Context, Config) error
	Stop(context.Context) error
	Running(context.Context) (bool, error)
	Clients(context.Context) ([]Client, error)
}

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type Service struct {
	backend Backend
	clock   Clock

	mu      sync.RWMutex
	enabled bool
	cfg     Config
	status  Status
}

func NewService(backend Backend, enabled bool, cfg Config) *Service {
	return NewServiceWithClock(backend, enabled, cfg, systemClock{})
}

func NewServiceWithClock(backend Backend, enabled bool, cfg Config, clock Clock) *Service {
	if clock == nil {
		clock = systemClock{}
	}
	cfg = normalizeConfig(cfg)
	name := "none"
	if backend != nil {
		name = backend.Name()
	}
	return &Service{
		backend: backend,
		clock:   clock,
		enabled: enabled,
		cfg:     cfg,
		status: Status{Enabled: enabled, Backend: name, ShareName: cfg.ShareName, SharePath: cfg.SharePath, Clients: []Client{}},
	}
}

func (s *Service) ApplySettings(ctx context.Context, enabled bool, cfg Config) error {
	normalized := normalizeConfig(cfg)
	s.mu.Lock()
	s.enabled = enabled
	s.cfg = normalized
	s.status.Enabled = enabled
	s.status.ShareName = normalized.ShareName
	s.status.SharePath = normalized.SharePath
	s.mu.Unlock()

	if s.backend == nil {
		s.setError("smb backend unavailable")
		return nil
	}

	if !enabled {
		if err := s.backend.Stop(ctx); err != nil {
			s.setError(err.Error())
			return err
		}
		s.mu.Lock()
		s.status.Running = false
		s.status.LastError = ""
		s.status.Clients = []Client{}
		s.status.UpdatedAt = s.clock.Now()
		s.mu.Unlock()
		return nil
	}

	if err := s.backend.Start(ctx, normalized); err != nil {
		s.setError(err.Error())
		return err
	}
	// SMB server can run even if client enumeration backend is unavailable.
	// Do not fail settings save when refresh cannot collect clients.
	if err := s.Refresh(ctx); err != nil {
		return nil
	}
	return nil
}

func (s *Service) Refresh(ctx context.Context) error {
	s.mu.RLock()
	enabled := s.enabled
	s.mu.RUnlock()

	if s.backend == nil {
		s.setError("smb backend unavailable")
		return nil
	}

	running, err := s.backend.Running(ctx)
	if err != nil {
		s.setError(err.Error())
		return err
	}
	clients := make([]Client, 0)
	lastError := ""
	if enabled && running {
		clients, err = s.backend.Clients(ctx)
		if err != nil {
			lastError = strings.TrimSpace(err.Error())
			clients = []Client{}
		}
		if clients == nil {
			clients = []Client{}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.Running = running
	s.status.Clients = clients
	s.status.LastError = lastError
	s.status.UpdatedAt = s.clock.Now()
	return nil
}

func (s *Service) Snapshot() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.status
	if s.status.Clients == nil {
		out.Clients = []Client{}
	} else {
		out.Clients = append([]Client{}, s.status.Clients...)
	}
	return out
}

func (s *Service) setError(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status.LastError = strings.TrimSpace(msg)
	s.status.UpdatedAt = s.clock.Now()
}

func normalizeConfig(cfg Config) Config {
	cfg.ShareName = strings.TrimSpace(cfg.ShareName)
	if cfg.ShareName == "" {
		cfg.ShareName = "shelfy"
	}
	cfg.SharePath = strings.TrimSpace(cfg.SharePath)
	return cfg
}
