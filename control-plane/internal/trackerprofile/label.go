package trackerprofile

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// NormalizeLabel is the Portal's label comparison (`normalizeLabel` in lib/taxonomy.ts): NFD,
// without diacritics (U+0300 to U+036F), lowercased and with collapsed spaces. This is how
// the publishing panel matches "1080p" or "WEB-DL" with the profile term — so two
// terms from the same axis whose normalized form is identical are, for the panel, the SAME label, and
// it has no way to choose between the two ids.
//
// The server uses the same rule to reject, in an imported file, two names that only differ in
// casing or diacritics, and to warn when a new term would split the name with one that the
// operator already has. Both sides must agree: change one, change the other (there is a parity
// test with the examples from taxonomy.test.ts).
func NormalizeLabel(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if r >= 0x0300 && r <= 0x036F {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(strings.ToLower(b.String())), " ")
}
