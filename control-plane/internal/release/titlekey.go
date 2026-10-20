package release

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// TitleKey identifies the WORK of a release, regardless of the version: two copies of the same
// movie in 720p and 1080p, or in the same 1080p with another codec, have the same key. It is the
// Control Plane's only definition of "same title" — the Analytics duplicates panel (#172) and the
// other-version warning on add (#37) read it from here, so they never disagree.
//
// Resolution stays OUT on purpose: it is precisely what tells the versions apart. PART goes in:
// "The Batman Saga - Phase 1" and "- Phase 2" are different works, and cleanTitle cuts both down
// to the same title when applying the tracker's alternative-title rule (#172).
type TitleKey struct {
	Title string // normalized title (lowercase, no accents, no punctuation)
	Year  int    // 0 = unknown
	SE    string // "S02", "S02E05" or ""
	Part  string // "phase 1", "vol 2" or ""
}

// KeyOf extracts the title key from a release name.
//
// The title is normalized beyond what Parse yields because the two sides of the comparison write
// the same title differently: the tracker catalog uses the human title ("Out of the Shadows:
// Part 2", "Amélie") and the file name, the scene form ("Out.Of.The.Shadows", "Amelie"). With no
// recognizable title, it falls back to the whole normalized name — never to an empty key, which
// would lump together everything that wasn't recognized.
func KeyOf(name string) TitleKey {
	i := Parse(name)
	title := normalizeTitle(i.Title)
	if title == "" {
		title = normalizeTitle(name)
	}
	se := ""
	if i.Season > 0 {
		se = fmt.Sprintf("S%02d", i.Season)
		if i.Episode > 0 {
			se += fmt.Sprintf("E%02d", i.Episode)
		}
	}
	return TitleKey{Title: title, Year: i.Year, SE: se, Part: strings.ToLower(i.Part)}
}

// SameWork says whether the two keys are the same work. A year missing on ONE side doesn't
// prevent the match: series file names often come without a year, and the catalog with one. Two
// years present and different keep them apart ("Dune" 1984 × 2021).
func (k TitleKey) SameWork(o TitleKey) bool {
	if k.Title == "" || k.Title != o.Title || k.SE != o.SE || k.Part != o.Part {
		return false
	}
	return k.Year == 0 || o.Year == 0 || k.Year == o.Year
}

// normalizeTitle reduces a title to lowercase words, without accents or punctuation, separated by
// a single space.
func normalizeTitle(s string) string {
	var b strings.Builder
	space := false
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		switch {
		case unicode.Is(unicode.Mn, r): // decomposed accent: dropped
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		default:
			space = true
		}
	}
	return b.String()
}
