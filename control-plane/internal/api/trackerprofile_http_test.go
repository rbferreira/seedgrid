package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/api"
	"github.com/rbferreira/seedgrid/control-plane/internal/audit"
	"github.com/rbferreira/seedgrid/control-plane/internal/auth"
	"github.com/rbferreira/seedgrid/control-plane/internal/command"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
	"github.com/rbferreira/seedgrid/control-plane/internal/tracker"
	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
	"github.com/rbferreira/seedgrid/control-plane/internal/unit3d"
	"github.com/rbferreira/seedgrid/control-plane/internal/users"
)

// FAKE credentials for the dummy tracker: they serve to prove that no profile API
// response returns them. They are nobody's secret.
const (
	siteToken   = "TOKEN-DA-API-DO-SITE-789" //nolint:gosec // G101: fake test credential
	sitePasskey = "PASSKEY-DO-ANNOUNCE-321"  //nolint:gosec // G101: fake test credential
)

// site is a mock UNIT3D: it serves /api/torrents/filter with the payload in the
// TorrentResource format (name and id of each axis in the same item), across 3 pages. The mode changes the
// behavior to exercise the tracker's faults.
type site struct {
	srv   *httptest.Server
	mode  atomic.Value // "ok" | "500" | "429-page-2"
	renam atomic.Bool  // the site admin renamed resolution 10 from "Other" to "Outros"
	hits  atomic.Int32
}

func torrentJSON(id int, cat string, catID int, typ string, typID int, res string, resID any) string {
	resJSON := "null"
	if res != "" {
		resJSON = strconv.Quote(res)
	}
	resIDJSON := "null"
	if resID != nil {
		resIDJSON = fmt.Sprint(resID)
	}
	return fmt.Sprintf(`{"type":"torrents","id":"%d","attributes":{"name":"Title %d","category":%q,"category_id":%d,`+
		`"type":%q,"type_id":%d,"resolution":%s,"resolution_id":%s,"size":1,"seeders":1,"leechers":0,`+
		`"times_completed":0,"download_link":"https://t/dl/%d?passkey=MUST-NOT-LEAK"}}`,
		id, id, cat, catID, typ, typID, resJSON, resIDJSON, id)
}

func newSite(t *testing.T) *site {
	t.Helper()
	s := &site{}
	s.mode.Store("ok")
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		if r.URL.Path != "/api/torrents/filter" || r.URL.Query().Get("api_token") != siteToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		switch s.mode.Load().(string) {
		case "500":
			http.Error(w, "internal site error", http.StatusInternalServerError)
			return
		case "429-page-2":
			if page == 2 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
		}
		other := "Other"
		if s.renam.Load() {
			other = "Outros"
		}
		pages := [][]string{
			{
				torrentJSON(1, "Filmes", 1, "WEB-DL", 4, "1080p", 3),
				torrentJSON(2, "Series", 2, "WEB-DL", 4, "2160p", 2),
				torrentJSON(3, "Games", 5, "PC", 50, "", nil),
			},
			{
				torrentJSON(4, "Filmes", 1, "Encode", 3, "1080p", 3),
				torrentJSON(5, "Animes", 3, "WEB-DL", 4, "1080p", 3),
			},
			{
				torrentJSON(6, "Programas", 9, "Windows", 71, "", nil),
				torrentJSON(7, "Series", 2, "WEB-DL", 4, other, 10),
			},
		}
		if page < 1 || page > len(pages) {
			http.Error(w, "out of catalog", http.StatusNotFound)
			return
		}
		next := "null"
		if page < len(pages) {
			next = fmt.Sprintf(`"https://site/api/torrents/filter?page=%d"`, page+1)
		}
		_, _ = fmt.Fprintf(w, `{"data":[%s],"meta":{"current_page":%d},"links":{"next":%s}}`,
			strings.Join(pages[page-1], ","), page, next)
	}))
	t.Cleanup(s.srv.Close)
	return s
}

