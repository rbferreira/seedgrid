package main

import (
	"context"
	"strings"
	"testing"
)

// TestRestartByCommand (ADR-045 §9): the Portal's restart runs the operator's command, and its
// failure, with what it printed, is the command's failure.
func TestRestartByCommand(t *testing.T) {
	if err := (restartByCommand{cmd: "true"}).Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown with a command that succeeds = %v", err)
	}
	err := (restartByCommand{cmd: "echo no such service >&2; exit 3"}).Shutdown(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no such service") {
		t.Errorf("Shutdown with a failing command = %v, want its output", err)
	}
}
