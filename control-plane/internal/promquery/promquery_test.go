package promquery

import (
	"strings"
	"testing"
)

// TestNetQueriesDoNotPinInterface guards against the regression that removed node-04 from the
// network charts: the queries had `device="eth0"` hardcoded, and every Node whose interface has
// a different name vanished from the series — no error, no empty series, simply absent. `eth0` is
// the common name in containers, not a guarantee: bare-metal with systemd uses predictable names
// (enp1s0/wlp2s0) and out-of-tree drivers invent their own (mlan0, on node-04's WiFi).
//
// The lesson the test locks in: excluding what you KNOW is virtual is robust; enumerating what
// you EXPECT to be physical is not.
func TestNetQueriesDoNotPinInterface(t *testing.T) {
	for _, k := range []string{"host_net_rx_bytes", "host_net_tx_bytes"} {
		q, ok := curated[k]
		if !ok {
			t.Fatalf("query %q disappeared from catalog", k)
		}
		if strings.Contains(q, `device="`) {
			t.Errorf("%s pins interface by name — Node with another name disappears from the graph: %s", k, q)
		}
		if !strings.Contains(q, "device!~") {
			t.Errorf("%s should EXCLUDE virtual ones instead of enumerating physical ones: %s", k, q)
		}
		// Without excluding the virtual ones, container traffic (which already goes out through the
		// physical interface) would be counted twice.
		for _, virtual := range []string{"lo", "docker", "veth", "br-"} {
			if !strings.Contains(q, virtual) {
				t.Errorf("%s does not exclude %q — container traffic double-counted", k, virtual)
			}
		}
	}
}

// TestCatalogKeysStable: the client never sends raw PromQL, it only picks keys
// (see the comment on `curated`). Removing or renaming a key breaks the Portal
// silently, so they are a contract.
func TestCatalogKeysStable(t *testing.T) {
	for _, k := range []string{
		"host_cpu_pct", "host_mem_used_bytes", "host_mem_total_bytes", "host_fs_avail_bytes",
		"host_net_rx_bytes", "host_net_tx_bytes",
		"host_disk_read_bytes", "host_disk_write_bytes", "host_disk_util_pct",
		"nodes", "torrents", "alerts_open",
	} {
		if _, ok := curated[k]; !ok {
			t.Errorf("key %q disappeared from catalog — the Portal API depends on it", k)
		}
	}
}
