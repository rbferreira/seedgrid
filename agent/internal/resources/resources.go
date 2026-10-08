// Package resources collects the Seed Node resource snapshot (disk, memory,
// CPU) reported to the Control Plane in the heartbeat. The Agent only reports raw
// values; the Health Score calculation is done by the Control Plane (ADR-003: Agent stateless).
package resources

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Snapshot represents the current resources of the Node.
type Snapshot struct {
	DiskTotalBytes uint64  `json:"disk_total_bytes"`
	DiskFreeBytes  uint64  `json:"disk_free_bytes"`
	MemTotalBytes  uint64  `json:"mem_total_bytes"`
	MemAvailBytes  uint64  `json:"mem_avail_bytes"`
	Load1          float64 `json:"load1"`
	CPUCores       int     `json:"cpu_cores"`
	HostUptimeSecs int64   `json:"host_uptime_seconds"`
}

// Collect gathers the snapshot. dataPath is the path to the torrent storage (e.g., /data);
// disk usage is measured on it. Partial failures do not zero out the rest (controlled degradation).
func Collect(dataPath string) Snapshot {
	s := Snapshot{CPUCores: runtime.NumCPU()}
	if dataPath == "" {
		dataPath = "/"
	}

	var st syscall.Statfs_t
	if err := syscall.Statfs(dataPath, &st); err == nil {
		// nolint:gosec // G115 (x2 below): FS block size is never negative.
		s.DiskTotalBytes = st.Blocks * uint64(st.Bsize) //nolint:gosec
		s.DiskFreeBytes = st.Bavail * uint64(st.Bsize)  //nolint:gosec // Bavail: available to non-root
	}

	total, avail := memInfo()
	s.MemTotalBytes, s.MemAvailBytes = total, avail
	s.Load1 = loadAvg1()
	s.HostUptimeSecs = hostUptime()
	return s
}

// virtualIface recognizes interfaces that are not the Node's uplink. The address
// that matters is the one from the physical interface; docker0/veth/br-* carry the same
// traffic internally and never have the Node's globally routable address.
func virtualIface(name string) bool {
	for _, p := range []string{"lo", "docker", "veth", "br-", "virbr", "tap", "dummy"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// GlobalIPv6 returns the Node's global (routable unicast) IPv6 address, or "" if there
// is none.
//
// This is the missing piece for the operator to open the BitTorrent port in the firewall: under CGNAT
// inbound connectability only exists via IPv6 (ADR-034), and the CP currently only stores the IPv4
// origin of the heartbeat (nodes.last_ip). Without this, figuring out what to open requires logging
// into the machine — and finding out that the address CHANGED (provider prefix rotation) does
// not happen at all.
//
// Discards loopback, link-local (fe80::/10) and ULA (fc00::/7, via IsPrivate), as well as
// down and virtual interfaces. With privacy extensions enabled there may be more than one
// global address; sorts them so the choice is stable across heartbeats and doesn't generate a false "changed
// address" every cycle. The bootstrap disables tempaddr precisely so the address
// announced to the tracker is the same one opened in the firewall.
func GlobalIPv6() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var candidates []net.IP
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 || virtualIface(ifc.Name) {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				candidates = append(candidates, ipn.IP)
			}
		}
	}
	return pickGlobalIPv6(candidates)
}

// pickGlobalIPv6 is the pure logic behind GlobalIPv6, separated to be testable
// without depending on the interfaces of the machine where the test runs.
func pickGlobalIPv6(ips []net.IP) string {
	var found []string
	for _, ip := range ips {
		// To4() != nil excludes IPv4 (and IPv4-mapped); IsGlobalUnicast already excludes
		// link-local, loopback and multicast; IsPrivate excludes ULA (fc00::/7).
		if ip == nil || ip.To4() != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
			continue
		}
		found = append(found, ip.String())
	}
	if len(found) == 0 {
		return ""
	}
	sort.Strings(found)
	return found[0]
}

