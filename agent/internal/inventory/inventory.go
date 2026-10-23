// Package inventory collects local Node information reported to the Control Plane.
// In V1 it collects essentials (OS, kernel, architecture); detailed collectors
// (disks, CPU, memory) evolve in subsequent increments.
package inventory

import (
	"os"
	"runtime"
	"strings"
)

// Info holds basic Node information.
type Info struct {
	Hostname      string
	OSName        string
	OSVersion     string
	KernelVersion string
	Architecture  string
	// MachineID is the machine's systemd identity (/etc/machine-id), "" when it has none. It
	// survives reboots and SeedGrid reinstalls, and tells the same machine from another one with
	// the same hostname (ADR-044).
	MachineID string
}

// Collect gathers available local information (Linux).
func Collect() Info {
	hostname, _ := os.Hostname()
	name, version := osRelease()
	return Info{
		Hostname:      hostname,
		OSName:        name,
		OSVersion:     version,
		KernelVersion: readTrim("/proc/sys/kernel/osrelease"),
		Architecture:  runtime.GOARCH,
		MachineID:     machineID("/etc/machine-id", "/var/lib/dbus/machine-id"),
	}
}

// machineID returns the first non-empty machine-id among paths. "uninitialized" is what systemd
// writes in an image not booted yet: it identifies nothing.
func machineID(paths ...string) string {
	for _, p := range paths {
		if v := readTrim(p); v != "" && v != "uninitialized" {
			return v
		}
	}
	return ""
}

// osRelease reads NAME and VERSION_ID from /etc/os-release.
func osRelease() (name, version string) {
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = strings.Trim(strings.TrimSpace(val), `"`)
		switch key {
		case "NAME":
			name = val
		case "VERSION_ID":
			version = val
		}
	}
	return name, version
}

func readTrim(path string) string {
	b, err := os.ReadFile(path) //nolint:gosec // G304: only called with internal literals (osrelease, machine-id)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
