// Package enrich orchestrates metadata enrichment for the browse catalog
// (#12) — UNIT3D only returns name/category/size/seeders on browse; the
// premium UI needs poster, translated title, genres, and language. This package
// chooses the provider by MEDIA TYPE (movie/TV → TMDB, game → IGDB, book/
// audiobook → Open Library) and caches for 24h to avoid hitting the API on every scroll.
//
// Provider choice:
//   - MediaTypeMovie/MediaTypeTV → tmdb.Details(tmdb_id, isTV)
//   - MediaTypeGame              → igdb.Details(igdb_id) or Search(name) as fallback
//   - MediaTypeBook/AudioBook    → openlibrary.Search(title, author, year)
//   - MediaTypeOther             → no enrichment (UI shows only release name)
//
// Silent failure: error/timeout/disabled-provider returns empty Meta{}; the UI
// has fallbacks (poster placeholder, type chip, release name instead of PT).
package enrich

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/googlebooks"
	"github.com/rbferreira/seedgrid/control-plane/internal/igdb"
	"github.com/rbferreira/seedgrid/control-plane/internal/openlibrary"
	"github.com/rbferreira/seedgrid/control-plane/internal/tmdb"
)

// MediaType classifies a catalog item. The UI chooses the icon, chip color, and
// poster layout based on this. "other" covers what we cannot identify
// (custom tracker category, misc, XXX).
type MediaType string

const (
	MediaTypeMovie     MediaType = "movie"
	MediaTypeTV        MediaType = "tv"
	MediaTypeGame      MediaType = "game"
	MediaTypeBook      MediaType = "book"
	MediaTypeAudioBook MediaType = "audiobook"
	MediaTypeOther     MediaType = "other"
)

// Meta is the enrichment result — all fields are optional (the
// handler merges it into BrowseItem and the UI handles each absence with a local fallback).
type Meta struct {
	MediaType    MediaType `json:"media_type"`
	Title        string    `json:"title,omitempty"`         // translated/local (pt-BR when available)
	OriginalName string    `json:"original_name,omitempty"` // language/original name
	PosterURL    string    `json:"poster_url,omitempty"`
	Overview     string    `json:"overview,omitempty"`
	Genres       []string  `json:"genres,omitempty"`
	OrigLanguage string    `json:"original_language,omitempty"` // "en", "ja", "pt", …
}

// reSeasonPattern: release name with S05 / S01E12 / Season 3 → it's a TV episode.
// Hard rule (if it matches here, forces TV even with ambiguous category like "Animes").
var reSeasonPattern = regexp.MustCompile(`(?i)\b(S\d{1,2}(E\d{1,3})?|Season[\s._-]?\d{1,2})\b`)

// DetectMediaType maps (category, type, releaseName) from UNIT3D to MediaType.
// Based on keywords (each tracker names them as they want: "Filmes", "Movies", "Movie") +
// hard rule by release name pattern (S01E12 is always TV).
//
// Order matters: audiobook before book (contains "book"); TV series before anime (anime
// can be movie OR TV — if it has S\d+ in the name it was already caught by the hard rule);
// book before "cinema" (otherwise "Cinema Silvestre" falls into movie).
//
// Common categories from a Brazilian tracker (measured in Aug/2026): Filmes, Séries, Animes, Desenhos,
// Documentários, Reality, Novelas, Games, Livros, AudioBooks, Curso, Kids, Músicas.
// Anime/cartoon/documentary/reality/soap opera → TV; ambiguous course/kids → falls to default.
func DetectMediaType(category, typeStr, releaseName string) MediaType {
	// Hard rule: name with S\d+/Season pattern is always TV. Covers the case where a site
	// classifies "Fighting Spirit S03 …" as "Animes" — ambiguous category, but the
	// name leaves no doubt it's a TV episode.
	if reSeasonPattern.MatchString(releaseName) {
		return MediaTypeTV
	}
	s := strings.ToLower(strings.TrimSpace(category + " " + typeStr))
	switch {
	case containsAny(s, "audiobook", "audiolivro", "audio book", "áudio-livro"):
		return MediaTypeAudioBook
	case containsAny(s, "book", "livro", "ebook", "e-book", "epub", "revista", "magazine", "comic", "quadrinho", "manga", "mangá"):
		return MediaTypeBook
	case containsAny(s, "game", "jogo", "console", "pc game", "playstation", "xbox", "nintendo", "switch", "rom"):
		return MediaTypeGame
	case containsAny(s, "serie", "series", "série", "tv show", "shows", "temporada", "season",
		"anime", "desenho", "cartoon", "document", "reality", "novela", "variety"):
		return MediaTypeTV
	case containsAny(s, "movie", "filme", "cinema", "film "):
		return MediaTypeMovie
	default:
		return MediaTypeOther
	}
}

