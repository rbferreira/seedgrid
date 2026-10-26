// Package trackerprofile stores the SITE PROFILE of a tracker (ADR-037, layer 2): the
// knowledge that belongs to the site and not the operator. Today there are two sections: the taxonomy (the IDs for
// category, type and resolution) and the description (#192: which blocks make up the upload
// description, in what order, and the MediaInfo tag); name format, rules and image
// host policies go here in the following issues (#191, #193), as new sections of the same record.
//
// The three layers of ADR-037 and where each one lives:
//
//  1. family (API shape, auth, multipart) — Go code, internal/unit3d;
//  2. site profile (taxonomy, …) — THIS package, tracker_profiles table, exportable
//     (#197): it is what someone with an account on the site discovers, tweaks and shares;
//  3. operator config (api_token, announce with passkey, anon, modq) —
//     trackers table, internal/tracker, NEVER exported.
//
// Layer 3 is kept OUT by design: no type in this package has a field for
// credentials, and queries from here only touch the trackers table to check existence
// (id, deleted_at) and read the name. An export that leaks the api_token due to
// a forgotten filter would first need a field that does not exist.
//
// Profile is UNTRUSTED CONTENT. The (id, name) pairs come from three places —
// discovery (a third-party site catalog), editing in the Portal and, soon, the
// import of a file contributed by someone —, and none is trusted. Everything goes
// through Validate* before hitting the database: types and ranges checked, names without control
// characters or invisible formatting, limited sizes, axes from a closed set.
package trackerprofile

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rbferreira/seedgrid/control-plane/internal/description"
	"github.com/rbferreira/seedgrid/control-plane/internal/imagehost"
)

const (
	// FamilyUNIT3D is the only family in V1 (ADR-037). The field exists so that a new
	// family (Gazelle) does not require remodeling the record.
	FamilyUNIT3D = "unit3d"

	// SchemaVersion is the version of the profile FORMAT (what the export from #197 stamps and the
	// import checks). It goes up when the format changes in a way that an old import would not
	// understand: a field that changes shape OR a new section. Without this, the first shared
	// profile becomes legacy the day it happens — and a file with a new section on an old
	// SeedGrid would fail with "unknown field" instead of saying it is from a newer
	// version. The import accepts from MinSchemaVersion up to this value.
	//
	// Deliberate exception (#192): an OPTIONAL and ADDITIVE section, which the export omits when at
	// default, does not bump the version. The description section is like this — a profile without it remains
	// exactly the v1 file, and one with it is a file that a SeedGrid prior to
	// #192 rejects with "unknown field" (not with unsupported_schema_version). The cost is
	// this, and it was accepted to not invalidate every already shared file.
	SchemaVersion = 1
)

// Taxonomy axes. CLOSED set: a new axis is a code and schema_version change,
// not a free-form field that arrives in a JSON.
const (
	AxisCategory   = "category"
	AxisType       = "type"
	AxisResolution = "resolution"
)

// Axes lists the known axes, in stable order.
var Axes = []string{AxisCategory, AxisType, AxisResolution}

// Origin of a term: where it came from, which decides who can change it.
const (
	// OriginSeed: seeded by migration (installations prior to #63, when 00043 seeded
	// a site's profile). It was not typed by the operator, so discovery can propose the
	// name the site uses.
	OriginSeed = "seed"
	// OriginDiscovered: came from discovery in the site catalog. Ditto.
	OriginDiscovered = "discovered"
	// OriginManual: the operator typed or renamed it by hand. It is NEVER overwritten by
	// discovery — it is the guarantee that makes it safe to run discovery again.
	OriginManual = "manual"
	// OriginImported: came from a profile file (#197), contributed by someone. Only
	// the local importer assigns it — the origin does not travel in the file (see exchange.go).
	// For the merge it counts as seed/discovered: it was not typed by THIS operator, so
	// discovery can propose the name their site actually uses. Distinct from the other two
	// because the operator needs to know the id came from outside and their site did not confirm it.
	OriginImported = "imported"
)

