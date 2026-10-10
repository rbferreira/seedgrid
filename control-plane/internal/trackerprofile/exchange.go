package trackerprofile

// The profile EXCHANGE format (ADR-037, #197): the file that a user with an account on the site
// exports and the next user imports without ever having needed an account anywhere.
//
// Two properties dictate the design:
//
//  1. Layer 3 (api_token, announce with passkey, anon, modq) is kept out BY DESIGN. The
//     Document is the Profile plus an identification envelope (format and site) — and neither
//     type has a field capable of carrying a credential. There is no serialization filter
//     for someone to forget: the export is json.Marshal of the type, and the type has nowhere
//     for the secret to go. Two tests guard this (reflection on the type; textual search for
//     the values in a fully filled tracker export).
//
//  2. The Document EMBEDS the Profile instead of listing the fields. A new profile section
//     (name format, rules, description, image host) enters the export WITHOUT touching this
//     file — and, for the same reason, no one needs to remember to export it. What does NOT come
//     for free is the import: a section is only applicable when it has validation and merge rules
//     (handledSections below), and a section without them is REJECTED at runtime (fail closed) and
//     failed by tests, instead of being accepted unchecked.
//
// The file is UNTRUSTED CONTENT (ADR-037): ParseDocument is the only entry point
// and checks format, version, family, keys, types, ranges and sizes before anything
// else touches the data. Nothing from the file becomes a request to the tracker — names are
// just labels and ids only travel in the publish POST after passing through here.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// FormatID identifies the file as a SeedGrid tracker profile. Without it, any JSON
	// (or from another tool) that happened to have the same keys would be accepted.
	FormatID = "seedgrid.tracker-profile"

	// MinSchemaVersion is the oldest format version that the import still understands. Bumping the
	// SchemaVersion without lowering this value forces writing the upgrade from the previous version.
	MinSchemaVersion = 1

	// MaxDocumentBytes limits the file. Three full axes (256 terms of 64 characters)
	// weigh ~270 KiB indented; the ceiling covers this and blocks the hostile body.
	MaxDocumentBytes = 512 << 10

	// maxDocumentDepth: document > site|taxonomy|description > axis|list > term = 4 levels
	// (description has 3). A future section fits in 8; a JSON with thousands of nested
	// brackets does not.
	maxDocumentDepth = 8

	maxHostLen = 253
)

// Site identifies where the profile came from — just identification, so the operator knows what
// they are importing and so the import warns when the file is from another site. Public name and host;
// never an API address nor announce.
type Site struct {
	Name string `json:"name"`
	Host string `json:"host,omitempty"`
}

// Document is the exchange file: the envelope (format and site) plus the Profile, which the JSON
// flattens (family, schema_version, taxonomy and whatever else the profile gains).
type Document struct {
	Format string `json:"format"`
	Site   Site   `json:"site"`
	Profile
}

// handledSections lists the CONTENT sections of the Profile that the import knows how to validate and
// apply. The export does not depend on this list — it exists so that a new section is never
// imported without validation (see unhandledSection and the test that covers it).
//
//   - taxonomy: validated by ValidateInput (and the repeated name rule), applied term by
//     term by PlanImport/ApplyImport;
//   - description (#192): validated by CleanDescription, applied as ONE unit by
//     planDescription/ApplyImportDescription — the section has no per-term origin, so the rule
//     is the manual one: what the operator configured is only changed if they ask.
//   - image_host (#193): validated by CleanImageHost, applied as ONE unit by
//     planImageHost/ApplyImportImageHost — same rule as description.
var handledSections = map[string]bool{"taxonomy": true, "description": true, "image_host": true}

// envelopeFields are the Profile fields that identify the document, they are not a section.
var envelopeFields = map[string]bool{"family": true, "schema_version": true}

// NewDocument builds the exchange file for a profile. What is local to the installation (the origin
// of each term) is zeroed out: the exchange format does not carry it.
func NewDocument(trackerName, trackerHost string, p Profile) Document {
	p.Taxonomy = withoutLocalState(p.Taxonomy)
	// The file carries the FORMAT version that this code emits, not the one from the database row: the
	// document is rendered now, with the sections that this code knows.
	p.SchemaVersion = SchemaVersion
	host := publicHost(trackerHost)
	return Document{
		Format:  FormatID,
		Site:    Site{Name: siteLabel(trackerName, host), Host: host},
		Profile: p,
	}
}

// withoutLocalState copies the taxonomy without the origin. Only the known axes: an axis that
// this import would reject must not go out in the export.
func withoutLocalState(t Taxonomy) Taxonomy {
	out := make(Taxonomy, len(Axes))
	for _, axis := range Axes {
		terms := append([]Term{}, t[axis]...)
		for i := range terms {
			terms[i].Origin = ""
		}
		out[axis] = terms
	}
	return normalize(out)
}

