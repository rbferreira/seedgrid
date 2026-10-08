package resources

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestBaseDisk(t *testing.T) {
	cases := []struct{ in, want string }{
		{"sda2", "sda"},
		{"sda", "sda"},
		{"sdb15", "sdb"},
		{"nvme0n1p2", "nvme0n1"},
		{"nvme0n1", "nvme0n1"},
		{"mmcblk0p2", "mmcblk0"},
		{"mmcblk0", "mmcblk0"},
		{"xvdb1", "xvdb"},
		{"vda", "vda"},
		{"loop0", "loop"}, // no valid 'p' → strips trailing digits
		{"1234", "1234"},  // only digits → returns as is (does not zero out)
		{"", ""},
		{"dm-0", "dm-"}, // device-mapper: no match in diskstats → util 0 (documented)
	}
	for _, c := range cases {
		if got := baseDisk(c.in); got != c.want {
			t.Errorf("baseDisk(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAllDigits(t *testing.T) {
	for _, s := range []string{"0", "12", "9999"} {
		if !allDigits(s) {
			t.Errorf("allDigits(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "1a", "p2", " "} {
		if allDigits(s) {
			t.Errorf("allDigits(%q) = true, want false", s)
		}
	}
}

func TestStorageHealthNotMountpoint(t *testing.T) {
	// A subdirectory of t.TempDir() is on the SAME filesystem as the parent, so it is not
	// a mountpoint → ok=false with explanatory detail, even if it is writable.
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatalf("prepare dir: %v", err)
	}
	ok, detail := StorageHealth(dir, true)
	if ok {
		t.Errorf("StorageHealth(%q, true) ok = true, want false (not a mountpoint)", dir)
	}
	if detail != "not a mountpoint" {
		t.Errorf("StorageHealth(%q, true) detail = %q, want %q", dir, detail, "not a mountpoint")
	}
}

// TestStorageHealthNoDedicatedDisk: with requireMount=false (single-disk Node, #221) the
// SAME folder that the test above fails — on the parent filesystem, writable — is healthy. It is
// what takes the Node out of the "storage_ok=false forever" limbo without the operator having done
// anything wrong.
func TestStorageHealthNoDedicatedDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatalf("prepare dir: %v", err)
	}
	ok, detail := StorageHealth(dir, false)
	if !ok || detail != "" {
		t.Errorf("StorageHealth(%q, false) = (%v, %q), want (true, \"\")", dir, ok, detail)
	}
	// The write probe must not leave a trace on the operator's storage.
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("write probe left files in %q: %v (err=%v)", dir, entries, err)
	}
}

// TestStorageHealthWritableMountpoint: the happy path of dedicated disk mode — a
// writable mountpoint passes. It uses /dev/shm (tmpfs mounted on any Linux with systemd or
// container); where it does not exist, or is not a mountpoint, the test has nothing to prove.
func TestStorageHealthWritableMountpoint(t *testing.T) {
	const shm = "/dev/shm"
	if mountpointFailure(shm) != "" {
		t.Skipf("%s is not a mountpoint in this environment", shm)
	}
	ok, detail := StorageHealth(shm, true)
	if !ok {
		t.Skipf("%s is not writable in this environment (%s)", shm, detail)
	}
	// And the no-mountpoint mode agrees: what is healthy requiring the disk is healthy without requiring it.
	if ok, detail := StorageHealth(shm, false); !ok {
		t.Errorf("StorageHealth(%q, false) = (false, %q), want true", shm, detail)
	}
}

// TestStorageHealthWriteAppliesInBothModes: dispensing the mountpoint does NOT dispense with writing.
// Each case fails on the write probe with requireMount=false — which is the only mode where
// step 2 is reached without first falling into "not a mountpoint".
func TestStorageHealthWriteAppliesInBothModes(t *testing.T) {
	// Path that is a file, not a directory: cannot create the probe inside it.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("prepare file: %v", err)
	}
	cases := []struct {
		name, path, detail string
	}{
		{"nonexistent path", filepath.Join(t.TempDir(), "does-not-exist"), "path does not exist"},
		{"path is a file", file, "write error: not a directory"},
	}
	for _, c := range cases {
		ok, detail := StorageHealth(c.path, false)
		if ok {
			t.Errorf("%s: StorageHealth(false) ok = true, want false (write must be valid without mountpoint)", c.name)
		}
		if detail != c.detail {
			t.Errorf("%s: StorageHealth(false) detail = %q, want %q", c.name, detail, c.detail)
		}
	}
}

