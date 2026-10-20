package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
)

// The other-version 409 is a contract with the Portal (#37): the dialog reads error.code, the
// versions from error.versions with the five fields the operator needs to decide, and error.adding.
func TestWriteOtherVersions_Contract(t *testing.T) {
	rec := httptest.NewRecorder()
	writeOtherVersions(rec, &torrent.OtherVersionsError{
		Versions: []torrent.Version{
			{Name: "Interstellar.2014.1080p.BluRay.x264-GRP", Resolution: "1080p", Codec: "x264", SizeBytes: 10_000, Node: "node-01"},
		},
		Adding: torrent.Version{Name: "Interstellar 2014 720p WEB-DL x265", Resolution: "720p", Codec: "x265", SizeBytes: 3_000},
	})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var body struct {
		Error struct {
			Code     string           `json:"code"`
			Message  string           `json:"message"`
			Versions []map[string]any `json:"versions"`
			Adding   map[string]any   `json:"adding"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "other_versions" || body.Error.Message == "" || len(body.Error.Versions) != 1 {
		t.Fatalf("body = %s", rec.Body.String())
	}
	for _, k := range []string{"name", "resolution", "codec", "size_bytes", "node"} {
		if _, ok := body.Error.Versions[0][k]; !ok {
			t.Errorf("version without the %q field: %s", k, rec.Body.String())
		}
	}
	if body.Error.Adding["resolution"] != "720p" || body.Error.Adding["codec"] != "x265" {
		t.Errorf("adding = %v, want the version being added", body.Error.Adding)
	}
}
