package api

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/tracker"
	"github.com/rbferreira/seedgrid/control-plane/internal/trackerprofile"
	"github.com/rbferreira/seedgrid/control-plane/internal/unit3d"
)

// The axes that discovery harvests and those the profile stores are two sets written in
// two packages (unit3d doesn't know trackerprofile). If they diverge, a discovered axis would be
// discarded without warning — this test is what ties the two together.
func TestDiscoveryAxesAreProfileAxes(t *testing.T) {
	for _, axis := range unit3d.TaxonomyAxes() {
		if !trackerprofile.KnownAxis(axis) {
			t.Errorf("discovery yields axis %q, which the profile does not know", axis)
		}
	}
	if len(unit3d.TaxonomyAxes()) != len(trackerprofile.Axes) {
		t.Errorf("discovery yields %d axes and the profile has %d — a profile axis would never be discovered",
			len(unit3d.TaxonomyAxes()), len(trackerprofile.Axes))
	}
}

func TestClampPages(t *testing.T) {
	for in, want := range map[int]int{
		0:                           unit3d.DefaultDiscoverPages,
		-5:                          unit3d.DefaultDiscoverPages,
		1:                           1,
		12:                          12,
		unit3d.MaxDiscoverPages:     unit3d.MaxDiscoverPages,
		unit3d.MaxDiscoverPages + 1: unit3d.MaxDiscoverPages,
		1 << 30:                     unit3d.MaxDiscoverPages,
	} {
		if got := clampPages(in); got != want {
			t.Errorf("clampPages(%d) = %d, want %d", in, got, want)
		}
	}
}

// The discovery response joins the scan to the current profile. What discovery harvested
// in a call appears as a proposal; what the profile already has appears as "same" or "renamed".
func TestBuildDiscoverResponse(t *testing.T) {
	current := trackerprofile.Taxonomy{
		trackerprofile.AxisCategory: {{ID: 1, Name: "Filmes", Kind: trackerprofile.KindMovie, Origin: trackerprofile.OriginSeed}},
	}
	disc := unit3d.Discovery{
		Axes: map[string][]unit3d.ObservedTerm{
			unit3d.AxisCategory: {{ID: 1, Name: "Filmes", Seen: 4}, {ID: 2, Name: "Series", Seen: 6}},
			unit3d.AxisType:     {{ID: 50, Name: "PC", Seen: 1}},
		},
		PagesScanned: 3,
		ItemsScanned: 150,
		Stopped:      unit3d.StopMaxPages,
	}

	got := buildDiscoverResponse(current, disc, 3)

	if got.PagesRequested != 3 || got.PagesScanned != 3 || got.ItemsScanned != 150 || got.StoppedReason != "max_pages" {
		t.Errorf("scan metadata = %+v", got)
	}
	if got.Family != trackerprofile.FamilyUNIT3D {
		t.Errorf("family = %q", got.Family)
	}
	if len(got.Proposal) != len(trackerprofile.Axes) {
		t.Errorf("proposal with %d axes, want %d (all present, even empty)", len(got.Proposal), len(trackerprofile.Axes))
	}
	if cat := got.Proposal[trackerprofile.AxisCategory]; len(cat) != 2 || cat[0].Status != trackerprofile.StatusSame || cat[1].Status != trackerprofile.StatusNew {
		t.Errorf("categories = %+v, want [same new]", cat)
	}
	if got.Warnings == nil {
		t.Error("Warnings nil: JSON would be null and Portal would have to check")
	}
	if !strings.Contains(got.Notice, "incomplete by definition") {
		t.Errorf("missing incompleteness warning in Notice: %q", got.Notice)
	}
}

// The tracker in the API is the public view of layer 3 plus the layer 2 profile — and still lacks
// announce or token. The tracker.Tracker embed flattens the fields into the same object.
func TestTrackerViewNoCredential(t *testing.T) {
	v := viewOf(tracker.Tracker{ID: "abc", Name: "Example", Host: "tracker.example", Source: "EXAMPLE", HasAPIToken: true}, nil)
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id", "name", "host", "has_api_token", "profile", "description_default"} {
		if _, ok := m[want]; !ok {
			t.Errorf("missing field %q: %s", want, raw)
		}
	}
	for _, banned := range []string{"announce", "api_token", "token"} {
		if _, ok := m[banned]; ok {
			t.Errorf("field %q cannot be present in response: %s", banned, raw)
		}
	}
	prof, _ := m["profile"].(map[string]any)
	if prof["family"] != "unit3d" || prof["schema_version"] != float64(1) {
		t.Errorf("tracker without profile line should have empty profile: %v", prof)
	}
}
