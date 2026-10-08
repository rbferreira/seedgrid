// Package description builds the BBCode description of an upload (#192).
//
// Previously the Portal built the description in TypeScript (portal/src/pages/publish/bbcode.ts), which
// kept it out of reach of any other API client and left no room for a site with
// different markup to diverge. Now the Control Plane builds it, using the PROFILE Config of the
// tracker (ADR-037, layer 2) — or the default —, and the Portal just asks for the ready text and delivers
// it to the operator, who still edits the field before publishing.
//
// The description is a sequence of BLOCKS, in Config order, separated by a blank
// line; an empty block is omitted. The set of blocks is CLOSED (a new block is a
// code change, not a free id that arrives in JSON), and what the profile decides is:
//
//   - which blocks are included and in what order (omitting from the list = disable);
//   - the tag that wraps MediaInfo (mediainfo, code, pre or spoiler), which varies from site to
//     site.
//
// With the default Config the result is byte-for-byte what TypeScript produced — labels,
// order, separator, and omission of empty ones. Parity is guarded by tests whose strings were
// calculated by running the original TS code, not written by hand.
//
// The Config is UNTRUSTED CONTENT when it arrives from an imported profile (ADR-037): that's why
// Validate checks everything and the tag is only interpolated after passing the closed list.
package description

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Media blocks (movie/show).
const (
	BlockPoster      = "poster"
	BlockSynopsis    = "synopsis"
	BlockTags        = "tags"
	BlockScreenshots = "screenshots"
	BlockMediaInfo   = "mediainfo"
)

// Game blocks.
const (
	BlockCover     = "cover"
	BlockTitle     = "title"
	BlockAbout     = "about"
	BlockGenres    = "genres"
	BlockPlatforms = "platforms"
)

// Tags that can wrap MediaInfo.
const (
	TagMediaInfo = "mediainfo"
	TagCode      = "code"
	TagPre       = "pre"
	TagSpoiler   = "spoiler"
)

// The order of each list is the default one (the same as the old TypeScript).
var (
	mediaBlocks = []string{BlockPoster, BlockSynopsis, BlockTags, BlockScreenshots, BlockMediaInfo}
	gameBlocks  = []string{BlockCover, BlockTitle, BlockAbout, BlockGenres, BlockPlatforms}
	mediaTags   = []string{TagMediaInfo, TagCode, TagPre, TagSpoiler}
)

// Config is what the site profile decides about the composition.
//
// Missing list (nil) means the default; EMPTY list (`[]`) means no blocks of that
// type — which is why the fields don't have omitempty: `[]` has to survive the round trip through
// JSON. Empty MediaInfoTag is the default (`mediainfo`). Language is the language of the content
// that goes to the site — the block headings and what TMDB answers in (#74); empty means English
// and is omitted, so a profile written before #74 reads (and exports) exactly as it did.
type Config struct {
	MediaBlocks  []string `json:"media_blocks"`
	GameBlocks   []string `json:"game_blocks"`
	MediaInfoTag string   `json:"media_info_tag"`
	Language     string   `json:"language,omitempty"`
}

// DefaultConfig returns the Config that reproduces what the Portal built before: all blocks,
// in the original order, with MediaInfo in [mediainfo]. It is also the CATALOG — as it lists all
// blocks, it is what an editor offers to enable, disable, and reorder.
func DefaultConfig() Config {
	return Config{
		MediaBlocks:  append([]string(nil), mediaBlocks...),
		GameBlocks:   append([]string(nil), gameBlocks...),
		MediaInfoTag: TagMediaInfo,
	}
}

// Normalized returns a copy with missing values resolved to the default: nil list
// becomes the default list, empty tag becomes `mediainfo`. Empty list stays empty. It never
// shares memory with the receiver.
func (c Config) Normalized() Config {
	def := DefaultConfig()
	out := Config{MediaBlocks: def.MediaBlocks, GameBlocks: def.GameBlocks, MediaInfoTag: def.MediaInfoTag, Language: c.Language}
	if c.MediaBlocks != nil {
		out.MediaBlocks = append([]string{}, c.MediaBlocks...)
	}
	if c.GameBlocks != nil {
		out.GameBlocks = append([]string{}, c.GameBlocks...)
	}
	if c.MediaInfoTag != "" {
		out.MediaInfoTag = c.MediaInfoTag
	}
	return out
}

