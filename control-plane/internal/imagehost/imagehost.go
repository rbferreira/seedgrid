// Package imagehost is a generic image uploader for hosts declared in the
// `image_host` section of the tracker profile (ADR-040). The shape is closed: multipart POST with
// one file part per request, key in a custom header, and the image URL
// comes out via dot-path on the JSON response. The profiles package embeds
// `Spec`, so this package does not import any other from the project to avoid cycles.
package imagehost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Spec describes an image host. The fields are populated by the tracker profile
// and validated before any use; the uploader does not revalidate.
type Spec struct {
	Name          string `json:"name"`
	UploadURL     string `json:"upload_url"`
	FileField     string `json:"file_field"`
	AuthHeader    string `json:"auth_header"`
	AuthPrefix    string `json:"auth_prefix"`
	URLPath       string `json:"url_path"`
	ThumbnailPath string `json:"thumbnail_path,omitempty"`
}

// Image is the result of an upload: the full image URL and the thumbnail URL.
type Image struct {
	Full  string
	Thumb string
}

// errRedirect is the internal error used by the client's CheckRedirect. It is not exported
// because the public contract is ErrRedirect; this one exists only so Go stops the redirect.
var errRedirect = errors.New("redirect")

// transport is the RoundTripper used by the HTTP client. nil = default. Variable so the
// test can point to an httptest.NewTLSServer and restore it at the end.
var transport http.RoundTripper

// Sentinel errors returned by Upload. The chain converts these errors into reasons
// from the closed set (chain.go); never leak raw text because the URL carries the key
// (imgbb case) and the response might bring third-party text.
var (
	ErrRedirect = errors.New("imagehost: redirect rejected")
	ErrTimeout  = errors.New("imagehost: timed out")
	ErrConnect  = errors.New("imagehost: connection failure")
	ErrNoURL    = errors.New("imagehost: response without URL")
)

// StatusError represents an HTTP response with a status other than 200/201. The chain
// converts it into the reason "HTTP <status>".
type StatusError struct {
	Status int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("imagehost: HTTP %d", e.Status)
}

// Regex for validating simple fields.
var (
	reFileField = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)
	reAuthHead  = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
	reSegment   = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
)

// blankLooking are the runes that look blank but are not spaces. Same set
// as trackerprofile.CleanName, replicated locally because importing that package
// would create a cycle (trackerprofile embeds Spec in #193 / ADR-040).
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

// FieldError is the validation rejection, with the field set aside for those composing the response
// (trackerprofile builds `image_host.<field>`). Msg never brings the entire received value.
type FieldError struct {
	Field string
	Msg   string
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Msg }

func fieldErr(field, format string, args ...any) error {
	return &FieldError{Field: field, Msg: fmt.Sprintf(format, args...)}
}

