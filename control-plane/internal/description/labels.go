package description

// Content languages a site profile can choose (#74). The language sets the headings of the
// upload description and the language TMDB answers in (synopsis and genres). It is a closed
// set: a new language is a code change here, with its headings.
const (
	LangEN   = "en"
	LangPTBR = "pt-BR"
)

// languages lists the accepted values, the default first.
var languages = []string{LangEN, LangPTBR}

// Languages returns the accepted content languages, the default first.
func Languages() []string { return append([]string(nil), languages...) }

// headings are the block titles of the description in each language. These are product text
// that goes to the tracker, not code: the pt-BR column is Portuguese on purpose
// (tools/language-allow.txt).
var headings = map[string]map[string]string{
	LangEN: {
		BlockSynopsis:  "Synopsis",
		BlockAbout:     "About",
		BlockGenres:    "Genres",
		BlockPlatforms: "Platforms",
	},
	LangPTBR: {
		BlockSynopsis:  "Sinopse",
		BlockAbout:     "Sobre",
		BlockGenres:    "Gêneros",
		BlockPlatforms: "Plataformas",
	},
}

// heading returns the title of a block in the language, or the English one for an unknown
// language (a hand-written database row: the composition never fails because of it).
func heading(lang, block string) string {
	if h, ok := headings[lang][block]; ok {
		return h
	}
	return headings[LangEN][block]
}