// Kind is the semantic role of a category — what the Portal needs to know beyond the number
// (show season/episode, search for game on IGDB). Empty = category without special
// behavior. It only exists on the category axis.
const (
	KindMovie = "movie"
	KindTV    = "tv"
	KindGame  = "game"
)

// Validation limits (untrusted content).
const (
	// MaxTermID: UNIT3D IDs are auto-increment from tiny tables (tens). A
	// 1 million ceiling does not hinder any real site and blocks junk and overflows.
	MaxTermID       = 1_000_000
	MaxTermsPerAxis = 256
	MaxNameRunes    = 64
)

// Term is a taxonomy entry: the site's (id, name) pair, plus what SeedGrid knows
// about it.
type Term struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
	// Origin is LOCAL state (who can change the name HERE). It is not part of the
	// exchange format: the export zeroes it out (omitempty) and the import rejects a file that brings it.
	Origin string `json:"origin,omitempty"`
}

// Taxonomy is axis → terms, sorted by id.
type Taxonomy map[string][]Term

// Profile is the site profile, as the API returns it. NO credentials, by design.
//
// Description (#192) is optional: nil = the default (the composition the Portal has always made), and it is
// omitted from the JSON — and therefore from the exchange file — while it is nil. When present, it has already
// gone through CleanDescription: it is always a valid and explicit Config (all lists and the tag
// filled in).
//
// ImageHost (#193) is optional: nil = no own image host, and it is omitted from the JSON while
// it is nil. When present, it has already gone through Validate: it is always a valid Spec.
type Profile struct {
	Family        string              `json:"family"`
	SchemaVersion int                 `json:"schema_version"`
	Taxonomy      Taxonomy            `json:"taxonomy"`
	Description   *description.Config `json:"description,omitempty"`
	ImageHost     *imagehost.Spec     `json:"image_host,omitempty"`
}

// Empty is the profile of a tracker that does not have anything yet: all axes present and
// empty (never null — the Portal iterates without checking).
func Empty() Profile {
	return Profile{Family: FamilyUNIT3D, SchemaVersion: SchemaVersion, Taxonomy: normalize(nil)}
}

// normalize ensures that all known axes exist as a list (never nil) and
// sorts each list by id. Unknown axes are preserved: a line written by
// a newer version cannot lose data just by passing through an older one.
func normalize(t Taxonomy) Taxonomy {
	out := make(Taxonomy, len(Axes))
	for axis, terms := range t {
		out[axis] = append([]Term{}, terms...)
	}
	for _, axis := range Axes {
		if out[axis] == nil {
			out[axis] = []Term{}
		}
	}
	for _, terms := range out {
		sort.SliceStable(terms, func(i, j int) bool { return terms[i].ID < terms[j].ID })
	}
	return out
}

// TermInput is a term as the operator sends it (editing). No origin: the origin is decided
// by the server, comparing with what is already recorded — the client does not say where it came from.
type TermInput struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

// ValidationError is an input rejection, with the field path (taxonomy.type[2].name).
// The Portal shows it as received.
type ValidationError struct {
	Path string
	Msg  string
	// Code distinguishes rejections that the client handles differently (future format version,
	// incompatible family, body too large). Empty = invalid_profile.
	Code string
}

func (e *ValidationError) Error() string { return e.Path + ": " + e.Msg }

// Error codes for an input rejection.
const (
	CodeInvalid            = "invalid_profile"
	CodeUnsupportedVersion = "unsupported_schema_version"
	CodeIncompatibleFamily = "incompatible_family"
	CodeTooLarge           = "payload_too_large"
	CodeNotExportable      = "profile_not_exportable"
)

// ErrorCode returns the rejection code for the HTTP response.
func (e *ValidationError) ErrorCode() string {
	if e.Code == "" {
		return CodeInvalid
	}
	return e.Code
}

func invalidCode(code, path, format string, args ...any) *ValidationError {
	return &ValidationError{Path: path, Msg: fmt.Sprintf(format, args...), Code: code}
}

func invalid(path, format string, args ...any) *ValidationError {
	return &ValidationError{Path: path, Msg: fmt.Sprintf(format, args...)}
}

