package api_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// The composition of the description per tracker (#192), via the full API against the test database:
// POST /publish/description (publish:write), PUT and DELETE /trackers/{id}/profile/description
// (trackers:write) and the section traveling in the profile export/import.

const (
	dPublish = "/api/v1/publish/description"
	dSection = "/api/v1/trackers/%s/profile/description"

	dMediaBody = `{"tracker_id":%q,"kind":"media","media":{` +
		`"poster":"https://image.tmdb.org/t/p/w500/abc.jpg","overview":"A witch faces the unknown.",` +
		`"keywords":"witchcraft, horror","screenshots":"[b]Screenshots[/b]\nS","mediainfo":"General\nFormat : MKV"}}`
	dGameBody = `{"tracker_id":%q,"kind":"game","game":{` +
		`"cover_url":"https://images.igdb.com/c.jpg","name":"Elden Ring","year":2022,"summary":"An RPG.",` +
		`"genres":["RPG","Adventure"],"platforms":["PC","PS5"]}}`

	// What the old TypeScript produced for the two entries above (and the default keeps producing).
	dMediaDefault = "[center][img]https://image.tmdb.org/t/p/w500/abc.jpg[/img][/center]\n\n" +
		"[b]Synopsis[/b]\nA witch faces the unknown.\n\n" +
		"[b]Tags[/b]\nwitchcraft, horror\n\n" +
		"[b]Screenshots[/b]\nS\n\n" +
		"[b]MediaInfo[/b]\n[mediainfo]General\nFormat : MKV[/mediainfo]"
	dGameDefault = "[center][img]https://images.igdb.com/c.jpg[/img][/center]\n\n" +
		"[b]Elden Ring (2022)[/b]\n\n" +
		"[b]About[/b]\nAn RPG.\n\n" +
		"[b]Genres[/b]\nRPG, Adventure\n\n" +
		"[b]Platforms[/b]\nPC, PS5"

	// A custom composition: MediaInfo first, in [code], without synopsis, tags or screenshots.
	dCustomCfg = `{"media_blocks":["mediainfo","poster"],"game_blocks":["title","platforms"],"media_info_tag":"code"}`
	dMediaCust = "[b]MediaInfo[/b]\n[code]General\nFormat : MKV[/code]\n\n[center][img]https://image.tmdb.org/t/p/w500/abc.jpg[/img][/center]"
	dGameCust  = "[b]Elden Ring (2022)[/b]\n\n[b]Platforms[/b]\nPC, PS5"
)

func dCompose(t *testing.T, h http.Handler, token, body string) (int, string) {
	t.Helper()
	return tpCall(t, h, http.MethodPost, dPublish, token, body)
}

func dText(t *testing.T, body string) string {
	t.Helper()
	var out map[string]string
	tpDecode(t, body, &out)
	if len(out) != 1 {
		t.Errorf("response should have only the description key: %s", body)
	}
	return out["description"]
}