func containsAny(hay string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

// Item is the minimum the Service needs to know about each catalog torrent. It's a
// local contract (doesn't pull the entire unit3d.BrowseItem) — fields were chosen
// to cover the providers without coupling to the handler's structs.
type Item struct {
	Category string
	Type     string
	Name     string // release name (used as fallback and to extract title/author for books)
	Year     int
	TMDB     int // UNIT3D tmdb_id (0 when missing)
	IGDB     int // rarely comes from UNIT3D — Search is the fallback
	// Lang is the site's content language (#74, the description section of its profile): TMDB
	// answers in it. Empty is English.
	Lang string
}

// Service orchestrates the clients and maintains a cache. Concurrency limited by
// semaphore prevents bursts of 50 parallel requests on scroll.
type Service struct {
	tmdb    *tmdb.Client
	igdb    *igdb.Client
	openlib *openlibrary.Client
	gbooks  *googlebooks.Client

	cache       sync.Map      // key → cacheEntry
	ttl         time.Duration // cache TTL (24h by default)
	perTimeout  time.Duration // item timeout (movie/TV/game: one external call)
	bookTimeout time.Duration // BOOK item timeout: cascade can make up to 4
	without     chan struct{} // concurrency limit
}

type cacheEntry struct {
	meta Meta
	at   time.Time
}

// New creates the service. Pass nil for what you don't have (Service silently skips).
// Concurrency=0 disables the limit; 8 is a good default (TMDB accepts much more, but
// we handle the entire page in ~1s).
func New(t *tmdb.Client, i *igdb.Client, ol *openlibrary.Client, gb *googlebooks.Client, concurrency int) *Service {
	if concurrency <= 0 {
		concurrency = 8
	}
	s := &Service{
		tmdb:       t,
		igdb:       i,
		openlib:    ol,
		gbooks:     gb,
		ttl:        24 * time.Hour,
		perTimeout: 5 * time.Second,
		// Book has a longer timeout because the cascade is up to 4 searches (2 providers × with/without
		// author), and each can retry on 429/503. With the same 5s as other types, the
		// last attempt — precisely the one that resolves the PT-BR catalog and recent
		// releases — would die at the deadline under the browse burst.
		bookTimeout: 15 * time.Second,
		without:     make(chan struct{}, concurrency),
	}
	return s
}

// EnrichBatch triggers parallel enrichment of items and returns, in the same
// order, the corresponding Meta values (result position i = input item i).
// An error in ONE item never aborts the batch — empty Meta means "no enrichment".
func (s *Service) EnrichBatch(ctx context.Context, items []Item) []Meta {
	out := make([]Meta, len(items))
	if s == nil || len(items) == 0 {
		// Without service, at least fills the media_type — UI uses it to choose the chip.
		for i, it := range items {
			out[i] = Meta{MediaType: DetectMediaType(it.Category, it.Type, it.Name)}
		}
		return out
	}
	var wg sync.WaitGroup
	for i, it := range items {
		wg.Add(1)
		go func(i int, it Item) {
			defer wg.Done()
			out[i] = s.enrichOne(ctx, it)
		}(i, it)
	}
	wg.Wait()
	return out
}

// enrichOne resolves ONE item: detects type, queries cache, calls the right provider
// with timeout. Never propagates errors — returns Meta{MediaType: …} in the worst case.
func (s *Service) enrichOne(ctx context.Context, it Item) Meta {
	mt := DetectMediaType(it.Category, it.Type, it.Name)
	base := Meta{MediaType: mt}
	key := cacheKey(mt, it)
	if key == "" {
		return base // no cacheable key (e.g., book without title) — only the type
	}
	if v, ok := s.cache.Load(key); ok {
		e := v.(cacheEntry)
		if time.Since(e.at) < s.ttl {
			return e.meta
		}
	}
	// Limits concurrency. Context canceled during wait: silently aborts.
	select {
	case s.without <- struct{}{}:
		defer func() { <-s.without }()
	case <-ctx.Done():
		return base
	}
	// Book/audiobook has its own timeout: provider cascade makes up to 4 searches, and the
	// timeout for other types (one call) was killing the last one — which resolves PT-BR and
	// recent releases.
	deadline := s.perTimeout
	if mt == MediaTypeBook || mt == MediaTypeAudioBook {
		deadline = s.bookTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	m := s.fetch(callCtx, mt, it)
	m.MediaType = mt // ensures the type even when the provider returns nothing
	// The caller's deadline ran out (the browse budget, #41): that isn't a provider failure.
	// Storing the empty Meta would leave the card without a poster for 24h; without storing it,
	// the next browse completes it. Only the item's own deadline counts as a cacheable failure.
	if ctx.Err() == nil {
		s.cache.Store(key, cacheEntry{meta: m, at: time.Now()})
	}
	return m
}

// fetch triggers the call to the correct provider. Failure returns as base Meta (the
// cache saves it anyway — prevents retry bursts when the provider is down).
func (s *Service) fetch(ctx context.Context, mt MediaType, it Item) Meta {
	base := Meta{MediaType: mt}
	switch mt {
	case MediaTypeMovie, MediaTypeTV:
		if s.tmdb == nil || !s.tmdb.Enabled() || it.TMDB <= 0 {
			return base
		}
		d, err := s.tmdb.Details(ctx, it.TMDB, mt == MediaTypeTV, it.Lang)
		if err != nil {
			return base
		}
		return Meta{
			MediaType:    mt,
			Title:        d.Title,
			OriginalName: d.OriginalName,
			PosterURL:    d.PosterURL,
			Overview:     d.Overview,
			Genres:       d.Genres,
			OrigLanguage: d.OrigLanguage,
		}

	case MediaTypeGame:
		if s.igdb == nil || !s.igdb.Enabled() {
			return base
		}
		// IGDB by ID when UNIT3D brought it; otherwise Search by title extracted from
		// release name, choosing the candidate by similarity (pickGame) instead of
		// the first on the list — the search is fuzzy and missed the game on short titles (#106).
		var g igdb.Game
		var err error
		if it.IGDB > 0 {
			g, err = s.igdb.Details(ctx, it.IGDB)
		} else {
			title := gameTitleFromName(it.Name)
			var list []igdb.Game
			list, err = s.igdb.Search(ctx, title, 5)
			if err == nil {
				var ok bool
				if g, ok = pickGame(list, title); !ok {
					err = igdb.ErrNotFound
				}
			}
		}
		if err != nil {
			return base
		}
		return Meta{
			MediaType: mt,
			Title:     g.Name,
			PosterURL: g.CoverURL,
			Overview:  g.Summary,
			Genres:    g.Genres,
		}

	case MediaTypeBook, MediaTypeAudioBook:
		if s.openlib == nil {
			return base
		}
		title, author, parsedYear := bookMeta(it.Name)
		// UNIT3D doesn't return release_year for book/audiobook — the year comes in the name
		// as [YYYY]. If the item came without year, we use the one parsed from the name.
		year := it.Year
		if year <= 0 {
			year = parsedYear
		}
		// Provider cascade (#110): Open Library first (open, no key) and
		// Google Books later, because OL doesn't index much of this catalog —
		// PT-BR children's audiobooks, recent releases, RPG zines. Each candidate
		// goes through the SAME identity guard; the first one to confirm, wins.
		b, ok := s.bookFromProviders(ctx, title, author, year)
		if !ok {
			return base
		}
		// We prefer the TITLE/AUTHOR we parse from the release (sites are
		// consistent: "Title - Author [Year]"). The one from Open Library sometimes comes
		// contaminated with publisher/subtitle glued together ("Fairy Tales Wish
		// PublisherBlocked for consignment") — the release name is cleaner. From
		// OL we only use cover, genres, and language (data we don't have locally).
		displayTitle := title
		if strings.TrimSpace(displayTitle) == "" {
			displayTitle = b.Title
		}
		displayAuthor := author
		if strings.TrimSpace(displayAuthor) == "" {
			displayAuthor = b.Author
		}
		return Meta{
			MediaType:    mt,
			Title:        displayTitle,
			OriginalName: displayAuthor, // book has no "original title" — we reuse the slot for the author
			PosterURL:    b.CoverURL,
			Genres:       b.Subjects,
			OrigLanguage: b.Language,
		}

	default:
		return base
	}
}

// cacheKey is the stable provider key for the item. Empty key = no caching.
func cacheKey(mt MediaType, it Item) string {
	switch mt {
	case MediaTypeMovie, MediaTypeTV:
		if it.TMDB <= 0 {
			return ""
		}
		return string(mt) + ":" + itoa(it.TMDB) + ":" + it.Lang
	case MediaTypeGame:
		if it.IGDB > 0 {
			return "game:" + itoa(it.IGDB)
		}
		if t := strings.TrimSpace(gameTitleFromName(it.Name)); t != "" {
			return "game:name:" + strings.ToLower(t)
		}
		return ""
	case MediaTypeBook, MediaTypeAudioBook:
		t, a, y := bookMeta(it.Name)
		if strings.TrimSpace(t) == "" {
			return ""
		}
		year := it.Year
		if year <= 0 {
			year = y
		}
		return string(mt) + ":" + strings.ToLower(t) + "|" + strings.ToLower(a) + "|" + itoa(year)
	default:
		return ""
	}
}

// Common scene groups in games (suffix "-GROUP" in the release name).
const sceneGroups = `RUNE|KaOs|TENOKE|CODEX|EMPRESS|SKIDROW|PLAZA|DODI|FLT|HOODLUM|` +
	`ElAmigos|FitGirl|Decepticon|SeleZen|SquishyFox|CPY|RELOADED|TiNYiSO|GOG`

var (
	// reGameTag: any [..] or (..) — in game names they are just scene metadata
	// ([MULTi15], [REPACK], [KaOs], [Full Game], [DLC's], [Bonus PSN Items]).
	reGameTag = regexp.MustCompile(`[\[\(][^\]\)]*[\]\)]`)

	// reGameGroup: group suffix at the end ("-KaOs", "-RUNE"). Matches a known group or
	// any ALLCAPS/numeric token — covers "-CL92086" and "INTERNAL-RUNE" (two
	// passes). Requires end of string to avoid breaking hyphenated titles ("Spider-Man").
	reGameGroup = regexp.MustCompile(`(?i)[-–]\s*(?:` + sceneGroups + `|[A-Z0-9]{2,})\s*$`)

	// reGameNoise marks where the TITLE ENDS and the release description begins. The cut is
	// at the first marker because everything after it is metadata, never a game name:
	// version, build, repack, languages, DLC, store, platform, year, build id.
	reGameNoise = regexp.MustCompile(`(?i)(?:[\s._-]|^)(?:` +
		`v\.?\s?\d` + // v1.2, v.1.0.6, v 2.618.0.0
		`|build\b|repack\b|internal\b|proper\b|update\b|crack\b|hotfix\b` +
		`|multi\d*\b|voices\d*\b|dlc|goty\b|gog\b|steam\b|epic\b|denuvo\b` +
		`|full[\s._-]*game\b|for[\s._-]*pc\b|ps[1-5]\b|xbox\b|switch\b|nsw\b` +
		`|edition\b|remaster(?:ed)?\b` +
		`|` + sceneGroups +
		`|\d{5,}` + // loose build id (24772279)
		`|(?:19|20)\d{2}(?:\b|$)` + // year
		`)`)

	// reGameEdition: edition qualifier left at the end after the cut at
	// "Edition" ("... Digital Deluxe" → "..."). Applied repeatedly.
	reGameEdition = regexp.MustCompile(`(?i)\s+(?:deluxe|digital|ultimate|gold|complete|` +
		`definitive|premium|standard|anniversary|bonus|directors?|cut)$`)

	reGameNum = regexp.MustCompile(`^\d+$`)
)

// gameTitleFromName extracts the game title from a scene release name, to search
// on IGDB. The release name describes the FILE ("Mortal.Shell.II.Build.24772279-CL92086
// [MULTi15] [REPACK] [KaOs]"); IGDB indexes the GAME ("Mortal Shell II"). Without this
// translation the search finds nothing — which was why no game in the catalog had a cover
// (#106): the term went in almost raw.
//
// The order matters: tags and group suffix are removed BEFORE the noise cut (otherwise the `[`
// hides the marker), and dots only become spaces AFTERWARDS (with dots, `.Build.` still
// matches `\bbuild\b`, but "v1.0" would become "v 1 0" and stop matching `v\.?\s?\d`).
func gameTitleFromName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}
	s = reGameTag.ReplaceAllString(s, " ")
	// Two passes resolve "INTERNAL-RUNE" (removes RUNE, then INTERNAL falls into noise).
	for i := 0; i < 3; i++ {
		next := strings.TrimSpace(reGameGroup.ReplaceAllString(s, ""))
		if next == s {
			break
		}
		s = next
	}
	if m := reGameNoise.FindStringIndex(s); m != nil {
		s = s[:m[0]]
	}
	s = strings.NewReplacer(".", " ", "_", " ", "’", "'").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.Trim(s, " -–|:,&")

	// Tail of loose numbers = collection or version ("Ratchet & Clank 1 2 3"), not
	// sequence. A single final number STAYS ("The Sinking City 2" is the title).
	toks := strings.Fields(s)
	cut := false
	for len(toks) >= 2 && reGameNum.MatchString(toks[len(toks)-1]) && reGameNum.MatchString(toks[len(toks)-2]) {
		toks = toks[:len(toks)-1]
		cut = true
	}
	if cut && len(toks) >= 2 && reGameNum.MatchString(toks[len(toks)-1]) {
		toks = toks[:len(toks)-1]
	}
	s = strings.Join(toks, " ")

	for i := 0; i < 3; i++ {
		next := strings.TrimSpace(reGameEdition.ReplaceAllString(s, ""))
		if next == s {
			break
		}
		s = next
	}
	return strings.Trim(s, " -–|:,")
}

