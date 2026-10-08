package description

import "testing"

// Parity with the TypeScript that assembled the description in the Portal (#192).
//
// The expected strings below were NOT hand-written: they were calculated by EXECUTING
// buildBBCode and buildGameBBCode — as they were in portal/src/pages/publish/bbcode.ts at commit
// 0525f0f (`git show 0525f0f:portal/src/pages/publish/bbcode.ts`) — over the inputs for each
// line, on Node 22, and pasted here. Go cannot run TS; what guarantees the parity
// going forward is this golden: if a change in description.go alters a single byte of the
// output with the default Config, the corresponding line fails.
//
// The lines exist for cases where parity is easy to get wrong: what the JS `trim()`
// removes (BOM, non-breaking space, U+2028/2029, the Zs) and what it DOES NOT remove (U+0085, U+180E,
// U+200B) — Go's `strings.TrimSpace` diverges in both directions —, the poster that is "truthy"
// only with spaces and enters without trim, the closing tag inside MediaInfo (unescaped), the
// game title that outputs even without a name, the negative year (truthy) and the list with an empty item
// (only the list size decides).
const (
	shots = "[b]Screenshots[/b]\n[url=https://i.ibb.co/aaa/1.png][img]https://i.ibb.co/aaa/1t.png[/img][/url] [url=https://i.ibb.co/bbb/2.png][img]https://i.ibb.co/bbb/2t.png[/img][/url]"
	mi    = "General\nUnique ID : 1 (0x1)\nFormat : Matroska\nFile size : 2.10 GiB\n\nVideo\nID : 1\nFormat : HEVC\nWidth : 1 920 pixels\n\nAudio #1\nLanguage : Portuguese\n\nAudio #2\nLanguage : English"
)