func TestPublishDescription_CompositionByTracker(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, _ := createTracker(t, h, admin, "Test Description", fake.srv.URL)

	// Without the section in the profile: the default, which is what the Portal has always assembled.
	code, body := dCompose(t, h, admin, fmt.Sprintf(dMediaBody, id))
	if code != http.StatusOK {
		t.Fatalf("media: %d %s", code, body)
	}
	if got := dText(t, body); got != dMediaDefault {
		t.Errorf("media (default):\n got %q\nwant %q", got, dMediaDefault)
	}
	code, body = dCompose(t, h, admin, fmt.Sprintf(dGameBody, id))
	if code != http.StatusOK {
		t.Fatalf("game: %d %s", code, body)
	}
	if got := dText(t, body); got != dGameDefault {
		t.Errorf("game (default):\n got %q\nwant %q", got, dGameDefault)
	}

	// With the section: the tracker profile composition.
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(dSection, id), admin, dCustomCfg); code != http.StatusOK {
		t.Fatalf("PUT description: %d %s", code, body)
	}
	_, body = dCompose(t, h, admin, fmt.Sprintf(dMediaBody, id))
	if got := dText(t, body); got != dMediaCust {
		t.Errorf("media (own):\n got %q\nwant %q", got, dMediaCust)
	}
	_, body = dCompose(t, h, admin, fmt.Sprintf(dGameBody, id))
	if got := dText(t, body); got != dGameCust {
		t.Errorf("game (own):\n got %q\nwant %q", got, dGameCust)
	}

	// The composition is PER TRACKER: the other one stays on the default.
	other, _ := createTracker(t, h, admin, "Other Tracker", fake.srv.URL)
	_, body = dCompose(t, h, admin, fmt.Sprintf(dMediaBody, other))
	if got := dText(t, body); got != dMediaDefault {
		t.Errorf("other tracker affected by first one's config: %q", got)
	}

	// Restoring reverts to the default.
	if code, body := tpCall(t, h, http.MethodDelete, fmt.Sprintf(dSection, id), admin, ""); code != http.StatusOK {
		t.Fatalf("DELETE description: %d %s", code, body)
	}
	_, body = dCompose(t, h, admin, fmt.Sprintf(dMediaBody, id))
	if got := dText(t, body); got != dMediaDefault {
		t.Errorf("after restore:\n got %q\nwant %q", got, dMediaDefault)
	}

	// Empty input composes empty text (and it's not an error): the Portal only requests when there is something to show.
	code, body = dCompose(t, h, admin, fmt.Sprintf(`{"tracker_id":%q,"kind":"media"}`, id))
	if code != http.StatusOK || dText(t, body) != "" {
		t.Errorf("empty media: %d %s", code, body)
	}
}

// The errors: nonexistent tracker is 404, invalid kind is 400 — and a hostile body doesn't pass.
func TestPublishDescription_Errors(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, _ := createTracker(t, h, admin, "Errors", fake.srv.URL)
	foreign, _ := createTracker(t, h, tokens["Publisher"], "From another operator", fake.srv.URL)

	cases := []struct {
		name  string
		token string
		body  string
		want  int
		code  string
	}{
		{"nonexistent tracker", admin, `{"tracker_id":"00000000-0000-0000-0000-000000000009","kind":"media"}`, 404, "tracker_not_found"},
		{"tracker with malformed id", admin, `{"tracker_id":"'; DROP TABLE trackers;--","kind":"media"}`, 404, "tracker_not_found"},
		{"tracker from another operator does not exist for the requester", admin, fmt.Sprintf(`{"tracker_id":%q,"kind":"media"}`, foreign), 404, "tracker_not_found"},
		{"invalid kind", admin, fmt.Sprintf(`{"tracker_id":%q,"kind":"tv"}`, id), 400, "invalid_request"},
		{"kind with different casing", admin, fmt.Sprintf(`{"tracker_id":%q,"kind":"Media"}`, id), 400, "invalid_request"},
		{"missing kind", admin, fmt.Sprintf(`{"tracker_id":%q}`, id), 400, "invalid_request"},
		{"missing tracker_id", admin, `{"kind":"media"}`, 400, "invalid_request"},
		{"blank tracker_id", admin, `{"tracker_id":"  ","kind":"media"}`, 400, "invalid_request"},
		{"invalid kind overrides nonexistent tracker", admin, `{"tracker_id":"00000000-0000-0000-0000-000000000009","kind":"x"}`, 400, "invalid_request"},
		{"empty body", admin, ``, 400, "invalid_body"},
		{"not JSON", admin, `{kind`, 400, "invalid_body"},
		{"unknown top-level field", admin, fmt.Sprintf(`{"tracker_id":%q,"kind":"media","html":"<script>"}`, id), 400, "invalid_body"},
		{"unknown field in media", admin, fmt.Sprintf(`{"tracker_id":%q,"kind":"media","media":{"posterr":"x"}}`, id), 400, "invalid_body"},
		{"wrong type", admin, fmt.Sprintf(`{"tracker_id":%q,"kind":"game","game":{"year":"2022"}}`, id), 400, "invalid_body"},
		{"body too large", admin, fmt.Sprintf(`{"tracker_id":%q,"kind":"media","media":{"mediainfo":%q}}`, id, strings.Repeat("a", 5<<20)), 413, "payload_too_large"},
		{"no write role in publication", tokens["Viewer"], fmt.Sprintf(`{"tracker_id":%q,"kind":"media"}`, id), 403, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := dCompose(t, h, tc.token, tc.body)
			if code != tc.want {
				t.Fatalf("status %d, want %d: %.200s", code, tc.want, body)
			}
			if tc.code != "" && !strings.Contains(body, `"`+tc.code+`"`) {
				t.Errorf("body does not contain code %q: %.200s", tc.code, body)
			}
		})
	}
	// Without token: 401.
	if code, _ := dCompose(t, h, "", fmt.Sprintf(`{"tracker_id":%q,"kind":"media"}`, id)); code != http.StatusUnauthorized {
		t.Errorf("no token: %d, want 401", code)
	}
}

