// Agent self-update (ADR-027): replaces its own binary with the one the Control Plane
// indicated and exits — systemd (Restart=always, see deploy/agent/seedgrid-agent.service)
// restarts the process on the new binary. Doesn't use `systemctl`: exiting is enough, so the
// Agent doesn't need new permissions.
//
// The step order is ADR-027's defense against a bad binary: download → check SHA-256
// → PROVE it runs (`--version`) → backup the previous → atomic swap. It only swaps a
// binary that has already proven to run on that host; wrong architecture, corrupted download and
// incompatible libc die at the smoke-test, before any effect.
package commands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/client"
)

const (
	// Binary download window (~6 MB) — generous for a Node on a bad link.
	updateDownloadTimeout = 5 * time.Minute
	// The smoke-test just prints a string and exits; if it goes beyond that, it's stuck.
	updateSmokeTimeout = 20 * time.Second
)

// updateAgent executes the `update_agent` command (ADR-027).
//
// DOES NOT kill the process: it signals `pendingExit`, and the Poller exits AFTER confirming
// the command to the CP — dying here would leave the command eternally DISPATCHED and the Portal
// would show failure on an update that succeeded.
func (e *Executor) updateAgent(ctx context.Context, p payload) client.CommandResult {
	if p.URL == "" {
		return failed("update_agent requires url")
	}
	if p.SHA256 == "" {
		return failed("update_agent requires sha256") // without a checksum there is nothing to validate
	}

	// The target is THIS process's binary. EvalSymlinks because if /usr/local/bin/seedgrid-agent
	// is a symlink, the actual file must be replaced — otherwise rename would replace
	// the link and the original would be left behind.
	self := e.selfPath
	if self == "" {
		exe, err := os.Executable()
		if err != nil {
			return failed(fmt.Sprintf("resolve own binary: %v", err))
		}
		self = exe
	}
	if resolved, rErr := filepath.EvalSymlinks(self); rErr == nil {
		self = resolved
	}

	// Downloads to the SAME directory as the binary: os.Rename is only atomic within the same
	// filesystem, and this atomicity ensures there is never a half-written
	// binary in /usr/local/bin (systemd could restart right on it).
	tmp := filepath.Join(filepath.Dir(self), ".seedgrid-agent.new")
	_ = os.Remove(tmp) // leftover from a previous interrupted attempt
	defer func() { _ = os.Remove(tmp) }()

	if err := downloadTo(ctx, p.URL, tmp); err != nil {
		return failed(fmt.Sprintf("download binary: %v", err))
	}
	sum, err := sha256File(tmp)
	if err != nil {
		return failed(fmt.Sprintf("binary checksum: %v", err))
	}
	if !strings.EqualFold(sum, p.SHA256) {
		return failed(fmt.Sprintf("checksum mismatch: expected %s, got %s", p.SHA256, sum))
	}
	if err := os.Chmod(tmp, 0o755); err != nil { //nolint:gosec // G302: it's the Agent binary, needs to be executable
		return failed(fmt.Sprintf("chmod binary: %v", err))
	}

	// Smoke-test — the heart of ADR-027. `--version` reads neither config nor network, so exit 0
	// proves this executable runs on this host. It's what separates a "downloaded file" from a
	// "working binary", and runs BEFORE touching the binary in production.
	smokeCtx, cancel := context.WithTimeout(ctx, updateSmokeTimeout)
	defer cancel()
	if out, err := exec.CommandContext(smokeCtx, tmp, "--version").CombinedOutput(); err != nil { //nolint:gosec // G204: tmp is the internal download path, integrity verified by SHA-256 above
		return failed(fmt.Sprintf("smoke-test failed (binary does not execute on this host): %v: %s",
			err, strings.TrimSpace(string(out))))
	}

	// Backs up the previous BEFORE swapping: it's what the unit's ExecStartPre restores if the
	// new binary proves invalid later (and what the operator uses manually if needed).
	prev := self + ".prev"
	_ = os.Remove(prev)
	if err := copyFile(self, prev); err != nil {
		return failed(fmt.Sprintf("save previous binary: %v", err))
	}
	if err := os.Rename(tmp, self); err != nil {
		return failed(fmt.Sprintf("replace binary: %v", err))
	}

	e.pendingExit = true
	return client.CommandResult{Status: "DONE"}
}

// downloadTo writes the URL to the given path, with its own timeout.
func downloadTo(ctx context.Context, url, dest string) error {
	dlCtx, cancel := context.WithTimeout(ctx, updateDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755) //nolint:gosec // G302: destination is the Agent binary, needs to be executable
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// sha256File returns the file's SHA-256 in lowercase hex.
func sha256File(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // G304: path is the internally downloaded Agent binary, not external input
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFile copies src to dst preserving the execute bit. A copy (not a hard link) so
// .prev survives anything that replaces the original's inode later.
func copyFile(src, dst string) error {
	in, err := os.Open(src) //nolint:gosec // G304: src is the Agent binary (self), not external input
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755) //nolint:gosec // G302: copy of the Agent binary (.prev), needs to be executable
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
