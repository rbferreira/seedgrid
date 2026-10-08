package unit3d

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// page builds a /api/torrents/filter page in the UNIT3D (JSON:API) format: each
// item brings the NAME and ID of each axis in the same `attributes` — it is the payload that discovery
// consumes. `next` links the page's links.next (empty = last).
func page(current int, next bool, items ...string) string {
	nextJSON := "null"
	if next {
		nextJSON = fmt.Sprintf(`"https://tracker.example/api/torrents/filter?page=%d"`, current+1)
	}
	return fmt.Sprintf(`{"data":[%s],"meta":{"current_page":%d,"per_page":50},"links":{"next":%s}}`,
		strings.Join(items, ","), current, nextJSON)
}

// item is a catalog torrent. The *_id come in exactly as the real payload: number, string
// or null — UNIT3D is loose with types and resolution is optional.
func item(id int, cat, catID, typ, typID, res, resID string) string {
	return fmt.Sprintf(`{"type":"torrents","id":"%d","attributes":{"name":"Title %d","category":%s,"category_id":%s,`+
		`"type":%s,"type_id":%s,"resolution":%s,"resolution_id":%s,"size":1,"seeders":1,"leechers":0,`+
		`"times_completed":0,"download_link":"https://t/dl/%d.key"}}`, id, id, cat, catID, typ, typID, res, resID, id)
}

// The *_id accompany the names in the same item and BrowseItem now carries them (#196) —
// before they were discarded. It covers the three ways UNIT3D sends a number.
func TestBrowse_LoadsTaxonomyIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page(1, false,
			item(1, `"Filmes"`, `1`, `"WEB-DL"`, `"4"`, `"1080p"`, `3`), // type_id as STRING
			item(2, `"Series"`, `2`, `"Encode"`, `3`, `null`, `null`),
		)))
	}))
	defer srv.Close()

	res, err := Browse(context.Background(), srv.URL, "tok", "", "")
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	a, b := res.Items[0], res.Items[1]
	if a.CategoryID != 1 || a.TypeID != 4 || a.ResolutionID != 3 {
		t.Errorf("item0 ids = (%d,%d,%d), want (1,4,3) — type_id came as string", a.CategoryID, a.TypeID, a.ResolutionID)
	}
	if b.CategoryID != 2 || b.TypeID != 3 || b.ResolutionID != 0 || b.Resolution != "" {
		t.Errorf("item1 = %+v, want missing resolution (0, \"\") when tracker sends null", b)
	}
}