// CPUSampler measures the Node's CPU UTILIZATION: the fraction of time the CPU was not
// idle, comparing /proc/stat between successive calls to Util(). Stateful,
// like the IOSampler — the Agent keeps one and calls Util() on every heartbeat (window =
// HB interval).
//
// Why not `load1 / cpu_cores`: Linux load counts processes waiting for I/O
// (D state), not just CPU. On a Node with a saturated disk the load goes up without the CPU being
// busy — exactly the Node where the metric matters most, and where it would lie. For the
// same reason `iowait` goes into the IDLE side of the calculation here: time waiting for disk
// is not busy CPU, and this distinction is the whole point of having the gauge.
//
// Caveat: when the Node is a container, the number depends on whether /proc/stat is
// virtualized per container — otherwise it reflects the entire host. Same
// caveat as node-exporter.
type CPUSampler struct {
	lastTotal uint64
	lastIdle  uint64
	primed    bool
}

// NewCPUSampler prepares the sampler (the 1st Util call just "arms" it).
func NewCPUSampler() *CPUSampler { return &CPUSampler{} }

// Util returns the % of busy CPU since the last call (0 on the 1st call or if
// unavailable). Clamped to [0,100].
func (s *CPUSampler) Util() float64 {
	if s == nil {
		return 0
	}
	total, idle, ok := cpuTimes()
	if !ok {
		return 0
	}
	return s.observe(total, idle)
}

// observe is the Util arithmetic isolated from the /proc/stat read, so the test
// exercises THIS code with synthetic windows instead of depending on the machine's CPU
// where it runs (and without duplicating the math in the test, which would test the copy).
func (s *CPUSampler) observe(total, idle uint64) float64 {
	prevTotal, prevIdle, primed := s.lastTotal, s.lastIdle, s.primed
	s.lastTotal, s.lastIdle, s.primed = total, idle, true
	if !primed {
		return 0
	}
	// Kernel counters are monotonic. If they went backwards (inconsistent
	// read, container recreated), discard the window instead of making up a number.
	if total <= prevTotal || idle < prevIdle {
		return 0
	}
	dTotal := float64(total - prevTotal)
	dIdle := float64(idle - prevIdle)
	util := (dTotal - dIdle) / dTotal * 100
	if util < 0 {
		util = 0
	}
	if util > 100 {
		util = 100
	}
	return util
}

// cpuTimes reads the aggregated line ("cpu ...") from /proc/stat and returns the total jiffies and
// the idle portion (idle + iowait). Fields: user nice system idle iowait irq softirq
// steal guest guest_nice.
func cpuTimes() (total, idle uint64, ok bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0, false
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 6 || fields[0] != "cpu" {
		return 0, 0, false
	}
	for i, v := range fields[1:] {
		n, perr := strconv.ParseUint(v, 10, 64)
		if perr != nil {
			return 0, 0, false
		}
		total += n
		if i == 3 || i == 4 { // idle, iowait
			idle += n
		}
	}
	return total, idle, true
}

