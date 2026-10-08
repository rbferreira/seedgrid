package unit3d

import "testing"

func baseParams() Params {
	return Params{
		Name: "Idiocracy 2006 1080p BluRay 2.0 x264-NOGROUP", Description: "synopsis",
		CategoryID: 1, TypeID: 3, ResolutionID: 3, TMDB: 123, IMDB: "tt0387808",
	}
}

// The token only freezes something if it's stable: Go maps have no iteration order,
// and without sorting keys the fingerprint would change between identical calls — the submit
// would reject legitimate submissions.
func TestFingerprint_StableBetweenCalls(t *testing.T) {
	p, data := baseParams(), []byte("d8:announce…e")
	first := Fingerprint(p, "x.torrent", data)
	for i := 0; i < 50; i++ {
		if got := Fingerprint(p, "x.torrent", data); got != first {
			t.Fatalf("unstable fingerprint on iteration %d: %q != %q", i, got, first)
		}
	}
}

// Every field in the form must change the fingerprint. An uncounted field would be a field
// that the operator can change after the preview without the submit noticing.
func TestFingerprint_ChangesWithEachField(t *testing.T) {
	data := []byte("torrent-bytes")
	base := Fingerprint(baseParams(), "x.torrent", data)

	for _, tc := range []struct {
		name string
		mute func(*Params)
	}{
		{"name", func(p *Params) { p.Name = "other name" }},
		{"description", func(p *Params) { p.Description = "other synopsis" }},
		{"category_id", func(p *Params) { p.CategoryID = 2 }},
		{"type_id", func(p *Params) { p.TypeID = 4 }},
		{"resolution_id", func(p *Params) { p.ResolutionID = 2 }},
		{"tmdb", func(p *Params) { p.TMDB = 999 }},
		{"imdb", func(p *Params) { p.IMDB = "tt9999999" }},
		{"tvdb", func(p *Params) { p.TVDB = 42 }},
		{"mal", func(p *Params) { p.MAL = 42 }},
		{"igdb", func(p *Params) { p.IGDB = 42 }},
		{"season", func(p *Params) { p.Season = 1 }},
		{"episode", func(p *Params) { p.Episode = 1 }},
		{"keywords", func(p *Params) { p.Keywords = "comedy" }},
		{"mediainfo", func(p *Params) { p.MediaInfo = "General…" }},
		{"anonymous", func(p *Params) { p.Anonymous = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := baseParams()
			tc.mute(&p)
			if got := Fingerprint(p, "x.torrent", data); got == base {
				t.Errorf("changing %s did not change the fingerprint — submit would accept an unverified record", tc.name)
			}
		})
	}
}

// Changing the file while keeping the form identical would publish OTHER content under the same
// human review. The fingerprint covers the .torrent for this reason.
func TestFingerprint_CoversTorrent(t *testing.T) {
	p := baseParams()
	base := Fingerprint(p, "x.torrent", []byte("content-a"))

	if Fingerprint(p, "x.torrent", []byte("content-b")) == base {
		t.Error("different .torrent bytes did not change the fingerprint")
	}
	if Fingerprint(p, "other.torrent", []byte("content-a")) == base {
		t.Error("different .torrent name did not change the fingerprint")
	}
}

// Fields is the only field assembly: the preview reads from it and so does the Upload. If someone
// creates a second assembly, the preview could lie again.
func TestFields_OptionalFieldsOnlyWithValue(t *testing.T) {
	f := Fields(baseParams())
	for _, k := range []string{"season_number", "episode_number", "keywords", "mediainfo"} {
		if _, ok := f[k]; ok {
			t.Errorf("%s was empty in payload", k)
		}
	}
	// Required fields are always included, even zeroed ones that the tracker accepts as 0.
	for _, k := range []string{"name", "description", "category_id", "type_id", "tmdb", "imdb", "tvdb", "mal", "igdb", "anonymous"} {
		if _, ok := f[k]; !ok {
			t.Errorf("%s missing from payload", k)
		}
	}
	// resolution_id is optional, but it's populated in baseParams.
	if f["resolution_id"] != "3" {
		t.Errorf("resolution_id = %q, want \"3\"", f["resolution_id"])
	}
	// IMDB goes numeric: without the "tt" and without leading zeros (the UNIT3D field is
	// an integer). "tt0387808" → "387808".
	if f["imdb"] != "387808" {
		t.Errorf("imdb = %q, want \"387808\" (without tt and without leading zeros)", f["imdb"])
	}
}