var parityMedia = []struct {
	name string
	in   MediaInput
	want string
}{
	{"fully populated", MediaInput{Poster: "https://image.tmdb.org/t/p/w500/abc.jpg", Overview: "A witch faces the unknown.", Keywords: "witchcraft, horror, forest", Screenshots: shots, MediaInfo: mi}, "[center][img]https://image.tmdb.org/t/p/w500/abc.jpg[/img][/center]\n\n[b]Synopsis[/b]\nA witch faces the unknown.\n\n[b]Tags[/b]\nwitchcraft, horror, forest\n\n" + shots + "\n\n[b]MediaInfo[/b]\n[mediainfo]" + mi + "[/mediainfo]"},
	{"only poster", MediaInput{Poster: "https://image.tmdb.org/t/p/w500/abc.jpg"}, "[center][img]https://image.tmdb.org/t/p/w500/abc.jpg[/img][/center]"},
	{"only synopsis", MediaInput{Overview: "Plain text."}, "[b]Synopsis[/b]\nPlain text."},
	{"only tags", MediaInput{Keywords: "action, adventure"}, "[b]Tags[/b]\naction, adventure"},
	{"only screenshots", MediaInput{Screenshots: shots}, shots},
	{"only mediainfo", MediaInput{MediaInfo: mi}, "[b]MediaInfo[/b]\n[mediainfo]" + mi + "[/mediainfo]"},
	{"all empty", MediaInput{}, ""},
	{"strings vazias", MediaInput{Poster: "", Overview: "", Keywords: "", Screenshots: "", MediaInfo: ""}, ""},
	{"only whitespace", MediaInput{Overview: "  \n\t ", Keywords: "   ", Screenshots: "\n\n", MediaInfo: " \r\n "}, ""},
	{"synopsis with spaces and breaks at the ends", MediaInput{Overview: "\n\n  First line.\nSecond line.  \n\t"}, "[b]Synopsis[/b]\nFirst line.\nSecond line."},
	{"poster with only space (is truthy: included without trim)", MediaInput{Poster: " "}, "[center][img] [/img][/center]"},
	{"poster with spaces at the ends (kept as is)", MediaInput{Poster: "  https://x/y.jpg  "}, "[center][img]  https://x/y.jpg  [/img][/center]"},
	{"BOM and non-breaking spaces at the ends (JS removes them)", MediaInput{Overview: "\ufefftexto\u00a0", Keywords: "\u00a0\u00a0a, b\ufeff"}, "[b]Synopsis[/b]\ntexto\n\n[b]Tags[/b]\na, b"},
	{"Unicode line and paragraph separators (JS removes them)", MediaInput{Overview: "\u2028texto\u2029", MediaInfo: "\u2028dump\u2029"}, "[b]Synopsis[/b]\ntexto\n\n[b]MediaInfo[/b]\n[mediainfo]dump[/mediainfo]"},
	{"Unicode Zs spaces (JS removes them)", MediaInput{Overview: "\u1680\u2000\u2003\u200atexto\u202f\u205f\u3000"}, "[b]Synopsis[/b]\ntexto"},
	{"U+0085 (NEL) is NOT a space for JS", MediaInput{Overview: "\u0085texto\u0085"}, "[b]Synopsis[/b]\n\u0085texto\u0085"},
	{"U+180E is NOT a space for JS", MediaInput{Overview: "\u180etexto\u180e"}, "[b]Synopsis[/b]\n\u180etexto\u180e"},
	{"U+200B (zero-width space) is NOT a space for JS", MediaInput{Overview: "\u200btexto\u200b"}, "[b]Synopsis[/b]\n\u200btexto\u200b"},
	{"only BOM is empty", MediaInput{Overview: "\ufeff", Keywords: "\u00a0"}, ""},
	{"mediainfo with CRLF", MediaInput{MediaInfo: "General\r\nFormat : Matroska\r\n"}, "[b]MediaInfo[/b]\n[mediainfo]General\r\nFormat : Matroska[/mediainfo]"},
	{"mediainfo containing the closing tag (unescaped)", MediaInput{MediaInfo: "line [/mediainfo] other"}, "[b]MediaInfo[/b]\n[mediainfo]line [/mediainfo] other[/mediainfo]"},
	{"acentos, emoji e CJK", MediaInput{Poster: "https://x/ç.jpg", Overview: "Ação, coração e 🎬 — 日本語", Keywords: "ficção científica", MediaInfo: "Título : Não é um 🐍"}, "[center][img]https://x/ç.jpg[/img][/center]\n\n[b]Synopsis[/b]\nAção, coração e 🎬 — 日本語\n\n[b]Tags[/b]\nficção científica\n\n[b]MediaInfo[/b]\n[mediainfo]Título : Não é um 🐍[/mediainfo]"},
	{"brackets and breaks inside fields", MediaInput{Overview: "[b]negrito[/b]\n\n[url=x]y[/url]", Screenshots: "[b]A[/b]\n\n\n[b]B[/b]\n"}, "[b]Synopsis[/b]\n[b]negrito[/b]\n\n[url=x]y[/url]\n\n[b]A[/b]\n\n\n[b]B[/b]"},
	{"screenshots with leading and trailing spaces", MediaInput{Screenshots: "  \n" + shots + "\n  "}, shots},
	{"poster + mediainfo", MediaInput{Poster: "https://x/p.jpg", MediaInfo: "General"}, "[center][img]https://x/p.jpg[/img][/center]\n\n[b]MediaInfo[/b]\n[mediainfo]General[/mediainfo]"},
	{"sinopse + screenshots", MediaInput{Overview: "S.", Screenshots: "[b]Screenshots[/b]\nx"}, "[b]Synopsis[/b]\nS.\n\n[b]Screenshots[/b]\nx"},
	{"tags + mediainfo without synopsis", MediaInput{Keywords: "k", MediaInfo: "m"}, "[b]Tags[/b]\nk\n\n[b]MediaInfo[/b]\n[mediainfo]m[/mediainfo]"},
}

