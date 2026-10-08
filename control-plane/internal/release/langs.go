package release

import (
	"regexp"
	"strings"
)

// Heuristic parsing of languages/subtitles from the release NAME (UNIT3D doesn't return
// these fields on browse and fetching mediainfo per item would be 50 requests/page on the
// tracker). It is CONSERVATIVE: if no clear marker is found, we return empty — the UI
// prefers showing nothing rather than showing wrong information.
//
// PT/BR scene token conventions (Brazilian trackers):
//   DUAL / DUAL AUDIO         → two audios (assumes PT-BR + original)
//   MULTI / MULTI2/3/4        → multiple audios (we don't know WHICH)
//   DUB / DUBLADO / NACIONAL  → PT-BR dubbed
//   LEG / LEGENDADO           → PT-BR subbed
//   VOSTFR                    → French with subtitles
//   SDH                       → subtitles for the deaf (English, usually)
//   SUBBED                    → subbed (English, usually)
//
// Note: DUAL/MULTI only reveal the NUMBER of tracks, not the languages. We mark
// it as "MULTI" in the UI (agnostic chip) so we don't invent a flag.

var (
	reDual     = regexp.MustCompile(`(?i)\b(dual[\s._-]?audio|dual)\b`)
	reMultiN   = regexp.MustCompile(`(?i)\bmulti[\s._-]?(\d)?\b`)
	reDubbed   = regexp.MustCompile(`(?i)\b(dub(bed|lado)?|nacional)\b`)
	reLegend   = regexp.MustCompile(`(?i)\b(leg(endado)?)\b`)
	reVOSTFR   = regexp.MustCompile(`(?i)\bvostfr\b`)
	reSDH      = regexp.MustCompile(`(?i)\bsdh\b`)
	reSubbed   = regexp.MustCompile(`(?i)\bsub(bed|s)?\b`)
	reResoluti = regexp.MustCompile(`(?i)\b(4320p|2160p|1080p|720p|576p|480p|360p)\b`)
	re4K       = regexp.MustCompile(`(?i)\b4k\b`)
)

// ParseAudioLangs extracts the list of audio tracks inferable from the name. Returns
// user-friendly labels ready for chips: "PT-BR DUB", "MULTI", etc. No recognized
// marker: empty slice (the UI omits the block).
func ParseAudioLangs(name string) []string {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	// DUB/NACIONAL alone is already a strong indicator of PT-BR (BR scene standard).
	if reDubbed.MatchString(name) {
		add("PT-BR DUB")
	}
	// DUAL: PT-BR + original — the PT-BR assumption comes from the context (BR tracker).
	if reDual.MatchString(name) {
		add("PT-BR DUB")
		add("Original")
	}
	// MULTI: uncertain number of tracks of unknown languages. Marks only as MULTI to
	// avoid inventing a flag that might be wrong.
	if reMultiN.MatchString(name) {
		add("MULTI")
	}
	return out
}

// ParseSubtitleLangs extracts the list of inferable subtitles. Same caution as audio.
func ParseSubtitleLangs(name string) []string {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		if seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	if reLegend.MatchString(name) {
		add("PT-BR LEG")
	}
	if reVOSTFR.MatchString(name) {
		add("FR LEG")
	}
	if reSDH.MatchString(name) {
		add("SDH")
	}
	// SUBBED is commonly English (international releases). We mark "EN SUB" only if
	// no PT-BR marker came along — avoids noise.
	if reSubbed.MatchString(name) && !reLegend.MatchString(name) {
		add("EN SUB")
	}
	return out
}

// ParseResolution normalizes the resolution extracted from the name ("4k" → "2160p"). Fallback
// for when UNIT3D doesn't return the resolution field in the filter (varies by tracker).
// "" if not found.
func ParseResolution(name string) string {
	if m := reResoluti.FindString(name); m != "" {
		return strings.ToLower(m)
	}
	if re4K.MatchString(name) {
		return "2160p"
	}
	return ""
}