// setupTrackerAPI brings up the full router, with the tracker and profile services, against the
// test database. It returns the handler, the session token for each role, and a read
// query (1st column) for the tests to check the audit trail. It does not tear down the
// schema: it creates its own user and trackers, to coexist with the other tests in the suite.
func setupTrackerAPI(t *testing.T) (http.Handler, map[string]string, func(sql string, args ...any) []string) {
	t.Helper()
	dsn := os.Getenv("SEEDGRID_API_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("SEEDGRID_API_TEST_DATABASE_URL/SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	// Trackers belong to the cluster (ADR-047): every test sees every tracker, and names and
	// the default are unique across the cluster. Each test starts with none, as a fresh cluster.
	if _, err := pool.Exec(context.Background(),
		`UPDATE trackers SET deleted_at = now(), is_default = false WHERE deleted_at IS NULL`); err != nil {
		t.Fatalf("clear trackers: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	authSvc := auth.NewService(pool, "reg-token")
	admin := fmt.Sprintf("admin_tp_%d", time.Now().UnixNano())
	if err := authSvc.Bootstrap(context.Background(), admin, "strong-password"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	usersSvc := users.NewService(pool)
	settingsStore := settings.New(pool)
	_ = settingsStore.Load(context.Background())
	commandSvc := command.NewService(pool)
	schedulerSvc := scheduler.NewService(pool)
	emitter := events.NewEmitter(pool)
	nodeSvc := node.NewService(pool, emitter)
	trackerSvc := tracker.NewService(pool)
	profileSvc := trackerprofile.NewService(pool)
	torrentSvc := torrent.NewService(pool, schedulerSvc, commandSvc, emitter)
	torrentSvc.SetTrackerProfileProvider(profileSvc)
	torrentSvc.SetImageHostKeyProvider(trackerSvc)

	handler := api.NewRouter(api.Deps{
		Logger:   logger,
		Pool:     pool,
		Auth:     authSvc,
		Users:    usersSvc,
		Audit:    audit.NewRecorder(pool, logger),
		Settings: settingsStore,
		Commands: commandSvc,
		Torrents: torrentSvc,
		Nodes:    nodeSvc,
		Trackers: trackerSvc,
		Profiles: profileSvc,
	})

	login := func(username string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			strings.NewReader(fmt.Sprintf(`{"username":%q,"password":"strong-password"}`, username))))
		if rec.Code != http.StatusOK {
			t.Fatalf("login %s: status %d: %s", username, rec.Code, rec.Body.String())
		}
		var out struct {
			Token string `json:"token"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return out.Token
	}
	tokens := map[string]string{"Administrator": login(admin)}
	for _, role := range []string{"Publisher", "Viewer"} {
		uname := fmt.Sprintf("%s_tp_%d", strings.ToLower(role), time.Now().UnixNano())
		if _, err := usersSvc.Create(context.Background(), users.CreateInput{
			Username: uname, Password: "strong-password", Roles: []string{role},
		}); err != nil {
			t.Fatalf("create %s: %v", role, err)
		}
		tokens[role] = login(uname)
	}

	// query returns the 1st column of each row, for the tests to read the audit trail.
	query := func(sql string, args ...any) []string {
		t.Helper()
		rows, err := pool.Query(context.Background(), sql, args...)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out = append(out, s)
		}
		return out
	}
	return handler, tokens, query
}

// tpCall executes the request and returns status + body.
func tpCall(t *testing.T, h http.Handler, method, path, token, body string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func tpDecode(t *testing.T, body string, dst any) {
	t.Helper()
	if err := json.Unmarshal([]byte(body), dst); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
}

type termOut struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Origin string `json:"origin"`
}

type profileOut struct {
	Family        string               `json:"family"`
	SchemaVersion int                  `json:"schema_version"`
	Taxonomy      map[string][]termOut `json:"taxonomy"`
}

type propItem struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	Seen          int    `json:"seen"`
	Status        string `json:"status"`
	CurrentName   string `json:"current_name"`
	CurrentOrigin string `json:"current_origin"`
	Protected     bool   `json:"protected"`
}

type discoverOut struct {
	Family         string                `json:"family"`
	PagesRequested int                   `json:"pages_requested"`
	PagesScanned   int                   `json:"pages_scanned"`
	ItemsScanned   int                   `json:"items_scanned"`
	StoppedReason  string                `json:"stopped_reason"`
	Proposal       map[string][]propItem `json:"proposal"`
	Rejected       int                   `json:"rejected"`
	Warnings       []string              `json:"warnings"`
	Notice         string                `json:"notice"`
}

func createTracker(t *testing.T, h http.Handler, token, name, apiURL string) (id string, raw string) {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"announce":"https://tracker.example.org/announce/%s","source":"TEST","api_url":%q,"api_token":%q}`,
		name, sitePasskey, apiURL, siteToken)
	code, resp := tpCall(t, h, http.MethodPost, "/api/v1/trackers", token, body)
	if code != http.StatusCreated {
		t.Fatalf("create tracker: %d %s", code, resp)
	}
	var out struct {
		ID string `json:"id"`
	}
	tpDecode(t, resp, &out)
	return out.ID, resp
}

func itemsOf(p map[string][]propItem, axis string) map[int]propItem {
	m := map[int]propItem{}
	for _, it := range p[axis] {
		m[it.ID] = it
	}
	return m
}

// The full cycle of #196 via the API: discover only PROPOSES; the operator adjusts manually; discovering
// again protects the adjustment; applying completes what's missing; running it again changes nothing; and
// the audit trail logs the apply (and only it).
func TestTrackerProfile_DiscoveryCycle(t *testing.T) {
	h, tokens, query := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, createResp := createTracker(t, h, admin, "Test Tracker", fake.srv.URL)

	// The tracker starts with an empty profile (all axes present) and NO credentials in the response.
	var created struct {
		Profile profileOut `json:"profile"`
	}
	tpDecode(t, createResp, &created)
	if created.Profile.Family != "unit3d" || created.Profile.SchemaVersion != 1 {
		t.Errorf("new profile = %+v, want family unit3d and schema_version 1", created.Profile)
	}
	for _, axis := range trackerprofile.Axes {
		if created.Profile.Taxonomy[axis] == nil || len(created.Profile.Taxonomy[axis]) != 0 {
			t.Errorf("axis %s of new profile = %v, want empty list", axis, created.Profile.Taxonomy[axis])
		}
	}

	// 1. Discover: 3 pages, end of catalog, everything "new" — and NOTHING saved.
	code, body := tpCall(t, h, http.MethodPost, "/api/v1/trackers/"+id+"/discover-taxonomy", admin, `{"pages":5}`)
	if code != http.StatusOK {
		t.Fatalf("discover: %d %s", code, body)
	}
	var d1 discoverOut
	tpDecode(t, body, &d1)
	if d1.PagesRequested != 5 || d1.PagesScanned != 3 || d1.ItemsScanned != 7 || d1.StoppedReason != "end_of_catalog" {
		t.Errorf("scan = %+v, want 5 requested/3 scanned/7 items/end_of_catalog", d1)
	}
	if !strings.Contains(d1.Notice, "only sees what has already been posted") {
		t.Errorf("response does not carry notice that discovery is incomplete: %q", d1.Notice)
	}
	cat := itemsOf(d1.Proposal, "category")
	if len(cat) != 5 || cat[2].Name != "Series" || cat[2].Seen != 2 || cat[2].Status != "new" {
		t.Errorf("proposed category = %+v", cat)
	}
	if typ := itemsOf(d1.Proposal, "type"); typ[50].Name != "PC" || typ[71].Name != "Windows" {
		t.Errorf("type proposal = %+v, want PC(50) and Windows(71) — which Portal never offered", typ)
	}
	_, list := tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	var l1 struct {
		Data []struct {
			ID      string     `json:"id"`
			Profile profileOut `json:"profile"`
		} `json:"data"`
	}
	tpDecode(t, list, &l1)
	for _, tr := range l1.Data {
		if tr.ID == id && len(tr.Profile.Taxonomy["category"]) != 0 {
			t.Fatal("discover WROTE to profile — discovery can only propose")
		}
	}
	if n := len(query(`SELECT action FROM audit_log WHERE path LIKE $1`, "%/trackers/"+id+"/discover-taxonomy")); n != 0 {
		t.Errorf("discover entered audit log (%d lines) — only proposes, does not change state", n)
	}

	// 2. The operator types manually: renames the series and confirms Movies.
	code, body = tpCall(t, h, http.MethodPut, "/api/v1/trackers/"+id+"/profile", admin,
		`{"taxonomy":{"category":[{"id":1,"name":"Filmes","kind":"movie"},{"id":2,"name":"Seriados","kind":"tv"}]}}`)
	if code != http.StatusOK {
		t.Fatalf("PUT profile: %d %s", code, body)
	}

	// 3. Discover AGAIN: the manual adjustment appears as protected.
	_, body = tpCall(t, h, http.MethodPost, "/api/v1/trackers/"+id+"/discover-taxonomy", admin, ``)
	var d2 discoverOut
	tpDecode(t, body, &d2)
	if d2.PagesRequested != unit3d.DefaultDiscoverPages {
		t.Errorf("empty body: pages_requested = %d, want default %d", d2.PagesRequested, unit3d.DefaultDiscoverPages)
	}
	c2 := itemsOf(d2.Proposal, "category")
	if c2[1].Status != "same" {
		t.Errorf("Filmes = %+v, want same", c2[1])
	}
	if c2[2].Status != "renamed" || !c2[2].Protected || c2[2].CurrentName != "Seriados" || c2[2].CurrentOrigin != "manual" {
		t.Errorf("Series = %+v, want renamed + protected (operator renamed it manually)", c2[2])
	}

	// 4. Apply EVERYTHING the proposal showed: only what's missing goes in, the manual stays.
	var items []string
	for axis, its := range d2.Proposal {
		for _, it := range its {
			items = append(items, fmt.Sprintf(`{"axis":%q,"id":%d,"name":%q}`, axis, it.ID, it.Name))
		}
	}
	applyBody := `{"items":[` + strings.Join(items, ",") + `]}`
	code, body = tpCall(t, h, http.MethodPost, "/api/v1/trackers/"+id+"/apply-taxonomy", admin, applyBody)
	if code != http.StatusOK {
		t.Fatalf("apply: %d %s", code, body)
	}
	var applied struct {
		Profile profileOut `json:"profile"`
		Applied []struct {
			Axis, Name, Action string
			ID                 int
		} `json:"applied"`
		Skipped []struct {
			Axis, Name, Reason string
			ID                 int
		} `json:"skipped"`
	}
	tpDecode(t, body, &applied)
	if len(applied.Applied) != 10 {
		t.Errorf("applied = %d, want 10 (3 categories + 4 types + 3 resolutions)", len(applied.Applied))
	}
	protected := 0
	for _, s := range applied.Skipped {
		if s.Reason == "protected" && s.Axis == "category" && s.ID == 2 {
			protected++
		}
	}
	if protected != 1 || len(applied.Skipped) != 2 {
		t.Errorf("ignored = %+v, want Filmes (same) and Series (protected)", applied.Skipped)
	}
	byID := map[int]termOut{}
	for _, tm := range applied.Profile.Taxonomy["category"] {
		byID[tm.ID] = tm
	}
	if byID[2].Name != "Seriados" || byID[2].Origin != "manual" || byID[2].Kind != "tv" {
		t.Errorf("manual adjustment was overwritten: %+v", byID[2])
	}
	if byID[9].Origin != "discovered" || byID[9].Name != "Programas" {
		t.Errorf("Programas should have entered as discovered: %+v", byID[9])
	}

	// 5. Running again is incremental: nothing new to apply.
	code, body = tpCall(t, h, http.MethodPost, "/api/v1/trackers/"+id+"/apply-taxonomy", admin, applyBody)
	if code != http.StatusOK {
		t.Fatalf("2º apply: %d %s", code, body)
	}
	var again struct {
		Applied []any `json:"applied"`
	}
	tpDecode(t, body, &again)
	if len(again.Applied) != 0 {
		t.Errorf("2nd apply applied %d items, want 0", len(again.Applied))
	}

	// 6. A `discovered` term follows the site: if the admin renamed it, the proposal offers the
	// renaming (without protection) and the apply executes it — unlike the manual.
	fake.renam.Store(true)
	_, body = tpCall(t, h, http.MethodPost, "/api/v1/trackers/"+id+"/discover-taxonomy", admin, ``)
	var d3 discoverOut
	tpDecode(t, body, &d3)
	r10 := itemsOf(d3.Proposal, "resolution")[10]
	if r10.Status != "renamed" || r10.Protected || r10.CurrentName != "Other" || r10.Name != "Outros" {
		t.Errorf("resolution 10 = %+v, want renamed and NOT protected (origin is discovered)", r10)
	}
	code, body = tpCall(t, h, http.MethodPost, "/api/v1/trackers/"+id+"/apply-taxonomy", admin,
		`{"items":[{"axis":"resolution","id":10,"name":"Outros"}]}`)
	if code != http.StatusOK || !strings.Contains(body, `"action":"renamed"`) {
		t.Errorf("apply rename: %d %s", code, body)
	}

	// 7. Audit: the manual edit and the applies, with the tracker name; never the discover.
	actions := query(`SELECT action FROM audit_log WHERE path LIKE $1 ORDER BY created_at`, "%/trackers/"+id+"/%")
	want := []string{
		"Edited the profile of tracker «Test Tracker»",
		"Applied the discovered taxonomy to tracker «Test Tracker»: 10 new, 0 renamed",
		"Applied the discovered taxonomy to tracker «Test Tracker»: 0 new, 0 renamed",
		"Applied the discovered taxonomy to tracker «Test Tracker»: 0 new, 1 renamed",
	}
	if strings.Join(actions, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit:\n got %q\nwant %q", actions, want)
	}

	// 8. Nothing from layer 3 appears in ANY profile response.
	for _, secret := range []string{siteToken, sitePasskey, "MUST-NOT-LEAK", "announce"} {
		for name, resp := range map[string]string{"create": createResp, "list": list, "discover": body} {
			if strings.Contains(resp, secret) {
				t.Errorf("response from %s leaks %q", name, secret)
			}
		}
	}
}

// Profile is untrusted content: every hostile payload is rejected BEFORE the database, with a 400 and
// a message saying which field failed — and never with a 500.
func TestTrackerProfile_HostilePayloads(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, _ := createTracker(t, h, admin, "Hostile", fake.srv.URL)
	put := "/api/v1/trackers/" + id + "/profile"
	apply := "/api/v1/trackers/" + id + "/apply-taxonomy"

	cases := []struct {
		name, method, path, body string
	}{
		{"non-JSON body", "PUT", put, `{taxonomy`},
		{"empty body", "PUT", put, ``},
		{"unknown top-level field", "PUT", put, `{"taxonomy":{"type":[]},"api_token":"x"}`},
		{"unknown field in term", "PUT", put, `{"taxonomy":{"type":[{"id":1,"name":"a","announce":"http://x"}]}}`},
		{"origin sent by client", "PUT", put, `{"taxonomy":{"type":[{"id":1,"name":"a","origin":"seed"}]}}`},
		{"empty taxonomy", "PUT", put, `{"taxonomy":{}}`},
		{"unknown axis", "PUT", put, `{"taxonomy":{"audio":[{"id":1,"name":"a"}]}}`},
		{"negative id", "PUT", put, `{"taxonomy":{"type":[{"id":-1,"name":"a"}]}}`},
		{"id out of range", "PUT", put, `{"taxonomy":{"type":[{"id":99999999999,"name":"a"}]}}`},
		{"id as string", "PUT", put, `{"taxonomy":{"type":[{"id":"1","name":"a"}]}}`},
		{"duplicate id", "PUT", put, `{"taxonomy":{"type":[{"id":1,"name":"a"},{"id":1,"name":"b"}]}}`},
		{"name with direction override", "PUT", put, `{"taxonomy":{"type":[{"id":1,"name":"a\u202eb"}]}}`},
		{"name with NUL", "PUT", put, `{"taxonomy":{"type":[{"id":1,"name":"a\u0000b"}]}}`},
		{"name too long", "PUT", put, `{"taxonomy":{"type":[{"id":1,"name":"` + strings.Repeat("x", 65) + `"}]}}`},
		{"kind in wrong axis", "PUT", put, `{"taxonomy":{"type":[{"id":1,"name":"a","kind":"movie"}]}}`},
		{"unknown kind", "PUT", put, `{"taxonomy":{"category":[{"id":1,"name":"a","kind":"admin"}]}}`},
		{"huge body", "PUT", put, `{"taxonomy":{"type":[{"id":1,"name":"` + strings.Repeat("x", 2<<20) + `"}]}}`},
		{"apply without items", "POST", apply, `{"items":[]}`},
		{"apply with hostile axis", "POST", apply, `{"items":[{"axis":"'; DROP TABLE trackers;--","id":1,"name":"a"}]}`},
		{"apply with origin seed", "POST", apply, `{"items":[{"axis":"type","id":1,"name":"a","origin":"seed"}]}`},
		{"apply with unknown field", "POST", apply, `{"items":[{"axis":"type","id":1,"name":"a","kind":"tv"}]}`},
		{"apply with hostile name", "POST", apply, `{"items":[{"axis":"type","id":1,"name":"a\u200bb"}]}`},
		{"apply with duplicate item", "POST", apply, `{"items":[{"axis":"type","id":1,"name":"a"},{"axis":"type","id":1,"name":"b"}]}`},
		{"discover with negative pages", "POST", "/api/v1/trackers/" + id + "/discover-taxonomy", `{"pages":-3}`},
		{"discover with unknown field", "POST", "/api/v1/trackers/" + id + "/discover-taxonomy", `{"url":"http://internal/"}`},
		{"discover with pages as string", "POST", "/api/v1/trackers/" + id + "/discover-taxonomy", `{"pages":"many"}`},
	}
	for _, tc := range cases {
		code, body := tpCall(t, h, tc.method, tc.path, admin, tc.body)
		if code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%.200s)", tc.name, code, body)
		}
	}

	// Nothing hostile reached the database: the profile remains empty.
	_, list := tpCall(t, h, http.MethodGet, "/api/v1/trackers", admin, "")
	if strings.Contains(list, `"name":"a`) {
		t.Errorf("hostile payload was persisted: %.300s", list)
	}

	// Nonexistent/malformed Tracker: 404, never 500 (uuid cast error).
	for _, bad := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid", "1%27%20OR%201=1"} {
		for _, r := range []struct{ method, suffix, body string }{
			{"PUT", "/profile", `{"taxonomy":{"type":[]}}`},
			{"POST", "/discover-taxonomy", ``},
			{"POST", "/apply-taxonomy", `{"items":[{"axis":"type","id":1,"name":"a"}]}`},
		} {
			if code, resp := tpCall(t, h, r.method, "/api/v1/trackers/"+bad+r.suffix, admin, r.body); code != http.StatusNotFound {
				t.Errorf("%s%s: status %d, want 404 (%.200s)", bad, r.suffix, code, resp)
			}
		}
	}
}

