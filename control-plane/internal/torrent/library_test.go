package torrent

import "testing"

func libraryOf(entries ...libraryEntry) *Library { return &Library{entries: entries} }

// The names and sizes come from a real cluster: items the name match didn't recognize and the exact
// size does (#37).
func TestLibrary_HasFile(t *testing.T) {
	lib := libraryOf(
		newLibraryEntry("aa", "Brotherhood.S02.1080p.NF.WEB-DL.DDP5.1.AV1.DUAL-HAO", 12_345_678_901, "node-01"),
		newLibraryEntry("bb", "Author_Name-The_Truth_Will_Prevail_3rd_edition-2022-AUDIOBOOK-POR-2023-GRP.m4b", 352_606_347, "node-02"),
		newLibraryEntry("cc", "Tiefsee.S01", 7_777_777_777, "node-03"),
	)
	cases := []struct {
		name    string
		size    int64
		catalog string
		files   []string
		want    bool
	}{
		{"same size, same work with the name written differently", 12_345_678_901,
			"Brotherhood S02 1080p NF WEB-DL DD+ 5.1 AV1 DUAL-HAO", nil, true},
		{"same size, title in another order, file named like the torrent", 352_606_347,
			"The Truth Will Prevail (3rd Edition) - Author Name [2023] [AUDIOBOOK]",
			[]string{"Author_Name-The_Truth_Will_Prevail_3rd_edition-2022-AUDIOBOOK-POR-2023-GRP.m4b"}, true},
		{"season with the title in another language: the folder is a prefix of the files", 7_777_777_777,
			"Below 2026 S01 1080p NF WEB-DL AAC2.0 AV1 DUAL-Lup1n",
			[]string{"Tiefsee.S01E01.1080p.NF.WEB-DL.AAC2.0.AV1.DUAL-Lup1n.mkv"}, true},
		{"same work, different size: it's another version, not the same file", 9_999_999_999,
			"Brotherhood S02 1080p NF WEB-DL DD+ 5.1 AV1 DUAL-HAO", nil, false},
		{"same size, different work: a coincidence doesn't hide the button", 12_345_678_901,
			"Inception 2010 1080p BluRay x264", []string{"Inception.2010.1080p.BluRay.x264.mkv"}, false},
		{"no size", 0, "Brotherhood S02 1080p NF WEB-DL DD+ 5.1 AV1 DUAL-HAO", nil, false},
	}
	for _, c := range cases {
		if got := lib.HasFile(c.size, c.catalog, c.files); got != c.want {
			t.Errorf("%s: HasFile = %v, want %v", c.name, got, c.want)
		}
	}
	if (*Library)(nil).HasFile(1, "x", nil) {
		t.Error("a nil library has nothing")
	}
}

func TestLibrary_OtherVersions(t *testing.T) {
	lib := libraryOf(
		newLibraryEntry("h1080", "Interstellar.2014.1080p.BluRay.x264-GRP", 10_000, "node-01"),
		newLibraryEntry("h2160", "Interstellar.2014.2160p.UHD.BluRay.x265-OTHER", 40_000, "node-02"),
		newLibraryEntry("hx", "Inception.2010.1080p.BluRay.x264-GRP", 9_000, "node-01"),
	)

	vs := lib.OtherVersions("Interstellar 2014 720p WEB-DL x264", "new", 0)
	if len(vs) != 2 {
		t.Fatalf("versions = %+v, want both Interstellar ones", vs)
	}
	// From the largest to the smallest, with what the operator needs to decide.
	if vs[0].Resolution != "2160p" || vs[0].Codec != "x265" || vs[0].SizeBytes != 40_000 || vs[0].Node != "node-02" {
		t.Errorf("1st version = %+v, want 2160p x265 40000 node-02", vs[0])
	}
	if vs[1].Resolution != "1080p" || vs[1].Codec != "x264" || vs[1].Name != "Interstellar.2014.1080p.BluRay.x264-GRP" {
		t.Errorf("2nd version = %+v, want the 1080p x264", vs[1])
	}

	// The file itself isn't "another version": by hash on add, by size in the catalog.
	if vs := lib.OtherVersions("Interstellar 2014 1080p BluRay x264", "h1080", 0); len(vs) != 1 || vs[0].Resolution != "2160p" {
		t.Errorf("excluding by hash = %+v, want only the 2160p", vs)
	}
	if vs := lib.OtherVersions("Interstellar 2014 1080p BluRay x264", "", 10_000); len(vs) != 1 || vs[0].Resolution != "2160p" {
		t.Errorf("excluding by size = %+v, want only the 2160p", vs)
	}
	if vs := lib.OtherVersions("Dune 2021 1080p", "", 0); len(vs) != 0 {
		t.Errorf("a work the cluster doesn't have = %+v, want none", vs)
	}
}
