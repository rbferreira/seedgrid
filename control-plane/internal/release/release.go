// Package release performs heuristic parsing of a release name ("scene" standard:
// Title.2023.1080p.WEB-DL.x264-GROUP) and maps it to the fields the upload API
// form requests (type, resolution, title, year, season/episode). Type and
// resolution are output as LABELS ("WEB-DL", "1080p"): each site numbers its own way, and
// the site profile translates the label to the site ID (ADR-037).
// It is heuristic by nature (ADR-020) — human review in the Portal is mandatory.
package release

import (
	"regexp"
	"strconv"
	"strings"
)

// Info is the result of parsing a release name.
type Info struct {
	Title      string `json:"title"`      // clean title (no year/tags)
	Year       int    `json:"year"`       // 0 if unknown
	Season     int    `json:"season"`     // 0 if not a series/pack
	Episode    int    `json:"episode"`    // 0 if there is no episode
	IsTV       bool   `json:"is_tv"`      // series (has S/E) vs movie
	Resolution string `json:"resolution"` // display label (e.g., "1080p")
	Type       string `json:"type"`       // display label (e.g., "WEB-DL")
	Group      string `json:"group"`      // release group (after "-")
	IMDB       string `json:"imdb"`       // e.g., tt0387808, when embedded in the name (Radarr/Plex)

	// Part: marker for a part of a divided work ("Phase 1", "Vol 2", "Part 3").
	//
	// It exists because `reAltDash` cuts everything after " - " to apply the
	// tracker rule (only the international title, not the translated one), and swallowed the marker:
	// "The Batman Saga - Phase 1" and "- Phase 2" became the SAME title, and Analytics
	// flagged one as an extra 3.25 GB copy to release (#172). They are different works.
	//
	// It stays OUT of `Title` on purpose: those fetching cover and metadata want the clean title.
	// Those who need to distinguish parts — the duplicate key — read this field.
	Part string `json:"part"` // "" when there is no marker

	// Release naming convention fields (standard.go) to generate the standardized name.
	Source       string   `json:"source"`        // BluRay, WEB-DL, HDTV… (source)
	Encoding     string   `json:"encoding"`      // x264, x265, H.264…
	Subsource    string   `json:"subsource"`     // AMZN, NF, HULU…
	Audio        string   `json:"audio"`         // DTS, DDP, FLAC…
	Channels     string   `json:"channels"`      // 2.0, 5.1…
	Flags        []string `json:"flags"`         // PROPER, REPACK, REMUX…
	StandardName string   `json:"standard_name"` // name in the naming standard (standardName)
}

var (
	reSepRun    = regexp.MustCompile(`[._]+`)
	reSpaces    = regexp.MustCompile(`\s+`)
	reSeasonEp  = regexp.MustCompile(`(?i)\bS(\d{1,2})[\s.]?E(\d{1,3})\b`)
	reSeasonPk  = regexp.MustCompile(`(?i)\bS(\d{1,2})\b`)
	reSeasonWrd = regexp.MustCompile(`(?i)\b(?:season|temporada)[\s.]?(\d{1,2})\b`)
	reYear      = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)
	reGroup     = regexp.MustCompile(`-([A-Za-z0-9]+)$`)
	reIMDB      = regexp.MustCompile(`(?i)tt(\d{7,8})`)
	reBrackets  = regexp.MustCompile(`[\(\[\{][^\)\]\}]*[\)\]\}]`) // groups ( ), [ ], { }
	// AKA ("also known as"): UNIT3D sites usually PROHIBIT alternative titles in the name — use only the
	// international title (before AKA). Cuts " AKA ..." (and "a k a", from "a.k.a.")
	// to the end of the title. E.g.: "In Your Name AKA In Your Name" → "In Your Name".
	reAKA = regexp.MustCompile(`(?i)\s+(?:aka|a k a)\s+.*$`)
	// PART marker after the hyphen. Requires the marker word followed by a number:
	// "- Phase 2" is another work, "- In Your Name" is a translated title, and only the number safely
	// separates the two cases. Without the number, treating it as an alternative title (which
	// `reAltDash` already does) is the correct behavior.
	rePart = regexp.MustCompile(`(?i)\s+-\s+(fase|parte|part|volume|vol|livro|book|cap[íi]tulo|cap|ato|act|disco|disc|cd)\s*\.?\s*(\d{1,3})\b`)
	// Alternative title via hyphen: common convention in BR releases "Original Title -
	// Translated Title" (no "AKA" marker). Same tracker rule (only international
	// title) — cuts from the first " - " (hyphen surrounded by spaces, not to be
	// confused with attached hyphens like in "Spider-Man"). E.g.: "Ender's Game - Ender's
	// Game" → "Ender's Game". Heuristic (ADR-020): human review in the
	// Portal covers the rare case of a legitimate official subtitle cut by mistake.
	reAltDash = regexp.MustCompile(`\s+-\s+.*$`)
)

// typeToken maps the markers of a release type to its label (evaluated in order).
type typeToken struct {
	needles []string
	label   string
}

// Order matters: "remux" before "bluray" (BD-Remux); "web-dl" before "webrip".
var typeTokens = []typeToken{
	{[]string{"remux"}, "Remux"},
	{[]string{"web-dl", "webdl", "web dl"}, "WEB-DL"},
	{[]string{"webrip", "web-rip", "web rip"}, "WEBRip"},
	{[]string{"hdtv"}, "HDTV"},
	{[]string{"bluray", "blu-ray", "bdrip", "brrip", "x264", "x265", "h264", "h265", "h.264", "h.265", "hevc", "avc"}, "Encode"},
}

// resTokenT maps the markers of a resolution to its label.
type resTokenT struct {
	needles []string
	label   string
}

