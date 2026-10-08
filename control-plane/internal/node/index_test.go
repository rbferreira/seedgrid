package node_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/node"
)

// TestIndexForPort locks the rule that gives the deterministic IPv6 suffix (ADR-038): the Node
// index is its position in the BitTorrent port sequence. The first four cases are the
// Nodes that already used ::5eed:1 to ::5eed:4 by manual convention — the rule must
// reproduce these numbers exactly, otherwise adopting it would change live Node addresses.
func TestIndexForPort(t *testing.T) {
	for _, tc := range []struct {
		port  int
		index int
		ok    bool
	}{
		{6881, 1, true},
		{6882, 2, true},
		{6883, 3, true},
		{6884, 4, true},
		{7880, 1000, true}, // last port in the sequence (btPortSlots = 1000)
		{6880, 0, false},   // right below the start
		{7881, 0, false},   // right above the end
		{0, 0, false},
		{-1, 0, false},
		{51413, 0, false}, // port from a manual install, outside the CP sequence
	} {
		idx, ok := node.IndexForPort(tc.port)
		if idx != tc.index || ok != tc.ok {
			t.Errorf("IndexForPort(%d) = (%d, %v), want (%d, %v)", tc.port, idx, ok, tc.index, tc.ok)
		}
	}
}

// TestBootstrapMirrorsPortSequence locks the rule mirror in the bootstrap. Without enroll
// (`bootstrap-node.sh --network-only`), the script calculates the index from the Node port
// with BT_PORT_BASE/BT_PORT_SLOTS. If someone changes the port sequence here and forgets there,
// `--network-only` would start applying a DIFFERENT IPv6 suffix than what onboarding applied
// — and the operator's firewall rule would point to nothing, silently.
func TestBootstrapMirrorsPortSequence(t *testing.T) {
	// internal/node → repository root.
	path := filepath.Join("..", "..", "..", "scripts", "bootstrap-node.sh")
	raw, err := os.ReadFile(path) //nolint:gosec // G304: fixed path to the repository's own script
	if err != nil {
		t.Skipf("bootstrap-node.sh missing (module outside the repository?): %v", err)
	}
	grab := func(name string) int {
		t.Helper()
		m := regexp.MustCompile(`(?m)^` + name + `=(\d+)$`).FindSubmatch(raw)
		if m == nil {
			t.Fatalf("%s not found in bootstrap-node.sh", name)
		}
		n, err := strconv.Atoi(string(m[1]))
		if err != nil {
			t.Fatalf("%s invalid: %v", name, err)
		}
		return n
	}
	base, slots := grab("BT_PORT_BASE"), grab("BT_PORT_SLOTS")

	for _, tc := range []struct {
		port  int
		index int
		ok    bool
	}{
		{base - 1, 0, false},
		{base, 1, true},
		{base + slots - 1, slots, true},
		{base + slots, 0, false},
	} {
		if idx, ok := node.IndexForPort(tc.port); idx != tc.index || ok != tc.ok {
			t.Errorf("bootstrap mirrors BT_PORT_BASE=%d BT_PORT_SLOTS=%d, but IndexForPort(%d) = (%d, %v), want (%d, %v)",
				base, slots, tc.port, idx, ok, tc.index, tc.ok)
		}
	}
}
