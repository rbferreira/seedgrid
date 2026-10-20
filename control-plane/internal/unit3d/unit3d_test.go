package unit3d

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestUploadSuccess(t *testing.T) {
	var gotToken, gotName, gotCat, gotImdb, gotSeason string
	var gotTorrent []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/torrents/upload" {
			http.Error(w, "path", 404)
			return
		}
		gotToken = r.URL.Query().Get("api_token")
		if err := r.ParseMultipartForm(1 << 20); err != nil { //nolint:gosec // G120: 1MB is already the explicit limit
			http.Error(w, err.Error(), 400)
			return
		}
		gotName = r.FormValue("name")
		gotCat = r.FormValue("category_id")
		gotImdb = r.FormValue("imdb")
		gotSeason = r.FormValue("season_number")
		f, _, err := r.FormFile("torrent")
		if err != nil {
			http.Error(w, "no torrent", 400)
			return
		}
		buf := make([]byte, 32)
		n, _ := f.Read(buf)
		gotTorrent = buf[:n]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":"https://tracker.test/torrent/download/123.abcpasskey","message":"Torrent uploaded successfully."}`))
	}))
	defer srv.Close()

	res, err := Upload(context.Background(), srv.URL, "secret-token", Params{
		Name:         "Inception 2010 1080p",
		Description:  "desc",
		CategoryID:   1,
		TypeID:       3,
		ResolutionID: 3,
		TMDB:         27205,
		IMDB:         "tt1375666",
		Anonymous:    false,
	}, "Inception", []byte("d8:announce..."))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if res.DownloadURL != "https://tracker.test/torrent/download/123.abcpasskey" {
		t.Errorf("download url = %q", res.DownloadURL)
	}
	if gotToken != "secret-token" {
		t.Errorf("api_token = %q, want secret-token", gotToken)
	}
	if gotName != "Inception 2010 1080p" || gotCat != "1" {
		t.Errorf("fields = name:%q cat:%q", gotName, gotCat)
	}
	if gotImdb != "1375666" {
		t.Errorf("imdb = %q, want 1375666 (without tt/zeros)", gotImdb)
	}
	if gotSeason != "" {
		t.Errorf("season_number = %q, want empty (movie)", gotSeason)
	}
	if string(gotTorrent) != "d8:announce..." {
		t.Errorf("torrent bytes = %q", gotTorrent)
	}
}

func TestUploadTVSeasonEpisode(t *testing.T) {
	var gotSeason, gotEpisode, gotTvdb string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20) //nolint:gosec // G120: 1MB is already the explicit limit
		gotSeason = r.FormValue("season_number")
		gotEpisode = r.FormValue("episode_number")
		gotTvdb = r.FormValue("tvdb")
		_, _ = w.Write([]byte(`{"data":"https://x/y","message":"ok"}`))
	}))
	defer srv.Close()

	_, err := Upload(context.Background(), srv.URL, "tok", Params{
		Name: "Show S02E05", Description: "d", CategoryID: 2, TypeID: 4,
		ResolutionID: 3, TMDB: 1, TVDB: 305288, Season: 2, Episode: 5,
	}, "Show", []byte("x"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if gotSeason != "2" || gotEpisode != "5" || gotTvdb != "305288" {
		t.Errorf("S/E/tvdb = %q/%q/%q", gotSeason, gotEpisode, gotTvdb)
	}
}

func TestUploadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		_, _ = w.Write([]byte(`{"message":"The tmdb field is required.","errors":{"tmdb":["required"]}}`))
	}))
	defer srv.Close()
	_, err := Upload(context.Background(), srv.URL, "tok", Params{Name: "x", Description: "d", CategoryID: 1, TypeID: 3}, "x", []byte("x"))
	if err == nil || !strings.Contains(err.Error(), "tmdb field is required") {
		t.Errorf("error = %v, want to contain the tracker message", err)
	}
}

func TestUploadMissingCreds(t *testing.T) {
	if _, err := Upload(context.Background(), "", "tok", Params{}, "x", []byte("x")); err == nil {
		t.Error("Upload without api_url should fail")
	}
	if _, err := Upload(context.Background(), "https://x", "", Params{}, "x", []byte("x")); err == nil {
		t.Error("Upload without api_token should fail")
	}
}

