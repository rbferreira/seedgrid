package rtorrent

import (
	"context"
	"strings"
	"testing"
)

func TestVersionAtLeast(t *testing.T) {
	for v, want := range map[string]bool{
		"0.9.8": true, "0.9.7": false, "0.9.10": true, "0.10.0": true, "0.16.25": true,
		"1.0": true, "0.9": false, "0.8.9": false, "garbage": false, "0.9.8-rc1": true,
	} {
		if got := versionAtLeast(v, minVersion); got != want {
			t.Errorf("versionAtLeast(%q) = %t, want %t", v, got, want)
		}
	}
}

func TestProbe(t *testing.T) {
	f := newFake()
	f.on("system.client_version", func([]any) (any, error) { return "0.9.8", nil })
	f.on("network.listen.port", func([]any) (any, error) { return 50000, nil })
	// rTorrent 0.16's names; the fallback to the old ones is TestConfigure's.
	f.on("network.listen.port.range", func([]any) (any, error) { return "50000-50010", nil })
	f.on("network.listen.port.random", func([]any) (any, error) { return 1, nil })
	f.on("directory.default", func([]any) (any, error) { return "/downloads/", nil })
	f.on("session.path", func([]any) (any, error) { return "/data/rtorrent/.session/", nil })
	u := f.serveSCGI(t)
	p, err := Probe(context.Background(), u)
	want := ProbeResult{Version: "0.9.8", Port: 50000, PortRange: "50000-50010", Random: true,
		Directory: "/downloads", Session: "/data/rtorrent/.session"}
	if err != nil || p != want {
		t.Errorf("Probe = %+v %v, want %+v", p, err, want)
	}

	f.on("system.client_version", func([]any) (any, error) { return "0.9.6", nil })
	if _, err := Probe(context.Background(), u); err == nil || !strings.Contains(err.Error(), "0.9.8 or later") {
		t.Errorf("Probe of 0.9.6 = %v, want it refused", err)
	}
}