// RBAC: publish:write (Administrator and Publisher), the same permission as /publish/infer.
func TestPublishDescription_RBAC(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	fake := newSite(t)
	id, _ := createTracker(t, h, tokens["Publisher"], "RBAC description", fake.srv.URL)
	body := fmt.Sprintf(dMediaBody, id)

	if code, b := dCompose(t, h, tokens["Publisher"], body); code != http.StatusOK {
		t.Errorf("Publisher on own tracker: %d %.200s, want 200", code, b)
	}
	if code, _ := dCompose(t, h, tokens["Viewer"], body); code != http.StatusForbidden {
		t.Errorf("Viewer: %d, want 403 (only has publish:read)", code)
	}
	// Tracker is per operator: the Administrator doesn't compose with the Publisher's tracker profile.
	if code, _ := dCompose(t, h, tokens["Administrator"], body); code != http.StatusNotFound {
		t.Errorf("Administrator on someone else's tracker: %d, want 404", code)
	}
}

// It only composes text: it doesn't go to the audit trail (ADR-025), just like infer.
func TestPublishDescription_NotAudited(t *testing.T) {
	h, tokens, query := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, _ := createTracker(t, h, admin, "No audit", fake.srv.URL)
	for i := 0; i < 3; i++ {
		if code, body := dCompose(t, h, admin, fmt.Sprintf(dMediaBody, id)); code != http.StatusOK {
			t.Fatalf("compose: %d %s", code, body)
		}
	}
	if n := len(query(`SELECT action FROM audit_log WHERE path LIKE $1`, "%/publish/description")); n != 0 {
		t.Errorf("composing description appeared in audit (%d rows) — it is only text", n)
	}
}

// ---- The section in the profile: PUT, DELETE, listing ----------------------------------------------

type sectionOut struct {
	MediaBlocks  []string `json:"media_blocks"`
	GameBlocks   []string `json:"game_blocks"`
	MediaInfoTag string   `json:"media_info_tag"`
}

type trackerListOut struct {
	Data []struct {
		ID      string `json:"id"`
		Profile struct {
			Description *sectionOut `json:"description"`
		} `json:"profile"`
		DescriptionDefault sectionOut `json:"description_default"`
	} `json:"data"`
}

func dListed(t *testing.T, h http.Handler, token, id string) (desc *sectionOut, def sectionOut, raw string) {
	t.Helper()
	code, body := tpCall(t, h, http.MethodGet, "/api/v1/trackers", token, "")
	if code != http.StatusOK {
		t.Fatalf("list: %d %s", code, body)
	}
	var list trackerListOut
	tpDecode(t, body, &list)
	for _, tr := range list.Data {
		if tr.ID == id {
			return tr.Profile.Description, tr.DescriptionDefault, body
		}
	}
	t.Fatalf("tracker %s is not in the listing", id)
	return nil, sectionOut{}, ""
}

