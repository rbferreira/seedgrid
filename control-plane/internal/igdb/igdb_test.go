package igdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearch(t *testing.T) {
	var tokenCalls, searchCalls int
	var gotBody, gotClientID, gotAuth string

	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		if r.URL.Query().Get("grant_type") != "client_credentials" {
			http.Error(w, "grant", 400)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"APPTOK","expires_in":5000000,"token_type":"bearer"}`))
	}))
	defer token.Close()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchCalls++
		gotClientID = r.Header.Get("Client-ID")
		gotAuth = r.Header.Get("Authorization")
		buf := make([]byte, 512)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
		  {"id":1074,"name":"Super Mario Odyssey","first_release_date":1509408000,"summary":"Explore incredible places.","genres":[{"name":"Platform"}],"platforms":[{"name":"Nintendo Switch"}],"cover":{"image_id":"co1mxf"}},
		  {"id":9999,"name":"Odyssey Mod","summary":"","genres":[],"platforms":[]}
		]`))
	}))
	defer api.Close()

	c := NewClient(func() string { return "cid" }, func() string { return "csecret" }).WithBases(token.URL, api.URL)
	if !c.Enabled() {
		t.Fatal("client with credentials should be Enabled")
	}

	games, err := c.Search(context.Background(), "Super Mario Odyssey", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("games = %d, want 2", len(games))
	}
	g := games[0]
	if g.IGDBID != 1074 || g.Name != "Super Mario Odyssey" || g.Year != 2017 {
		t.Errorf("game0 = %+v (want id 1074, 2017)", g)
	}
	if g.CoverURL != coverBase+"co1mxf.jpg" {
		t.Errorf("cover = %q", g.CoverURL)
	}
	if len(g.Genres) != 1 || g.Genres[0] != "Platform" || len(g.Platforms) != 1 {
		t.Errorf("genres/platforms = %+v / %+v", g.Genres, g.Platforms)
	}
	// Auth / apicalypse
	if gotClientID != "cid" || gotAuth != "Bearer APPTOK" {
		t.Errorf("headers: Client-ID=%q Authorization=%q", gotClientID, gotAuth)
	}
	if !strings.Contains(gotBody, `search "Super Mario Odyssey";`) {
		t.Errorf("body apicalypse = %q", gotBody)
	}

	// 2nd search reuses the cached token (doesn't hit Twitch again).
	if _, err := c.Search(context.Background(), "Zelda", 10); err != nil {
		t.Fatalf("Search 2: %v", err)
	}
	if tokenCalls != 1 {
		t.Errorf("tokenCalls = %d, want 1 (token should be cached)", tokenCalls)
	}
	if searchCalls != 2 {
		t.Errorf("searchCalls = %d, want 2", searchCalls)
	}
}

func TestSearchDisabled(t *testing.T) {
	c := NewClient(func() string { return "" }, func() string { return "" })
	if c.Enabled() {
		t.Fatal("client without credentials should not be Enabled")
	}
	if _, err := c.Search(context.Background(), "x", 5); err != ErrDisabled {
		t.Errorf("err = %v, want ErrDisabled", err)
	}
}
