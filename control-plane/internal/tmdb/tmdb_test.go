package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDisabled(t *testing.T) {
	c := NewClient(func() string { return "" })
	if c.Enabled() {
		t.Fatal("Enabled() = true without key")
	}
	if _, err := c.Lookup(context.Background(), "X", 2020, false, "pt-BR"); !errors.Is(err, ErrDisabled) {
		t.Errorf("Lookup without key = %v, want ErrDisabled", err)
	}
}

// ADR-029: the key comes from a provider read per call. Changing it in the Portal (here, changing what the
// closure returns) works without recreating the client — enable/disable and change the api_key
// on the fly, without redeploy.
func TestKeyHotReload(t *testing.T) {
	key := "" // "empty in the catalog"
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Query().Get("api_key")
		_, _ = w.Write([]byte(`{"results":[{"id":1,"title":"X","release_date":"2020-01-01"}]}`))
	}))
	defer srv.Close()
	c := NewClient(func() string { return key }).WithBase(srv.URL)

	if c.Enabled() {
		t.Error("no key in catalog, should be disabled")
	}
	key = "nova-chave" // operator filled it in the panel
	if !c.Enabled() {
		t.Error("after setting the key, should enable without recreating the client")
	}
	if _, err := c.Lookup(context.Background(), "X", 2020, false, "pt-BR"); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if seen != "nova-chave" {
		t.Errorf("api_key sent = %q, want the CURRENT key from the provider", seen)
	}
	key = "" // operator cleared → shuts down immediately
	if c.Enabled() {
		t.Error("after clearing the key, should disable")
	}
}

func TestLookupMovie(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/3/search/movie"):
			if r.URL.Query().Get("query") != "Inception" || r.URL.Query().Get("year") != "2010" {
				t.Errorf("unexpected query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"results":[{"id":27205,"title":"Inception","overview":"Cobb...","poster_path":"/x.jpg","release_date":"2010-07-15"}]}`))
		case r.URL.Path == "/3/movie/27205/external_ids":
			_, _ = w.Write([]byte(`{"imdb_id":"tt1375666"}`))
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer srv.Close()

	c := NewClient(func() string { return "k" }).WithBase(srv.URL)
	got, err := c.Lookup(context.Background(), "Inception", 2010, false, "pt-BR")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.TMDBID != 27205 || got.IMDBID != "tt1375666" || got.Title != "Inception" || got.Year != 2010 {
		t.Errorf("result = %+v", got)
	}
	if got.IsTV {
		t.Errorf("IsTV = true, want movie")
	}
	if !strings.HasSuffix(got.PosterURL, "/x.jpg") {
		t.Errorf("poster = %q", got.PosterURL)
	}
}

func TestLookupTV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/3/search/tv"):
			if r.URL.Query().Get("first_air_date_year") != "2016" {
				t.Errorf("missing first_air_date_year: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"results":[{"id":66732,"name":"Stranger Things","first_air_date":"2016-07-15"}]}`))
		case r.URL.Path == "/3/tv/66732/external_ids":
			_, _ = w.Write([]byte(`{"imdb_id":"tt4574334","tvdb_id":305288}`))
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer srv.Close()

	c := NewClient(func() string { return "k" }).WithBase(srv.URL)
	got, err := c.Lookup(context.Background(), "Stranger Things", 2016, true, "pt-BR")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.TMDBID != 66732 || got.IMDBID != "tt4574334" || got.TVDBID != 305288 || !got.IsTV {
		t.Errorf("result = %+v", got)
	}
}

func TestDetailsMovie(t *testing.T) {
	// Detail in pt-BR already comes with translated title and synopsis — no need to fall back to en-US.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/movie/27205" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("language") != "pt-BR" {
			t.Errorf("language = %q, want pt-BR", r.URL.Query().Get("language"))
		}
		_, _ = w.Write([]byte(`{
			"id": 27205,
			"title": "A Origem",
			"original_title": "Inception",
			"overview": "Dom Cobb é um ladrão...",
			"poster_path": "/x.jpg",
			"release_date": "2010-07-15",
			"original_language": "en",
			"genres": [{"id":28,"name":"Ação"},{"id":878,"name":"Ficção Científica"}]
		}`))
	}))
	defer srv.Close()

	c := NewClient(func() string { return "k" }).WithBase(srv.URL)
	d, err := c.Details(context.Background(), 27205, false, "pt-BR")
	if err != nil {
		t.Fatalf("Details: %v", err)
	}
	if d.Title != "A Origem" || d.OriginalName != "Inception" {
		t.Errorf("titles = %q / %q", d.Title, d.OriginalName)
	}
	if d.Year != 2010 || d.OrigLanguage != "en" {
		t.Errorf("year/lang = %d / %q", d.Year, d.OrigLanguage)
	}
	if len(d.Genres) != 2 || d.Genres[0] != "Ação" {
		t.Errorf("genres = %v", d.Genres)
	}
	if !strings.HasSuffix(d.PosterURL, "/x.jpg") {
		t.Errorf("poster = %q", d.PosterURL)
	}
	if d.IsTV {
		t.Error("IsTV = true, want movie")
	}
}