// Marshal serializes the human-readable file: indented (PR review and diff) and without escaping
// <, > and & ("Comics & Manga" stays as is).
func (d Document) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Filename is the suggested name for the download: only [a-z0-9-], derived from the host (or the name),
// so it never carries a character that the Content-Disposition header treats as structure.
func (d Document) Filename() string {
	base := d.Site.Host
	if base == "" {
		base = d.Site.Name
	}
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(base) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "tracker"
	}
	return "seedgrid-profile-" + d.Family + "-" + slug + ".json"
}

// siteLabel is the site name in the file, from the label the operator gave the tracker.
//
// The name is FREE text from the operator, and whoever pastes the entire announce there (or a piece of it) leaks the
// passkey in the file that will be shared. Therefore, a name that fails CleanName OR
// that seems to contain a credential (a slash, "?" or "=", or an alphanumeric sequence of 20
// or more characters) is NEITHER cleaned nor truncated — truncating a 90-character announce would leave
// the first ~27, passkey included, in the file: the name is replaced by the public host (if it fits)
// or by "tracker". The export always generates a file that the import itself accepts.
func siteLabel(raw, host string) string {
	if name, err := CleanName(raw); err == nil && !looksLikeCredential(name) {
		return name
	}
	if host != "" && utf8.RuneCountInString(host) <= MaxNameRunes {
		return host
	}
	return "tracker"
}

// AuditLabel is the tracker name for the Audit trail (ADR-025): it goes through siteLabel — the
// operator's free text does not carry a credential pasted there into a log that many people
// read — and it loses the « and » quotes, which the trail uses to delimit the name and which would allow forging
// the rest of the line. Nothing that came from an imported file enters the trail.
func AuditLabel(trackerName string) string {
	return siteLabel(strings.NewReplacer("«", "", "»", "").Replace(trackerName), "")
}

// credentialRun is the length from which a sequence of letters and digits is treated as a
// possible credential: passkeys and tokens have 32 or more; a legitimate tracker name does not have
// a 20-letter word.
const credentialRun = 20