// pickGame chooses among IGDB candidates instead of accepting the first one.
//
// IGDB search is fuzzy and, with short titles, the 1st result is often another game:
// "Returnal" returned "Return or No Return" and "Lifted" returned "The Lift" (#106). Since
// the cover is the most visible element in the catalog, a WRONG cover is worse than no cover —
// so we require some relation: identical name, prefix, or at least one significant
// token in common. Without intersection, it returns `false` and the UI stays on the placeholder.
func pickGame(list []igdb.Game, query string) (igdb.Game, bool) {
	q := keyTokens(query)
	if len(q) == 0 {
		if len(list) > 0 {
			return list[0], true
		}
		return igdb.Game{}, false
	}
	// Stores the INDEX of the weak candidate, not a pointer to the element: pointing
	// into a slice passed as a parameter is fragile (and gosec complains).
	common := -1
	for i := range list {
		if titleMatches(query, list[i].Name) {
			return list[i], true
		}
		// Last resort, and only for GAME: a significant token in common. It's valid because
		// there is no author to cross-reference and IGDB sometimes indexes the collection instead of the
		// standalone title ("Ratchet & Clank 1 2 3" → "Ratchet & Clank Collection").
		if common < 0 {
			for _, t := range keyTokens(list[i].Name) {
				if containsToken(q, t) {
					common = i
					break
				}
			}
		}
	}
	if common >= 0 {
		return list[common], true
	}
	return igdb.Game{}, false
}

