package rtorrent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// minVersion is the oldest rTorrent the adapter supports (ADR-045 §2): d.multicall2 and
// system.client_version.
var minVersion = [3]int{0, 9, 8}

// ProbeResult is what the bootstrap needs to know of an rTorrent before enrolling it (ADR-045 §8):
// it reads the machine's settings instead of choosing them.
type ProbeResult struct {
	Version   string
	Port      int    // the port it listens on now
	PortRange string // network.port_range, "a-b"
	Random    bool   // network.port_random: a port picked in the range at each start
	Directory string // directory.default, as rTorrent sees it
	Session   string // session.path, as rTorrent sees it (where Export reads the .torrent files)
}

// Probe reads an rTorrent's version, port and folders, and refuses one older than 0.9.8.
func Probe(ctx context.Context, rawURL string) (ProbeResult, error) {
	a, err := New(rawURL)
	if err != nil {
		return ProbeResult{}, err
	}
	var p ProbeResult
	if p.Version, err = a.Version(ctx); err != nil {
		return p, fmt.Errorf("rTorrent at %s does not answer: %w", rawURL, err)
	}
	if !versionAtLeast(p.Version, minVersion) {
		return p, fmt.Errorf("rTorrent %s: SeedGrid needs 0.9.8 or later", p.Version)
	}
	read := func(cmd string) (any, error) {
		v, err := a.call(ctx, cmd, "")
		if err != nil {
			return nil, fmt.Errorf("rtorrent: %s: %w", cmd, err)
		}
		return v, nil
	}
	var v [5]any
	for i, cmd := range []string{portCmd, portRangeCmd, portRandomCmd, "directory.default", "session.path"} {
		if v[i], err = read(cmd); err != nil {
			return p, err
		}
	}
	p.Port = int(num(v[0]))
	p.PortRange = str(v[1])
	p.Random = num(v[2]) != 0
	p.Directory = strings.TrimRight(str(v[3]), "/")
	p.Session = strings.TrimRight(str(v[4]), "/")
	return p, nil
}

// The listening port's commands. rTorrent 0.16 renamed network.port_range and network.port_random
// to network.listen.port.range and network.listen.port.random; call tries the new name first.
const (
	portCmd       = "network.listen.port"
	portRangeCmd  = "network.listen.port.range"
	portRandomCmd = "network.listen.port.random"
)

// renamed are the commands rTorrent 0.16 renamed, with their name before it.
var renamed = map[string]string{
	portRangeCmd:           "network.port_range",
	portRangeCmd + ".set":  "network.port_range.set",
	portRandomCmd:          "network.port_random",
	portRandomCmd + ".set": "network.port_random.set",
}

// call runs a command, and its pre-0.16 name when this rTorrent does not know the new one.
func (a *Adapter) call(ctx context.Context, cmd string, params ...any) (any, error) {
	v, err := a.rpc.call(ctx, cmd, params...)
	var f *Fault
	if old, ok := renamed[cmd]; ok && errors.As(err, &f) && f.Code == -506 { // method not defined
		return a.rpc.call(ctx, old, params...)
	}
	return v, err
}

// versionAtLeast compares "0.9.8"-style versions.
func versionAtLeast(v string, want [3]int) bool {
	parts := strings.SplitN(v, ".", 3)
	for i := 0; i < 3; i++ {
		n := 0
		if i < len(parts) {
			digits := parts[i] // its leading digits: "8-rc1" is 8
			if end := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }); end >= 0 {
				digits = digits[:end]
			}
			var err error
			if n, err = strconv.Atoi(digits); err != nil {
				return false
			}
		}
		if n != want[i] {
			return n > want[i]
		}
	}
	return true
}
