package imgbb

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestNetworkErrorDoesNotLeakKey ensures the imgbb connection error DOES NOT leak the key
// (ADR-040, defect (i)). The *url.Error from http.Client includes the full URL, and here it
// carries the key in the query (`?key=<key>`). stripURL strips the URL before returning it.
func TestNetworkErrorDoesNotLeakKey(t *testing.T) {
	// Points to a closed server: the connection is refused and the http.Client
	// returns a *url.Error with the full URL (including the key in the query).
	srv := httptest.NewServer(nil)
	srvURL := srv.URL
	srv.Close()

	old := uploadURL
	uploadURL = srvURL
	defer func() { uploadURL = old }()

	key := "SECRET-KEY-12345"
	_, err := Upload(context.Background(), key, [][]byte{[]byte("frame")})
	if err == nil {
		t.Fatal("expected network error, got nil")
	}

	errStr := err.Error()
	// The error MUST NOT contain the key or the URL.
	if strings.Contains(errStr, key) {
		t.Errorf("network error leaked the key: %q contains %q", errStr, key)
	}
	if strings.Contains(errStr, srvURL) {
		t.Errorf("network error leaked the URL: %q contains %q", errStr, srvURL)
	}
	if strings.Contains(errStr, "key=") {
		t.Errorf("network error leaked the query: %q contains 'key='", errStr)
	}
	// It must contain "connect to imgbb" to be readable.
	if !strings.Contains(errStr, "connect to imgbb") {
		t.Errorf("error should say 'connect to imgbb': %q", errStr)
	}
}
