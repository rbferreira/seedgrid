package release

import (
	"strings"
	"testing"
)

func TestParseMovie(t *testing.T) {
	got := Parse("Inception.2010.1080p.BluRay.x264-GROUP")
	if got.Title != "Inception" {
		t.Errorf("title = %q, want Inception", got.Title)
	}
	if got.Year != 2010 {
		t.Errorf("year = %d, want 2010", got.Year)
	}
	if got.IsTV {
		t.Errorf("IsTV = true, want movie")
	}
	if got.Resolution != "1080p" {
		t.Errorf("res = %q, want 1080p", got.Resolution)
	}
	if got.Type != "Encode" {
		t.Errorf("type = %q, want Encode", got.Type)
	}
	if got.Group != "GROUP" {
		t.Errorf("group = %q, want GROUP", got.Group)
	}
}

func TestParseWebDL2160(t *testing.T) {
	got := Parse("Some Movie 2023 2160p AMZN WEB-DL DDP5.1 x265-ABC")
	if got.Title != "Some Movie" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Year != 2023 {
		t.Errorf("year = %d", got.Year)
	}
	if got.Resolution != "2160p" {
		t.Errorf("res = %q, want 2160p", got.Resolution)
	}
	// WEB-DL must take precedence over x265 (Encode).
	if got.Type != "WEB-DL" {
		t.Errorf("type = %q, want WEB-DL", got.Type)
	}
}

func TestParseTVEpisode(t *testing.T) {
	got := Parse("The.Show.S02E05.1080p.WEB-DL.x264-XYZ")
	if !got.IsTV {
		t.Fatalf("IsTV = false, want series")
	}
	if got.Season != 2 || got.Episode != 5 {
		t.Errorf("S/E = %d/%d, want 2/5", got.Season, got.Episode)
	}
	if got.Title != "The Show" {
		t.Errorf("title = %q, want 'The Show'", got.Title)
	}
}

func TestParseSeasonPack(t *testing.T) {
	got := Parse("The.Show.S03.1080p.WEB-DL-XYZ")
	if !got.IsTV || got.Season != 3 || got.Episode != 0 {
		t.Errorf("season pack: IsTV=%v season=%d ep=%d, want true/3/0", got.IsTV, got.Season, got.Episode)
	}
	if got.Title != "The Show" {
		t.Errorf("title = %q", got.Title)
	}
}

func TestParseRemuxPrecedence(t *testing.T) {
	got := Parse("Film.2019.2160p.UHD.BluRay.REMUX.HEVC-GRP")
	if got.Type != "Remux" {
		t.Errorf("type = %q, want Remux — remux takes precedence over bluray", got.Type)
	}
	if got.Resolution != "2160p" {
		t.Errorf("res = %q, want 2160p", got.Resolution)
	}
}

func TestParseSeasonWord(t *testing.T) {
	got := Parse("My Series Season 4 720p HDTV")
	if !got.IsTV || got.Season != 4 {
		t.Errorf("IsTV=%v season=%d, want true/4", got.IsTV, got.Season)
	}
	if got.Resolution != "720p" || got.Type != "HDTV" {
		t.Errorf("res/type = %q/%q, want 720p/HDTV", got.Resolution, got.Type)
	}
	if got.Title != "My Series" {
		t.Errorf("title = %q", got.Title)
	}
}

func TestParseRadarrStyle(t *testing.T) {
	got := Parse("Idiocracy (2006) {imdb-tt0387808} [Bluray-1080p][MP2 2.0][x264].mkv")
	if got.Title != "Idiocracy" {
		t.Errorf("title = %q, want Idiocracy (without parentheses/brackets)", got.Title)
	}
	if got.Year != 2006 {
		t.Errorf("year = %d, want 2006", got.Year)
	}
	if got.IMDB != "tt0387808" {
		t.Errorf("imdb = %q, want tt0387808 (extracted from name)", got.IMDB)
	}
	if got.Resolution != "1080p" {
		t.Errorf("res = %q, want 1080p", got.Resolution)
	}
	if got.Type != "Encode" {
		t.Errorf("type = %q, want Encode (Bluray x264)", got.Type)
	}
	if got.IsTV {
		t.Errorf("IsTV = true, want movie")
	}
}