func TestTrackerDescription_PutDeleteEListagem(t *testing.T) {
	h, tokens, query := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, createResp := createTracker(t, h, admin, "Edit Description", fake.srv.URL)

	// The creation already brings the catalog (the default, with all blocks) and no custom section: the
	// key doesn't even exist in the profile.
	var created struct {
		Profile            map[string]json.RawMessage `json:"profile"`
		DescriptionDefault sectionOut                 `json:"description_default"`
	}
	tpDecode(t, createResp, &created)
	wantDefault := sectionOut{
		MediaBlocks:  []string{"poster", "synopsis", "tags", "screenshots", "mediainfo"},
		GameBlocks:   []string{"cover", "title", "about", "genres", "platforms"},
		MediaInfoTag: "mediainfo",
	}
	if _, has := created.Profile["description"]; has {
		t.Errorf("profile without section serialized the description key: %s", createResp)
	}
	if !reflect.DeepEqual(created.DescriptionDefault, wantDefault) {
		t.Errorf("description_default = %+v, want %+v", created.DescriptionDefault, wantDefault)
	}

	// PUT saves and returns the profile.
	code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(dSection, id), admin, dCustomCfg)
	if code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}
	var put struct {
		Family      string      `json:"family"`
		Description *sectionOut `json:"description"`
	}
	tpDecode(t, body, &put)
	if put.Family != "unit3d" || put.Description == nil || put.Description.MediaInfoTag != "code" ||
		fmt.Sprint(put.Description.MediaBlocks) != "[mediainfo poster]" || fmt.Sprint(put.Description.GameBlocks) != "[title platforms]" {
		t.Errorf("PUT response: %s", body)
	}
	desc, def, _ := dListed(t, h, admin, id)
	if desc == nil || desc.MediaInfoTag != "code" || !reflect.DeepEqual(def, wantDefault) {
		t.Errorf("listing: section %+v, default %+v", desc, def)
	}

	// The explicit section: what didn't come in the body becomes the default, and the empty list is "no blocks".
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(dSection, id), admin, `{"media_blocks":[],"media_info_tag":"pre"}`); code != http.StatusOK {
		t.Fatalf("partial PUT: %d %s", code, body)
	}
	desc, _, _ = dListed(t, h, admin, id)
	if desc == nil || desc.MediaBlocks == nil || len(desc.MediaBlocks) != 0 || !reflect.DeepEqual(desc.GameBlocks, wantDefault.GameBlocks) || desc.MediaInfoTag != "pre" {
		t.Errorf("section after partial PUT: %+v", desc)
	}
	if _, body := dCompose(t, h, admin, fmt.Sprintf(dMediaBody, id)); dText(t, body) != "" {
		t.Errorf("with empty media list the description should be empty: %s", body)
	}

	// Editing the taxonomy (the profile PUT) does NOT delete the section.
	if code, body := tpCall(t, h, http.MethodPut, "/api/v1/trackers/"+id+"/profile", admin, exFullBody); code != http.StatusOK {
		t.Fatalf("PUT profile: %d %s", code, body)
	}
	if desc, _, _ = dListed(t, h, admin, id); desc == nil || desc.MediaInfoTag != "pre" {
		t.Errorf("editing taxonomy deleted the section: %+v", desc)
	}

	// DELETE restores: the section disappears and the tracker reverts to the default.
	code, body = tpCall(t, h, http.MethodDelete, fmt.Sprintf(dSection, id), admin, "")
	if code != http.StatusOK {
		t.Fatalf("DELETE: %d %s", code, body)
	}
	if desc, _, _ = dListed(t, h, admin, id); desc != nil {
		t.Errorf("section survived DELETE: %+v", desc)
	}
	if code, _ := tpCall(t, h, http.MethodDelete, fmt.Sprintf(dSection, id), admin, ""); code != http.StatusOK {
		t.Errorf("repeated DELETE: %d, want 200 (idempotent)", code)
	}

	// Audit: every write, with the tracker name — and the taxonomy keeps its own.
	edits := query(`SELECT action FROM audit_log WHERE path LIKE $1 ORDER BY created_at`, "%/trackers/"+id+"/profile/description")
	want := []string{
		"Edited the description layout of tracker «Edit Description»",
		"Edited the description layout of tracker «Edit Description»",
		"Reset the description layout of tracker «Edit Description» to its default",
		"Reset the description layout of tracker «Edit Description» to its default",
	}
	if strings.Join(edits, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit:\n got %q\nwant %q", edits, want)
	}

	// No layer 3 credentials in any response.
	for _, secret := range []string{siteToken, sitePasskey, "announce"} {
		if strings.Contains(createResp+body, secret) {
			t.Errorf("a response leaks %q", secret)
		}
	}
}

