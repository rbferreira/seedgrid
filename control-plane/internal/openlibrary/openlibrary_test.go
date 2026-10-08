package openlibrary

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchTitleAuthorYearMatching(t *testing.T) {
	// docs[0] has exact cover + author + year → must be chosen, even if docs[1]
	// has a shorter title (ranking prioritized by anchors).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("title") != "The Pragmatic Programmer" {
			t.Errorf("title = %q", r.URL.Query().Get("title"))
		}
		_, _ = w.Write([]byte(`{"docs":[
			{"title":"The Pragmatic Programmer","author_name":["Andrew Hunt","David Thomas"],"first_publish_year":1999,"cover_i":8567890,"subject":["programming","software","engineering","general"],"language":["eng"]},
			{"title":"The Pragmatic Programmer's Journal","author_name":["Random"],"first_publish_year":2010,"cover_i":0}
		]}`))
	}))
	defer srv.Close()

	c := NewClient().WithBase(srv.URL)
	b, err := c.Search(context.Background(), "The Pragmatic Programmer", "Andrew Hunt", 1999)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if b.Title != "The Pragmatic Programmer" {
		t.Errorf("title = %q", b.Title)
	}
	if b.Author != "Andrew Hunt et al." {
		t.Errorf("author = %q", b.Author)
	}
	if b.Year != 1999 {
		t.Errorf("year = %d", b.Year)
	}
	if !strings.Contains(b.CoverURL, "8567890") || !strings.HasSuffix(b.CoverURL, "-M.jpg") {
		t.Errorf("cover = %q", b.CoverURL)
	}
	if len(b.Subjects) != 4 {
		t.Errorf("subjects = %v (want 4)", b.Subjects)
	}
	if b.Language != "en" {
		t.Errorf("language = %q, want en", b.Language)
	}
}

func TestSearchWeakMatchWithoutAnchor(t *testing.T) {
	// Only partial title, no cover / no year / no author → low score → ErrNotFound.
	// The UI falls back to neutral mode instead of showing any book that "matches" the text.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"docs":[
			{"title":"Something About Programming","first_publish_year":0,"cover_i":0}
		]}`))
	}))
	defer srv.Close()
	c := NewClient().WithBase(srv.URL)
	if _, err := c.Search(context.Background(), "Programming", "", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("Search weak = %v, want ErrNotFound", err)
	}
}

func TestSearchMARCTranslation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"docs":[
			{"title":"Dom Casmurro","author_name":["Machado de Assis"],"first_publish_year":1899,"cover_i":1,"language":["por"]}
		]}`))
	}))
	defer srv.Close()
	c := NewClient().WithBase(srv.URL)
	b, err := c.Search(context.Background(), "Dom Casmurro", "Machado", 1899)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if b.Language != "pt" {
		t.Errorf("MARC for did not become pt: %q", b.Language)
	}
}

func TestSearchEmpty(t *testing.T) {
	c := NewClient()
	if _, err := c.Search(context.Background(), "", "", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("Search without title = %v, want ErrNotFound", err)
	}
}
