package googlebooks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func init() { retryWaits = []time.Duration{time.Millisecond, time.Millisecond} }

// TestSearchTransientRetry: 503 on the 1st try cannot turn into "no cover" — which is what
// hid covers the API had, when enrich triggers the page in parallel (#110).
func TestSearchTransientRetry(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"items":[{"volumeInfo":{"title":"O Alfaiate Valentão",
			"authors":["Irmãos Grimm"],"publishedDate":"2021-02-23",
			"imageLinks":{"thumbnail":"http://books.google.com/x.jpg"}}}]}`))
	}))
	defer srv.Close()

	b, err := NewClient(nil).WithBase(srv.URL).Search(context.Background(), "O Alfaiate Valentão", "Irmãos Grimm", 2020)
	if err != nil {
		t.Fatalf("should have repeated and found: %v", err)
	}
	if b.CoverURL != "https://books.google.com/x.jpg" {
		t.Errorf("unexpected cover: %q", b.CoverURL)
	}
	if n < 2 {
		t.Errorf("expected 2 attempts, got %d", n)
	}
}

// TestSearchDoesNotRepeat4xx: 400 is a malformed request/invalid key — retrying wastes quota.
func TestSearchDoesNotRepeat4xx(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	if _, err := NewClient(nil).WithBase(srv.URL).Search(context.Background(), "X", "", 0); err == nil {
		t.Fatal("expected error")
	}
	if n != 1 {
		t.Errorf("should not retry 4xx; attempts: %d", n)
	}
}