// catalogServer serves pages in sequence: page N is pages[N-1]. It counts the
// requests, to prove that the page limit is actually respected.
func catalogServer(t *testing.T, pages []string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		n, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if n < 1 || n > len(pages) {
			http.Error(w, "out of catalog", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(pages[n-1]))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// The core case: the same pair (name, id) appears on multiple pages and becomes ONE term, with a
// count of how many times it was seen; what the tracker sends as null does not become a term.
func TestDiscover_AggregatesAndDeduplicatesPairs(t *testing.T) {
	srv, hits := catalogServer(t, []string{
		page(1, true,
			item(1, `"Filmes"`, `1`, `"WEB-DL"`, `4`, `"1080p"`, `3`),
			item(2, `"Filmes"`, `1`, `"Encode"`, `3`, `"1080p"`, `3`),
			item(3, `"Games"`, `5`, `"PC"`, `50`, `null`, `null`),
		),
		page(2, true,
			item(4, `"Series"`, `2`, `"WEB-DL"`, `4`, `"2160p"`, `2`),
			item(5, `"Filmes"`, `1`, `"WEB-DL"`, `4`, `"1080p"`, `3`),
		),
		page(3, false,
			item(6, `"Programas"`, `9`, `"Windows"`, `71`, `""`, `0`),
		),
	})

	d := DiscoverTaxonomy(context.Background(), srv.URL, "tok", DiscoverOptions{MaxPages: 10})

	if d.Err != nil {
		t.Fatalf("Err = %v, want nil", d.Err)
	}
	if d.Stopped != StopEndOfCatalog {
		t.Errorf("Stopped = %q, want %q (page 3 has no links.next)", d.Stopped, StopEndOfCatalog)
	}
	if d.PagesScanned != 3 || d.ItemsScanned != 6 || hits.Load() != 3 {
		t.Errorf("pages=%d items=%d requests=%d, want 3/6/3", d.PagesScanned, d.ItemsScanned, hits.Load())
	}

	want := map[string][]ObservedTerm{
		AxisCategory: {
			{1, "Filmes", 3}, {2, "Series", 1}, {5, "Games", 1}, {9, "Programas", 1},
		},
		AxisType: {
			{3, "Encode", 1}, {4, "WEB-DL", 3}, {50, "PC", 1}, {71, "Windows", 1},
		},
		// Two items without resolution (null and "") are left out — null is not a term.
		AxisResolution: {
			{2, "2160p", 1}, {3, "1080p", 3},
		},
	}
	for axis, terms := range want {
		got := d.Axes[axis]
		if fmt.Sprint(got) != fmt.Sprint(terms) {
			t.Errorf("axis %s:\n got %v\nwant %v", axis, got, terms)
		}
	}
}

// The page limit is obeyed and the result says it STOPPED at the limit — which means
// "there is more catalog that was not seen", unlike end_of_catalog.
func TestDiscover_RespectsPageCeiling(t *testing.T) {
	var pages []string
	for i := 1; i <= 6; i++ {
		pages = append(pages, page(i, true, item(i, `"Filmes"`, `1`, `"Encode"`, `3`, `"1080p"`, `3`)))
	}
	srv, hits := catalogServer(t, pages)

	d := DiscoverTaxonomy(context.Background(), srv.URL, "tok", DiscoverOptions{MaxPages: 2})

	if d.Stopped != StopMaxPages || d.PagesScanned != 2 || hits.Load() != 2 {
		t.Errorf("Stopped=%q pages=%d requests=%d, want max_pages/2/2", d.Stopped, d.PagesScanned, hits.Load())
	}
}

// An absurd request does not turn into an absurd scan: the limit holds even if the caller
// asks for more — the operator's request quota on the site is theirs, not discovery's.
func TestDiscover_RequestAboveCeilingIsLimited(t *testing.T) {
	var calls int
	fetch := func(_ context.Context, _ string) (BrowseResult, error) {
		calls++
		return BrowseResult{NextCursor: strconv.Itoa(calls + 1)}, nil
	}
	d := Discover(context.Background(), fetch, DiscoverOptions{MaxPages: 100000})
	if calls != MaxDiscoverPages || d.Stopped != StopMaxPages {
		t.Errorf("requests=%d Stopped=%q, want %d/max_pages", calls, d.Stopped, MaxDiscoverPages)
	}

	calls = 0
	_ = Discover(context.Background(), fetch, DiscoverOptions{}) // zero = default
	if calls != DefaultDiscoverPages {
		t.Errorf("without MaxPages: requests=%d, want default %d", calls, DefaultDiscoverPages)
	}
}

// Rate limit during the scan does NOT discard what was already gathered: it returns pages
// 1 and 2 with Stopped=rate_limited and the typed error, and the caller shows the partial proposal.
func TestDiscover_RateLimitInMiddleReturnsPartial(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Query().Get("page") == "3" {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		n, _ := strconv.Atoi(r.URL.Query().Get("page"))
		_, _ = w.Write([]byte(page(n, true, item(n, `"Filmes"`, `1`, `"Encode"`, `3`, `"1080p"`, `3`))))
	}))
	defer srv.Close()

	d := DiscoverTaxonomy(context.Background(), srv.URL, "tok", DiscoverOptions{MaxPages: 10})

	if d.Stopped != StopRateLimited || !errors.Is(d.Err, ErrRateLimited) {
		t.Errorf("Stopped=%q Err=%v, want rate_limited with ErrRateLimited", d.Stopped, d.Err)
	}
	if d.PagesScanned != 2 || len(d.Axes[AxisCategory]) != 1 || d.Axes[AxisCategory][0].Seen != 2 {
		t.Errorf("partial = pages %d, categories %v; want 2 pages and Filmes seen 2x", d.PagesScanned, d.Axes[AxisCategory])
	}
	if hits.Load() != 3 {
		t.Errorf("requests = %d, want 3 (and none after 429)", hits.Load())
	}
}

