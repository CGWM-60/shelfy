package terminal

import "testing"

func TestDefaultShellFallback(t *testing.T) {
	t.Setenv("SHELL", "")
	if got := DefaultShell(); got != "/bin/sh" {
		t.Fatalf("expected /bin/sh, got=%q", got)
	}
}

func TestDefaultShellEnv(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	if got := DefaultShell(); got != "/bin/bash" {
		t.Fatalf("expected /bin/bash, got=%q", got)
	}
}
