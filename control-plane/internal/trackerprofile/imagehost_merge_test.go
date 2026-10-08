package trackerprofile

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/imagehost"
)

// The image_host section (#193) in the import merge, in JSONB form and strict reading of the
// file — the same rule as the description section.

func ihSpec() imagehost.Spec {
	return imagehost.Spec{
		Name:          "ptpimg",
		UploadURL:     "https://ptpimg.me/upload.php",
		FileField:     "file-upload",
		AuthHeader:    "X-API-Key",
		URLPath:       "data.image_url",
		ThumbnailPath: "data.thumb_url",
	}
}

func ihPtr(s imagehost.Spec) *imagehost.Spec { return &s }

func docWithIH(s *imagehost.Spec) Document {
	d := docOf(testSite, nil)
	d.ImageHost = s
	return d
}

// The name is normalized (the CleanName rule): spaces and tabs collapsed, no control characters
// recorded in the profile.
func TestCleanImageHost_NormalizesName(t *testing.T) {
	in := ihSpec()
	in.Name = "  My\tHost \n "
	got, err := CleanImageHost(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "My Host" {
		t.Errorf("name = %q, want %q", got.Name, "My Host")
	}
	in.Name = "Host" + string(rune(0x202E))
	if _, err := CleanImageHost(in); err == nil {
		t.Error("name with U+202E accepted")
	}
}

// Rejecting a malformed upload_url does not return the value (ADR-037 says never to echo it).
func TestCleanImageHost_MalformedURLDoesNotEcho(t *testing.T) {
	in := ihSpec()
	in.UploadURL = "https://a/%zz-SECRET"
	_, err := CleanImageHost(in)
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("error = %v", err)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) || ve.Path != "image_host.upload_url" {
		t.Errorf("error path: %v", err)
	}
}