// ContentLanguage is the effective content language: Language, or English when it is empty.
func (c Config) ContentLanguage() string {
	if c.Language == "" {
		return LangEN
	}
	return c.Language
}

// Equal tells if two Configs compose the same way (compared already normalized: nil and the
// explicit default are equal).
func (c Config) Equal(o Config) bool {
	a, b := c.Normalized(), o.Normalized()
	return a.MediaInfoTag == b.MediaInfoTag && a.ContentLanguage() == b.ContentLanguage() && equalStrings(a.MediaBlocks, b.MediaBlocks) && equalStrings(a.GameBlocks, b.GameBlocks)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Error is the rejection of a Config, with the field path (media_blocks[1], media_info_tag).
type Error struct {
	Path string
	Msg  string
}

func (e *Error) Error() string { return "description." + e.Path + ": " + e.Msg }

// Validate checks the Config without normalizing it: unknown block, repeated block, list larger
// than the set of blocks, and tag outside the closed list. Returns *Error. The Config may come from
// a third-party file, so the message never returns the received value in full.
func (c Config) Validate() error {
	if err := validateBlocks("media_blocks", c.MediaBlocks, mediaBlocks); err != nil {
		return err
	}
	if err := validateBlocks("game_blocks", c.GameBlocks, gameBlocks); err != nil {
		return err
	}
	if c.MediaInfoTag != "" && !contains(mediaTags, c.MediaInfoTag) {
		return &Error{Path: "media_info_tag", Msg: fmt.Sprintf("unknown tag %q (use %s)", clip(c.MediaInfoTag, 32), strings.Join(mediaTags, ", "))}
	}
	if c.Language != "" && !contains(languages, c.Language) {
		return &Error{Path: "language", Msg: fmt.Sprintf("unknown language %q (use %s)", clip(c.Language, 32), strings.Join(languages, ", "))}
	}
	return nil
}

func validateBlocks(path string, ids, known []string) error {
	// The limit comes before any other check: a list of thousands of items doesn't even
	// get traversed.
	if len(ids) > len(known) {
		return &Error{Path: path, Msg: fmt.Sprintf("%d blocks (maximum %d: %s)", len(ids), len(known), strings.Join(known, ", "))}
	}
	seen := make(map[string]bool, len(ids))
	for i, id := range ids {
		at := fmt.Sprintf("%s[%d]", path, i)
		if !contains(known, id) {
			return &Error{Path: at, Msg: fmt.Sprintf("unknown block %q (use %s)", clip(id, 32), strings.Join(known, ", "))}
		}
		if seen[id] {
			return &Error{Path: at, Msg: fmt.Sprintf("repeated block %q", id)}
		}
		seen[id] = true
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// clip truncates s to n characters to fit in an error message.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// MediaInput is the data for a movie or show. The JSON names are the contract for
// POST /api/v1/publish/description.
type MediaInput struct {
	// Poster is the image URL (TMDB). It goes in as received — no trim —, just like in TypeScript.
	Poster   string `json:"poster"`
	Overview string `json:"overview"`
	// Keywords: the release tags, already as text.
	Keywords string `json:"keywords"`
	// Screenshots is the ready BBCode block, generated by the Control Plane (ADR-026).
	Screenshots string `json:"screenshots"`
	MediaInfo   string `json:"mediainfo"`
}

// GameInput is the data for a game (IGDB candidate).
type GameInput struct {
	CoverURL  string   `json:"cover_url"`
	Name      string   `json:"name"`
	Year      int      `json:"year"`
	Summary   string   `json:"summary"`
	Genres    []string `json:"genres"`
	Platforms []string `json:"platforms"`
}

// ComposeMedia builds the description of a movie or show. Zero Config equals the default.
func ComposeMedia(in MediaInput, cfg Config) string {
	cfg = cfg.Normalized()
	tag := cfg.MediaInfoTag
	if !contains(mediaTags, tag) {
		// Defense in depth: the tag is interpolated in BBCode, and only the closed list passes.
		// A value outside it (a hand-written row in the database) falls back to the default instead of being emitted.
		tag = TagMediaInfo
	}
	parts := make([]string, 0, len(cfg.MediaBlocks))
	for _, id := range cfg.MediaBlocks {
		if b := mediaBlock(id, in, tag, cfg.Language); b != "" {
			parts = append(parts, b)
		}
	}
	return strings.Join(parts, "\n\n")
}

// ComposeGame builds the description of a game. Zero Config equals the default.
func ComposeGame(in GameInput, cfg Config) string {
	cfg = cfg.Normalized()
	parts := make([]string, 0, len(cfg.GameBlocks))
	for _, id := range cfg.GameBlocks {
		if b := gameBlock(id, in, cfg.Language); b != "" {
			parts = append(parts, b)
		}
	}
	return strings.Join(parts, "\n\n")
}

func mediaBlock(id string, in MediaInput, tag, lang string) string {
	switch id {
	case BlockPoster:
		if in.Poster == "" {
			return ""
		}
		return "[center][img]" + in.Poster + "[/img][/center]"
	case BlockSynopsis:
		return titled(heading(lang, BlockSynopsis), jsTrim(in.Overview))
	case BlockTags:
		return titled("Tags", jsTrim(in.Keywords))
	case BlockScreenshots:
		return jsTrim(in.Screenshots)
	case BlockMediaInfo:
		mi := jsTrim(in.MediaInfo)
		if mi == "" {
			return ""
		}
		return "[b]MediaInfo[/b]\n[" + tag + "]" + mi + "[/" + tag + "]"
	}
	return ""
}

func gameBlock(id string, in GameInput, lang string) string {
	switch id {
	case BlockCover:
		if in.CoverURL == "" {
			return ""
		}
		return "[center][img]" + in.CoverURL + "[/img][/center]"
	case BlockTitle:
		// The title ALWAYS comes out (even without a name): that's what TypeScript did. The year is "truthy"
		// in JS — any non-zero value.
		title := in.Name
		if in.Year != 0 {
			title = fmt.Sprintf("%s (%d)", in.Name, in.Year)
		}
		return "[b]" + title + "[/b]"
	case BlockAbout:
		return titled(heading(lang, BlockAbout), jsTrim(in.Summary))
	case BlockGenres:
		// Only the list length decides (like TS's `g.genres?.length`): [""] still emits the block.
		if len(in.Genres) == 0 {
			return ""
		}
		return "[b]" + heading(lang, BlockGenres) + "[/b]\n" + strings.Join(in.Genres, ", ")
	case BlockPlatforms:
		if len(in.Platforms) == 0 {
			return ""
		}
		return "[b]" + heading(lang, BlockPlatforms) + "[/b]\n" + strings.Join(in.Platforms, ", ")
	}
	return ""
}

// titled is "[b]label[/b]\nbody", or "" when there is no body.
func titled(label, body string) string {
	if body == "" {
		return ""
	}
	return "[b]" + label + "[/b]\n" + body
}

// jsTrim reproduces JavaScript's String.prototype.trim, and NOT strings.TrimSpace: the two
// diverge at U+FEFF (JS removes it, Go doesn't) and at U+0085 (Go removes it, JS doesn't). In a
// description pasted from TMDB, the BOM and non-breaking space actually appear, and byte-for-byte
// parity requires JS's set.
func jsTrim(s string) string { return strings.TrimFunc(s, isJSSpace) }

// isJSSpace is ECMAScript's WhiteSpace + LineTerminator: TAB, VT, FF, SP, NBSP, BOM (U+FEFF),
// any Zs (U+1680, U+2000–U+200A, U+202F, U+205F, U+3000), LF, CR, U+2028, and U+2029.
func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}