// StorageHealth checks if the torrent storage (path, e.g., /data) is actually
// mounted and writable. It's the signal the Control Plane uses to alert/health when
// the Node's USB drive fails: systemd/kernel keeps the mountpoint pointing to a dead
// device (the mount gets "stuck"), so the path still exists but either loses the
// mountpoint (falls back to the parent fs) or becomes read-only / fails on write.
//
// requireMount tells if the operator DECLARED a dedicated disk for this Node
// (SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT, written by the bootstrap during installation):
//
//   - true: returns ok=true ONLY if path is a mountpoint AND is writable. This is the
//     protection against the disk that didn't mount — without it the Engine would write to the
//     system disk and the Node would look healthy (docs/06 - Storage.md §35);
//   - false: the Node uses the system disk (VM, mini PC, single-disk NAS) and there is no
//     mountpoint to require. Only the write is checked; free space is tracked by
//     NODE_DISK_FULL.
//
// The expectation comes from outside on purpose and is NEVER inferred here: inferring "it's a local
// folder, so I don't require a mountpoint" would fail exactly in the case the rule protects, since
// a disk that didn't mount is indistinguishable from a local folder.
//
// When ok=false, detail brings a short reason in PT-BR.
func StorageHealth(path string, requireMount bool) (ok bool, detail string) {
	// 1) Is it a mountpoint? Only with a declared dedicated disk (requireMount): without it there is no
	// mountpoint to require and this step doesn't exist — the write below applies in both modes.
	if requireMount {
		if why := mountpointFailure(path); why != "" {
			return false, why
		}
	}

	// 2) Is it writable? Creates a test file, writes 1 byte and removes it.
	// Distinguishes read-only (mount stuck in RO) from other write errors.
	testFile := filepath.Join(path, ".seedgrid-wtest")
	f, err := os.OpenFile(testFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // G304: path comes from the operator's config (dataPath), not from external input
	if err != nil {
		if isReadOnly(err) {
			return false, "read-only"
		}
		return false, writeFailure(err)
	}
	if _, werr := f.Write([]byte{0}); werr != nil {
		_ = f.Close()
		_ = os.Remove(testFile)
		if isReadOnly(werr) {
			return false, "read-only"
		}
		return false, writeFailure(werr)
	}
	_ = f.Close()
	_ = os.Remove(testFile)
	return true, ""
}

// mountpointFailure returns "" when path is a mountpoint, or the reason (in PT-BR) when
// it is not or when it couldn't be checked. Compares the device (st_dev) of path with that of
// the parent directory: if they are equal, path is on the SAME filesystem as the parent → not a mountpoint.
func mountpointFailure(path string) string {
	var st, parent syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return fmt.Sprintf("error checking: %v", err)
	}
	if err := syscall.Stat(filepath.Dir(path), &parent); err != nil {
		return fmt.Sprintf("error checking: %v", err)
	}
	if st.Dev == parent.Dev {
		return "not a mountpoint"
	}
	return ""
}

// writeFailure describes the write failure for the OPERATOR, not the one who wrote the
// code.
//
// The `%v` of an *os.PathError embeds the path, so the alert arrived as
// "write error: open /data/.seedgrid-wtest: input/output error" — telling the
// operator to look for a file the Agent itself creates and deletes (#178). The problem is the
// /data, never the probe. Unwraps down to the errno and returns a short phrase, like the rest
// of this function ("not a mountpoint", "read-only").
//
// EIO gets its own text because it is the classic symptom of an underpowered USB drive — the case
// documented for node-03 — and a raw "input/output error" requires knowing the errno by heart.
func writeFailure(err error) string {
	switch {
	case errors.Is(err, syscall.EIO):
		return "device not responding (I/O error — check disk cable and power)"
	case errors.Is(err, syscall.ENOSPC):
		return "no space left on device"
	case errors.Is(err, syscall.ENOENT):
		return "path does not exist"
	}
	// If errno is unknown: uses the UNWRAPPED error, which does not carry the probe path.
	var pe *os.PathError
	if errors.As(err, &pe) && pe.Err != nil {
		return "write error: " + pe.Err.Error()
	}
	return "write error: " + err.Error()
}

// isReadOnly recognizes read-only or no-permission mount errors (a dead device
// usually turns into EROFS; permission denied becomes EACCES/EPERM).
func isReadOnly(err error) bool {
	return errors.Is(err, syscall.EROFS) ||
		errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPERM)
}

// hostUptime reads the host uptime in seconds from /proc/uptime (first field).
func hostUptime() int64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return int64(secs)
}

// memInfo reads MemTotal and MemAvailable from /proc/meminfo (values in kB → bytes).
func memInfo() (total, avail uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total = kb * 1024
		case "MemAvailable:":
			avail = kb * 1024
		}
	}
	return total, avail
}