// uuidRe catches the passkey in UUID format (groups of hexadecimal separated by hyphens), which the
// 20 consecutive letters rule does not catch because the hyphens break the sequence.
var uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func looksLikeCredential(name string) bool {
	if strings.ContainsAny(name, `/\?=`) || uuidRe.MatchString(name) {
		return true
	}
	run := 0
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if run++; run >= credentialRun {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}

var hostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// publicHost extracts the hostname (without port) from the announce host, or "" if it is not a
// valid DNS hostname. The site's public identification comes from here, nothing else.
func publicHost(raw string) string {
	h := strings.TrimSpace(raw)
	// SplitHostPort accepts anything after the last ":" as a port ("https" + "//x.org"):
	// only the numeric port is extracted, and the rest falls into name validation.
	if host, port, err := net.SplitHostPort(h); err == nil && isPort(port) {
		h = host
	}
	h = strings.ToLower(strings.TrimSuffix(h, "."))
	if validHost(h) {
		return h
	}
	return ""
}

func validHost(h string) bool { return len(h) <= maxHostLen && hostRe.MatchString(h) }

func isPort(s string) bool {
	if s == "" || len(s) > 5 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---- Input -----------------------------------------------------------------

// ParseDocument is the ONLY entry point for a profile file. It returns the CLEAN
// and validated document — importers use this value, never the raw bytes.
//
// Defense order: size → UTF-8 → shape (keys, repetition, depth, single value)
// → types (unknown field is an error) → content (format, version, family, site, sections).
func ParseDocument(raw []byte) (Document, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xEF\xBB\xBF")) // BOM de editor do Windows
	if len(raw) > MaxDocumentBytes {
		return Document{}, invalidCode(CodeTooLarge, "document", "%d bytes (max %d)", len(raw), MaxDocumentBytes)
	}
	if !utf8.Valid(raw) {
		return Document{}, invalid("document", "file is not valid UTF-8")
	}
	if err := checkShape(raw); err != nil {
		return Document{}, err
	}
	var doc Document
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return Document{}, decodeError(err)
	}
	return doc.validated()
}

// checkShape verifies what the typed decoder lets through, reading the tokens:
//
//   - keys only [a-z0-9_]: encoding/json matches keys to fields ignoring case and even
//     Unicode folding (a key swapping the "k" in "kind" for the Kelvin sign (U+212A) would match "kind"), and another
//     parser would read the same file differently. Requiring ASCII lowercase eliminates this case;
//   - repeated key in the same object: the decoder silently keeps the last one, and whoever reviews
//     the file in a PR would read the first one;
//   - limited depth and ONE value at the top (nothing after the document).
func checkShape(raw []byte) error {
	type frame struct {
		obj     bool
		keys    map[string]struct{}
		wantKey bool
	}
	var stack []frame
	done := false
	afterValue := func() {
		if len(stack) == 0 {
			done = true
		} else if top := &stack[len(stack)-1]; top.obj {
			top.wantKey = true
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid("document", "malformed JSON: %s", syntaxDetail(err))
		}
		if done {
			return invalid("document", "data after document")
		}
		switch v := tok.(type) {
		case json.Delim:
			if v == '{' || v == '[' {
				stack = append(stack, frame{obj: v == '{', keys: map[string]struct{}{}, wantKey: v == '{'})
				if len(stack) > maxDocumentDepth {
					return invalid("document", "nesting above %d levels", maxDocumentDepth)
				}
				continue
			}
			stack = stack[:len(stack)-1]
			afterValue()
		case string:
			if len(stack) > 0 {
				if top := &stack[len(stack)-1]; top.obj && top.wantKey {
					if !keyRe.MatchString(v) {
						return invalid("document", "invalid key %q (only lowercase a-z, 0-9 and _)", truncate(v, 32))
					}
					if _, dup := top.keys[v]; dup {
						return invalid("document", "duplicate key %q", truncate(v, 32))
					}
					top.keys[v] = struct{}{}
					top.wantKey = false
					continue
				}
			}
			afterValue()
		default:
			afterValue()
		}
	}
	if !done || len(stack) != 0 {
		return invalid("document", "incomplete JSON")
	}
	return nil
}

var keyRe = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

func syntaxDetail(err error) string {
	var syn *json.SyntaxError
	if errors.As(err, &syn) {
		return fmt.Sprintf("position %d", syn.Offset)
	}
	return "invalid syntax"
}

// decodeError translates the decoder error into a short refusal. It never returns the received value:
// the file is hostile and the message goes to the Portal and the log.
func decodeError(err error) error {
	var ute *json.UnmarshalTypeError
	switch {
	case errors.As(err, &ute):
		path := jsonPath(ute.Field)
		if path == "" {
			path = "document"
		}
		return invalid(truncate(path, 80), "wrong type: expected %s", typeName(ute.Type))
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return invalid("document", "unknown field %s", truncate(strings.TrimPrefix(err.Error(), "json: unknown field "), 48))
	default:
		return invalid("document", "invalid JSON")
	}
}

// jsonPath turns the decoder's field path into the document's own path. Since Go 1.24 the path
// also names the embedded structs ("Profile.taxonomy.id" for the Profile embedded in Document); the
// document's keys are lower case and Go type names are not, so the leading upper-case parts go.
func jsonPath(field string) string {
	parts := strings.Split(field, ".")
	i := 0
	for i < len(parts)-1 && parts[i] != "" && unicode.IsUpper(rune(parts[i][0])) {
		i++
	}
	return strings.Join(parts[i:], ".")
}

func typeName(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.String:
		return "string"
	case reflect.Slice, reflect.Array:
		return "list"
	case reflect.Map, reflect.Struct:
		return "object"
	case reflect.Bool:
		return "true/false"
	default:
		return t.Kind().String()
	}
}