// TestBrowse covers catalog parsing (backlog #12) and PAGE pagination in
// NO search mode (bug #9): even without `name`, browse hits /api/torrents/filter (the cursor of
// /api/torrents is broken in Meilisearch) and returns the next page via links.next.
func TestBrowse(t *testing.T) {
	var gotPath, gotName, gotPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotName, gotPage = r.URL.Path, r.URL.Query().Get("name"), r.URL.Query().Get("page")
		if r.URL.Query().Get("api_token") != "tok" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
		  {"type":"torrent","id":"42","attributes":{"name":"Filme X 2020 1080p","category":"Filmes","type":"Encode","size":123456,"seeders":10,"leechers":2,"times_completed":5,"tmdb_id":27205,"imdb_id":"tt1375666","release_year":2020,"details_link":"https://t/details/42","download_link":"https://t/dl/42.key"}},
		  {"type":"torrent","id":99,"attributes":{"name":"Curso Y","category":"Cursos","type":"Outros","size":"10","seeders":"1","leechers":0,"times_completed":0,"tmdb_id":null,"imdb_id":null,"release_year":"2019","details_link":"","download_link":""}}
		],"meta":{"current_page":1,"per_page":50},"links":{"next":"https://x/api/torrents/filter?page=2"}}`))
	}))
	defer srv.Close()

	res, err := Browse(context.Background(), srv.URL, "tok", "", "")
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if gotPath != "/api/torrents/filter" {
		t.Errorf("path = %q, want /api/torrents/filter (fix bug #9)", gotPath)
	}
	if gotName != "" {
		t.Errorf("param name = %q, want empty in browse mode", gotName)
	}
	if gotPage != "1" {
		t.Errorf("param page = %q, want 1 (default)", gotPage)
	}
	if res.NextCursor != "2" {
		t.Errorf("next_cursor = %q, want 2 (current_page+1)", res.NextCursor)
	}
	if len(res.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(res.Items))
	}
	a := res.Items[0]
	if a.ID != "42" || a.Name != "Filme X 2020 1080p" || a.Size != 123456 || a.Seeders != 10 || a.TMDB != 27205 || a.IMDB != "tt1375666" {
		t.Errorf("unexpected item0: %+v", a)
	}
	// download_link stays in the Links map (server-side), not in BrowseItem.
	if res.Links["42"] != "https://t/dl/42.key" {
		t.Errorf("Links[42] = %q, want https://t/dl/42.key", res.Links["42"])
	}
	// numeric id → string; imdb null → ""; numerics as STRING converted (the case
	// that broke parsing: release_year/size/seeders coming as text in UNIT3D).
	if b := res.Items[1]; b.ID != "99" || b.IMDB != "" || b.Size != 10 || b.Seeders != 1 || b.Year != 2019 {
		t.Errorf("item1 = %+v (want id=99 imdb=\"\" size=10 seeders=1 year=2019)", b)
	}
}

// TestBrowseSearch covers server-side search (#13): with term, it hits the endpoint
// /api/torrents/filter (/api/torrents ignores `name`), sends `name` + `page` and
// returns the next page when the tracker points to links.next.
func TestBrowseSearch(t *testing.T) {
	var gotPath, gotName, gotPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotName = r.URL.Query().Get("name")
		gotPage = r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")
		// Page pagination: current_page=2 + links.next present → next is 3.
		_, _ = w.Write([]byte(`{"data":[{"id":"7","attributes":{"name":"Super Mario"}}],
		  "meta":{"current_page":2},"links":{"next":"https://x/api/torrents/filter?page=3"}}`))
	}))
	defer srv.Close()
	res, err := Browse(context.Background(), srv.URL, "tok", "2", "Mario")
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if gotPath != "/api/torrents/filter" {
		t.Errorf("path = %q, want /api/torrents/filter", gotPath)
	}
	if gotName != "Mario" {
		t.Errorf("param name = %q, want Mario", gotName)
	}
	if gotPage != "2" {
		t.Errorf("param page = %q, want 2", gotPage)
	}
	if res.NextCursor != "3" {
		t.Errorf("next_cursor = %q, want 3 (current_page+1)", res.NextCursor)
	}
	if len(res.Items) != 1 || res.Items[0].Name != "Super Mario" {
		t.Errorf("items = %+v", res.Items)
	}
}

// TestBrowseSearchLastPage: without links.next (last page), it doesn't return next.
func TestBrowseSearchLastPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"9","attributes":{"name":"Mario Kart"}}],
		  "meta":{"current_page":5},"links":{"next":null}}`))
	}))
	defer srv.Close()
	res, err := Browse(context.Background(), srv.URL, "tok", "5", "Mario")
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if res.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty (last page)", res.NextCursor)
	}
}

// TestFetchTorrent covers the download (backlog #12): resolves download_link via
// /api/torrents/{id} and downloads the bencode bytes.
func TestFetchTorrent(t *testing.T) {
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/torrents/42":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"attributes":{"download_link":"` + base + `/download/42.key"}}}`))
		case "/download/42.key":
			_, _ = w.Write([]byte("d8:announce10:http://x/ae")) // bencode
		default:
			http.Error(w, "404", 404)
		}
	}))
	defer srv.Close()
	base = srv.URL

	data, err := FetchTorrent(context.Background(), srv.URL, "tok", "42")
	if err != nil {
		t.Fatalf("FetchTorrent: %v", err)
	}
	if !looksBencode(data) || string(data) != "d8:announce10:http://x/ae" {
		t.Errorf("downloaded bytes = %q", data)
	}
}

