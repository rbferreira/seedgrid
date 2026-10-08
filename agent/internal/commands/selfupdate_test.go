package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/rbferreira/seedgrid/agent/internal/client"
)

// fakeBinary writes a "binary" (sh script) that exits with the given code upon receiving
// --version, and returns path + sha256. It serves as an update candidate: the ADR-027
// smoke-test only requires the executable to run and exit 0.
func fakeBinary(t *testing.T, dir, name string, exitCode int) (string, string) {
	t.Helper()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nexit " + strconv.Itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil { //nolint:gosec // G306: it is a fake executable script for the test
		t.Fatalf("write fake binary: %v", err)
	}
	sum := sha256.Sum256([]byte(script))
	return path, hex.EncodeToString(sum[:])
}

// serveFile starts an HTTP server that returns the given file.
func serveFile(t *testing.T, path string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/seedgrid-agent"
}

// execFrom runs the updateAgent pointing the target to `self` (a disposable copy), instead
// of the runner's own binary — which is what os.Executable() would return.
func execFrom(t *testing.T, self string, p payload) client.CommandResult {
	t.Helper()
	e := &Executor{selfPath: self}
	return e.updateAgent(context.Background(), p)
}

func TestUpdateAgentSwapBinary(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("depends on sh script exec")
	}
	dir := t.TempDir()
	self, _ := fakeBinary(t, dir, "seedgrid-agent", 0) // "current" binary
	candidate, sum := fakeBinary(t, dir, "new", 0)     // valid candidate

	res := execFrom(t, self, payload{URL: serveFile(t, candidate), SHA256: sum, Version: "v9.9.9"})

	if res.Status != "DONE" {
		t.Fatalf("expected DONE; got %s (%s)", res.Status, res.Error)
	}
	if _, err := os.Stat(self + ".prev"); err != nil {
		t.Error("the previous binary should have been saved in .prev (manual rollback/ExecStartPre)")
	}
	got, _ := os.ReadFile(self)       //nolint:gosec // G304: test fixture (t.TempDir())
	want, _ := os.ReadFile(candidate) //nolint:gosec // G304: test fixture (t.TempDir())
	if string(got) != string(want) {
		t.Error("the binary in use should have been replaced by the new one")
	}
	if _, err := os.Stat(filepath.Join(dir, ".seedgrid-agent.new")); !os.IsNotExist(err) {
		t.Error("the temporary download file should have been removed")
	}
}

// Wrong checksum = corrupted download or binary swapped mid-way: it must be rejected
// BEFORE any effect. It is the ADR-027 integrity guarantee.
func TestUpdateAgentWrongChecksumNoSwitch(t *testing.T) {
	dir := t.TempDir()
	self, selfSum := fakeBinary(t, dir, "seedgrid-agent", 0)
	candidate, _ := fakeBinary(t, dir, "new", 0)

	res := execFrom(t, self, payload{
		URL:    serveFile(t, candidate),
		SHA256: "deadbeef00000000000000000000000000000000000000000000000000000000",
	})

	if res.Status != "FAILED" {
		t.Fatalf("wrong checksum should FAIL; got %s", res.Status)
	}
	sumNow, _ := sha256File(self)
	if sumNow != selfSum {
		t.Error("the binary in use MUST NOT be touched when the checksum does not match")
	}
	if _, err := os.Stat(self + ".prev"); err == nil {
		t.Error("should not have even created .prev — the failure is prior to the swap")
	}
}

// The case ADR-027 exists to prevent: the file downloads and the checksum matches, but the
// binary DOES NOT RUN on that host (wrong architecture is the real case — /node/ was serving
// ARM64 to an amd64 fleet). The smoke-test must catch this before the swap.
func TestUpdateAgentFailedSmokeNoSwitch(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("depends on sh script exec")
	}
	dir := t.TempDir()
	self, selfSum := fakeBinary(t, dir, "seedgrid-agent", 0)
	bad, sum := fakeBinary(t, dir, "bad", 1) // runs, but exits != 0

	res := execFrom(t, self, payload{URL: serveFile(t, bad), SHA256: sum})

	if res.Status != "FAILED" {
		t.Fatalf("binary that fails smoke should FAIL; got %s", res.Status)
	}
	sumNow, _ := sha256File(self)
	if sumNow != selfSum {
		t.Error("the binary in use MUST NOT be replaced by one that does not execute")
	}
}

func TestUpdateAgentRequiresURLAndChecksum(t *testing.T) {
	dir := t.TempDir()
	self, _ := fakeBinary(t, dir, "seedgrid-agent", 0)

	if res := execFrom(t, self, payload{SHA256: "abc"}); res.Status != "FAILED" {
		t.Error("update_agent without url should fail")
	}
	if res := execFrom(t, self, payload{URL: "http://x/y"}); res.Status != "FAILED" {
		t.Error("update_agent without sha256 should fail")
	}
}

// The ack must go out BEFORE the process dies, otherwise the command stays DISPATCHED
// forever and the Portal shows failure on an update that succeeded. Here: the exit request
// stays pending and is only consumed by whoever has already confirmed.
func TestPendingExitOnlyAfterAck(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("depends on sh script exec")
	}
	dir := t.TempDir()
	self, _ := fakeBinary(t, dir, "seedgrid-agent", 0)
	candidate, sum := fakeBinary(t, dir, "new", 0)

	e := &Executor{selfPath: self}
	res := e.updateAgent(context.Background(), payload{URL: serveFile(t, candidate), SHA256: sum})
	if res.Status != "DONE" {
		t.Fatalf("expected DONE; got %s (%s)", res.Status, res.Error)
	}
	if !e.pendingExit {
		t.Fatal("the swap should leave the exit PENDING (the Poller exits, after the ack)")
	}
	if !e.TakePendingExit() {
		t.Error("TakePendingExit should return true on 1st call")
	}
	if e.TakePendingExit() {
		t.Error("TakePendingExit should clear the request (do not exit twice)")
	}
}