// The section is untrusted content at EVERY write point: every hostile payload is rejected
// before the database — 400 with the field that failed, never 500 — and nothing is saved.
func TestTrackerDescription_HostilePayloads(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, _ := createTracker(t, h, admin, "Hostile", fake.srv.URL)
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(dSection, id), admin, dCustomCfg); code != http.StatusOK {
		t.Fatalf("initial PUT: %d %s", code, body)
	}

	cases := []struct {
		name, body, path, code string
	}{
		{"unknown block", `{"media_blocks":["poster","x"]}`, "description.media_blocks[1]", "invalid_profile"},
		{"game block in media list", `{"media_blocks":["cover"]}`, "description.media_blocks[0]", "invalid_profile"},
		{"repeated block", `{"game_blocks":["title","title"]}`, "description.game_blocks[1]", "invalid_profile"},
		{"unknown tag", `{"media_info_tag":"html"}`, "description.media_info_tag", "invalid_profile"},
		{"tag that tries to close BBCode", `{"media_info_tag":"mediainfo][img=http://internal/"}`, "description.media_info_tag", "invalid_profile"},
		{"tag with different case", `{"media_info_tag":"PRE"}`, "description.media_info_tag", "invalid_profile"},
		{"list larger than block set", `{"media_blocks":["poster","synopsis","tags","screenshots","mediainfo","poster"]}`, "description.media_blocks", "invalid_profile"},
		{"wrong type: list that is text", `{"media_blocks":"poster"}`, "", "invalid_body"},
		{"wrong type: item that is number", `{"media_blocks":[1]}`, "", "invalid_body"},
		{"unknown field", `{"media_blocks":["poster"],"html":"<script>"}`, "", "invalid_body"},
		{"empty body", ``, "", "invalid_body"},
		{"not JSON", `{media`, "", "invalid_body"},
		{"2 MiB body", `{"media_info_tag":"` + strings.Repeat("a", 2<<20) + `"}`, "", "invalid_body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(dSection, id), admin, tc.body)
			if code != http.StatusBadRequest || !strings.Contains(body, `"`+tc.code+`"`) {
				t.Fatalf("%d %.300s, want 400 %s", code, body, tc.code)
			}
			if tc.path != "" && !strings.Contains(body, tc.path) {
				t.Errorf("rejection does not mention field %q: %.300s", tc.path, body)
			}
			if len(body) > 1000 {
				t.Errorf("rejection returns %d bytes: received value must not be returned", len(body))
			}
		})
	}
	// Nothing that was rejected was saved: the section is the previous one.
	desc, _, _ := dListed(t, h, admin, id)
	if desc == nil || desc.MediaInfoTag != "code" || fmt.Sprint(desc.MediaBlocks) != "[mediainfo poster]" {
		t.Errorf("a rejected write changed the section: %+v", desc)
	}
}