// Validate applies all rules from the "Validation" section of ADR-040. Messages in PT-BR
// name the field so the operator knows what to fix.
func (s Spec) Validate() error {
	// name: same rules as trackerprofile.CleanName.
	if !utf8.ValidString(s.Name) {
		return fieldErr("name", "invalid UTF-8")
	}
	name := strings.Join(strings.Fields(s.Name), " ")
	if name == "" {
		return fieldErr("name", "empty")
	}
	if n := utf8.RuneCountInString(name); n > 64 {
		return fieldErr("name", "%d runes (maximum 64)", n)
	}
	for _, r := range name {
		switch {
		case unicode.Is(unicode.Cf, r):
			return fieldErr("name", "invisible formatting (U+%04X)", r)
		case unicode.Is(unicode.Cc, r), unicode.Is(unicode.Co, r), unicode.Is(unicode.Cs, r), r == utf8.RuneError:
			return fieldErr("name", "invalid character (U+%04X)", r)
		case blankLooking[r]:
			return fieldErr("name", "looks blank (U+%04X)", r)
		}
	}

	// upload_url: absolute, https, DNS name, no port, no userinfo, no fragment.
	if len(s.UploadURL) > 2048 {
		return fieldErr("upload_url", "%d characters (maximum 2048)", len(s.UploadURL))
	}
	u, err := url.Parse(s.UploadURL)
	if err != nil {
		return fieldErr("upload_url", "malformed URL")
	}
	if !u.IsAbs() {
		return fieldErr("upload_url", "relative")
	}
	if u.Scheme != "https" {
		return fieldErr("upload_url", "scheme %q (https only)", u.Scheme)
	}
	if u.User != nil {
		return fieldErr("upload_url", "userinfo forbidden")
	}
	if u.Fragment != "" {
		return fieldErr("upload_url", "fragment forbidden")
	}
	host := u.Hostname()
	if host == "" {
		return fieldErr("upload_url", "empty host")
	}
	if u.Port() != "" {
		return fieldErr("upload_url", "explicit port forbidden")
	}
	// IP literal, IPv4 or IPv6 (u.Hostname() already strips the brackets from [::1]).
	if net.ParseIP(host) != nil {
		return fieldErr("upload_url", "IP literal forbidden")
	}
	// localhost: exact, .localhost suffix, with or without the trailing dot, case-insensitive.
	dns := strings.TrimSuffix(strings.ToLower(host), ".")
	if dns == "localhost" || strings.HasSuffix(dns, ".localhost") {
		return fieldErr("upload_url", "localhost forbidden")
	}
	// Last label digits only or 0x…: not a DNS name, it's an IPv4 written in another way
	// that the resolver accepts (127.1, 2130706433, 0x7f.1).
	last := dns[strings.LastIndex(dns, ".")+1:]
	if allDigits(last) || strings.HasPrefix(last, "0x") {
		return fieldErr("upload_url", "numeric host forbidden")
	}

	// file_field: simple identifier.
	if !reFileField.MatchString(s.FileField) {
		return fieldErr("file_field", "invalid")
	}

	// auth_header: header token, outside the forbidden list (case-insensitive).
	if !reAuthHead.MatchString(s.AuthHeader) {
		return fieldErr("auth_header", "invalid")
	}
	low := strings.ToLower(s.AuthHeader)
	prohibited := []string{
		"host", "content-type", "content-length", "transfer-encoding",
		"connection", "cookie", "upgrade", "te", "trailer",
	}
	for _, p := range prohibited {
		if low == p {
			return fieldErr("auth_header", "%q forbidden", s.AuthHeader)
		}
	}
	if strings.HasPrefix(low, "proxy-") {
		return fieldErr("auth_header", "Proxy- prefix forbidden")
	}

	// auth_prefix: 0 to 32 printable ASCII.
	if len(s.AuthPrefix) > 32 {
		return fieldErr("auth_prefix", "%d characters (maximum 32)", len(s.AuthPrefix))
	}
	for _, r := range s.AuthPrefix {
		if r < 0x20 || r > 0x7E {
			return fieldErr("auth_prefix", "non-printable ASCII character (U+%04X)", r)
		}
	}

	// url_path: 1 to 5 segments.
	if err := validatePath(s.URLPath, "url_path"); err != nil {
		return err
	}

	// thumbnail_path: optional; if not empty, same rules.
	if s.ThumbnailPath != "" {
		if err := validatePath(s.ThumbnailPath, "thumbnail_path"); err != nil {
			return err
		}
	}

	return nil
}

// validatePath checks if the path has 1 to 5 dot-separated segments, each
// matching the pattern [A-Za-z0-9_]{1,64}.
func validatePath(path, field string) error {
	if path == "" {
		return fieldErr(field, "empty")
	}
	segs := strings.Split(path, ".")
	if len(segs) > 5 {
		return fieldErr(field, "%d segments (maximum 5)", len(segs))
	}
	for _, seg := range segs {
		if !reSegment.MatchString(seg) {
			return fieldErr(field, "invalid segment %q", seg)
		}
	}
	return nil
}