var resTokens = []resTokenT{
	{[]string{"4320p", "8k"}, "4320p"},
	{[]string{"2160p", "4k", "uhd"}, "2160p"},
	{[]string{"1080p", "1080i"}, "1080p"},
	{[]string{"720p"}, "720p"},
	{[]string{"480p", "576p"}, "480p"},
}

// Parse extracts the fields from a release name. It never fails: whatever it doesn't recognize
// is left zeroed (the Portal fills/adjusts it).
func Parse(name string) Info {
	raw := stripExt(strings.TrimSpace(name))
	// Normalizes "scene" separators (dots/underscores) to spaces, preserving
	// the raw version to find the group (after the "-").
	spaced := reSpaces.ReplaceAllString(reSepRun.ReplaceAllString(raw, " "), " ")
	lower := strings.ToLower(spaced)

	var info Info

	// Release group: "-GROUP" suffix in the raw name.
	if m := reGroup.FindStringSubmatch(strings.TrimSpace(raw)); m != nil {
		info.Group = m[1]
	}
	// IMDB embedded in the name (Radarr/Plex standard: {imdb-tt0387808}).
	if m := reIMDB.FindStringSubmatch(raw); m != nil {
		info.IMDB = "tt" + m[1]
	}

	// Season/episode (SxxEyy) → series; otherwise season pack (Sxx) or "Season N".
	tvIdx := -1
	if m := reSeasonEp.FindStringSubmatchIndex(spaced); m != nil {
		info.IsTV = true
		info.Season, _ = strconv.Atoi(spaced[m[2]:m[3]])
		info.Episode, _ = strconv.Atoi(spaced[m[4]:m[5]])
		tvIdx = m[0]
	} else if m := reSeasonWrd.FindStringSubmatchIndex(spaced); m != nil {
		info.IsTV = true
		info.Season, _ = strconv.Atoi(spaced[m[2]:m[3]])
		tvIdx = m[0]
	} else if m := reSeasonPk.FindStringSubmatchIndex(spaced); m != nil {
		info.IsTV = true
		info.Season, _ = strconv.Atoi(spaced[m[2]:m[3]])
		tvIdx = m[0]
	}

	// Year: first plausible year.
	yearIdx := -1
	if m := reYear.FindStringSubmatchIndex(spaced); m != nil {
		info.Year, _ = strconv.Atoi(spaced[m[2]:m[3]])
		yearIdx = m[0]
	}

	// Resolution and type by scanning tokens (in lower-case).
	for _, r := range resTokens {
		if containsAny(lower, r.needles) {
			info.Resolution = r.label
			break
		}
	}
	for _, tt := range typeTokens {
		if containsAny(lower, tt.needles) {
			info.Type = tt.label
			break
		}
	}

	// Title: everything before the first marker (year or S/E), whichever comes first.
	cut := len(spaced)
	if yearIdx >= 0 && yearIdx < cut {
		cut = yearIdx
	}
	if tvIdx >= 0 && tvIdx < cut {
		cut = tvIdx
	}
	// The part comes OUT before cleanTitle, which would discard it along with the alternative title.
	if m := rePart.FindStringSubmatch(spaced[:cut]); m != nil {
		info.Part = ucwords(strings.ToLower(m[1])) + " " + m[2]
	}
	info.Title = cleanTitle(spaced[:cut])
	if info.Title == "" {
		// No markers: uses the entire clean name (minus the group).
		info.Title = cleanTitle(stripGroup(spaced))
	}

	// Release naming convention (standard.go): extracts source/encoding/subsource/audio/
	// channels/flags from a string with normalized brackets/parentheses (like the
	// clean() in parser.js) and generates the standardized name (stringify).
	tagStr := normalizeForTags(raw)
	info.Source = matchTag(tagStr, sourceRules)
	info.Encoding = matchTag(tagStr, encodingRules)
	info.Subsource = matchTag(tagStr, subsourceRules)
	info.Audio = matchTag(tagStr, audioRules)
	info.Channels = matchTag(tagStr, channelsRules)
	info.Flags = matchAll(tagStr, flagRules)
	info.StandardName = standardName(info)
	return info
}

// containsAny reports if hay contains any of the needles (simple substring).
func containsAny(hay string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

// cleanTitle normalizes the title: removes groups inside brackets/parentheses (Radarr/Plex
// standard), collapses spaces and trims loose punctuation/brackets. It does NOT strip the group
// suffix (reGroup) — the caller already cut the name before the year/season, so a trailing hyphen
// here is part of the title (e.g., "Spider-Man"), not a release group; the
// no-marker path (below) already applies stripGroup explicitly before calling.
func cleanTitle(s string) string {
	s = reAKA.ReplaceAllString(s, "")       // cuts alternative title "AKA ..." (tracker rule)
	s = reAltDash.ReplaceAllString(s, "")   // cuts alternative title "- Translated" (same rule)
	s = reBrackets.ReplaceAllString(s, " ") // remove entire ( ), [ ], { }
	s = reSpaces.ReplaceAllString(s, " ")
	// trims loose punctuation/brackets at the edges (e.g., "Idiocracy (" -> "Idiocracy").
	return strings.TrimSpace(strings.Trim(s, " -._([{)]}"))
}

func stripGroup(s string) string {
	return reGroup.ReplaceAllString(strings.TrimSpace(s), "")
}

var reExt = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|ts|m2ts|iso|nfo|srt|mov|wmv|flac|mp3|m4v|webm)$`)

// stripExt removes a known media extension from the end of the name.
func stripExt(s string) string {
	return reExt.ReplaceAllString(strings.TrimSpace(s), "")
}
