package description

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// ---- Each block, on its own -------------------------------------------------------

// only returns the Config that binds a single block.
func onlyMedia(id string) Config { return Config{MediaBlocks: []string{id}} }
func onlyGame(id string) Config  { return Config{GameBlocks: []string{id}} }

func TestComposeMedia_EachBlockAlone(t *testing.T) {
	full := MediaInput{
		Poster:      "https://img/p.jpg",
		Overview:    "  A synopsis.  ",
		Keywords:    " action, drama ",
		Screenshots: "\n[b]Screenshots[/b]\n[img]x[/img]\n",
		MediaInfo:   "\nGeneral\nFormat : MKV\n",
	}
	cases := []struct {
		block string
		in    MediaInput
		want  string
	}{
		{BlockPoster, full, "[center][img]https://img/p.jpg[/img][/center]"},
		{BlockSynopsis, full, "[b]Synopsis[/b]\nA synopsis."},
		{BlockTags, full, "[b]Tags[/b]\naction, drama"},
		{BlockScreenshots, full, "[b]Screenshots[/b]\n[img]x[/img]"},
		{BlockMediaInfo, full, "[b]MediaInfo[/b]\n[mediainfo]General\nFormat : MKV[/mediainfo]"},
		// Each one disappears when the data is empty — a block without content never leaves a dangling title.
		{BlockPoster, MediaInput{}, ""},
		{BlockSynopsis, MediaInput{Overview: " \n "}, ""},
		{BlockTags, MediaInput{Keywords: "\t"}, ""},
		{BlockScreenshots, MediaInput{Screenshots: "\n\n"}, ""},
		{BlockMediaInfo, MediaInput{MediaInfo: "  "}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.block+"/"+tc.want, func(t *testing.T) {
			if got := ComposeMedia(tc.in, onlyMedia(tc.block)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestComposeGame_EachBlockAlone(t *testing.T) {
	full := GameInput{
		CoverURL:  "https://img/c.jpg",
		Name:      "Elden Ring",
		Year:      2022,
		Summary:   " A game. ",
		Genres:    []string{"RPG", "Action"},
		Platforms: []string{"PC", "PS5"},
	}
	cases := []struct {
		block string
		in    GameInput
		want  string
	}{
		{BlockCover, full, "[center][img]https://img/c.jpg[/img][/center]"},
		{BlockTitle, full, "[b]Elden Ring (2022)[/b]"},
		{BlockAbout, full, "[b]About[/b]\nA game."},
		{BlockGenres, full, "[b]Genres[/b]\nRPG, Action"},
		{BlockPlatforms, full, "[b]Platforms[/b]\nPC, PS5"},
		{BlockCover, GameInput{}, ""},
		{BlockAbout, GameInput{Summary: " "}, ""},
		{BlockGenres, GameInput{}, ""},
		{BlockPlatforms, GameInput{Platforms: []string{}}, ""},
		// The title is the exception: it always comes out, just like in TypeScript.
		{BlockTitle, GameInput{}, "[b][/b]"},
		{BlockTitle, GameInput{Name: "Tetris"}, "[b]Tetris[/b]"},
	}
	for _, tc := range cases {
		t.Run(tc.block+"/"+tc.want, func(t *testing.T) {
			if got := ComposeGame(tc.in, onlyGame(tc.block)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ---- The Config changes the result ----------------------------------------------------

var mediaFull = MediaInput{
	Poster:      "https://img/p.jpg",
	Overview:    "Sinopse.",
	Keywords:    "k1, k2",
	Screenshots: "[b]Screenshots[/b]\nS",
	MediaInfo:   "dump",
}

const (
	bPoster = "[center][img]https://img/p.jpg[/img][/center]"
	bSyn    = "[b]Synopsis[/b]\nSinopse."
	bTags   = "[b]Tags[/b]\nk1, k2"
	bShots  = "[b]Screenshots[/b]\nS"
)

func TestComposeMedia_ConfigChangesOrderAndOmitsBlocks(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"default na ordem original", DefaultConfig(), strings.Join([]string{bPoster, bSyn, bTags, bShots, "[b]MediaInfo[/b]\n[mediainfo]dump[/mediainfo]"}, "\n\n")},
		{"ordem invertida", Config{MediaBlocks: []string{BlockMediaInfo, BlockScreenshots, BlockTags, BlockSynopsis, BlockPoster}},
			strings.Join([]string{"[b]MediaInfo[/b]\n[mediainfo]dump[/mediainfo]", bShots, bTags, bSyn, bPoster}, "\n\n")},
		{"subset: what is not in the list is left out", Config{MediaBlocks: []string{BlockPoster, BlockMediaInfo}},
			bPoster + "\n\n[b]MediaInfo[/b]\n[mediainfo]dump[/mediainfo]"},
		{"mediainfo before poster", Config{MediaBlocks: []string{BlockMediaInfo, BlockPoster}},
			"[b]MediaInfo[/b]\n[mediainfo]dump[/mediainfo]\n\n" + bPoster},
		{"empty list disables everything (not the default)", Config{MediaBlocks: []string{}}, ""},
		{"null list is the default", Config{MediaBlocks: nil, MediaInfoTag: TagCode},
			strings.Join([]string{bPoster, bSyn, bTags, bShots, "[b]MediaInfo[/b]\n[code]dump[/code]"}, "\n\n")},
		{"game list does not affect media list", Config{GameBlocks: []string{}}, ComposeMedia(mediaFull, DefaultConfig())},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComposeMedia(mediaFull, tc.cfg); got != tc.want {
				t.Errorf("got:  %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestComposeMedia_TagDoMediaInfo(t *testing.T) {
	in := MediaInput{MediaInfo: "x"}
	for _, tag := range []string{TagMediaInfo, TagCode, TagPre, TagSpoiler} {
		want := "[b]MediaInfo[/b]\n[" + tag + "]x[/" + tag + "]"
		if got := ComposeMedia(in, Config{MediaInfoTag: tag}); got != want {
			t.Errorf("tag %s: got %q, want %q", tag, got, want)
		}
	}
	// An empty tag is the default.
	if got := ComposeMedia(in, Config{}); got != "[b]MediaInfo[/b]\n[mediainfo]x[/mediainfo]" {
		t.Errorf("empty tag: %q", got)
	}
	// Defense in depth: a tag outside the closed list (row inserted manually into the database) is never
	// interpolated into BBCode — it falls back to the default.
	for _, bad := range []string{"b", "url=http://x", "mediainfo]x[/mediainfo][img]", "PRE", " pre", "pre\n"} {
		got := ComposeMedia(in, Config{MediaInfoTag: bad})
		if got != "[b]MediaInfo[/b]\n[mediainfo]x[/mediainfo]" {
			t.Errorf("invalid tag %q was emitted: %q", bad, got)
		}
	}
}

func TestComposeGame_ConfigChangesOrderAndOmitsBlocks(t *testing.T) {
	in := GameInput{CoverURL: "https://img/c.jpg", Name: "X", Year: 2020, Summary: "S.", Genres: []string{"A"}, Platforms: []string{"PC"}}
	const (
		cover = "[center][img]https://img/c.jpg[/img][/center]"
		title = "[b]X (2020)[/b]"
		about = "[b]About[/b]\nS."
		gen   = "[b]Genres[/b]\nA"
		plat  = "[b]Platforms[/b]\nPC"
	)
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"default", DefaultConfig(), strings.Join([]string{cover, title, about, gen, plat}, "\n\n")},
		{"only title and about", Config{GameBlocks: []string{BlockTitle, BlockAbout}}, title + "\n\n" + about},
		{"ordem invertida", Config{GameBlocks: []string{BlockPlatforms, BlockGenres, BlockTitle}}, plat + "\n\n" + gen + "\n\n" + title},
		{"no title: it only appears if in the list", Config{GameBlocks: []string{BlockCover, BlockAbout}}, cover + "\n\n" + about},
		{"empty list disables everything", Config{GameBlocks: []string{}}, ""},
		{"MediaInfo tag and media list do not affect game", Config{MediaBlocks: []string{}, MediaInfoTag: TagPre}, strings.Join([]string{cover, title, about, gen, plat}, "\n\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComposeGame(in, tc.cfg); got != tc.want {
				t.Errorf("got:  %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// An unknown id in the list (invalid config that bypassed validation) is ignored, never
// emitted as text nor does it cause a panic.
func TestCompose_UnknownIdInListIsIgnored(t *testing.T) {
	if got := ComposeMedia(mediaFull, Config{MediaBlocks: []string{"x", BlockPoster, "[b]"}}); got != bPoster {
		t.Errorf("media: %q", got)
	}
	if got := ComposeGame(GameInput{Name: "X"}, Config{GameBlocks: []string{"poster", BlockTitle}}); got != "[b]X[/b]" {
		t.Errorf("game: %q", got)
	}
}

// ---- O default e a Normalized --------------------------------------------------------

func TestDefaultConfig_AOrdemEOCatalogo(t *testing.T) {
	d := DefaultConfig()
	if want := []string{"poster", "synopsis", "tags", "screenshots", "mediainfo"}; !reflect.DeepEqual(d.MediaBlocks, want) {
		t.Errorf("default media blocks = %v, want %v", d.MediaBlocks, want)
	}
	if want := []string{"cover", "title", "about", "genres", "platforms"}; !reflect.DeepEqual(d.GameBlocks, want) {
		t.Errorf("default game blocks = %v, want %v", d.GameBlocks, want)
	}
	if d.MediaInfoTag != "mediainfo" {
		t.Errorf("tag do default = %q", d.MediaInfoTag)
	}
	if err := d.Validate(); err != nil {
		t.Errorf("default is not valid: %v", err)
	}
}

// Whoever receives the default can modify it: the mutation does not leak into the next default.
func TestDefaultConfig_NoMemorySharing(t *testing.T) {
	a := DefaultConfig()
	a.MediaBlocks[0] = "lixo"
	a.GameBlocks[0] = "lixo"
	if b := DefaultConfig(); b.MediaBlocks[0] != BlockPoster || b.GameBlocks[0] != BlockCover {
		t.Errorf("default was modified by receiver: %v", b)
	}
}

func TestNormalized(t *testing.T) {
	def := DefaultConfig()
	if got := (Config{}).Normalized(); !reflect.DeepEqual(got, def) {
		t.Errorf("normalized zero = %+v, want default", got)
	}
	// An explicit empty remains empty, and does not become nil nor the default.
	got := Config{MediaBlocks: []string{}, GameBlocks: []string{}}.Normalized()
	if got.MediaBlocks == nil || len(got.MediaBlocks) != 0 || got.GameBlocks == nil || len(got.GameBlocks) != 0 {
		t.Errorf("empty list did not survive: %+v", got)
	}
	if got.MediaInfoTag != TagMediaInfo {
		t.Errorf("empty tag did not become the default: %q", got.MediaInfoTag)
	}
	// Does not share memory with the input.
	in := Config{MediaBlocks: []string{BlockTags, BlockPoster}}
	out := in.Normalized()
	out.MediaBlocks[0] = "lixo"
	if in.MediaBlocks[0] != BlockTags {
		t.Error("Normalized returned a slice that shares memory with the input")
	}
}

func TestEqual(t *testing.T) {
	if !(Config{}).Equal(DefaultConfig()) {
		t.Error("zero Config and default compare equal")
	}
	if !(Config{MediaInfoTag: "mediainfo"}).Equal(Config{}) {
		t.Error("explicit tag = empty tag")
	}
	if (Config{MediaBlocks: []string{}}).Equal(Config{}) {
		t.Error("empty list is not the default")
	}
	if (Config{MediaBlocks: []string{BlockPoster}}).Equal(Config{MediaBlocks: []string{BlockPoster, BlockTags}}) {
		t.Error("lists differ")
	}
	if (Config{MediaBlocks: []string{BlockTags, BlockPoster}}).Equal(Config{MediaBlocks: []string{BlockPoster, BlockTags}}) {
		t.Error("order matters")
	}
	if (Config{MediaInfoTag: TagPre}).Equal(Config{}) {
		t.Error("tags differ")
	}
}

// The empty list must survive the round trip through JSON — it's what writes it to the profile and
// returns it to the Portal. An omitempty would silently turn it into the default.
func TestConfig_JSONPreservesEmptyList(t *testing.T) {
	raw, err := json.Marshal(Config{MediaBlocks: []string{}, GameBlocks: []string{BlockTitle}, MediaInfoTag: TagPre})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"media_blocks":[],"game_blocks":["title"],"media_info_tag":"pre"}`; string(raw) != want {
		t.Fatalf("JSON = %s, want %s", raw, want)
	}
	var back Config
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.MediaBlocks == nil || len(back.MediaBlocks) != 0 {
		t.Errorf("empty list came back as %#v", back.MediaBlocks)
	}
	// And what didn't come in the JSON is nil, which means the default.
	var partial Config
	if err := json.Unmarshal([]byte(`{"media_info_tag":"code"}`), &partial); err != nil {
		t.Fatal(err)
	}
	if partial.MediaBlocks != nil || partial.GameBlocks != nil {
		t.Errorf("missing field should be nil: %+v", partial)
	}
}

// ---- Validation -------------------------------------------------------------------------

func TestValidate(t *testing.T) {
	long := strings.Repeat("a", 5000)
	cases := []struct {
		name     string
		cfg      Config
		wantPath string // "" = valid
	}{
		{"default", DefaultConfig(), ""},
		{"zero", Config{}, ""},
		{"listas vazias", Config{MediaBlocks: []string{}, GameBlocks: []string{}}, ""},
		{"ordem diferente", Config{MediaBlocks: []string{BlockMediaInfo, BlockPoster}, GameBlocks: []string{BlockPlatforms, BlockTitle}}, ""},
		{"as quatro tags", Config{MediaInfoTag: TagSpoiler}, ""},

		{"unknown id in media", Config{MediaBlocks: []string{"poster", "x"}}, "media_blocks[1]"},
		{"id desconhecido no jogo", Config{GameBlocks: []string{"cover", "z"}}, "game_blocks[1]"},
		{"game block in media list", Config{MediaBlocks: []string{BlockCover}}, "media_blocks[0]"},
		{"media block in game list", Config{GameBlocks: []string{BlockPoster}}, "game_blocks[0]"},
		{"duplicate id in media", Config{MediaBlocks: []string{"poster", "tags", "poster"}}, "media_blocks[2]"},
		{"id duplicado no jogo", Config{GameBlocks: []string{"title", "title"}}, "game_blocks[1]"},
		{"different case is another id", Config{MediaBlocks: []string{"Poster"}}, "media_blocks[0]"},
		{"empty id", Config{MediaBlocks: []string{""}}, "media_blocks[0]"},
		{"list larger than block set", Config{MediaBlocks: []string{"poster", "synopsis", "tags", "screenshots", "mediainfo", "poster"}}, "media_blocks"},
		{"huge list is not traversed", Config{GameBlocks: make([]string, 100000)}, "game_blocks"},
		{"invalid tag", Config{MediaInfoTag: "html"}, "media_info_tag"},
		{"tag with different case", Config{MediaInfoTag: "Pre"}, "media_info_tag"},
		{"tag that tries to close BBCode", Config{MediaInfoTag: "mediainfo][img"}, "media_info_tag"},
		{"tag gigante", Config{MediaInfoTag: long}, "media_info_tag"},
		{"id gigante", Config{MediaBlocks: []string{long}}, "media_blocks[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantPath == "" {
				if err != nil {
					t.Fatalf("rejected a valid config: %v", err)
				}
				return
			}
			var de *Error
			if !errors.As(err, &de) {
				t.Fatalf("error = %#v, want *Error", err)
			}
			if de.Path != tc.wantPath {
				t.Errorf("path = %q, want %q", de.Path, tc.wantPath)
			}
			// The received value never comes back entirely in the message (it might come from a third-party file).
			if len(err.Error()) > 400 {
				t.Errorf("message with %d bytes returns the received value: %.120s…", len(err.Error()), err.Error())
			}
		})
	}
}

// The message states what is accepted, so whoever edits the JSON knows how to fix it.
func TestValidate_TheMessageSaysWhatIsAccepted(t *testing.T) {
	err := Config{MediaBlocks: []string{"x"}}.Validate()
	for _, id := range []string{"poster", "synopsis", "tags", "screenshots", "mediainfo"} {
		if !strings.Contains(err.Error(), id) {
			t.Errorf("message does not list %q: %v", id, err)
		}
	}
	err = Config{MediaInfoTag: "x"}.Validate()
	for _, tag := range []string{"mediainfo", "code", "pre", "spoiler"} {
		if !strings.Contains(err.Error(), tag) {
			t.Errorf("message does not list tag %q: %v", tag, err)
		}
	}
}

// ---- jsTrim ---------------------------------------------------------------------------

// The set of JS's String.prototype.trim, and where it diverges from Go's strings.TrimSpace.
func TestJSTrim(t *testing.T) {
	// The characters in question are invisible in the source code; that's why they are entered by code point.
	r := func(cps ...rune) string { return string(cps) }
	const (
		bom  = 0xFEFF
		nbsp = 0x00A0
		nel  = 0x0085
		mvs  = 0x180E
		zwsp = 0x200B
	)
	cases := []struct {
		name, in, want string
	}{
		{"common spaces and breaks", " \t\r\n\v\f x \n", "x"},
		{"BOM: JS removes, Go does not", r(bom) + "x" + r(bom), "x"},
		{"NBSP", r(nbsp) + "x" + r(nbsp), "x"},
		{"line and paragraph separators", r(0x2028) + "x" + r(0x2029), "x"},
		{"os Zs", r(0x1680, 0x2000, 0x2005, 0x200A) + "x" + r(0x202F, 0x205F, 0x3000), "x"},
		{"NEL: Go removes, JS does not", r(nel) + "x" + r(nel), r(nel) + "x" + r(nel)},
		{"U+180E is not a space", r(mvs) + "x" + r(mvs), r(mvs) + "x" + r(mvs)},
		{"zero-width space is not a space", r(zwsp) + "x" + r(zwsp), r(zwsp) + "x" + r(zwsp)},
		{"only space", " " + r(nbsp, bom) + " ", ""},
		{"the core is preserved", "a " + r(nbsp) + "\n b", "a " + r(nbsp) + "\n b"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		if got := jsTrim(tc.in); got != tc.want {
			t.Errorf("%s: jsTrim(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
	// Proof that strings.TrimSpace is NOT enough: it's the reason the function exists.
	if strings.TrimSpace(r(bom)+"x") == "x" {
		t.Error("Go's TrimSpace now removes the BOM: review the need for jsTrim")
	}
	if strings.TrimSpace(r(nel)+"x") != "x" {
		t.Error("Go's TrimSpace no longer removes the NEL: review the need for jsTrim")
	}
}

// The headings follow the content language of the profile (#74); the default is English.
func TestComposeHeadingsFollowTheLanguage(t *testing.T) {
	game := GameInput{Name: "X", Summary: "S.", Genres: []string{"RPG"}, Platforms: []string{"PC"}}
	media := MediaInput{Overview: "O."}
	cases := map[string][]string{
		"":       {"[b]Synopsis[/b]", "[b]About[/b]", "[b]Genres[/b]", "[b]Platforms[/b]"},
		LangEN:   {"[b]Synopsis[/b]", "[b]About[/b]", "[b]Genres[/b]", "[b]Platforms[/b]"},
		LangPTBR: {"[b]Sinopse[/b]", "[b]Sobre[/b]", "[b]Gêneros[/b]", "[b]Plataformas[/b]"},
	}
	for lang, want := range cases {
		got := ComposeMedia(media, Config{Language: lang}) + "\n" + ComposeGame(game, Config{Language: lang})
		for _, h := range want {
			if !strings.Contains(got, h) {
				t.Errorf("language %q: missing %s in %q", lang, h, got)
			}
		}
	}
}

func TestValidateLanguage(t *testing.T) {
	if err := (Config{Language: "pt-BR"}).Validate(); err != nil {
		t.Errorf("pt-BR rejected: %v", err)
	}
	err := (Config{Language: "klingon"}).Validate()
	var de *Error
	if !errors.As(err, &de) || de.Path != "language" {
		t.Errorf("unknown language: err = %v, want *Error at language", err)
	}
	if (Config{}).ContentLanguage() != LangEN || (Config{Language: LangEN}).Equal(Config{}) == false {
		t.Error("the default content language should be English, equal to an explicit en")
	}
	if (Config{Language: "pt-BR"}).Equal(Config{}) {
		t.Error("pt-BR and the default compose differently and must not be Equal")
	}
}
