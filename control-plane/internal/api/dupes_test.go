package api

import (
	"context"
	"errors"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/unit3d"
)

// Search is an aid, not a gate: an offline tracker cannot prevent the operator from
// publishing. This is the behavior #186 requires — failure becomes an empty list + warning,
// never 5xx.
func TestDupeEvidence_TrackerUnavailableDoesNotBlock(t *testing.T) {
	boom := func(context.Context, string, string, string, string) (unit3d.BrowseResult, error) {
		return unit3d.BrowseResult{}, errors.New("connection refused")
	}

	resp, err := dupeEvidence(context.Background(), boom, "https://tracker.invalid", "tok", "Idiocracy.2006.1080p.x264.BluRay-NOGROUP")
	if err == nil {
		t.Fatal("the search error must reach the caller to be logged; got nil")
	}
	if got := resp["candidates"].([]dupeCandidate); len(got) != 0 {
		t.Fatalf("candidates with failed search: %d, want 0", len(got))
	}
	if resp["note"] == nil || resp["note"] == "" {
		t.Error("no note explaining to the operator why the list came back empty")
	}
	if resp["query"] != "Idiocracy" {
		t.Errorf("query = %q, want %q", resp["query"], "Idiocracy")
	}
}

// Search goes by TITLE. Sending the entire default name in the UNIT3D filter matches
// nothing — and a search that never finds is worse than none, because it teaches the operator
// to ignore the stage.
func TestDupeQuery(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"full standard name", "Idiocracy.2006.1080p.x264.BluRay.2.0-NOGROUP", "Idiocracy"},
		{"title with multiple words", "The.Matrix.Reloaded.2003.2160p.REMUX-GROUP", "The Matrix Reloaded"},
		{"series", "Severance.S02E01.1080p.AMZN.WEB-DL-GROUP", "Severance"},
		{"with nothing parseable falls back to original", "aaaa", "aaaa"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := dupeQuery(tc.in); got != tc.want {
				t.Errorf("dupeQuery(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The year flags, it doesn't filter: a false positive just costs the operator a look; a
// false negative means publishing a duplicate.
func TestDupeCandidates_YearMarkDoesNotFilter(t *testing.T) {
	items := []unit3d.BrowseItem{
		{Name: "Idiocracy 2006 1080p", Year: 2006},
		{Name: "Idiocracy 2006 720p", Year: 2005}, // divergent year in the catalog
		{Name: "Idiocracy without year"},          // Year zero
	}
	got := dupeCandidates(items, 2006)
	if len(got) != 3 {
		t.Fatalf("candidates = %d, want 3 (the year cannot hide anyone)", len(got))
	}
	if !got[0].SameYear {
		t.Error("item with the same year should be marked")
	}
	if got[1].SameYear || got[2].SameYear {
		t.Error("item with a divergent or missing year cannot be marked")
	}
}

// Title search brings the entire franchise; past a certain point the list stops
// informing and starts getting in the way.
func TestDupeCandidates_RespectsCeiling(t *testing.T) {
	items := make([]unit3d.BrowseItem, maxDupeCandidates+10)
	if got := len(dupeCandidates(items, 0)); got != maxDupeCandidates {
		t.Errorf("candidates = %d, want %d", got, maxDupeCandidates)
	}
}

// Without an inferred year, nothing is flagged — there's no "same year" against no year.
func TestDupeCandidates_WithoutYearDoesNotMark(t *testing.T) {
	got := dupeCandidates([]unit3d.BrowseItem{{Year: 2006}}, 0)
	if got[0].SameYear {
		t.Error("marked same_year without a reference year")
	}
}
