package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/imagehost"
	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
)

// The 200 response from the screenshots routes (#193) always returns `failures` as a list — empty
// when the first host wins —, so the client doesn't have to distinguish null from [].
func TestScreenshotResponse_FailuresAlwaysListed(t *testing.T) {
	b, err := json.Marshal(newScreenshotResponse(torrent.ScreenshotResult{BBCode: "[img]x[/img]", Host: "imgbb"}))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != `{"bbcode":"[img]x[/img]","host":"imgbb","failures":[]}` {
		t.Errorf("response = %s", got)
	}

	b, _ = json.Marshal(newScreenshotResponse(torrent.ScreenshotResult{
		BBCode: "x", Host: "imgbb", Failures: []imagehost.Failure{{Host: "img.a.org", Reason: "HTTP 500"}},
	}))
	if !strings.Contains(string(b), `"failures":[{"host":"img.a.org","reason":"HTTP 500"}]`) {
		t.Errorf("response with failure = %s", b)
	}
}
