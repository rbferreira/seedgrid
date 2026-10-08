package torrent

import "testing"

func TestParseMagnet(t *testing.T) {
	hash, name := parseMagnet("magnet:?xt=urn:btih:DD8255ECDC7CA55FB0BBF81323D87062DB1F6D1C&dn=Big+Buck+Bunny&tr=udp://x")
	if hash != "dd8255ecdc7ca55fb0bbf81323d87062db1f6d1c" {
		t.Errorf("hash = %q", hash)
	}
	if name != "Big Buck Bunny" {
		t.Errorf("name = %q", name)
	}
}

func TestParseMagnet_NoHash(t *testing.T) {
	hash, _ := parseMagnet("magnet:?dn=No+Hash")
	if hash != "" {
		t.Errorf("hash = %q, want empty", hash)
	}
}

func TestNameInCluster(t *testing.T) {
	cluster := []string{
		NormalizeName("SamUploadAssistantSetup.exe"),
		NormalizeName("Idiocracy.2006.1080p.x264.BluRay.2.0-NOGROUP"),
	}
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"extra extension in cluster", "SamUploadAssistantSetup", true}, // catalog without .exe
		{"exact match", "Idiocracy.2006.1080p.x264.BluRay.2.0-NOGROUP", true},
		{"another release", "Ikka 2026 1080p NF WEB-DL", false},
		{"short prefix does not match", "Sam", false}, // < 10 chars → no false positives
	}
	for _, c := range cases {
		if got := NameInCluster(c.in, cluster); got != c.want {
			t.Errorf("%s: NameInCluster(%q) = %v, want %v", c.name, c.in, got, c.want)
		}
	}
}
