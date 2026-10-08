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
	}
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
	b, err := os.ReadFile(path) //nolint:gosec // G304: only called with internal literal ("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
