package api

import "testing"

func TestMigrateAuditDesc(t *testing.T) {
	cases := []struct {
		name        string
		torrentName string
		sourceNode  string
		destNode    string
		want        string
	}{
		{
			name:        "known names",
			torrentName: "Movie XPTO",
			sourceNode:  "seedgrid-node-01",
			destNode:    "seedgrid-node-02",
			want:        "Migrated torrent «Movie XPTO» from Node «seedgrid-node-01» to Node «seedgrid-node-02»",
		},
		{
			name: "all unknown",
			want: "Migrated a torrent from a Node to a Node",
		},
		{
			name:        "only known torrent",
			torrentName: "Movie XPTO",
			want:        "Migrated torrent «Movie XPTO» from a Node to a Node",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := migrateAuditDesc(c.torrentName, c.sourceNode, c.destNode)
			if got != c.want {
				t.Fatalf("migrateAuditDesc() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestLabelOrFallback(t *testing.T) {
	if got := labelOrFallback("seedgrid-node-01", "unknown"); got != "seedgrid-node-01" {
		t.Fatalf("labelOrFallback with label = %q, want original label", got)
	}
	if got := labelOrFallback("", "unknown"); got != "unknown" {
		t.Fatalf("labelOrFallback with empty label = %q, want fallback", got)
	}
}

func TestPluralTorrents(t *testing.T) {
	cases := map[int]string{0: "0 torrents", 1: "1 torrent", 2: "2 torrents"}
	for n, want := range cases {
		if got := pluralTorrents(n); got != want {
			t.Fatalf("pluralTorrents(%d) = %q, want %q", n, got, want)
		}
	}
}