// Requesting more pages than the ceiling doesn't turn into a larger scan; and the tracker's rate limit
// midway through the scan returns what has already been collected, warning that it stopped.
func TestTrackerProfile_PageCapAndRateLimit(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	fake := newSite(t)
	id, _ := createTracker(t, h, admin, "Cap", fake.srv.URL)
	path := "/api/v1/trackers/" + id + "/discover-taxonomy"

	_, body := tpCall(t, h, http.MethodPost, path, admin, `{"pages":100000}`)
	var d discoverOut
	tpDecode(t, body, &d)
	if d.PagesRequested != unit3d.MaxDiscoverPages {
		t.Errorf("pages_requested = %d, want the ceiling %d", d.PagesRequested, unit3d.MaxDiscoverPages)
	}

	fake.mode.Store("429-page-2")
	code, body := tpCall(t, h, http.MethodPost, path, admin, `{"pages":5}`)
	if code != http.StatusOK {
		t.Fatalf("429 midway: %d %s", code, body)
	}
	var partial discoverOut
	tpDecode(t, body, &partial)
	if partial.StoppedReason != "rate_limited" || partial.PagesScanned != 1 || len(partial.Proposal["category"]) == 0 {
		t.Errorf("partial = %+v, want rate_limited with 1st page retained", partial)
	}
}