// bookCandidate is the minimum the guard needs from any book provider — the
// Open Library and Google Books clients don't share a type, but answer the
// same question.
type bookCandidate struct {
	Title    string
	Author   string
	CoverURL string
	Subjects []string
	Language string
}

/*
bookFromProviders tries the providers in order and returns the FIRST candidate that passes
the identity guard (#108): the title must match and, when we know the author, the
authorship must confirm. A failed candidate doesn't block the next one — which is exactly
the case for PT-BR books, where Open Library returns an English homonym and Google
Books has the right edition.

Attempts, in order:
 1. Open Library with title+author;
 2. Open Library with title only (PT-BR books usually don't have the author indexed);
 3. Google Books with title+author;
 4. Google Books with title only.

If none pass, it returns false and the UI stays on the placeholder — a wrong cover is more misleading
than a missing cover, because no one suspects it.
*/
func (s *Service) bookFromProviders(ctx context.Context, title, author string, year int) (bookCandidate, bool) {
	hasAuthor := strings.TrimSpace(author) != ""

	// checks only the IDENTITY (is this the book?). Having a cover is another question, answered
	// below — mixing the two made the cascade stop at the first provider that
	// identified the book even WITHOUT a cover, and never ask the next one.
	identifies := func(c bookCandidate) bool {
		if !titleMatches(title, c.Title) {
			return false
		}
		if hasAuthor && !authorMatches(author, c.Author) {
			return false
		}
		return c.Title != ""
	}

	searchOL := func(a string) (bookCandidate, bool) {
		if s.openlib == nil {
			return bookCandidate{}, false
		}
		b, err := s.openlib.Search(ctx, title, a, year)
		if err != nil {
			return bookCandidate{}, false
		}
		return bookCandidate{Title: b.Title, Author: b.Author, CoverURL: b.CoverURL,
			Subjects: b.Subjects, Language: b.Language}, true
	}
	searchGB := func(a string) (bookCandidate, bool) {
		if s.gbooks == nil {
			return bookCandidate{}, false
		}
		b, err := s.gbooks.Search(ctx, title, a, year)
		if err != nil {
			return bookCandidate{}, false
		}
		return bookCandidate{Title: b.Title, Author: b.Author, CoverURL: b.CoverURL,
			Subjects: b.Subjects, Language: b.Language}, true
	}

	// Order: the two providers WITH author first, then the two without. The search with
	// author is the high-precision one — it's what separates homonyms — so spending the first
	// two attempts on it reaches the answer in fewer calls. Measured: "Night Songs"
	// (Alli Dyer, 2026) was only resolved by Google Books, and in the old order
	// (OL w/author → OL w/o author → GB) this fell on the 3rd attempt, ~3.7s idle — under the
	// browse burst the item deadline was exceeded beforehand and the book ended up without a cover.
	attempts := []func() (bookCandidate, bool){
		func() (bookCandidate, bool) { return searchOL(author) },
		func() (bookCandidate, bool) { return searchGB(author) },
	}
	if hasAuthor { // without author only makes sense if there was an author to discard
		attempts = append(attempts,
			func() (bookCandidate, bool) { return searchOL("") },
			func() (bookCandidate, bool) { return searchGB("") },
		)
	}
	// The cascade only STOPS when it finds a cover. An identified candidate but without a cover is
	// saved as a fallback — it serves for language and genres — and the search continues on the
	// next provider. This was exactly the case for "Night Songs" (Alli Dyer, 2026):
	// Open Library confirmed the book without having a cover, the cascade ended there and Google
	// Books, which HAS the cover, was never queried.
	var withoutCover *bookCandidate
	for _, try := range attempts {
		c, ok := try()
		if !ok || !identifies(c) {
			continue
		}
		if c.CoverURL != "" {
			return c, true
		}
		if withoutCover == nil {
			first := c
			withoutCover = &first
		}
	}
	if withoutCover != nil {
		return *withoutCover, true
	}
	return bookCandidate{}, false
}