// Failure on the FIRST page: there is nothing to show, and Err reaches the caller to become
// an error response instead of an empty proposal that would look like "catalog without taxonomy".
func TestDiscover_FailsOnFirstPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := DiscoverTaxonomy(context.Background(), srv.URL, "tok", DiscoverOptions{})
	if d.Err == nil || d.PagesScanned != 0 || d.Stopped != StopError {
		t.Errorf("Err=%v pages=%d Stopped=%q, want error/0/error", d.Err, d.PagesScanned, d.Stopped)
	}
}

// Time budget exceeded: returns what it gathered. The Control Plane HTTP server
// closes the response in 30 s, so a slow scan must finish BEFORE that with a
// partial proposal, and not be cut off without any response.
func TestDiscover_TimeBudgetReturnsPartial(t *testing.T) {
	calls := 0
	fetch := func(ctx context.Context, _ string) (BrowseResult, error) {
		calls++
		if calls == 1 {
			return BrowseResult{
				Items:      []BrowseItem{{Category: "Filmes", CategoryID: 1}},
				NextCursor: "2",
			}, nil
		}
		<-ctx.Done() // the second page never responds
		return BrowseResult{}, ctx.Err()
	}

	d := Discover(context.Background(), fetch, DiscoverOptions{MaxPages: 5, Budget: 50 * time.Millisecond})

	if d.Stopped != StopTimeBudget || d.PagesScanned != 1 || len(d.Axes[AxisCategory]) != 1 {
		t.Errorf("Stopped=%q pages=%d categories=%v, want time_budget/1/[Filmes]", d.Stopped, d.PagesScanned, d.Axes[AxisCategory])
	}
}

// CALLER cancellation (the operator closed the tab) is not a budget overrun: it is an error.
func TestDiscover_CallerCancellationIsNotBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fetch := func(ctx context.Context, _ string) (BrowseResult, error) { return BrowseResult{}, ctx.Err() }

	d := Discover(ctx, fetch, DiscoverOptions{})
	if d.Stopped != StopError {
		t.Errorf("Stopped = %q, want %q", d.Stopped, StopError)
	}
}

// id <= 0 and blank name do not become a term; leading/trailing spaces do not create a "new" pair.
func TestDiscover_IgnoresEmptyAndTrimsNames(t *testing.T) {
	fetch := func(_ context.Context, _ string) (BrowseResult, error) {
		return BrowseResult{Items: []BrowseItem{
			{Category: "Filmes", CategoryID: 1},
			{Category: "  Filmes  ", CategoryID: 1},
			{Category: "no id", CategoryID: 0},
			{Category: "   ", CategoryID: 7},
			{Category: "Negative", CategoryID: -3},
		}}, nil
	}
	d := Discover(context.Background(), fetch, DiscoverOptions{})
	got := d.Axes[AxisCategory]
	if len(got) != 1 || got[0].Name != "Filmes" || got[0].Seen != 2 {
		t.Errorf("categories = %v, want only Filmes seen 2x", got)
	}
}

// The http.Client network error carries the entire URL — and here the URL has the api_token in the
// query. Without truncation, "the tracker is down" becomes the operator's token in the response and
// the log. The test uses an already closed server to trigger a real connection error.
func TestBrowse_NetworkErrorDoesNotLeakToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	const token = "SECRET-api-token-123" //nolint:gosec // G101: fake test token
	_, err := Browse(context.Background(), url, token, "", "")
	if err == nil {
		t.Fatal("Browse against closed server returned nil")
	}
	if strings.Contains(err.Error(), token) || strings.Contains(err.Error(), "api_token") {
		t.Errorf("error message leaks the URL with the token: %v", err)
	}
}
