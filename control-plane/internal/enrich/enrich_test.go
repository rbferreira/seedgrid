package enrich

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/googlebooks"
	"github.com/rbferreira/seedgrid/control-plane/internal/igdb"
	"github.com/rbferreira/seedgrid/control-plane/internal/openlibrary"
	"github.com/rbferreira/seedgrid/control-plane/internal/tmdb"
)

func TestDetectMediaType(t *testing.T) {
	cases := []struct {
		cat, typ, name string
		want           MediaType
	}{
		{"Filmes", "WEB-DL", "Inception 2010 1080p", MediaTypeMovie},
		{"Movies", "BluRay", "", MediaTypeMovie},
		{"Séries", "HDTV", "", MediaTypeTV},
		{"TV Shows", "", "", MediaTypeTV},
		{"Games", "PC", "", MediaTypeGame},
		{"Jogos", "PlayStation 5", "", MediaTypeGame},
		{"E-book", "", "", MediaTypeBook},
		{"Livros", "epub", "", MediaTypeBook},
		{"Audiobook", "", "", MediaTypeAudioBook},
		{"Audiolivro", "", "", MediaTypeAudioBook},
		// Anime/cartoon/reality/soap opera/documentary → TV (common Brazilian tracker categories).
		{"Animes", "WEB-DL", "Some Anime 2020", MediaTypeTV},
		{"Desenhos", "", "", MediaTypeTV},
		{"Documentários", "", "", MediaTypeTV},
		{"Reality Show", "", "", MediaTypeTV},
		{"Novelas", "", "", MediaTypeTV},
		// Hard rule: S\d+ in the name forces TV even with an ambiguous category (Animes for
		// movie+series; any XXX). This is the case of "Fighting Spirit S03 …" which fell
		// into Others.
		{"Animes", "Encode", "Fighting Spirit S03 1080p NF WEB-DL AAC2.0 H.264-BiOMA", MediaTypeTV},
		{"Animes", "Encode", "Grimgar Ashes and Illusions S01 1080p BluRay", MediaTypeTV},
		{"Whatever", "", "Show Name S05E12 1080p", MediaTypeTV},
		{"Whatever", "", "Show Name Season 3 1080p", MediaTypeTV},
		// No match — falls into Others.
		{"XXX", "1080p", "", MediaTypeOther},
		{"", "", "", MediaTypeOther},
	}
	for _, tc := range cases {
		if got := DetectMediaType(tc.cat, tc.typ, tc.name); got != tc.want {
			t.Errorf("DetectMediaType(%q,%q,%q) = %s, want %s", tc.cat, tc.typ, tc.name, got, tc.want)
		}
	}
}

func TestEnrichBatchNil(t *testing.T) {
	// Nil service = only returns MediaType (the UI can still show the chip).
	var s *Service
	got := s.EnrichBatch(context.Background(), []Item{
		{Category: "Filmes", Type: "WEB-DL", TMDB: 27205},
		{Category: "Games", Type: "PC"},
	})
	if len(got) != 2 || got[0].MediaType != MediaTypeMovie || got[1].MediaType != MediaTypeGame {
		t.Errorf("Enrich nil = %+v", got)
	}
}

func TestEnrichMovieUsesCache(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte(`{"id":27205,"title":"A Origem","original_title":"Inception","overview":"Cobb...","poster_path":"/x.jpg","release_date":"2010-07-15","original_language":"en","genres":[{"id":28,"name":"Ação"}]}`))
	}))
	defer srv.Close()
	tc := tmdb.NewClient(func() string { return "k" }).WithBase(srv.URL)
	s := New(tc, nil, nil, nil, 4)
	items := []Item{
		{Category: "Filmes", Type: "WEB-DL", TMDB: 27205},
		{Category: "Filmes", Type: "WEB-DL", TMDB: 27205}, // equal → cache
	}
	got := s.EnrichBatch(context.Background(), items)
	if got[0].Title != "A Origem" {
		t.Errorf("[0] title = %q", got[0].Title)
	}
	if got[1].Title != "A Origem" {
		t.Errorf("[1] title = %q", got[1].Title)
	}
	// Note: both concurrent calls may go through the initial cache-miss
	// (there is no singleflight). What matters is that a 3rd call NOW uses the cache.
	got2 := s.EnrichBatch(context.Background(), []Item{{Category: "Filmes", Type: "WEB-DL", TMDB: 27205}})
	if got2[0].Title != "A Origem" {
		t.Errorf("[cache] title = %q", got2[0].Title)
	}
	// hits must be <= 2 (never 3 — the 3rd one hits the cache).
	if h := atomic.LoadInt32(&hits); h > 2 {
		t.Errorf("hits = %d, want <= 2 (3rd should fetch from cache)", h)
	}
}