// titleMatches decides if a provider result is REALLY the requested item. It's the same
// question for game and book, so the rule is just one: identical name, word-boundary prefix,
// or the candidate containing ALL significant tokens from the request (covers
// extra subtitle/edition in the provider's catalog). Anything looser than this
// has already pasted a cover from another title — see pickGame and the book guard.
func titleMatches(query, candidate string) bool {
	q, c := keyTokens(query), keyTokens(candidate)
	if len(q) == 0 || len(c) == 0 {
		return false
	}
	qs, cs := strings.Join(q, " "), strings.Join(c, " ")
	if qs == cs {
		return true
	}
	if strings.HasPrefix(cs, qs+" ") || strings.HasPrefix(qs, cs+" ") {
		return true
	}
	for _, t := range q {
		if !containsToken(c, t) {
			return false
		}
	}
	return true
}

// authorMatches confirms AUTHORSHIP when we know it. It exists because homonym titles are
// common in books: "Hocus Pocus" by Brinda Charry (2026) was showing the cover of "Hocus
// pocus" by Titania Hardie (1996), and "Night Songs" by Alli Dyer the one by Charles L. Grant
// (1984). One common surname is enough — Open Library varies the name format ("J.K.
// Rowling", "Rowling, J. K.") and requiring equality would reject a legitimate match. A candidate
// without an author DOES NOT confirm: nothing to cross-reference is the same as not knowing.
func authorMatches(want, got string) bool {
	w, g := keyTokens(want), keyTokens(got)
	if len(w) == 0 || len(g) == 0 {
		return false
	}
	for _, t := range w {
		if containsToken(g, t) {
			return true
		}
	}
	return false
}