// KnownAxis reports whether the axis is one of the known ones.
func KnownAxis(axis string) bool {
	for _, a := range Axes {
		if a == axis {
			return true
		}
	}
	return false
}

// CleanName normalizes and validates a term name: valid UTF-8, collapsed spaces,
// 1 to MaxNameRunes characters, no control or invisible formatting characters.
//
// The name is just a label (the POST to the tracker carries the ID), but a hostile label still does damage:
// the direction override (U+202E) reverses text on screen, zero-width makes two
// "identical" terms different, and control breaks logs and terminals. Blocking all this at input
// is cheaper than remembering to escape it at every output.
func CleanName(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", fmt.Errorf("invalid UTF-8")
	}
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" {
		return "", fmt.Errorf("empty name")
	}
	if n := utf8.RuneCountInString(name); n > MaxNameRunes {
		return "", fmt.Errorf("name with %d characters (maximum %d)", n, MaxNameRunes)
	}
	for _, r := range name {
		switch {
		case unicode.IsControl(r):
			return "", fmt.Errorf("contains control character (U+%04X)", r)
		case unicode.Is(unicode.Cf, r):
			return "", fmt.Errorf("contains invisible formatting character (U+%04X)", r)
		case unicode.Is(unicode.Co, r), unicode.Is(unicode.Cs, r), r == utf8.RuneError:
			return "", fmt.Errorf("contains invalid character (U+%04X)", r)
		case blankLooking[r]:
			return "", fmt.Errorf("contains character that looks like whitespace but is not a space (U+%04X)", r)
		}
	}
	return name, nil
}

// blankLooking are runes that are NOT spaces (strings.Fields does not collapse them), NOT
// control or formatting (Cc/Cf categories do not catch them) and yet draw nothing:
// Hangul fillers, empty Braille, Khmer inherent vowels, the grapheme
// joiner. A name made only of them looks empty on screen and passes "1 to 64 characters"; in the middle of
// a name, it makes two "identical" terms distinct.
var blankLooking = map[rune]bool{
	0x034F: true, // COMBINING GRAPHEME JOINER
	0x115F: true, // HANGUL CHOSEONG FILLER
	0x1160: true, // HANGUL JUNGSEONG FILLER
	0x17B4: true, // KHMER VOWEL INHERENT AQ
	0x17B5: true, // KHMER VOWEL INHERENT AA
	0x2800: true, // BRAILLE PATTERN BLANK
	0x3164: true, // HANGUL FILLER
	0xFFA0: true, // HALFWIDTH HANGUL FILLER
}

func validOrigin(o string) bool {
	return o == OriginSeed || o == OriginDiscovered || o == OriginManual || o == OriginImported
}

// yieldsToDiscovery says whether discovery can propose another name for a term from this source.
// FAIL CLOSED: only the three known sources not typed by the operator yield;
// manual, and any value this code does not know (a line written by another version,
// or by hand in the database), is treated as protected — the error must fall on the side of not touching it.
func yieldsToDiscovery(origin string) bool {
	return origin == OriginSeed || origin == OriginDiscovered || origin == OriginImported
}

func validateID(id int) error {
	if id < 1 || id > MaxTermID {
		return fmt.Errorf("id %d out of range 1..%d", id, MaxTermID)
	}
	return nil
}

func validateKind(axis, kind string) error {
	if kind == "" {
		return nil
	}
	if axis != AxisCategory {
		return fmt.Errorf("kind only exists on the %s axis", AxisCategory)
	}
	switch kind {
	case KindMovie, KindTV, KindGame:
		return nil
	}
	return fmt.Errorf("unknown kind %q (movie, tv or game)", kind)
}

// CleanDescription checks the description section (#192) and returns it NORMALIZED — explicit lists and tag,
// never nil. It is the only entry point for the section: editing, restoring, and importing go
// through here before anything touches the database, because Config can come from a third-party
// file and the tag is interpolated in BBCode. Rejects with *ValidationError (400 invalid_profile),
// with the field path: description.media_blocks[1], description.media_info_tag.
func CleanDescription(c description.Config) (description.Config, error) {
	if err := c.Validate(); err != nil {
		var de *description.Error
		if errors.As(err, &de) {
			return description.Config{}, invalid("description."+de.Path, "%s", de.Msg)
		}
		return description.Config{}, invalid("description", "%v", err)
	}
	return c.Normalized(), nil
}