func TestEnrichSilentFailure(t *testing.T) {
	// Disabled provider (empty key) → base Meta, without breaking the batch.
	tc := tmdb.NewClient(func() string { return "" })
	s := New(tc, nil, nil, nil, 4)
	got := s.EnrichBatch(context.Background(), []Item{{Category: "Filmes", Type: "WEB-DL", TMDB: 27205}})
	if got[0].MediaType != MediaTypeMovie {
		t.Errorf("mediatype = %s", got[0].MediaType)
	}
	if got[0].Title != "" || got[0].PosterURL != "" {
		t.Errorf("with provider disabled, should come back empty: %+v", got[0])
	}
}

func TestBookMeta(t *testing.T) {
	cases := []struct {
		in                    string
		wantTitle, wantAuthor string
		wantYear              int
	}{
		// Title - Author [YYYY] [AUDIOBOOK]
		{"O Alfaiate Valentão - Irmãos Grimm [2020] [AUDIOBOOK]", "O Alfaiate Valentão", "Irmãos Grimm", 2020},
		{"Harry Potter e a Câmara Secreta - J.K. Rowling [2023] [AUDIOBOOK]", "Harry Potter e a Câmara Secreta", "J.K. Rowling", 2023},
		// Author after the LAST " - " (title may contain " - " inside parentheses)
		// Parentheses now COME OUT of the title (#108): with them inside, the Open
		// Library search failed; and since they come out before the split, the first " - " is already the author's.
		{"MEU CAMISA 10 (Jogos Indecentes - Livro 1) - Alessandra Ferrer [2024]", "MEU CAMISA 10", "Alessandra Ferrer", 2024},
		// Year in parentheses, no author
		{"Dom Casmurro (1899)", "Dom Casmurro", "", 1899},
		// Title only, no year
		{"Alguma Coisa Solta", "Alguma Coisa Solta", "", 0},
		// Multiple tags to strip
		{"Coleção de Livros sobre Go - Vários Autores [2021] [PACK] [INGLÊS E PORTUGUÊS]", "Coleção de Livros sobre Go", "Vários Autores", 2021},
		// [ENG] was not in the keyword list and polluted the AUTHOR — the cause of #108.
		{"Something Rad - Nikki Witt [2023] [ENG]", "Something Rad", "Nikki Witt", 2023},
		{"Monster Core 3 - King [2023] [ENG]", "Monster Core 3", "King", 2023},
		// Arbitrary bracket in the title (Japanese edition) also comes out.
		{"Monster [Kansenban] [PACK] - Naoki Urasawa", "Monster", "Naoki Urasawa", 0},
		// Author list: keeps the first one; "(Second Edition)" comes out with the parentheses.
		{"A Global History Of Architecture (Second Edition) Francis D.K. Ching - Mark Jarzombek - Vikramaditya Prakash", "A Global History Of Architecture Francis D.K. Ching", "Mark Jarzombek", 0},
		// Subtitle after " - " would go into the author's slot with the LAST split.
		{"An Apothecary Afield: The Silverwort Harvest - A Cozy Fantasy In a Small Village - Francis Kay [2026] [ENG]", "An Apothecary Afield: The Silverwort Harvest", "A Cozy Fantasy In a Small Village", 2026},
	}
	for _, tc := range cases {
		gt, ga, gy := bookMeta(tc.in)
		if gt != tc.wantTitle || ga != tc.wantAuthor || gy != tc.wantYear {
			t.Errorf("bookMeta(%q) = %q/%q/%d, want %q/%q/%d", tc.in, gt, ga, gy, tc.wantTitle, tc.wantAuthor, tc.wantYear)
		}
	}
}