// RBAC in the standard of the tracker endpoints: writing the section is trackers:write (the Viewer doesn't
// pass, the owner Publisher does) and a tracker from another operator doesn't exist for whoever asks.
func TestTrackerDescription_RBAC(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	fake := newSite(t)
	id, _ := createTracker(t, h, tokens["Publisher"], "RBAC section", fake.srv.URL)
	other, _ := createTracker(t, h, tokens["Administrator"], "Do admin", fake.srv.URL)

	routes := []struct{ method, body string }{{http.MethodPut, dCustomCfg}, {http.MethodDelete, ""}}
	for _, r := range routes {
		if code, _ := tpCall(t, h, r.method, fmt.Sprintf(dSection, id), tokens["Viewer"], r.body); code != http.StatusForbidden {
			t.Errorf("Viewer on %s: %d, want 403", r.method, code)
		}
		if code, body := tpCall(t, h, r.method, fmt.Sprintf(dSection, id), tokens["Publisher"], r.body); code != http.StatusOK {
			t.Errorf("Publisher owner on %s: %d %.200s, want 200", r.method, code, body)
		}
		if code, _ := tpCall(t, h, r.method, fmt.Sprintf(dSection, other), tokens["Publisher"], r.body); code != http.StatusNotFound {
			t.Errorf("Publisher on admin's tracker, %s: %d, want 404", r.method, code)
		}
		if code, _ := tpCall(t, h, r.method, fmt.Sprintf(dSection, id), "", r.body); code != http.StatusUnauthorized {
			t.Errorf("no token on %s: %d, want 401", r.method, code)
		}
	}
	// Reading (the listing loads the section and the catalog) is trackers:read — the Viewer has it. The listing is
	// per operator: the Viewer doesn't have own trackers (empty list), and the Publisher sees theirs with the
	// composition catalog.
	if code, body := tpCall(t, h, http.MethodGet, "/api/v1/trackers", tokens["Viewer"], ""); code != http.StatusOK {
		t.Errorf("Viewer reading trackers: %d %.200s, want 200", code, body)
	}
	if code, body := tpCall(t, h, http.MethodGet, "/api/v1/trackers", tokens["Publisher"], ""); code != http.StatusOK || !strings.Contains(body, `"description_default"`) {
		t.Errorf("Publisher reading trackers: %d %.200s, want 200 with composition catalog", code, body)
	}
}

// ---- The section travels in export and import ---------------------------------------------------

func TestTrackerDescription_ProfileExportAndImport(t *testing.T) {
	h, tokens, query := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	from, _ := exFilledTracker(t, h, admin, "Source Desc")
	if code, body := tpCall(t, h, http.MethodPut, fmt.Sprintf(dSection, from), admin, dCustomCfg); code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, body)
	}

	// The file brings the section — and it's still schema_version 1.
	code, file, _ := exExport(t, h, admin, from)
	if code != http.StatusOK {
		t.Fatalf("export: %d %s", code, file)
	}
	var doc struct {
		SchemaVersion int         `json:"schema_version"`
		Description   *sectionOut `json:"description"`
	}
	tpDecode(t, file, &doc)
	if doc.SchemaVersion != 1 || doc.Description == nil || doc.Description.MediaInfoTag != "code" {
		t.Fatalf("exported file: %s", file)
	}
	for _, secret := range []string{siteToken, sitePasskey, exSourceTag, "api_token", "announce"} {
		if strings.Contains(file, secret) {
			t.Errorf("the export leaks %q", secret)
		}
	}

	// Destination without section: the preview says what it would apply and does NOT save; the import applies.
	fake := newSite(t)
	to := exTracker(t, h, admin, "Target Desc", fake.srv.URL)
	code, body := tpCall(t, h, http.MethodPost, fmt.Sprintf(exPreview, to), admin, file)
	if code != http.StatusOK || !strings.Contains(body, `"description":{"status":"new"`) {
		t.Fatalf("preview: %d %s", code, body)
	}
	if desc, _, _ := dListed(t, h, admin, to); desc != nil {
		t.Error("preview saved the section")
	}
	code, body = tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to), admin, file)
	if code != http.StatusOK {
		t.Fatalf("import: %d %s", code, body)
	}
	if desc, _, _ := dListed(t, h, admin, to); desc == nil || desc.MediaInfoTag != "code" || fmt.Sprint(desc.GameBlocks) != "[title platforms]" {
		t.Errorf("import did not apply the section: %+v", desc)
	}
	if _, b := dCompose(t, h, admin, fmt.Sprintf(dMediaBody, to)); dText(t, b) != dMediaCust {
		t.Errorf("imported tracker does not compose like the source: %s", b)
	}

	// Operator section is protected: another config, and the import doesn't replace it without request.
	mine := `{"media_blocks":["synopsis"],"media_info_tag":"pre"}`
	if code, b := tpCall(t, h, http.MethodPut, fmt.Sprintf(dSection, to), admin, mine); code != http.StatusOK {
		t.Fatalf("PUT: %d %s", code, b)
	}
	code, body = tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to), admin, file)
	if code != http.StatusOK || !strings.Contains(body, `"description":{"status":"protected"`) {
		t.Fatalf("import over own config: %d %s", code, body)
	}
	if desc, _, _ := dListed(t, h, admin, to); desc == nil || desc.MediaInfoTag != "pre" {
		t.Errorf("operator config was overwritten without request: %+v", desc)
	}
	// Requesting (?overwrite=description), it is replaced.
	code, body = tpCall(t, h, http.MethodPost, fmt.Sprintf(exImport, to)+"?overwrite=description", admin, file)
	if code != http.StatusOK || !strings.Contains(body, `"description":{"status":"replaced"`) {
		t.Fatalf("import with overwrite=description: %d %s", code, body)
	}
	if desc, _, _ := dListed(t, h, admin, to); desc == nil || desc.MediaInfoTag != "code" {
		t.Errorf("section was not replaced on request: %+v", desc)
	}

	// Audit: the apply says what it did with the section; the preview doesn't enter.
	var imports []string
	for _, a := range query(`SELECT action FROM audit_log WHERE path LIKE $1 ORDER BY created_at`, "%/trackers/"+to+"/profile/import") {
		if strings.HasPrefix(a, "Imported a profile from a file into tracker") {
			imports = append(imports, a[strings.LastIndex(a, "; description layout"):])
		}
	}
	want := []string{"; description layout: applied", "; description layout: kept the operator's", "; description layout: replaced on request"}
	if strings.Join(imports, "|") != strings.Join(want, "|") {
		t.Errorf("imports audit:\n got %q\nwant %q", imports, want)
	}
}