func TestDetailsTVFallbackOverviewEN(t *testing.T) {
	// TV without pt-BR translation: overview comes empty in pt-BR, needs to fall back to en-US.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/3/tv/66732" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		calls++
		if r.URL.Query().Get("language") == "pt-BR" {
			// translated title exists, but overview came empty
			_, _ = w.Write([]byte(`{"id":66732,"name":"Stranger Things","original_name":"Stranger Things","overview":"","first_air_date":"2016-07-15","original_language":"en"}`))
			return
		}
		if r.URL.Query().Get("language") == "en-US" {
			_, _ = w.Write([]byte(`{"overview":"When a young boy vanishes..."}`))
			return
		}
	}))
	defer srv.Close()

	c := NewClient(func() string { return "k" }).WithBase(srv.URL)
	d, err := c.Details(context.Background(), 66732, true, "pt-BR")
	if err != nil {
		t.Fatalf("Details: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (pt-BR + en-US fallback)", calls)
	}
	if !strings.HasPrefix(d.Overview, "When a young boy") {
		t.Errorf("en-US fallback overview failed: %q", d.Overview)
	}
	if !d.IsTV {
		t.Error("IsTV = false, want TV")
	}
}

func TestDetailsDisabledOrInvalid(t *testing.T) {
	c := NewClient(func() string { return "" })
	if _, err := c.Details(context.Background(), 1, false, "pt-BR"); !errors.Is(err, ErrDisabled) {
		t.Errorf("Details without key = %v, want ErrDisabled", err)
	}
	c2 := NewClient(func() string { return "k" })
	if _, err := c2.Details(context.Background(), 0, false, "pt-BR"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Details id=0 = %v, want ErrNotFound", err)
	}
}

func TestLookupNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	c := NewClient(func() string { return "k" }).WithBase(srv.URL)
	if _, err := c.Lookup(context.Background(), "Nada", 0, false, "pt-BR"); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty Lookup = %v, want ErrNotFound", err)
	}
}

// English content (#74): the site asks for en-US, and an empty overview is just empty —
// there is no second call, since English is already the fallback language.
func TestDetailsEnglishAsksEnUSOnce(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.URL.Query().Get("language"); got != "en-US" {
			t.Errorf("language = %q, want en-US", got)
		}
		_, _ = w.Write([]byte(`{"id":1,"title":"Inception","original_title":"Inception","overview":""}`))
	}))
	defer srv.Close()
	c := NewClient(func() string { return "k" }).WithBase(srv.URL)
	if _, err := c.Details(context.Background(), 1, false, "en"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("%d calls, want 1 (no fallback call for English)", calls)
	}
}