func TestEnrichBook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "author=Andrew") {
			t.Errorf("missing author in search: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"docs":[{"title":"The Pragmatic Programmer","author_name":["Andrew Hunt"],"first_publish_year":1999,"cover_i":123,"language":["eng"]}]}`))
	}))
	defer srv.Close()
	ol := openlibrary.NewClient().WithBase(srv.URL)
	s := New(nil, nil, ol, nil, 2)
	got := s.EnrichBatch(context.Background(), []Item{
		{Category: "E-book", Name: "The Pragmatic Programmer - Andrew Hunt (1999).epub", Year: 1999},
	})
	if got[0].MediaType != MediaTypeBook {
		t.Errorf("mediatype = %s", got[0].MediaType)
	}
	if got[0].Title != "The Pragmatic Programmer" {
		t.Errorf("title = %q", got[0].Title)
	}
	if got[0].OriginalName != "Andrew Hunt" {
		t.Errorf("author (em OriginalName) = %q", got[0].OriginalName)
	}
	if !strings.Contains(got[0].PosterURL, "123") {
		t.Errorf("cover = %q", got[0].PosterURL)
	}
}

// TestGameTitleFromName uses REAL game release names from a UNIT3D site (#106) — the
// case that motivated the fix: none of them found a cover because the term went nearly raw
// to IGDB. Every expected title here was verified against the IGDB API in fact.
func TestGameTitleFromName(t *testing.T) {
	cases := []struct{ in, want string }{
		// build id + concatenated group + scene tags
		{"Mortal.Shell.II.Build.24772279-CL92086 [MULTi15] [REPACK] [KaOs]", "Mortal Shell II"},
		{"Assassins.Creed.II.Build.1843459 [MULTi11] [REPACK] [KaOs]", "Assassins Creed II"},
		{"Tales of Arise Build 12823607 [Full Game] [SquishyFox]", "Tales of Arise"},
		// version in multiple formats
		{"Resident Evil - Requiem Deluxe Edition v.1.1.2 Build 22277314 [MULTI] [DLC's] [REPACK] [Decepticon]", "Resident Evil - Requiem"},
		{"Ratchet & Clank: Rift Apart v. 2.618.0.0 [ENG / MULTI] [DLC] [REPACK] [Decepticon]", "Ratchet & Clank: Rift Apart"},
		{"Gothic.1.Remake.v171864 [REPACK] [MULTI10] [KaOs]", "Gothic 1 Remake"},
		{"007.First.Light.v1.1.0.REPACK-KaOs", "007 First Light"},
		{"Returnal v.1.0 11083317 [DLC]  [ENG / MULTI] | [RePack] [SeleZen]", "Returnal"},
		// trailing number that IS the title must not be dropped
		{"The.Sinking.City.2 [MULTI15] [REPACK] [KaOs]", "The Sinking City 2"},
		{"Marvel’s Spider-Man 2 : Digital Deluxe Edition v2.727 [All DLCs] [Bonus PSN Items] [MULTi26] [DODI Repack]", "Marvel's Spider-Man 2"},
		// tail of loose numbers (compilation) is dropped entirely
		{"Ratchet.&.Clank.1.2.3.PS2.for.PC. [REPACK] [KaOs]", "Ratchet & Clank"},
		// group with no tag at all, and two-part group
		{"Duskfade-RUNE", "Duskfade"},
		{"MARVEL Tokon Fighting Souls INTERNAL-RUNE", "MARVEL Tokon Fighting Souls"},
		// year in the middle + group after
		{"Lifted v1.2.8813 2026 - ARM [MULTI]", "Lifted"},
		{"Cheap Car Repair v1.3.0030 2026 - TENOKE [MULTI]", "Cheap Car Repair"},
		{"Assassin's Creed Black Flag Resynced v1.06 2026 - voices38 [MULTI] [+DLC]", "Assassin's Creed Black Flag Resynced"},
		// colon and "GOG Full" at the end
		{"A Plague Tale: Requiem v.20230706 [MULTI5] GOG Full", "A Plague Tale: Requiem"},
		// clean title remains intact
		{"Hollow Knight Silksong", "Hollow Knight Silksong"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := gameTitleFromName(tc.in); got != tc.want {
			t.Errorf("gameTitleFromName(%q)\n  = %q\nwant %q", tc.in, got, tc.want)
		}
	}
}

// TestPickGame covers the secondary bug in #106: the IGDB search is fuzzy and the first
// result is sometimes a different game. A wrong cover is worse than no cover.
func TestPickGame(t *testing.T) {
	g := func(n string) igdb.Game { return igdb.Game{Name: n} }

	// Real case: searching for "Returnal" brought up "Return or No Return" first.
	if got, ok := pickGame([]igdb.Game{g("Return or No Return"), g("Returnal")}, "Returnal"); !ok || got.Name != "Returnal" {
		t.Errorf("exact should beat 1st in list, got %q (ok=%v)", got.Name, ok)
	}
	// No relation at all: no poster (it was "Lifted" → "The Lift").
	if got, ok := pickGame([]igdb.Game{g("The Lift")}, "Lifted"); ok {
		t.Errorf("should not match without common token, got %q", got.Name)
	}
	// Prefix is valid (extra edition/subtitle in the IGDB catalog).
	if got, ok := pickGame([]igdb.Game{g("Gothic 1 Remake Deluxe")}, "Gothic 1 Remake"); !ok || got.Name != "Gothic 1 Remake Deluxe" {
		t.Errorf("prefix should match, got %q (ok=%v)", got.Name, ok)
	}
	// Common token is the last resort (compilation).
	if _, ok := pickGame([]igdb.Game{g("Ratchet & Clank Collection")}, "Ratchet & Clank"); !ok {
		t.Error("common token should match")
	}
	// Difference only in stop word and punctuation = exact match.
	if _, ok := pickGame([]igdb.Game{g("Assassin's Creed II")}, "Assassins Creed II"); !ok {
		t.Error("punctuation/apostrophe should not prevent matching")
	}
	// Empty list does not panic.
	if _, ok := pickGame(nil, "Qualquer Coisa"); ok {
		t.Error("empty list should not match")
	}
}

// TestTitleMatches and TestAuthorMatches cover the identity guard (#106/#108): what
// separates "the provider found the item" from "the provider found SOMETHING".
func TestTitleMatches(t *testing.T) {
	cases := []struct {
		query, cand string
		want        bool
	}{
		{"Mortal Shell II", "Mortal Shell II", true},
		{"Assassins Creed II", "Assassin's Creed II", true}, // punctuation does not count
		{"Gothic 1 Remake", "Gothic 1 Remake Deluxe", true}, // prefix at word boundary
		{"Ratchet & Clank", "Ratchet & Clank Collection", true},
		{"Lifted", "The Lift", false}, // loose prefix would match — it must not
		{"Returnal", "Return or No Return", false},
		{"Night Songs", "Song of Silver, Flame Like Night", false},
		{"Monster Core 3", "GameShark", false},
		{"", "Qualquer", false},
	}
	for _, c := range cases {
		if got := titleMatches(c.query, c.cand); got != c.want {
			t.Errorf("titleMatches(%q, %q) = %v, want %v", c.query, c.cand, got, c.want)
		}
	}
}

func TestAuthorMatches(t *testing.T) {
	cases := []struct {
		want, got string
		ok        bool
	}{
		{"Brinda Charry", "Titania Hardie", false}, // the homonym from #108
		{"Alli Dyer", "Charles L. Grant", false},
		{"Patricia Potter", "Patricia Potter", true},
		{"J.K. Rowling", "Rowling, J. K.", true}, // OL varies the name format
		{"Dante King", "Dante King et al.", true},
		{"Fiona Baker", "", false}, // candidate without author confirms nothing
		{"", "Someone", false},
	}
	for _, c := range cases {
		if got := authorMatches(c.want, c.got); got != c.ok {
			t.Errorf("authorMatches(%q, %q) = %v, want %v", c.want, c.got, got, c.ok)
		}
	}
}

// TestEnrichBookCascade covers #110: when Open Library returns a HOMONYM (same
// title, different author), the guard rejects it and the search continues on Google Books, which has
// the right edition. This is the case for PT-BR collections and recent releases.
func TestEnrichBookCascade(t *testing.T) {
	var olHits, gbHits int32
	ol := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&olHits, 1)
		// Homonym: same title, DIFFERENT author — must be rejected.
		_, _ = w.Write([]byte(`{"docs":[{"title":"Hocus Pocus","author_name":["Titania Hardie"],
			"first_publish_year":1996,"cover_i":42,"language":["eng"]}]}`))
	}))
	defer ol.Close()
	gb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&gbHits, 1)
		_, _ = w.Write([]byte(`{"items":[{"volumeInfo":{"title":"Hocus Pocus",
			"authors":["Brinda Charry"],"publishedDate":"2026-02",
			"categories":["Fiction"],"language":"en",
			"imageLinks":{"thumbnail":"http://books.google.com/capa.jpg"}}}]}`))
	}))
	defer gb.Close()

	svc := New(nil, nil, openlibrary.NewClient().WithBase(ol.URL), googlebooks.NewClient(nil).WithBase(gb.URL), 2)
	results := svc.EnrichBatch(context.Background(), []Item{
		{Category: "Livros", Name: "Hocus Pocus - Brinda Charry [2026] [ENG]"},
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 meta, got %d", len(results))
	}
	m := results[0]
	if m.PosterURL != "https://books.google.com/capa.jpg" {
		t.Errorf("should use Google Books cover (and force https), got %q", m.PosterURL)
	}
	if olHits == 0 {
		t.Error("Open Library should be tried FIRST")
	}
	if gbHits == 0 {
		t.Error("Google Books should be tried after homonym rejection")
	}
}

