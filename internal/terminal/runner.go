package terminal

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
)

type Session interface {
	io.ReadWriteCloser
}

type Runner interface {
	Open(ctx context.Context) (Session, error)
}

type commandFactory interface {
	CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd
}

type ptyFactory interface {
	Start(cmd *exec.Cmd) (*os.File, error)
}

type defaultCommandFactory struct{}

func (defaultCommandFactory) CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, arg...)
}

type defaultPTYFactory struct{}

func (defaultPTYFactory) Start(cmd *exec.Cmd) (*os.File, error) {
	return pty.Start(cmd)
}

type LocalRunner struct {
	shell string
	args  []string

	cmd commandFactory
	pty ptyFactory
}

func NewLocalRunner(shell string, args ...string) *LocalRunner {
	if shell == "" {
		shell = DefaultShell()
	}
	if len(args) == 0 {
		args = []string{"-i"}
	}
	return &LocalRunner{
		shell: shell,
		args:  args,
		cmd:   defaultCommandFactory{},
		pty:   defaultPTYFactory{},
	}
}

func DefaultShell() string {
	value := os.Getenv("SHELL")
	if value == "" {
		return "/bin/sh"
	}
	return value
}

func (r *LocalRunner) Open(ctx context.Context) (Session, error) {
	cmd := r.cmd.CommandContext(ctx, r.shell, r.args...)
	ptmx, err := r.pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	out := &localSession{
		file:    ptmx,
		process: cmd.Process,
	}
	return out, nil
}

type localSession struct {
	file    *os.File
	process *os.Process
	once    sync.Once
}

func (s *localSession) Read(p []byte) (int, error) {
	return s.file.Read(p)
}

func (s *localSession) Write(p []byte) (int, error) {
	return s.file.Write(p)
}

func (s *localSession) Close() error {
	var err error
	s.once.Do(func() {
		if s.process != nil {
			_ = s.process.Kill()
		}
		err = s.file.Close()
	})
	return err
}
