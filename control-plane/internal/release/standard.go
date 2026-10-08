package release

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// This file generates the standardized release name in the naming convention used
// by private trackers (the upload rules of UNIT3D sites, inherited from the "scene"):
//
//   Title Year [S01E01] Resolution [Streaming] Source AudioChannels VideoCodec [Flags]-GROUP
//
// The tags (BluRay, WEB-DL, x264, DDP, 5.1, …) are the common vocabulary of this convention.
// Missing fields are omitted; no group becomes NOGROUP. The operator checks and edits the name
// in the Portal before publishing — each site may have an extra requirement.

// tagRule maps a canonical tag (e.g., "BluRay") to the patterns that identify it.
type tagRule struct {
	label    string
	patterns []string
}

// In each list the first matching rule wins: the most specific pattern comes before the
// most generic ("dvdrip" before "dvd", "dts-hd" before "dts").
var (
	sourceRules = []tagRule{
		{"CAM", []string{"camrip", "cam-rip", "cam", "ts", "telesync", "pdvd"}},
		{"TC", []string{"tc", "telecine"}},
		{"SCREENER", []string{"dvdscr", "dvd-scr", "dvdscreener", "screener", "scr", "ddc"}},
		{"R5", []string{"r5"}},
		{"DVDRip", []string{"dvdrip", "dvd-rip"}},
		{"BDRip", []string{"bdrip", "bd-rip", "brrip", "br-rip"}},
		{"WEB-DL", []string{"webdl", "web-dl"}},
		{"WEBRip", []string{"webrip", "web-rip"}},
		{"DVD-R", []string{"dvdr", "dvd-r", "dvd-5", "dvd-9", "r6-dvd", "dvd"}},
		{"BluRay", []string{"bluray", "blu-ray", "bdr"}},
		{"BDSCR", []string{"bluray-scr", "bdscr"}},
		{"PDTV", []string{"pdtv"}},
		{"SDTV", []string{"sdtv", "dsr", "dsrip", "satrip", "dthrip", "dvbrip"}},
		{"HDTV", []string{"hdtvrip", "hdtv-rip", "hdtv"}},
	}
	subsourceRules = []tagRule{
		{"AMZN", []string{"amzn", "amazon"}},
		{"NF", []string{"nf", "netflix"}},
		{"VUDU", []string{"vudu"}},
		{"HULU", []string{"hulu"}},
		{"YT", []string{"yt", "youtube"}},
	}
	encodingRules = []tagRule{
		{"DivX", []string{"divx"}},
		{"XviD", []string{"xvid"}},
		{"x264", []string{"x264", "x.264", "x 264"}},
		{"x265", []string{"x265", "x.265", "x 265"}},
		{"H.264", []string{"h264", "h.264", "h 264"}},
		{"H.265", []string{"h265", "h.265", "h 265"}},
		{"AVC", []string{"avc"}},
		{"VC-1", []string{"vc-1", "vc1"}},
	}
	audioRules = []tagRule{
		{"DTS-HD", []string{"dts-hd"}},
		{"DTSHD-MA", []string{"dts-ma"}},
		{"DTS-X", []string{"dts-x", "dts x", "dtsx"}},
		{"DTS", []string{"dts"}},
		{"TRUEHD", []string{"truehd"}},
		{"ATMOS", []string{"atmos"}},
		{"DDP", []string{"ddp", "dd+"}}, // Dolby Digital Plus — the convention writes "DDP" (not "DD+")
		{"DD", []string{"dd"}},
		{"AAC", []string{"aac"}},
		{"FLAC", []string{"flac"}},
	}
	channelsRules = []tagRule{
		{"1.0", []string{"1.0", "1 0"}},
		{"2.0", []string{"2.0", "2 0"}},
		{"5.1", []string{"5.1", "5 1"}},
		{"6.1", []string{"6.1", "6 1"}},
		{"7.1", []string{"7.1", "7 1"}},
		{"9.1", []string{"9.1", "9 1"}},
	}
	// Release flags; the order does not matter (all that match are included).
	flagRules = []tagRule{
		{"PROPER", []string{"proper"}},
		{"REPACK", []string{"repack"}},
		{"EXTENDED", []string{"extended"}},
		{"REMUX", []string{"remux"}},
		{"UNRATED", []string{"unrated"}},
		{"LIMITED", []string{"limited"}},
		{"REMASTERED", []string{"remastered"}},
		{"UNCUT", []string{"uncut"}},
		{"COMPLETE", []string{"complete"}},
		{"INTERNAL", []string{"internal"}},
		{"DC", []string{"dc"}},
		{"3D", []string{"3d"}},
		{"IMAX", []string{"imax"}},
		{"HDR", []string{"hdr", "hdr10"}},
		// Language (Brazilian trackers): at the end of the name, after the video codec.
		{"DUAL", []string{"dual"}},
		{"DUBLADO", []string{"dublado"}},
		{"LEGENDADO", []string{"legendado", "legendas"}},
	}
)

// matchTag searches, in order, for the first tag whose pattern appears in the name (with
// token boundaries: space, dot, hyphen or edge). Returns "" if nothing matches.
func matchTag(lowerSpaced string, rules []tagRule) string {
	for _, r := range rules {
		for _, p := range r.patterns {
			if hasToken(lowerSpaced, p) {
				return r.label
			}
		}
	}
	return ""
}