// Tracker failure on the FIRST page turns into a 502 with a useful message — and the message never
// contains the operator's api_token, not even on network error (the request URL carries the token).
func TestTrackerProfile_TrackerFailureNoTokenLeak(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]

	down := newSite(t)
	downURL := down.srv.URL
	down.srv.Close() // connection refused: the network error brings the entire URL
	idDown, _ := createTracker(t, h, admin, "Down", downURL)

	broken := newSite(t)
	broken.mode.Store("500")
	idBroken, _ := createTracker(t, h, admin, "Broken", broken.srv.URL)

	for name, id := range map[string]string{"down": idDown, "500": idBroken} {
		code, body := tpCall(t, h, http.MethodPost, "/api/v1/trackers/"+id+"/discover-taxonomy", admin, ``)
		if code != http.StatusBadGateway || !strings.Contains(body, "discover_failed") {
			t.Errorf("%s: %d %s, want 502 discover_failed", name, code, body)
		}
		if strings.Contains(body, siteToken) || strings.Contains(body, "api_token") {
			t.Errorf("%s: the error response leaks the token: %s", name, body)
		}
	}
}

// Tracker without API integration (no api_url/token) has no catalog to scan.
func TestTrackerProfile_NoAPIIntegration(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin := tokens["Administrator"]
	code, body := tpCall(t, h, http.MethodPost, "/api/v1/trackers", admin,
		`{"name":"No API","announce":"https://tracker.example.org/announce/abc"}`)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %s", code, body)
	}
	var tr struct {
		ID string `json:"id"`
	}
	tpDecode(t, body, &tr)
	code, body = tpCall(t, h, http.MethodPost, "/api/v1/trackers/"+tr.ID+"/discover-taxonomy", admin, ``)
	if code != http.StatusBadRequest || !strings.Contains(body, "tracker_without_api") {
		t.Errorf("%d %s, want 400 tracker_without_api", code, body)
	}
}