// The file is third-party content: the hostile section is rejected in the preview and in the
// import, before the database, and nothing is saved.
func TestTrackerDescription_HostileImport(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id := exTracker(t, h, admin, "Hostile Import Desc", fake.srv.URL)

	mk := func(section string) string {
		return `{"format":"seedgrid.tracker-profile","site":{"name":"S"},"family":"unit3d","schema_version":1,"taxonomy":{},"description":` + section + `}`
	}
	cases := map[string]string{
		"unknown block":                  mk(`{"media_blocks":["poster","x"]}`),
		"tag that escapes BBCode":        mk(`{"media_info_tag":"pre][url=http://internal/]"}`),
		"list with 30k items":            mk(`{"media_blocks":[` + strings.Repeat(`"poster",`, 30000) + `"poster"]}`),
		"unknown field in section":       mk(`{"html":"<script>"}`),
		"section that is not an object":  mk(`"poster"`),
		"item that is not text":          mk(`{"media_blocks":[{"a":1}]}`),
		"uppercase key in section":       mk(`{"Media_Info_Tag":"pre"}`),
		"duplicate key in section":       mk(`{"media_info_tag":"pre","media_info_tag":"code"}`),
		"spoofed origin in section":      mk(`{"origin":"manual"}`),
		"section inside another section": mk(`{"description":{"media_info_tag":"pre"}}`),
		"depth above limit":              mk(`{"media_blocks":[[[[[[[[[["poster"]]]]]]]]]]}`),
		"second unknown section":         strings.Replace(mk(`{}`), `"taxonomy":{}`, `"taxonomy":{},"rules":{"x":1}`, 1),
		"description with huge value":    mk(`{"media_info_tag":"` + strings.Repeat("a", 600<<10) + `"}`),
	}
	for name, file := range cases {
		for _, route := range []string{exPreview, exImport} {
			code, body := tpCall(t, h, http.MethodPost, fmt.Sprintf(route, id), admin, file)
			want := http.StatusBadRequest
			if strings.Contains(name, "huge") {
				want = http.StatusRequestEntityTooLarge
			}
			if code != want {
				t.Errorf("%s on %s: %d %.200s, want %d", name, route, code, body, want)
			}
			if len(body) > 1000 {
				t.Errorf("%s: rejection returns %d bytes", name, len(body))
			}
		}
	}
	if desc, _, _ := dListed(t, h, admin, id); desc != nil {
		t.Errorf("rejected file saved the section: %+v", desc)
	}
}