// Host returns the DNS name of the upload_url. The caller has already validated the Spec, so the parse
// shouldn't fail; if it does, it returns "" instead of panicking.
func (s Spec) Host() string {
	u, err := url.Parse(s.UploadURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Upload sends each frame (JPEG) to the host described by spec and returns the images
// (full + thumbnail) in the SAME input order. It DOES NOT revalidate the Spec — it was already
// validated at every write point (profile routes, import).
//
// The shape is closed (ADR-040): one multipart POST per frame, with ONE file part
// (`image/jpeg`), the key only in the auth header, and no other authentication header.
// Redirect (3xx) is a failure: Go drops `Authorization` on a redirect to another
// domain, but passes custom headers (`X-API-Key`) — following the redirect would take
// the key to where no one registered it.
func Upload(ctx context.Context, spec Spec, key string, frames [][]byte) ([]Image, error) {
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errRedirect
		},
	}
	imgs := make([]Image, 0, len(frames))
	for i, frame := range frames {
		img, err := uploadOne(ctx, client, spec, key, i, frame)
		if err != nil {
			return nil, err
		}
		imgs = append(imgs, img)
	}
	return imgs, nil
}

// uploadOne sends a frame to the host and extracts the URL from the response. The index i is used
// only for the filename (`frame-01.jpg`, `frame-02.jpg`, …).
func uploadOne(ctx context.Context, client *http.Client, spec Spec, key string, i int, frame []byte) (Image, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Part header: file field defined by the spec, sequential filename.
	// CreatePart requires textproto.MIMEHeader (not http.Header, despite having the same structure).
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="frame-%02d.jpg"`, spec.FileField, i+1))
	h.Set("Content-Type", "image/jpeg")
	part, err := mw.CreatePart(h)
	if err != nil {
		return Image{}, fmt.Errorf("multipart: %w", err)
	}
	if _, err := part.Write(frame); err != nil {
		return Image{}, fmt.Errorf("write frame: %w", err)
	}
	if err := mw.Close(); err != nil {
		return Image{}, fmt.Errorf("close multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spec.UploadURL, &buf)
	if err != nil {
		return Image{}, fmt.Errorf("build request: %w", err)
	}
	// Closed shape: only the multipart Content-Type and the auth header.
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(spec.AuthHeader, spec.AuthPrefix+key)

	resp, err := client.Do(req)
	if err != nil {
		// Classifies the error into one of the sentinels — never leaks the raw *url.Error,
		// which carries the URL (and, in the imgbb case, the key in the query).
		if errors.Is(err, errRedirect) {
			return Image{}, ErrRedirect
		}
		if ctx.Err() != nil {
			return Image{}, ErrTimeout
		}
		return Image{}, ErrConnect
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return Image{}, &StatusError{Status: resp.StatusCode}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		if ctx.Err() != nil {
			return Image{}, ErrTimeout
		}
		return Image{}, ErrConnect
	}

	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return Image{}, ErrNoURL
	}

	full, ok := pathLookup(doc, spec.URLPath)
	if !ok || full == "" {
		return Image{}, ErrNoURL
	}

	thumb := full // default: if there is no thumbnail, use the full one (same behavior as imgbb).
	if spec.ThumbnailPath != "" {
		if t, found := pathLookup(doc, spec.ThumbnailPath); found && t != "" {
			thumb = t
		}
	}

	return Image{Full: full, Thumb: thumb}, nil
}

// pathLookup extracts a string value from a decoded JSON (map[string]any / []any)
// following a dot path. A numeric segment (digits only, no sign) indexes
// []any; out of bounds = not found. A non-numeric segment is a key of
// map[string]any. The final value must be a non-empty string.
func pathLookup(doc any, path string) (string, bool) {
	segs := strings.Split(path, ".")
	cur := doc
	for _, seg := range segs {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[seg]
			if !ok {
				return "", false
			}
			cur = v
		case []any:
			if !allDigits(seg) {
				return "", false
			}
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return "", false
			}
			cur = node[idx]
		default:
			return "", false
		}
	}
	s, ok := cur.(string)
	return s, ok
}

// allDigits says whether the string contains only ASCII digits. Used to decide if a
// path segment is an array index or a map key.
func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