// REGRESSION #207 — tracker refusal cannot turn into "invalid .torrent".
//
// A UNIT3D site started refusing specific torrents with a 302 to the details page and then to
// /login. Since http.Client follows redirects, what reached the parser was the HTML of the
// login page with status 200, and the error came out as if the FILE was invalid —
// leading to investigating the wrong place.
func TestDownloadFromLink_RedirectRefusalNoInvalidFile(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download/42.key":
			requests++
			http.Redirect(w, r, "/login", http.StatusFound)
		case "/login":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!DOCTYPE html><html><body>log in to your account</body></html>"))
		default:
			http.Error(w, "404", 404)
		}
	}))
	defer srv.Close()

	_, err := DownloadFromLink(context.Background(), srv.URL+"/download/42.key", "tok")
	if err == nil {
		t.Fatal("expected error")
	}
	var denied *DeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("expected DeniedError, got %T: %v", err, err)
	}
	if denied.FinalPath != "/login" {
		t.Errorf("FinalPath = %q, want /login", denied.FinalPath)
	}
	if strings.Contains(err.Error(), "not a valid .torrent") {
		t.Errorf("message still blames the file: %v", err)
	}
	// Refusal cannot turn into a second request: repeating doesn't change the verdict and doubles the
	// load against a tracker that is already refusing.
	if requests != 1 {
		t.Errorf("expected 1 download request, got %d", requests)
	}
}

// 429 has its own message and carries the Retry-After: a raw "status 429" doesn't tell the operator
// that they just need to wait.
func TestDownloadFromLink_RateLimitHasOwnMessage(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := DownloadFromLink(context.Background(), srv.URL+"/download/42.key", "tok")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got: %v", err)
	}
	if !strings.Contains(err.Error(), "60") {
		t.Errorf("message lost Retry-After: %v", err)
	}
	// Insisting right after a 429 spends the quota we are waiting to refill.
	if requests != 1 {
		t.Errorf("expected 1 request after 429, got %d", requests)
	}
}

// Without redirect, a non-bencode body remains an "invalid file" — but saying
// what arrived, not just what it isn't.
func TestDownloadFromLink_InvalidBodyDescribesWhatArrived(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not   bencode"))
	}))
	defer srv.Close()

	_, err := DownloadFromLink(context.Background(), srv.URL+"/download/42.key", "tok")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not a valid .torrent") {
		t.Errorf("expected invalid file error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "bytes") || !strings.Contains(err.Error(), "not") {
		t.Errorf("error does not describe the received body: %v", err)
	}
}

// hangingTracker never answers: it holds each request until the client gives up. It counts the
// requests received, so the test can prove how many times the client insisted.
func hangingTracker(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

// REGRESSION #35 — a tracker that doesn't answer becomes ErrTimeout, not "context canceled".
//
// A tracker went a minute without answering the download. The first attempt used up the
// http.Client's 60s, the second started with the request already cancelled, and the report came
// out as "context canceled (without api_token: … Client.Timeout …)" — in a generic 502. A timeout
// must not turn into the second attempt: the token doesn't speed the tracker up.
func TestDownloadFromLink_TimeoutDoesNotRetry(t *testing.T) {
	srv, requests := hangingTracker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := DownloadFromLink(ctx, srv.URL+"/download/42.key", "tok")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got: %v", err)
	}
	if strings.Contains(err.Error(), "context") {
		t.Errorf("message still talks about the context, not the tracker: %v", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s; the caller's deadline was not respected", d)
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("expected 1 request to the tracker, got %d", n)
	}
}

// The same holds when what hangs is the /api/torrents/{id} query, before the download.
func TestFetchTorrent_TimeoutOnQuery(t *testing.T) {
	srv, requests := hangingTracker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := FetchTorrent(ctx, srv.URL, "tok", "42")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got: %v", err)
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("expected 1 request to the tracker, got %d", n)
	}
}

// REGRESSION #41 — the catalog browse also says the tracker didn't answer, so the route answers
// 504 with the reason instead of the proxy's generic 502.
func TestBrowse_TimeoutBecomesErrTimeout(t *testing.T) {
	srv, requests := hangingTracker(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := Browse(ctx, srv.URL, "tok", "", "")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("expected ErrTimeout, got: %v", err)
	}
	if strings.Contains(err.Error(), "tok") {
		t.Errorf("the error leaks the token: %v", err)
	}
	if n := requests.Load(); n != 1 {
		t.Errorf("expected 1 request to the tracker, got %d", n)
	}
}

// A network error that isn't a timeout (connection refused) follows the old path: it doesn't
// become ErrTimeout, which would tell the operator to wait for something that won't come back.
func TestDownloadFromLink_ConnectionRefusedIsNotTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	link := srv.URL + "/download/42.key"
	srv.Close()

	_, err := DownloadFromLink(context.Background(), link, "tok")
	if err == nil || errors.Is(err, ErrTimeout) {
		t.Fatalf("expected a connection error (not ErrTimeout), got: %v", err)
	}
}

// Happy path still works with ONE request.
func TestDownloadFromLink_Success(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte("d8:announce10:http://x/ae"))
	}))
	defer srv.Close()

	data, err := DownloadFromLink(context.Background(), srv.URL+"/download/42.key", "tok")
	if err != nil {
		t.Fatalf("DownloadFromLink: %v", err)
	}
	if string(data) != "d8:announce10:http://x/ae" {
		t.Errorf("bytes = %q", data)
	}
	if requests != 1 {
		t.Errorf("expected 1 request, got %d", requests)
	}
}