// TestEnrichBookOpenLibraryWins: when Open Library confirms title AND author,
// Google Books should not even be queried (open provider first, without spending quota).
func TestEnrichBookOpenLibraryWins(t *testing.T) {
	var gbHits int32
	ol := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"docs":[{"title":"Monster Core 2","author_name":["Dante King"],
			"first_publish_year":2019,"cover_i":7,"language":["eng"]}]}`))
	}))
	defer ol.Close()
	gb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&gbHits, 1)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer gb.Close()

	svc := New(nil, nil, openlibrary.NewClient().WithBase(ol.URL), googlebooks.NewClient(nil).WithBase(gb.URL), 2)
	m := svc.EnrichBatch(context.Background(), []Item{
		{Category: "Livros", Name: "Monster Core 2 - Dante King [2019] [ENG]"},
	})[0]
	if m.PosterURL == "" {
		t.Error("Open Library should have resolved")
	}
	if gbHits != 0 {
		t.Errorf("Google Books should not be queried (%d calls)", gbHits)
	}
}

// TestEnrichBookWithoutCoverDoesNotInterrupt: Open Library confirms the book but does NOT have a cover;
// the cascade must continue to Google Books instead of stopping there. This was the bug that
// left "Night Songs" (Alli Dyer, 2026) without a poster even with the cover available (#110).
func TestEnrichBookWithoutCoverDoesNotInterrupt(t *testing.T) {
	ol := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// right author, right title, NO cover_i
		_, _ = w.Write([]byte(`{"docs":[{"title":"Night Songs","author_name":["Alli Dyer"],
			"first_publish_year":2026,"language":["eng"]}]}`))
	}))
	defer ol.Close()
	gb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"items":[{"volumeInfo":{"title":"Night Songs",
			"authors":["Alli Dyer"],"publishedDate":"2026-08-11",
			"imageLinks":{"thumbnail":"http://books.google.com/ns.jpg"}}}]}`))
	}))
	defer gb.Close()

	svc := New(nil, nil, openlibrary.NewClient().WithBase(ol.URL), googlebooks.NewClient(nil).WithBase(gb.URL), 2)
	m := svc.EnrichBatch(context.Background(), []Item{
		{Category: "Livros", Name: "Night Songs - Alli Dyer [2026] [ENG]"},
	})[0]
	if m.PosterURL != "https://books.google.com/ns.jpg" {
		t.Errorf("should have proceeded to Google Books; got %q", m.PosterURL)
	}
}

// The same work in two content languages is two cache entries (#74): a pt-BR site and an
// English one browsing the same title must not share the synopsis.
func TestCacheKeySeparatesLanguages(t *testing.T) {
	pt := cacheKey(MediaTypeMovie, Item{TMDB: 27205, Lang: "pt-BR"})
	en := cacheKey(MediaTypeMovie, Item{TMDB: 27205, Lang: "en"})
	if pt == "" || en == "" || pt == en {
		t.Errorf("cache keys pt=%q en=%q, want two different non-empty keys", pt, en)
	}
}
