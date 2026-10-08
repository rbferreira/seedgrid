package inventory

import (
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
