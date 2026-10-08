package agentconfig

import (
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

func itemFor(t *testing.T, items []Item, key string) Item {
	t.Helper()
	for _, it := range items {
		if it.Spec.Key == key {
			return it
		}
	}
	t.Fatalf("key %s missing from result", key)
	return Item{}
}

// The precedence of ADR-039 (the two tiers that live in the Control Plane): Node override >
// cluster value, and the cluster value is the operator's when they set it, otherwise the default
// from the catalog.
func TestEffectivePrecedence(t *testing.T) {
	store := settings.NewForTest(map[string]string{
		"engine.max_active_downloads": "6", // the operator changed the cluster
	})
	overrides := map[string]string{
		"engine.max_active_downloads": "3", // and node-04 has its own value
		"engine.max_active_checking":  "2", // override where the cluster follows the default
	}
	items := effective(store, overrides)

	dl := itemFor(t, items, "engine.max_active_downloads")
	if dl.Value != "3" || dl.Source != SourceNode || dl.Override == nil || *dl.Override != "3" {
		t.Errorf("downloads = %+v, want override 3 coming from Node", dl)
	}
	if dl.ClusterValue != "6" || !dl.ClusterExplicit {
		t.Errorf("downloads: cluster = %q/explicit=%v, want 6/true (what the Node would have without override)", dl.ClusterValue, dl.ClusterExplicit)
	}

	ck := itemFor(t, items, "engine.max_active_checking")
	if ck.Value != "2" || ck.Source != SourceNode || ck.ClusterValue != "1" || ck.ClusterExplicit {
		t.Errorf("checking = %+v, want override 2 over Cluster default 1", ck)
	}

	hb := itemFor(t, items, "agent.heartbeat_interval")
	if hb.Value != "10s" || hb.Source != SourceCluster || hb.Override != nil {
		t.Errorf("heartbeat = %+v, want Cluster default 10s without override", hb)
	}
}

// Without any override, every Node gets the cluster value: it's the "New Node is born equal to
// the others" that issue #144 asks for.
func TestEffectiveWithoutOverrideFollowsCluster(t *testing.T) {
	items := effective(settings.NewForTest(nil), nil)
	if len(items) != len(settings.NodeSpecs()) {
		t.Fatalf("items = %d, want %d (all PerNode keys)", len(items), len(settings.NodeSpecs()))
	}
	for _, it := range items {
		if it.Source != SourceCluster || it.Value != it.Spec.Default || it.Override != nil {
			t.Errorf("%s = %+v, want catalog default from Cluster", it.Spec.Key, it)
		}
	}
}

// An override that the catalog stopped accepting (tighter limit in a new version) or that was
// manually edited in the database MUST NOT reach the Agent: it falls back to the cluster value.
func TestEffectiveIgnoresInvalidOverride(t *testing.T) {
	items := effective(settings.NewForTest(nil), map[string]string{
		"engine.max_active_downloads": "-1", // unlimited in qBittorrent: it would crash an Atom
		"agent.heartbeat_interval":    "0s", // ticker.Reset(0) panics
		"key.that.does.not.exist":     "x",
	})
	if it := itemFor(t, items, "engine.max_active_downloads"); it.Value != "10" || it.Source != SourceCluster {
		t.Errorf("downloads = %+v, want Cluster (invalid override ignored)", it)
	}
	if it := itemFor(t, items, "agent.heartbeat_interval"); it.Value != "10s" || it.Source != SourceCluster {
		t.Errorf("heartbeat = %+v, want Cluster (invalid override ignored)", it)
	}
}

func TestVersionDeterministicAndValueSensitive(t *testing.T) {
	a := map[string]string{"x": "1", "y": "2"}
	b := map[string]string{"y": "2", "x": "1"} // same thing, different insertion order
	if Version(a) != Version(b) {
		t.Error("version must not depend on map order")
	}
	if len(Version(a)) != 12 {
		t.Errorf("version = %q, want 12 hex", Version(a))
	}
	if Version(a) == Version(map[string]string{"x": "1", "y": "3"}) {
		t.Error("different value must yield different version")
	}
	if Version(a) == Version(map[string]string{"x": "1"}) {
		t.Error("missing key must yield different version")
	}
	// The key/value boundary matters: "a=bc" and "ab=c" cannot collide.
	if Version(map[string]string{"a": "bc"}) == Version(map[string]string{"ab": "c"}) {
		t.Error("versions collide at key/value boundary")
	}
}

func TestStatus(t *testing.T) {
	v := "abc123def456"
	other := "000000000000"
	empty := ""
	cases := []struct {
		name    string
		applied *string
		want    string
	}{
		{"never reported (Agent prior to ADR-039)", nil, StatusUnsupported},
		{"reported empty", &empty, StatusUnsupported},
		{"reported today's version", &v, StatusInSync},
		{"reported an old version", &other, StatusPending},
	}
	for _, c := range cases {
		st := NodeState{EffectiveVersion: v, AppliedVersion: c.applied}
		if got := st.Status(); got != c.want {
			t.Errorf("%s: status = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLabelPrefersAlias(t *testing.T) {
	nick := "Room rack"
	if got := (NodeState{Hostname: "seedgrid-node-04", DisplayName: &nick}).Label(); got != nick {
		t.Errorf("Label = %q, want the nickname", got)
	}
	empty := ""
	if got := (NodeState{Hostname: "seedgrid-node-04", DisplayName: &empty}).Label(); got != "seedgrid-node-04" {
		t.Errorf("Label with empty display name = %q, want hostname", got)
	}
}