// validated verifies the CONTENT and returns the clean document (normalized names, terms by
// id, all axes present).
func (d Document) validated() (Document, error) {
	if d.Format != FormatID {
		return Document{}, invalid("format", "not a SeedGrid tracker profile (expected format %q)", FormatID)
	}
	switch {
	case d.SchemaVersion < 1:
		return Document{}, invalid("schema_version", "must be an integer ≥ 1")
	case d.SchemaVersion > SchemaVersion:
		return Document{}, invalidCode(CodeUnsupportedVersion, "schema_version",
			"the file is in format v%d and this SeedGrid supports up to v%d — update SeedGrid to import it",
			d.SchemaVersion, SchemaVersion)
	case d.SchemaVersion < MinSchemaVersion:
		return Document{}, invalidCode(CodeUnsupportedVersion, "schema_version",
			"the file is in format v%d, too old (the oldest supported version is v%d)", d.SchemaVersion, MinSchemaVersion)
	}
	if d.Family != FamilyUNIT3D {
		return Document{}, invalid("family", "family %q not supported (this version supports: %s)", truncate(d.Family, 32), FamilyUNIT3D)
	}

	name, err := CleanName(d.Site.Name)
	if err != nil {
		return Document{}, invalid("site.name", "%v", err)
	}
	host := strings.ToLower(d.Site.Host)
	if host != "" && !validHost(host) {
		return Document{}, invalid("site.host", "must be a hostname (without scheme, port, path, or credentials)")
	}

	if section := unhandledSection(d.Profile); section != "" {
		return Document{}, invalid(section, "this profile section is not yet importable by this version of SeedGrid")
	}

	in := make(map[string][]TermInput, len(d.Taxonomy))
	for _, axis := range sortedKeys(d.Taxonomy) {
		terms := make([]TermInput, 0, len(d.Taxonomy[axis]))
		for i, t := range d.Taxonomy[axis] {
			if t.Origin != "" {
				return Document{}, invalid(fmt.Sprintf("taxonomy.%s[%d].origin", truncate(axis, 32), i),
					"origin is not part of the exchange format (it is local to each installation)")
			}
			terms = append(terms, TermInput{ID: t.ID, Name: t.Name, Kind: t.Kind})
		}
		in[axis] = terms
	}
	clean, err := ValidateInput(in)
	if err != nil {
		return Document{}, err
	}
	// Two names that only differ in case or accent are, for the publishing panel, the SAME
	// label — it matches by name and wouldn't be able to choose between the two ids (a new term
	// could obscure the correct one). The file is refused instead of imported with the ambiguity.
	for _, axis := range sortedKeys(clean) {
		first := make(map[string]int, len(clean[axis]))
		for i, t := range clean[axis] {
			key := NormalizeLabel(t.Name)
			if j, dup := first[key]; dup {
				return Document{}, invalid(fmt.Sprintf("taxonomy.%s[%d].name", axis, i),
					"duplicate name in axis: same as id %d, ignoring case and accents", clean[axis][j].ID)
			}
			first[key] = i
		}
	}
	tax := make(Taxonomy, len(clean))
	for axis, terms := range clean {
		for _, t := range terms {
			tax[axis] = append(tax[axis], Term{ID: t.ID, Name: t.Name, Kind: t.Kind})
		}
	}

	// The description section (#192) goes through the SAME validation as manual editing: the file is from a
	// third party, and the MediaInfo tag becomes BBCode in what the site publishes.
	if d.Description != nil {
		clean, err := CleanDescription(*d.Description)
		if err != nil {
			return Document{}, err
		}
		d.Description = &clean
	}

	// The image_host section (#193) goes through the SAME validation as manual editing: the file is from a
	// third party, and the fields end up in HTTP headers and publishing URLs.
	if d.ImageHost != nil {
		clean, err := CleanImageHost(*d.ImageHost)
		if err != nil {
			return Document{}, err
		}
		d.ImageHost = &clean
	}

	d.Site = Site{Name: name, Host: host}
	d.Taxonomy = normalize(tax)
	return d, nil
}

// CheckTarget verifies if the file fits the target tracker: the profile family must
// be the same (a UNIT3D profile does not describe a Gazelle site).
func (d Document) CheckTarget(target Profile) error {
	if d.Family != target.Family {
		return invalidCode(CodeIncompatibleFamily, "family", "the file is from family %q and this tracker is %q",
			truncate(d.Family, 32), truncate(target.Family, 32))
	}
	return nil
}

// jsonName is the JSON name of a struct field ("" if not serialized).
func jsonName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return ""
	}
	if name, _, _ := strings.Cut(tag, ","); name != "" {
		return name
	}
	return strings.ToLower(f.Name)
}

// sectionNames lists the content sections of the Profile, discovered via reflection — this is what
// allows exporting and enforcing the import of a new section without listing it manually.
func sectionNames() []string {
	rt := reflect.TypeOf(Profile{})
	var out []string
	for i := 0; i < rt.NumField(); i++ {
		if name := jsonName(rt.Field(i)); name != "" && !envelopeFields[name] {
			out = append(out, name)
		}
	}
	return out
}

// unhandledSection returns the first profile section that the import DOES NOT know how to validate but
// that came filled in the file ("" if none). It is fail closed: a section added to the
// Profile without import validation is refused, not accepted without verification.
func unhandledSection(p Profile) string { return unhandledSectionOf(p, handledSections) }

// unhandledSectionOf is unhandledSection over any struct and any set of handled
// sections — separated so the test can exercise fail closed with a section that the Profile does not yet
// have.
func unhandledSectionOf(v any, handled map[string]bool) string {
	rv := reflect.ValueOf(v)
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		name := jsonName(rt.Field(i))
		if name == "" || envelopeFields[name] || handled[name] {
			continue
		}
		if !rv.Field(i).IsZero() {
			return name
		}
	}
	return ""
}