// keyTokens normalizes a name into comparable tokens: lowercase, no punctuation and
// no stop words ("the", "of", "a"...) — which would tie any pair of titles.
func keyTokens(s string) []string {
	// Apostrophe is DELETED, not treated as a separator: otherwise "Assassin's" would become
	// ["assassin","s"] and never match "Assassins" from the release name (the uploader
	// almost always omits the apostrophe). Applies to the curly one too.
	low := strings.NewReplacer("'", "", "’", "").Replace(strings.ToLower(s))
	var out []string
	for _, raw := range strings.FieldsFunc(low, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		switch raw {
		case "the", "of", "a", "an", "and", "e", "de", "da", "do", "el", "la":
			continue
		}
		out = append(out, raw)
	}
	return out
}

func containsToken(list []string, t string) bool {
	for _, x := range list {
		if x == t {
			return true
		}
	}
	return false
}

// reBookYear captures year in brackets or parentheses in the name ([2020], (1899)) —
// since UNIT3D doesn't populate release_year for book/audiobook, the year almost always comes
// here in the release name (common site pattern).
var reBookYear = regexp.MustCompile(`[\[\(](\d{4})[\]\)]`)

// reBookTag removes ANY [...] or (...) from the book/audiobook release name. Before,
// the list was of known keywords (AUDIOBOOK, PACK, ENGLISH, ePUB…) and a
// site wrote "[ENG]", which wasn't in it: the author became "Nikki Witt [ENG]",
// the author search failed and the title-only fallback pasted a NAMESAKE cover — the
// "Hocus Pocus" by Brinda Charry showed the one by Titania Hardie (#108). Keeping a
// keyword list is a losing race against what the uploader types; in a book
// release name, what's inside brackets/parentheses is metadata, not the title.
var reBookTag = regexp.MustCompile(`\s*[\[\(][^\]\)]*[\]\)]\s*`)