// RBAC in the standard of tracker endpoints: reading the profile (it comes in the listing) is trackers:read;
// editing, discovering, and applying are trackers:write, which only the Administrator holds (ADR-047).
func TestTrackerProfile_RBAC(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	fake := newSite(t)
	id, _ := createTracker(t, h, tokens["Administrator"], "RBAC", fake.srv.URL)

	routes := []struct{ method, path, body string }{
		{"PUT", "/api/v1/trackers/" + id + "/profile", `{"taxonomy":{"type":[{"id":1,"name":"a"}]}}`},
		{"POST", "/api/v1/trackers/" + id + "/discover-taxonomy", ``},
		{"POST", "/api/v1/trackers/" + id + "/apply-taxonomy", `{"items":[{"axis":"type","id":2,"name":"b"}]}`},
	}
	for _, r := range routes {
		for _, role := range []string{"Viewer", "Publisher"} {
			if code, _ := tpCall(t, h, r.method, r.path, tokens[role], r.body); code != http.StatusForbidden {
				t.Errorf("%s on %s %s: %d, want 403", role, r.method, r.path, code)
			}
		}
		if code, body := tpCall(t, h, r.method, r.path, tokens["Administrator"], r.body); code != http.StatusOK {
			t.Errorf("Administrator on %s %s: %d %.200s, want 200", r.method, r.path, code, body)
		}
	}
	// Reading (the listing loads the profile) is trackers:read, and the listing is the cluster's: the
	// Viewer and the Publisher see the Administrator's tracker with its profile.
	for _, role := range []string{"Viewer", "Publisher"} {
		if code, body := tpCall(t, h, http.MethodGet, "/api/v1/trackers", tokens[role], ""); code != http.StatusOK ||
			!strings.Contains(body, id) || !strings.Contains(body, `"profile"`) {
			t.Errorf("%s reading trackers: %d %.200s, want 200 with the tracker and its profile", role, code, body)
		}
	}
}

