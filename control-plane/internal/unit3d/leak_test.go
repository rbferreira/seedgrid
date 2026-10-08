package unit3d

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Known FAKE credentials: the test looks for them in the error message.
const (
	knownToken   = "SECRET-api-token-123"   //nolint:gosec // G101: fake test token
	passkeyKnown = "PASSKEY-DO-LINK-456789" //nolint:gosec // G101: fake test passkey
)

// trackerCalls are the four ways to talk to the tracker that include a credential in the URL:
// the api_token in the query (Upload, Browse, FetchTorrent) and the passkey in the download_link
// (fetchTorrentBytes, via DownloadFromLink, which still retries with the api_token).
func callsToTracker(base string) map[string]func() error {
	ctx := context.Background()
	return map[string]func() error{
		"Upload": func() error {
			_, err := Upload(ctx, base, knownToken, Params{Name: "x", CategoryID: 1, TypeID: 1}, "x.torrent", []byte("d4:infod4:name1:xee"))
			return err
		},
		"Browse": func() error {
			_, err := Browse(ctx, base, knownToken, "", "")
			return err
		},
		"FetchTorrent": func() error {
			_, err := FetchTorrent(ctx, base, knownToken, "42")
			return err
		},
		"DownloadFromLink/fetchTorrentBytes": func() error {
			_, err := DownloadFromLink(ctx, base+"/download/42?passkey="+passkeyKnown, knownToken)
			return err
		},
	}
}

func noLeak(t *testing.T, name string, err error) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: expected error", name)
		return
	}
	msg := err.Error()
	for _, forbidden := range []string{knownToken, passkeyKnown, "api_token=", "passkey="} {
		if strings.Contains(msg, forbidden) {
			t.Errorf("%s: error message contains %q — leaks credential: %v", name, forbidden, msg)
		}
	}
}

// The http.Client error contains the ENTIRE request URL, and the URL includes the api_token in the
// query (or the passkey, in the download_link). A closed server forces a real connection
// error; none of the functions can return the credential in the message — which goes to the
// Portal and to the log (#220).
func TestNetworkError_DoesNotLeakCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close()

	for name, call := range callsToTracker(base) {
		noLeak(t, name, call())
	}
}

// url.Parse also returns a *url.Error with the entire URL — when BUILDING the
// request, before any network. An api_url with an invalid port is enough to trigger it.
func TestRequestBuildError_NoCredentialLeak(t *testing.T) {
	for name, call := range callsToTracker("http://127.0.0.1:invalid-port") {
		noLeak(t, name, call())
	}
}

// Truncating the URL cannot hide the diagnosis: the cause remains in the message.
func TestNetworkError_PreservesCause(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := srv.URL
	srv.Close()

	err := callsToTracker(base)["FetchTorrent"]()
	if err == nil || !strings.Contains(err.Error(), "query torrent on tracker") || !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("message lost context or cause: %v", err)
	}
}