// TestStorageHealthReadOnly: folder without write permission is "read-only" in
// single-disk mode too. Skipped as root, which ignores directory permissions.
func TestStorageHealthReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permission; the case runs when the test is run without privilege")
	}
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0o500); err != nil { //nolint:gosec // G301: 0500 is the point of the test (directory without write)
		t.Fatalf("prepare dir: %v", err)
	}
	ok, detail := StorageHealth(dir, false)
	if ok || detail != "read-only" {
		t.Errorf("StorageHealth(RO, false) = (%v, %q), want (false, %q)", ok, detail, "read-only")
	}
}

func TestStorageHealthNonexistent(t *testing.T) {
	// Path that does not exist → stat fails → ok=false with check reason.
	path := filepath.Join(t.TempDir(), "does-not-exist")
	ok, detail := StorageHealth(path, true)
	if ok {
		t.Errorf("StorageHealth(%q, true) ok = true, want false (nonexistent path)", path)
	}
	if detail == "" {
		t.Errorf("StorageHealth(%q, true) detail empty, want error reason", path)
	}
}

func TestIOSamplerUtilPrimesAtZero(t *testing.T) {
	// Without a resolved device, Util is always 0 (controlled degradation).
	s := &IOSampler{device: ""}
	if got := s.Util(); got != 0 {
		t.Errorf("Util() with empty device = %v, want 0", got)
	}
	// Nil-safe.
	var ns *IOSampler
	if got := ns.Util(); got != 0 {
		t.Errorf("Util() nil = %v, want 0", got)
	}
}

func TestVirtualIface(t *testing.T) {
	for _, name := range []string{"lo", "docker0", "veth1a2b3c", "br-af2fe7f9d09d", "virbr0", "tap0", "dummy0"} {
		if !virtualIface(name) {
			t.Errorf("virtualIface(%q) = false, want true", name)
		}
	}
	// Real uplink names of fleet Nodes: eth0 on CTs, enp1s0/mlan0 on bare-metal.
	for _, name := range []string{"eth0", "enp1s0", "wlp2s0", "mlan0", "eno1"} {
		if virtualIface(name) {
			t.Errorf("virtualIface(%q) = true, want false (physical uplink)", name)
		}
	}
}

func TestPickGlobalIPv6(t *testing.T) {
	ip := func(s string) net.IP { return net.ParseIP(s) }

	// Without any global: "" (Node without IPv6 — under CGNAT this means unreachable,
	// but the Control Plane judges this, not this function).
	if got := pickGlobalIPv6([]net.IP{ip("192.168.0.10"), ip("fe80::1"), ip("::1"), ip("fd00::1")}); got != "" {
		t.Errorf("pickGlobalIPv6(no global) = %q, want empty", got)
	}

	// Discards IPv4, link-local, loopback and ULA; returns the global.
	got := pickGlobalIPv6([]net.IP{
		ip("192.168.0.162"),
		ip("fe80::72ab:6bac:8483:31e7"),
		ip("fd12:3456::1"),
		ip("2001:db8:60b:c000:6e27:750a:3005:3b00"),
	})
	if want := "2001:db8:60b:c000:6e27:750a:3005:3b00"; got != want {
		t.Errorf("pickGlobalIPv6 = %q, want %q", got, want)
	}

	// With privacy extensions there are TWO globals. The choice must be stable across
	// heartbeats — if it oscillated, the CP would see "address changed" every cycle and the
	// prefix rotation alert would turn into pure noise.
	a := []net.IP{ip("2001:db8:60b:c000:8973:95f4:14f3:1a48"), ip("2001:db8:60b:c000:6e27:750a:3005:3b00")}
	b := []net.IP{ip("2001:db8:60b:c000:6e27:750a:3005:3b00"), ip("2001:db8:60b:c000:8973:95f4:14f3:1a48")}
	if pickGlobalIPv6(a) != pickGlobalIPv6(b) {
		t.Errorf("unstable choice: %q vs %q with the same list in a different order",
			pickGlobalIPv6(a), pickGlobalIPv6(b))
	}

	// Empty input and nil must not panic.
	if got := pickGlobalIPv6(nil); got != "" {
		t.Errorf("pickGlobalIPv6(nil) = %q, want empty", got)
	}
	if got := pickGlobalIPv6([]net.IP{nil}); got != "" {
		t.Errorf("pickGlobalIPv6([nil]) = %q, want empty", got)
	}
}