// IOSampler measures the UTILIZATION of the disk serving the storage (dataPath): the fraction of
// time the disk was busy (busy%), comparing /proc/diskstats between
// successive calls to Util(). It is the SATURATION signal — pegging at ~100% means
// the disk is the bottleneck (which was the case with the Pi's underpowered USB drive). Stateful: the
// Agent keeps a sampler and calls Util() every heartbeat (window = HB interval).
//
// Caveat: resolves the physical disk of dataPath via /proc/mounts. In device-mapper or
// a Node that is a container the name doesn't match diskstats → returns 0 (not meaningful
// there: the disk belongs to the host, shared); on bare-metal Nodes (Pi, /dev/sdaN) it works.
type IOSampler struct {
	device   string // physical disk of dataPath (e.g., "sda"); "" if unresolved
	lastIOms uint64
	lastAt   time.Time
	primed   bool
}

// NewIOSampler resolves the disk serving dataPath and prepares the sampler.
func NewIOSampler(dataPath string) *IOSampler {
	if dataPath == "" {
		dataPath = "/"
	}
	return &IOSampler{device: diskDevice(dataPath)}
}

// Util returns the % of time the disk was busy since the last call
// (0 on the 1st call or if unavailable). Clamped to [0,100].
func (s *IOSampler) Util() float64 {
	if s == nil || s.device == "" {
		return 0
	}
	ioms, ok := diskIOms(s.device)
	now := time.Now()
	if !ok {
		return 0
	}
	if !s.primed {
		s.lastIOms, s.lastAt, s.primed = ioms, now, true
		return 0
	}
	dtMs := now.Sub(s.lastAt).Milliseconds()
	dIO := int64(ioms) - int64(s.lastIOms) //nolint:gosec // G115: kernel I/O ms counter, always small
	s.lastIOms, s.lastAt = ioms, now
	if dtMs <= 0 || dIO < 0 {
		return 0
	}
	util := float64(dIO) / float64(dtMs) * 100
	if util > 100 {
		util = 100
	}
	return util
}

// diskDevice resolves the physical disk serving path via /proc/mounts (the mountpoint that
// is the longest prefix of path) → device basename → base disk (strips the partition).
func diskDevice(path string) string {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	best, bestDev := -1, ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		dev, mnt := fields[0], fields[1]
		if !strings.HasPrefix(dev, "/dev/") {
			continue
		}
		if (path == mnt || strings.HasPrefix(path, strings.TrimRight(mnt, "/")+"/")) && len(mnt) > best {
			best, bestDev = len(mnt), dev
		}
	}
	if bestDev == "" {
		return ""
	}
	return baseDisk(bestDev[strings.LastIndexByte(bestDev, '/')+1:])
}

// baseDisk reduces a partition name to the base disk: sda2→sda, nvme0n1p2→nvme0n1,
// mmcblk0p2→mmcblk0. device-mapper names (without this pattern) return as is
// (won't match in diskstats → util 0, acceptable).
func baseDisk(part string) string {
	if i := strings.LastIndexByte(part, 'p'); i > 0 && i+1 < len(part) &&
		part[i-1] >= '0' && part[i-1] <= '9' && allDigits(part[i+1:]) {
		return part[:i] // nvme0n1p2 / mmcblk0p2
	}
	// nvme/mmcblk are already the entire disk (end in a digit, without pN suffix):
	// nvme0n1, mmcblk0. Without this, the digit trimming below would mangle them.
	if strings.HasPrefix(part, "nvme") || strings.HasPrefix(part, "mmcblk") {
		return part
	}
	j := len(part)
	for j > 0 && part[j-1] >= '0' && part[j-1] <= '9' {
		j--
	}
	if j == 0 {
		return part
	}
	return part[:j]
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// diskIOms reads the "time spent in I/O (ms)" field (13th field) of the disk in /proc/diskstats.
func diskIOms(device string) (uint64, bool) {
	f, err := os.Open("/proc/diskstats")
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 13 || fields[2] != device {
			continue
		}
		v, err := strconv.ParseUint(fields[12], 10, 64) // ms doing I/Os
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

// loadAvg1 reads the 1-minute load average from /proc/loadavg.
func loadAvg1() float64 {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return v
}
