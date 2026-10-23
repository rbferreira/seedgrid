package inventory

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCollect(t *testing.T) {
	info := Collect()
	if info.Hostname == "" {
		t.Error("empty hostname")
	}
	if info.Architecture != runtime.GOARCH {
		t.Errorf("arch = %q, want %q", info.Architecture, runtime.GOARCH)
	}
}

func TestOSRelease(t *testing.T) {
	// osRelease should not panic even without a readable /etc/os-release.
	name, version := osRelease()
	_ = name
	_ = version
}

// TestMachineID (ADR-044): the first machine-id that identifies something; a not-yet-booted
// image's "uninitialized" does not.
func TestMachineID(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	empty := write("empty", "\n")
	uninit := write("uninit", "uninitialized\n")
	good := write("good", "0123456789abcdef0123456789abcdef\n")
	if got := machineID(filepath.Join(dir, "missing"), empty, uninit, good); got != "0123456789abcdef0123456789abcdef" {
		t.Errorf("machineID = %q, want the real one", got)
	}
	if got := machineID(empty, uninit); got != "" {
		t.Errorf("machineID = %q, want none", got)
	}
}