var parityGame = []struct {
	name string
	in   GameInput
	want string
}{
	{"fully populated", GameInput{CoverURL: "https://images.igdb.com/c.jpg", Name: "Elden Ring", Year: 2022, Summary: "An action RPG.", Genres: []string{"RPG", "Adventure"}, Platforms: []string{"PC", "PlayStation 5"}}, "[center][img]https://images.igdb.com/c.jpg[/img][/center]\n\n[b]Elden Ring (2022)[/b]\n\n[b]About[/b]\nAn action RPG.\n\n[b]Genres[/b]\nRPG, Adventure\n\n[b]Platforms[/b]\nPC, PlayStation 5"},
	{"no year", GameInput{CoverURL: "https://images.igdb.com/c.jpg", Name: "Tetris", Year: 0, Summary: "Classic.", Genres: []string{"Puzzle"}, Platforms: []string{"Game Boy"}}, "[center][img]https://images.igdb.com/c.jpg[/img][/center]\n\n[b]Tetris[/b]\n\n[b]About[/b]\nClassic.\n\n[b]Genres[/b]\nPuzzle\n\n[b]Platforms[/b]\nGame Boy"},
	{"no cover", GameInput{Name: "Pong", Year: 1972, Summary: "S.", Genres: []string{"Arcade"}, Platforms: []string{"Arcade"}}, "[b]Pong (1972)[/b]\n\n[b]About[/b]\nS.\n\n[b]Genres[/b]\nArcade\n\n[b]Platforms[/b]\nArcade"},
	{"name only", GameInput{Name: "X"}, "[b]X[/b]"},
	{"name and year only", GameInput{Name: "X", Year: 1999}, "[b]X (1999)[/b]"},
	{"empty cover, summary only spaces", GameInput{CoverURL: "", Name: "X", Year: 2000, Summary: "  \n ", Genres: []string{}, Platforms: []string{}}, "[b]X (2000)[/b]"},
	{"null genres and null platforms", GameInput{Name: "X", Year: 2001, Summary: "r"}, "[b]X (2001)[/b]\n\n[b]About[/b]\nr"},
	{"single genre and empty platforms", GameInput{Name: "X", Genres: []string{"Action"}, Platforms: []string{}}, "[b]X[/b]\n\n[b]Genres[/b]\nAction"},
	{"platforms only", GameInput{Name: "X", Platforms: []string{"PC", "Switch", "Xbox"}}, "[b]X[/b]\n\n[b]Platforms[/b]\nPC, Switch, Xbox"},
	{"empty name and no year", GameInput{Name: "", Year: 0}, "[b][/b]"},
	{"empty name with year", GameInput{Name: "", Year: 2020}, "[b] (2020)[/b]"},
	{"ano negativo (truthy no JS)", GameInput{Name: "X", Year: -5}, "[b]X (-5)[/b]"},
	{"summary with spaces and BOM at the ends", GameInput{Name: "X", Summary: "\ufeff  Text.\u00a0\n"}, "[b]X[/b]\n\n[b]About[/b]\nText."},
	{"list with one empty genre (size decides)", GameInput{Name: "X", Genres: []string{""}, Platforms: []string{""}}, "[b]X[/b]\n\n[b]Genres[/b]\n\n\n[b]Platforms[/b]\n"},
	{"genres with comma and spaces preserved", GameInput{Name: "X", Genres: []string{" A ", "B,C"}, Platforms: []string{"PC "}}, "[b]X[/b]\n\n[b]Genres[/b]\n A , B,C\n\n[b]Platforms[/b]\nPC "},
	{"acentos e emoji", GameInput{CoverURL: "https://x/ç.jpg", Name: "Pokémon 🎮", Year: 1996, Summary: "Capture them now — 日本", Genres: []string{"Action"}, Platforms: []string{"Nintendo Switch"}}, "[center][img]https://x/ç.jpg[/img][/center]\n\n[b]Pokémon 🎮 (1996)[/b]\n\n[b]About[/b]\nCapture them now — 日本\n\n[b]Genres[/b]\nAction\n\n[b]Platforms[/b]\nNintendo Switch"},
	{"name with brackets", GameInput{Name: "[Beta] X", Year: 2010}, "[b][Beta] X (2010)[/b]"},
}

func TestComposeMedia_ParityWithTypeScript(t *testing.T) {
	for _, tc := range parityMedia {
		t.Run(tc.name, func(t *testing.T) {
			// Config zero and default Config are the same thing: both must match the TS.
			if got := ComposeMedia(tc.in, Config{}); got != tc.want {
				t.Errorf("Config zero:\n  got:  %q\n  want: %q", got, tc.want)
			}
			if got := ComposeMedia(tc.in, DefaultConfig()); got != tc.want {
				t.Errorf("Config default:\n  got:  %q\n  want: %q", got, tc.want)
			}
		})
	}
}

func TestComposeGame_ParityWithTypeScript(t *testing.T) {
	for _, tc := range parityGame {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComposeGame(tc.in, Config{}); got != tc.want {
				t.Errorf("Config zero:\n  got:  %q\n  want: %q", got, tc.want)
			}
			if got := ComposeGame(tc.in, DefaultConfig()); got != tc.want {
				t.Errorf("Config default:\n  got:  %q\n  want: %q", got, tc.want)
			}
		})
	}
}