// Trackers belong to the cluster and the Administrator manages them (ADR-047): the Publisher picks
// trackers but does not create, edit, remove or set the default; names are unique across the cluster.
func TestTrackers_ManagedByTheAdministrator(t *testing.T) {
	h, tokens, _ := setupTrackerAPI(t)
	admin, pub := tokens["Administrator"], tokens["Publisher"]
	fake := newSite(t)
	id, _ := createTracker(t, h, admin, "Cluster", fake.srv.URL)

	writes := []struct{ method, path, body string }{
		{"POST", "/api/v1/trackers", `{"name":"Own","announce":"https://tracker.example.org/announce/k"}`},
		{"PUT", "/api/v1/trackers/" + id, `{"name":"Renamed","announce":""}`},
		{"POST", "/api/v1/trackers/" + id + "/default", ``},
		{"DELETE", "/api/v1/trackers/" + id, ``},
	}
	for _, r := range writes {
		if code, _ := tpCall(t, h, r.method, r.path, pub, r.body); code != http.StatusForbidden {
			t.Errorf("Publisher on %s %s: %d, want 403", r.method, r.path, code)
		}
	}
	if code, body := tpCall(t, h, http.MethodPost, "/api/v1/trackers", admin,
		`{"name":"Cluster","announce":"https://other.example.org/announce/k"}`); code != http.StatusConflict {
		t.Errorf("second tracker named Cluster: %d %.200s, want 409", code, body)
	}
	for _, r := range writes[1:] {
		if code, body := tpCall(t, h, r.method, r.path, admin, r.body); code >= 300 {
			t.Errorf("Administrator on %s %s: %d %.200s, want success", r.method, r.path, code, body)
		}
	}
}
