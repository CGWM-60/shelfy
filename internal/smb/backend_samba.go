package smb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Process interface {
	Wait() error
	Kill() error
}

type CommandRunner interface {
	Start(context.Context, string, []string) (Process, error)
	Output(context.Context, string, []string) ([]byte, error)
}

type osExecRunner struct{}

type osProcess struct{ cmd *exec.Cmd }

func (osExecRunner) Start(ctx context.Context, name string, args []string) (Process, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &osProcess{cmd: cmd}, nil
}

func (osExecRunner) Output(ctx context.Context, name string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

func (p *osProcess) Wait() error { return p.cmd.Wait() }
func (p *osProcess) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

type FileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(name string, data []byte, perm os.FileMode) error
	Remove(name string) error
}

type osFS struct{}

func (osFS) MkdirAll(path string, perm os.FileMode) error  { return os.MkdirAll(path, perm) }
func (osFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}
func (osFS) Remove(name string) error { return os.Remove(name) }

type SambaBackend struct {
	runner       CommandRunner
	fs           FileSystem
	smbBinary    string
	statusBinary string
	runtimeDir   string

	mu       sync.Mutex
	proc     Process
	confPath string
}

func NewSambaBackend(runner CommandRunner, fs FileSystem, smbBinary, statusBinary, runtimeDir string) *SambaBackend {
	if runner == nil {
		runner = osExecRunner{}
	}
	if fs == nil {
		fs = osFS{}
	}
	if strings.TrimSpace(smbBinary) == "" {
		smbBinary = "smbd"
	}
	if strings.TrimSpace(statusBinary) == "" {
		statusBinary = "smbstatus"
	}
	if strings.TrimSpace(runtimeDir) == "" {
		runtimeDir = os.TempDir()
	}
	return &SambaBackend{runner: runner, fs: fs, smbBinary: smbBinary, statusBinary: statusBinary, runtimeDir: runtimeDir}
}

func (b *SambaBackend) Name() string { return "samba" }

func (b *SambaBackend) Start(ctx context.Context, cfg Config) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.proc != nil {
		return nil
	}
	if strings.TrimSpace(cfg.SharePath) == "" {
		return errors.New("smb share path is required")
	}
	if err := b.fs.MkdirAll(cfg.SharePath, 0o755); err != nil {
		return err
	}
	if err := b.fs.MkdirAll(b.runtimeDir, 0o755); err != nil {
		return err
	}
	confPath := filepath.Join(b.runtimeDir, "shelfy-smb.conf")
	payload := renderSambaConfig(cfg)
	if err := b.fs.WriteFile(confPath, []byte(payload), 0o644); err != nil {
		return err
	}
	args := []string{"--foreground", "--no-process-group", "--configfile=" + confPath}
	proc, err := b.runner.Start(ctx, b.smbBinary, args)
	if err != nil {
		_ = b.fs.Remove(confPath)
		return err
	}
	b.proc = proc
	b.confPath = confPath
	go b.waitProcess(proc)
	return nil
}

func (b *SambaBackend) waitProcess(proc Process) {
	_ = proc.Wait()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.proc == proc {
		b.proc = nil
		if b.confPath != "" {
			_ = b.fs.Remove(b.confPath)
			b.confPath = ""
		}
	}
}

func (b *SambaBackend) Stop(ctx context.Context) error {
	_ = ctx
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.proc == nil {
		return nil
	}
	err := b.proc.Kill()
	b.proc = nil
	if b.confPath != "" {
		_ = b.fs.Remove(b.confPath)
		b.confPath = ""
	}
	return err
}

func (b *SambaBackend) Running(ctx context.Context) (bool, error) {
	_ = ctx
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.proc != nil, nil
}

func (b *SambaBackend) Clients(ctx context.Context) ([]Client, error) {
	out, err := b.runner.Output(ctx, b.statusBinary, []string{"--json"})
	if err != nil {
		return nil, err
	}
	clients, err := parseSambaStatusJSON(out)
	if err != nil {
		return nil, err
	}
	return clients, nil
}

func renderSambaConfig(cfg Config) string {
	shareName := cfg.ShareName
	if strings.TrimSpace(shareName) == "" {
		shareName = "shelfy"
	}
	return fmt.Sprintf(`[global]
  server string = shelfy
  workgroup = WORKGROUP
  map to guest = Bad User
  guest account = nobody
  disable netbios = yes
  smb ports = 445
  log level = 1

[%s]
  path = %s
  browseable = yes
  read only = yes
  guest ok = yes
`, shareName, cfg.SharePath)
}

var _ = time.Second