// CleanImageHost checks the image_host section (#193) and returns it ready for use. It is the only
// entry point for the section: editing, restoring, and importing go through here before anything
// touches the database, because Spec can come from a third-party file and the fields end up in
// HTTP headers and publish URLs. The name comes out normalized by CleanName (the same rule
// as term names: collapsed spaces, no control or invisible). Rejects with
// *ValidationError (400 invalid_profile), with the field path (image_host.upload_url, …).
func CleanImageHost(s imagehost.Spec) (imagehost.Spec, error) {
	name, err := CleanName(s.Name)
	if err != nil {
		return imagehost.Spec{}, invalid("image_host.name", "%v", err)
	}
	s.Name = name
	if err := s.Validate(); err != nil {
		var fe *imagehost.FieldError
		if errors.As(err, &fe) {
			return imagehost.Spec{}, invalid("image_host."+fe.Field, "%s", fe.Msg)
		}
		return imagehost.Spec{}, invalid("image_host", "%v", err)
	}
	return s, nil
}

// ValidateInput checks a taxonomy edit and returns the already cleaned terms. An axis
// missing from the map is left out (the edit does not touch it); a present axis, even empty,
// REPLACES the current one — hence "missing" and "empty" are different things and no client
// deletes an axis out of forgetfulness.
func ValidateInput(in map[string][]TermInput) (map[string][]TermInput, error) {
	out := make(map[string][]TermInput, len(in))
	for _, axis := range sortedKeys(in) {
		path := "taxonomy." + axis
		if !KnownAxis(axis) {
			return nil, invalid("taxonomy", "unknown axis %q", truncate(axis, 32))
		}
		terms := in[axis]
		if len(terms) > MaxTermsPerAxis {
			return nil, invalid(path, "%d terms (maximum %d)", len(terms), MaxTermsPerAxis)
		}
		seen := make(map[int]bool, len(terms))
		clean := make([]TermInput, 0, len(terms))
		for i, t := range terms {
			tp := fmt.Sprintf("%s[%d]", path, i)
			if err := validateID(t.ID); err != nil {
				return nil, invalid(tp+".id", "%v", err)
			}
			if seen[t.ID] {
				return nil, invalid(tp+".id", "id %d repeated in axis", t.ID)
			}
			seen[t.ID] = true
			name, err := CleanName(t.Name)
			if err != nil {
				return nil, invalid(tp+".name", "%v", err)
			}
			if err := validateKind(axis, t.Kind); err != nil {
				return nil, invalid(tp+".kind", "%v", err)
			}
			clean = append(clean, TermInput{ID: t.ID, Name: name, Kind: t.Kind})
		}
		out[axis] = clean
	}
	return out, nil
}

// ApplyEdit applies a validated edit over the current taxonomy and decides the source of each
// term: what the operator did not touch (same id, same name) keeps its source; what they
// created or renamed becomes manual — and becomes protected from discovery. Changing only the kind
// does not change the source: discovery never touches kind, so there is nothing to protect.
func ApplyEdit(current Taxonomy, in map[string][]TermInput) Taxonomy {
	out := normalize(current)
	for axis, terms := range in {
		before := make(map[int]Term, len(out[axis]))
		for _, t := range out[axis] {
			before[t.ID] = t
		}
		next := make([]Term, 0, len(terms))
		for _, t := range terms {
			origin := OriginManual
			if old, ok := before[t.ID]; ok && old.Name == t.Name && validOrigin(old.Origin) {
				origin = old.Origin
			}
			next = append(next, Term{ID: t.ID, Name: t.Name, Kind: t.Kind, Origin: origin})
		}
		out[axis] = next
	}
	return normalize(out)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// truncate cuts s to n characters to fit in an error message: the value can be arbitrary-sized
// garbage coming from outside and must not be returned or logged in full.
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}