func TestCPUSamplerUtil(t *testing.T) {
	// Nil-safe and without "arming": the 1st read only stores the base, does not invent a number.
	var ns *CPUSampler
	if got := ns.Util(); got != 0 {
		t.Errorf("Util() nil = %v, want 0", got)
	}
	if got := NewCPUSampler().Util(); got != 0 {
		t.Errorf("1st Util() = %v, want 0 (only arms the sampler)", got)
	}

	primed := func() *CPUSampler {
		return &CPUSampler{lastTotal: 10_000, lastIdle: 8_000, primed: true}
	}

	// Window of 1000 jiffies with 250 idle → 75% busy.
	if got := primed().observe(11_000, 8_250); got < 74.9 || got > 75.1 {
		t.Errorf("util = %v, want ~75", got)
	}

	// Counter going backwards (inconsistent read, container recreated):
	// discards the window instead of reporting an invented number.
	if got := primed().observe(9_000, 7_000); got != 0 {
		t.Errorf("total regressing = %v, want 0", got)
	}

	// Window without progress (dTotal = 0) cannot divide by zero.
	if got := primed().observe(10_000, 8_000); got != 0 {
		t.Errorf("dTotal=0 = %v, want 0", got)
	}

	// Nothing idle in the window: clamps at 100, does not overflow.
	if got := primed().observe(11_000, 8_000); got != 100 {
		t.Errorf("no idle = %v, want 100", got)
	}

	// iowait counts as IDLE: it is the distinction that load average does not make and the reason
	// the bar exists. 1000 jiffies, 200 idle + 700 iowait → 10% CPU busy.
	if got := primed().observe(11_000, 8_900); got < 9.9 || got > 10.1 {
		t.Errorf("with iowait = %v, want ~10", got)
	}
}

// The alert reached the operator as "write error: open /data/.seedgrid-wtest:
// input/output error", making him look for a file that the Agent itself creates and
// deletes (#178). The problem is always /data, never the probe.
func TestWriteFailure(t *testing.T) {
	probe := "/data/.seedgrid-wtest"

	t.Run("EIO gets operator text and does not leak probe", func(t *testing.T) {
		got := writeFailure(&os.PathError{Op: "open", Path: probe, Err: syscall.EIO})
		if strings.Contains(got, ".seedgrid-wtest") {
			t.Errorf("leaked probe file: %q", got)
		}
		if !strings.Contains(got, "I/O") {
			t.Errorf("EIO should become I/O error text, got %q", got)
		}
	})

	t.Run("unknown errno unwraps PathError", func(t *testing.T) {
		got := writeFailure(&os.PathError{Op: "open", Path: probe, Err: syscall.EBUSY})
		if strings.Contains(got, probe) {
			t.Errorf("leaked probe path: %q", got)
		}
		if !strings.Contains(got, "write error") {
			t.Errorf("expected prefix 'write error', got %q", got)
		}
	})

	t.Run("no space and missing path have their own text", func(t *testing.T) {
		if got := writeFailure(&os.PathError{Path: probe, Err: syscall.ENOSPC}); got != "no space left on device" {
			t.Errorf("ENOSPC = %q", got)
		}
		if got := writeFailure(&os.PathError{Path: probe, Err: syscall.ENOENT}); got != "path does not exist" {
			t.Errorf("ENOENT = %q", got)
		}
	})

	t.Run("simple error, without PathError, does not break", func(t *testing.T) {
		if got := writeFailure(errors.New("raw failure")); got != "write error: raw failure" {
			t.Errorf("simple error = %q", got)
		}
	})
}
