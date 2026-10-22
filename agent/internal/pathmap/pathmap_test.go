package pathmap

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	m, err := Parse(" /downloads=/mnt/user/torrents ; /movies=/mnt/media/movies/ ;")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []string{"/mnt/user/torrents", "/mnt/media/movies"}
	if got := m.HostRoots(); !reflect.DeepEqual(got, want) {
		t.Errorf("HostRoots = %v, want %v", got, want)
	}

	for _, bad := range []string{"/downloads", "=/mnt", "/downloads=", "downloads=/mnt", "/downloads=mnt"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) accepted an invalid entry", bad)
		}
	}
	if m, err := Parse(""); err != nil || len(m.HostRoots()) != 0 {
		t.Errorf("Parse(\"\") = (%v, %v), want the identity map", m, err)
	}
}

func TestTranslate(t *testing.T) {
	m, err := Parse("/data=/srv/a;/data/movies=/srv/b;/downloads=/mnt/user/torrents")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ engine, host string }{
		{"/downloads/Some.Movie.2020", "/mnt/user/torrents/Some.Movie.2020"},
		{"/downloads", "/mnt/user/torrents"},
		// The longest prefix wins.
		{"/data/movies/x.mkv", "/srv/b/x.mkv"},
		{"/data/tv/y.mkv", "/srv/a/tv/y.mkv"},
	}
	for _, c := range cases {
		if got := m.ToHost(c.engine); got != c.host {
			t.Errorf("ToHost(%q) = %q, want %q", c.engine, got, c.host)
		}
		if got := m.ToEngine(c.host); got != c.engine {
			t.Errorf("ToEngine(%q) = %q, want %q", c.host, got, c.engine)
		}
	}
	// A prefix matches whole components only, and an unmapped path is unchanged.
	for _, p := range []string{"/database/x", "/downloads2/x", "/other/y", ""} {
		if got := m.ToHost(p); got != p {
			t.Errorf("ToHost(%q) = %q, want it unchanged", p, got)
		}
	}
}

func TestIdentity(t *testing.T) {
	var m Map
	if got := m.ToHost("/data/x"); got != "/data/x" {
		t.Errorf("zero Map ToHost = %q", got)
	}
	if got := m.ToEngine("/data/x"); got != "/data/x" {
		t.Errorf("zero Map ToEngine = %q", got)
	}
}

func TestRootMapping(t *testing.T) {
	m, err := Parse("/=/host/root")
	if err != nil {
		t.Fatal(err)
	}
	if got := m.ToHost("/a/b"); got != "/host/root/a/b" {
		t.Errorf("ToHost under / = %q", got)
	}
	if got := m.ToEngine("/host/root/a"); got != "/a" {
		t.Errorf("ToEngine to / = %q", got)
	}
}
