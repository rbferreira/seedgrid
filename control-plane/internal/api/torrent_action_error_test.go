package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/torrent"
)

// Action on a migrating torrent (#87) returns a 409 with a stable code — the Portal and those
// who automate the API distinguish "try again later" from a real error (500).
func TestTorrentActionError_MigrationInFlight(t *testing.T) {
	// Wrapped: the service can return the error with context.
	err := fmt.Errorf("pause: %w", torrent.ErrMigrationInFlight)
	if !errors.Is(err, torrent.ErrMigrationInFlight) {
		t.Fatal("precondition: wrapped error must match errors.Is")
	}

	rec := httptest.NewRecorder()
	torrentActionError(rec, Deps{}, err)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if e := json.Unmarshal(rec.Body.Bytes(), &body); e != nil {
		t.Fatalf("body is not JSON: %v (%s)", e, rec.Body.String())
	}
	if body.Error.Code != "migration_in_flight" {
		t.Errorf("code = %q, want migration_in_flight", body.Error.Code)
	}
	if body.Error.Message == "" {
		t.Error("the PT-BR message is what the operator reads in the Portal toast")
	}
}