// TestParseAKA: UNIT3D sites usually forbid alternative titles ("AKA ...") in the name — the parser
// must keep only the international title (before the AKA). Covers "AKA" and "a.k.a.".
func TestParseAKA(t *testing.T) {
	for _, name := range []string{
		"In Your Name AKA Em Teu Nome 2010 1080p AMZN WEB-DL DD+2.0 H.264",
		"In.Your.Name.aka.Em.Teu.Nome.2010.1080p.WEB-DL.H264",
		"In.Your.Name.a.k.a.Em.Teu.Nome.2010.1080p.WEB-DL.H264",
	} {
		got := Parse(name)
		if got.Title != "In Your Name" {
			t.Errorf("Parse(%q) title = %q, want 'In Your Name' (without AKA)", name, got.Title)
		}
		if got.Year != 2010 {
			t.Errorf("Parse(%q) year = %d, want 2010", name, got.Year)
		}
		if strings.Contains(strings.ToLower(got.StandardName), "aka") {
			t.Errorf("standard_name = %q should not contain 'aka'", got.StandardName)
		}
	}
	// "aka" in the middle of a word MUST NOT be cut (e.g.: Osaka).
	if got := Parse("Osaka Elegy 1936 1080p BluRay x264"); got.Title != "Osaka Elegy" {
		t.Errorf("title = %q, want 'Osaka Elegy' (do not cut 'aka' inside a word)", got.Title)
	}
}

// TestParseAltTitleDash: BR convention "Original Title - Translated Title" (without
// AKA marker) — same tracker rule, and regression for a real bug (upload postponed
// on a UNIT3D site: "Ender's Game - Jogo Do Exterminador 2013 1080p BDRip DUBLADO-NOGROUP",
// where the stray "-" survived in the standard_name).
func TestParseAltTitleDash(t *testing.T) {
	got := Parse("Ender's Game - Jogo Do Exterminador 2013 1080p BDRip DUBLADO-NOGROUP")
	if got.Title != "Ender's Game" {
		t.Errorf("title = %q, want 'Ender's Game' (no translated title after the hyphen)", got.Title)
	}
	if got.Year != 2013 {
		t.Errorf("year = %d, want 2013", got.Year)
	}
	if strings.Contains(got.StandardName, " - ") {
		t.Errorf("standard_name = %q should not contain a stray hyphen in the middle", got.StandardName)
	}
	// A hyphen without spaces is part of the title and must not be cut.
	if got := Parse("Spider-Man 2002 1080p BluRay x264"); got.Title != "Spider-Man" {
		t.Errorf("title = %q, want 'Spider-Man' (do not split on an attached hyphen)", got.Title)
	}
}

func TestParseUnknown(t *testing.T) {
	got := Parse("random_file_without_tags")
	if got.Title != "random file without tags" {
		t.Errorf("title = %q", got.Title)
	}
	if got.Year != 0 || got.Resolution != "" || got.Type != "" || got.IsTV {
		t.Errorf("unknown fields should be empty (and movie): %+v", got)
	}
}

// The part marker must survive the alternative title cut. `reAltDash`
// exists for the tracker rule ("Original - Translated" → international only) and was swallowing
// "- Phase 2" along with it, collapsing two works into a single title (#172).
func TestParse_Part(t *testing.T) {
	cases := []struct {
		name      string
		wantTitle string
		wantPart  string
	}{
		{"A Saga do Batman - Fase 1", "A Saga do Batman", "Fase 1"},
		{"A Saga do Batman - Fase 2", "A Saga do Batman", "Fase 2"},
		{"Duna - Parte 2 (2024) 1080p", "Duna", "Parte 2"},
		{"Sandman - Vol 3", "Sandman", "Vol 3"},
		{"Sandman - Volume 10", "Sandman", "Volume 10"},
		{"O Senhor dos Anéis - Livro 2", "O Senhor dos Anéis", "Livro 2"},
		{"Alguma Coisa - Capitulo 4", "Alguma Coisa", "Capitulo 4"},
		{"Alguma Coisa - Cap. 5", "Alguma Coisa", "Cap 5"},
		// The translated title is still truncated without becoming a part: it has no number.
		{"In Your Name - Em Teu Nome", "In Your Name", ""},
		{"Interstellar (2014) 1080p", "Interstellar", ""},
		// "Captain" is not "Cap N": the marker requires a number right after.
		{"Capitão America - O Retorno", "Capitão America", ""},
	}
	for _, c := range cases {
		got := Parse(c.name)
		if got.Title != c.wantTitle {
			t.Errorf("Parse(%q).Title = %q, want %q", c.name, got.Title, c.wantTitle)
		}
		if got.Part != c.wantPart {
			t.Errorf("Parse(%q).Part = %q, want %q", c.name, got.Part, c.wantPart)
		}
	}
}