// matchAll returns all matching tags (for flags).
func matchAll(lowerSpaced string, rules []tagRule) []string {
	var out []string
	for _, r := range rules {
		for _, p := range r.patterns {
			if hasToken(lowerSpaced, p) {
				out = append(out, r.label)
				break
			}
		}
	}
	return out
}

// hasToken checks if needle appears as a delimited token in hay. hay must be
// normalized (see normalizeForTags): brackets/parentheses/dots become spaces, only
// hyphens and alphanumerics remain — so the boundary is space, hyphen or edge.
func hasToken(hay, needle string) bool {
	// escapes the needle metacharacters (e.g., "dd+", "web-dl", "2 0").
	re := regexp.MustCompile(`(^|[ \-])` + regexp.QuoteMeta(needle) + `([ \-]|$)`)
	return re.MatchString(hay)
}

// reTagNorm normalizes separators to match tags:
// brackets/braces/parentheses/punctuation/space/dot/underscore → space; hyphen stays.
var reTagNorm = regexp.MustCompile(`[\[\]\(\)\{\}\;\:\!\s\\._]+`)

// reAudioGlued splits the audio codec glued to the channels (#187).
//
// The problem: "DDP5.1" goes through reTagNorm and becomes "ddp5 1". Then the codec is not
// delimited ("ddp5" ≠ "ddp") and the channel was split in half ("5 1" lost the left
// boundary) — BOTH fields are lost at once, and the operator had to fix it manually.
//
// The splitting is restricted to known codecs on purpose. Breaking every
// letter→digit boundary would fix "ddp5" and break legitimate "mp2"/"dd"; and breaking
// digit→letter would destroy "1080p".
var reAudioGlued = buildAudioGluedRe()

// buildAudioGluedRe derives the regex from audioRules itself — hardcoding the list here
// would desync on the day a new codec is added there.
func buildAudioGluedRe() *regexp.Regexp {
	var pats []string
	for _, r := range audioRules {
		for _, p := range r.patterns {
			if strings.Contains(p, " ") {
				continue // a pattern that already comes separated does not stick to a digit
			}
			pats = append(pats, p)
		}
	}
	// Longest first: regexp alternation is first-match, and "dd" before "ddp"
	// would leave the "p" out of the codec, silently swapping DDP for DD.
	sort.SliceStable(pats, func(i, j int) bool { return len(pats[i]) > len(pats[j]) })
	for i, p := range pats {
		pats[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile(`(^|[ \-])(` + strings.Join(pats, "|") + `)(\d)`)
}

// normalizeForTags returns the string ready to match tags (lower + separators → space,
// and the audio codec unglued from the channels).
func normalizeForTags(name string) string {
	s := strings.ToLower(reTagNorm.ReplaceAllString(name, " "))
	return reAudioGlued.ReplaceAllString(s, "$1$2 $3")
}

// ucwords capitalizes the first letter of each word (keeps the rest lowercase).
func ucwords(s string) string {
	parts := strings.Fields(strings.ToLower(s))
	for i, w := range parts {
		if w == "" {
			continue
		}
		r := []rune(w)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

// standardName builds the name in the naming convention, reading the fields already extracted in
// Info. Order (checked with an approved upload on a UNIT3D site, e.g.:
// "Better Watch Out 2017 1080p AMZN WEB-DL DDP5.1 H.264 DUAL-cza"):
//
//	Title Year [SxxEyy] Resolution [Streaming] Source Audio+Channels VideoCodec [Flags]-GROUP
//
// Separator = SPACE (not dot — UNIT3D sites reject or delay names with dots). The audio
// codec and channels are glued (DDP+5.1 = "DDP5.1"). Empty fields are omitted;
// no group → NOGROUP.
func standardName(info Info) string {
	var tokens []string
	// title (each word is a token) — Ucwords, separated by space.
	tokens = append(tokens, strings.Fields(ucwords(info.Title))...)
	push := func(s string) {
		if s != "" {
			tokens = append(tokens, s)
		}
	}
	if info.Year > 0 {
		push(strconv.Itoa(info.Year))
	}
	if se := seasonEpisode(info); se != "" { // S01E01, omitted for movies
		push(se)
	}
	push(info.Resolution)
	push(info.Subsource)             // streaming (AMZN/NF/DSNP…) before the source
	push(info.Source)                // WEB-DL / BluRay / HDTV
	push(info.Audio + info.Channels) // codec+channels joined: "DDP5.1", "DTS5.1", "2.0"
	push(info.Encoding)              // video codec (H.264/x265…) AFTER the source
	for _, f := range info.Flags {   // DUAL, REMUX, PROPER… at the end
		push(f)
	}

	group := info.Group
	if group == "" {
		group = "NOGROUP"
	}
	return strings.Join(tokens, " ") + "-" + group
}

// seasonEpisode formats S01E01 (episode optional); "" if not a series.
func seasonEpisode(info Info) string {
	if info.Season <= 0 {
		return ""
	}
	s := "S" + pad2(info.Season)
	if info.Episode > 0 {
		s += "E" + pad2(info.Episode)
	}
	return s
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