// TestImport_FileWithOnlyImageHostSection tests that a file with only image_host works.
func TestImport_FileWithOnlyImageHostSection(t *testing.T) {
	mine := ihSpec()
	doc := docOf(testSite, nil)
	doc.ImageHost = &mine

	plan, err := PlanImport(Empty(), doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Changes() {
		t.Error("Changes() = false, but the section would be applied")
	}
	for _, w := range plan.Warnings {
		if strings.Contains(w, "no term") {
			t.Errorf("empty file warning in a file that contains the section: %q", w)
		}
	}

	// Protected and with no terms: nothing changes.
	cur := Empty()
	cur.ImageHost = ihPtr(imagehost.Spec{
		Name:       "other",
		UploadURL:  "https://other.com/upload",
		FileField:  "file",
		AuthHeader: "X-Key",
		URLPath:    "url",
	})
	plan, err = PlanImport(cur, doc, testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changes() {
		t.Errorf("Changes() = true with protected section and no terms: %+v", plan.ImageHost)
	}
}

// TestImport_FileWithoutImageHostSectionDeletesNothing tests that a file without image_host does not delete it from the profile.
func TestImport_FileWithoutImageHostSectionDeletesNothing(t *testing.T) {
	mine := ihSpec()
	cur := profileOf(Taxonomy{})
	cur.ImageHost = &mine
	plan, err := PlanImport(cur, docOf(testSite, Taxonomy{AxisType: {{ID: 3, Name: "Encode"}}}), testSite, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ImageHost != nil {
		t.Errorf("plan with section for a file without it: %+v", plan.ImageHost)
	}
	if got := ApplyImportImageHost(&mine, plan); got != &mine {
		t.Errorf("profile section changed: %+v", got)
	}
}

// TestImport_IdempotentAndPureImageHostSection tests that reimporting the same file changes nothing.
func TestImport_IdempotentAndPureImageHostSection(t *testing.T) {
	file := ihSpec()
	cur := Empty()
	plan, _ := PlanImport(cur, docWithIH(&file), testSite, nil)
	first := ApplyImportImageHost(cur.ImageHost, plan)

	// Reimporting the same file over the result: everything "same", nothing to write in the section.
	cur.ImageHost = first
	plan2, _ := PlanImport(cur, docWithIH(&file), testSite, nil)
	if plan2.ImageHost.Status != StatusSame || plan2.imageHostChanges() {
		t.Errorf("reimport should be same: %+v", plan2.ImageHost)
	}
	// Does not mutate the input nor share memory with the plan.
	first.Name = "garbage"
	if plan.ImageHost.Config.Name == "garbage" {
		t.Error("applied section shares memory with the plan")
	}
}

// TestParseOverwrite_ImageHost tests that "image_host" is accepted as an overwrite request.
func TestParseOverwrite_ImageHost(t *testing.T) {
	got, err := ParseOverwrite([]string{"image_host", "category:2"})
	if err != nil {
		t.Fatal(err)
	}
	if !got[ImageHostKey] || !got[OverwriteKey{Axis: "category", ID: 2}] || len(got) != 2 {
		t.Errorf("requests = %v", got)
	}
	for _, bad := range []string{"ImageHost", "image_host:1", "image_hosts", "image_host "} {
		if _, err := ParseOverwrite([]string{bad}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// The message states the two accepted formats.
	_, err = ParseOverwrite([]string{"x"})
	if err == nil || !strings.Contains(err.Error(), "image_host") {
		t.Errorf("rejection does not mention image_host: %v", err)
	}
}

// TestImport_OverwriteRequestImageHostNoMatchBecomesWarning tests that requesting overwrite with nothing protected becomes a warning.
func TestImport_OverwriteRequestImageHostNoMatchBecomesWarning(t *testing.T) {
	file := ihSpec()
	plan, err := PlanImport(Empty(), docWithIH(&file), testSite, map[OverwriteKey]bool{ImageHostKey: true})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range plan.Warnings {
		if strings.Contains(w, "override request") {
			found = true
		}
	}
	if !found {
		t.Errorf("missing warning for unmatched request: %v", plan.Warnings)
	}
	// But when there is something to overwrite, the request matches and raises no warning.
	cur := Empty()
	cur.ImageHost = ihPtr(imagehost.Spec{
		Name:       "other",
		UploadURL:  "https://other.com/upload",
		FileField:  "file",
		AuthHeader: "X-Key",
		URLPath:    "url",
	})
	plan, _ = PlanImport(cur, docWithIH(&file), testSite, map[OverwriteKey]bool{ImageHostKey: true})
	for _, w := range plan.Warnings {
		if strings.Contains(w, "override request") {
			t.Errorf("spurious warning: %q", w)
		}
	}
}

// ---- JSONB Format -----------------------------------------------------------------------

// TestDecodeEncode_ImageHostLegacyLineUnchanged tests that a line without image_host is read and rewritten unchanged.
func TestDecodeEncode_ImageHostLegacyLineUnchanged(t *testing.T) {
	const legacy = `{"category":[{"id":1,"name":"Filmes","kind":"movie","origin":"seed"}],"resolution":[],"type":[{"id":3,"name":"Encode","origin":"manual"}]}`
	p, err := decodeProfile(FamilyUNIT3D, 1, []byte(legacy))
	if err != nil {
		t.Fatal(err)
	}
	if p.ImageHost != nil {
		t.Errorf("legacy line parsed with section: %+v", p.ImageHost)
	}
	raw, err := encodeProfile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != legacy {
		t.Errorf("legacy line was rewritten differently:\n got %s\nwant %s", raw, legacy)
	}
}

// TestDecodeEncode_ImageHostSectionRoundTrip tests that the section is preserved on round trip.
func TestDecodeEncode_ImageHostSectionRoundTrip(t *testing.T) {
	spec := ihSpec()
	in := Profile{Family: FamilyUNIT3D, SchemaVersion: 1, Taxonomy: normalize(Taxonomy{AxisType: {{ID: 3, Name: "Encode", Origin: OriginManual}}}), ImageHost: &spec}
	raw, err := encodeProfile(in)
	if err != nil {
		t.Fatal(err)
	}
	// The section lives at the root, alongside the axes, under the reserved key.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"category", "type", "resolution", "image_host"} {
		if _, ok := top[key]; !ok {
			t.Errorf("missing key %q in document: %s", key, raw)
		}
	}
	out, err := decodeProfile(FamilyUNIT3D, 1, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Errorf("round trip:\n got %+v\nwant %+v", out, in)
	}
	// The section does NOT become a taxonomy axis.
	if _, ok := out.Taxonomy["image_host"]; ok {
		t.Error("image_host section appeared as taxonomy axis")
	}
}

// TestDecodeProfile_ImageHostUnreadableDocumentIsError tests that malformed sections are an error.
func TestDecodeProfile_ImageHostUnreadableDocumentIsError(t *testing.T) {
	cases := map[string]string{
		"image_host string":    `{"image_host":"x"}`,
		"image_host list":      `{"image_host":[]}`,
		"image_host with type": `{"image_host":{"name":123}}`,
		"image_host inside ax": `{"type":{"image_host":{}}}`,
	}
	for name, raw := range cases {
		if _, err := decodeProfile(FamilyUNIT3D, 1, []byte(raw)); err == nil {
			t.Errorf("%s: unreadable document accepted", name)
		}
	}
	// null and empty document are a profile without a section.
	for _, raw := range []string{``, `{}`, `null`, `{"image_host":null}`} {
		p, err := decodeProfile(FamilyUNIT3D, 1, []byte(raw))
		if err != nil || p.ImageHost != nil {
			t.Errorf("%q: section = %+v, error = %v, want profile without section", raw, p.ImageHost, err)
		}
	}
}

// ---- O Profile na API -------------------------------------------------------------------

// TestProfile_ImageHostAppearsOnlyWhenFilled tests that the section is only serialized when present.
func TestProfile_ImageHostAppearsOnlyWhenFilled(t *testing.T) {
	raw, _ := json.Marshal(Empty())
	if strings.Contains(string(raw), "image_host") {
		t.Errorf("profile without section serialized the key: %s", raw)
	}
	spec := ihSpec()
	p := Empty()
	p.ImageHost = &spec
	raw, _ = json.Marshal(p)
	if !strings.Contains(string(raw), `"image_host":{`) {
		t.Errorf("profile with section did not serialize: %s", raw)
	}
}

// ---- Export -----------------------------------------------------------------------------

// TestExport_NoImageHostSectionNoKey tests that without the section, the key does not appear.
func TestExport_NoImageHostSectionNoKey(t *testing.T) {
	raw, _ := NewDocument("Example", "tracker.example.org", fullProfile()).Marshal()
	if strings.Contains(string(raw), "image_host") {
		t.Errorf("export without section contains the key:\n%s", raw)
	}
}

// TestImageHostSectionIsHandledOnImport tests that the section is handled in the import.
func TestImageHostSectionIsHandledOnImport(t *testing.T) {
	if !contains(sectionNames(), "image_host") {
		t.Fatalf("image_host section is not a Profile section: %v", sectionNames())
	}
	if !handledSections["image_host"] {
		t.Error("image_host is not in handledSections: import would reject it (fail closed)")
	}
	if got := unhandledSection(Profile{ImageHost: &imagehost.Spec{}}); got != "" {
		t.Errorf("handled section rejected as %q", got)
	}
}

// TestExport_InvalidImageHostSectionNotExported tests that an invalid section is not exported.
func TestExport_InvalidImageHostSectionNotExported(t *testing.T) {
	bad := imagehost.Spec{Name: ""}
	p := fullProfile()
	p.ImageHost = &bad
	doc := NewDocument("Example", "tracker.example.org", p)
	if _, err := doc.validated(); err == nil {
		t.Fatal("document with invalid spec passed export validation")
	}
}

// ---- Import: the file is hostile content ------------------------------------------------

func withImageHost(v any) string {
	return mutate(func(m map[string]any) { m["image_host"] = v })
}

// TestParseDocument_SectionImageHostHostile tests that hostile payloads are rejected.
func TestParseDocument_SectionImageHostHostile(t *testing.T) {
	long := strings.Repeat("y", 1<<16)
	cases := []struct {
		name, body, path string
	}{
		{"empty name", withImageHost(map[string]any{
			"name": "", "upload_url": "https://ptpimg.me/upload.php",
			"file_field": "file-upload", "auth_header": "X-API-Key",
			"url_path": "data.image_url",
		}), "image_host.name"},
		{"upload_url http (https only)", withImageHost(map[string]any{
			"name": "ptpimg", "upload_url": "http://ptpimg.me/upload.php",
			"file_field": "file-upload", "auth_header": "X-API-Key",
			"url_path": "data.image_url",
		}), "image_host.upload_url"},
		{"invalid file_field", withImageHost(map[string]any{
			"name": "ptpimg", "upload_url": "https://ptpimg.me/upload.php",
			"file_field": "file upload", "auth_header": "X-API-Key",
			"url_path": "data.image_url",
		}), "image_host.file_field"},
		{"forbidden auth_header (Host)", withImageHost(map[string]any{
			"name": "ptpimg", "upload_url": "https://ptpimg.me/upload.php",
			"file_field": "file-upload", "auth_header": "Host",
			"url_path": "data.image_url",
		}), "image_host.auth_header"},
		{"empty url_path", withImageHost(map[string]any{
			"name": "ptpimg", "upload_url": "https://ptpimg.me/upload.php",
			"file_field": "file-upload", "auth_header": "X-API-Key",
			"url_path": "",
		}), "image_host.url_path"},
		{"huge name", withImageHost(map[string]any{
			"name": long, "upload_url": "https://ptpimg.me/upload.php",
			"file_field": "file-upload", "auth_header": "X-API-Key",
			"url_path": "data.image_url",
		}), "image_host.name"},
		{"unknown field in section", withImageHost(map[string]any{
			"name": "ptpimg", "upload_url": "https://ptpimg.me/upload.php",
			"file_field": "file-upload", "auth_header": "X-API-Key",
			"url_path": "data.image_url", "html": "<script>",
		}), ""},
		{"section that is a list", withImageHost([]any{"ptpimg"}), ""},
		{"section that is text", withImageHost("ptpimg"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseDocument([]byte(tc.body))
			ve := asErr(t, err)
			if tc.path != "" && ve.Path != tc.path {
				t.Errorf("path = %q, want %q (%s)", ve.Path, tc.path, ve.Msg)
			}
			// The received value never returns intact: it can be megabytes.
			if len(ve.Error()) > 500 {
				t.Errorf("rejection returns %d bytes: %.100s…", len(ve.Error()), ve.Error())
			}
		})
	}
}