// bookMeta extracts title, author, and year from the book/audiobook release name. Formats
// seen on a Brazilian UNIT3D site:
//
//	"The Bully Tailor - Brothers Grimm [2020] [AUDIOBOOK]"
//	"Harry Potter and the Chamber of Secrets - J.K. Rowling [2023] [AUDIOBOOK]"
//	"Collection of Books on Go - Various Authors [2021] [PACK] [ENGLISH AND PORTUGUESE]"
//	"MY NUMBER 10 (Indecent Games - Book 1) - Alessandra Ferrer [2024]"
//	"Dom Casmurro (1899)"  ← year in parentheses, no author
//
// Heuristic: extracts year (first [YYYY]/(YYYY)), clears metadata tags, and splits
// by " - " assuming "Title - Author" (the common pattern). Without "-"
// in the middle, the whole name becomes the title and the author comes empty.
func bookMeta(name string) (title, author string, year int) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", 0
	}
	// Year: first reasonable [YYYY] or (YYYY) (1500-2100 to avoid catching "Book 1").
	if m := reBookYear.FindStringSubmatch(name); m != nil {
		if y, err := strconv.Atoi(m[1]); err == nil && y >= 1500 && y <= 2100 {
			year = y
		}
	}
	// Removes everything inside [...] and (...) that matches a known tag — including the
	// [YYYY] itself (already extracted above) to not dirty the "- Author" parse.
	name = reBookTag.ReplaceAllString(name, " ")
	name = reBookYear.ReplaceAllString(name, " ")
	name = strings.Join(strings.Fields(name), " ") // collapses multiple spaces
	// Removes common extension if left over (rare on this tracker but robust).
	for _, ext := range []string{".epub", ".mobi", ".pdf", ".azw3", ".mp3", ".m4b", ".m4a"} {
		if strings.HasSuffix(strings.ToLower(name), ext) {
			name = strings.TrimSpace(name[:len(name)-len(ext)])
		}
	}
	// Splits "Title - Author" by the FIRST " - ". Before it was the last, to handle
	// "MY NUMBER 10 (Indecent Games - Book 1) - Alessandra Ferrer" — but now the
	// parentheses are already removed above, so the first separator is the author's. With the last,
	// a title with a subtitle after " - " put the subtitle in place of the author
	// ("An Apothecary Afield: ... - A Cozy Fantasy In a Small Village ...").
	if i := strings.Index(name, " - "); i > 0 {
		title = strings.TrimSpace(name[:i])
		author = strings.TrimSpace(name[i+3:])
		// Multiple authors ("Ching - Jarzombek - Prakash"): keeps the first — the
		// Open Library matches on any of the work's authors.
		if j := strings.Index(author, " - "); j > 0 {
			author = strings.TrimSpace(author[:j])
		}
		return title, author, year
	}
	return name, "", year
}

func itoa(n int) string { return strconv.Itoa(n) }
