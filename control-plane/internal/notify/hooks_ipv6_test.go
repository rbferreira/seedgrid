package notify

import "testing"

// ipv6Prefix is the field the firewall automation actually consumes: the rule needs the
// /64 network object, not the address. It was the literal that caused the incident on
// 05-06/09/2026 — the rule survived the rotation pointing to a dead address.
func TestIPv6Prefix(t *testing.T) {
	cases := []struct {
		name, addr, want string
	}{
		{"endereço de Node (EUI-64)", "2001:db8:60b:c000:be24:11ff:fe12:dee1", "2001:db8:60b:c000::/64"},
		{"prefixo antigo do incidente", "2001:db8:601:d300:be24:11ff:fe12:dee1", "2001:db8:601:d300::/64"},
		{"forma comprimida ainda resolve", "2001:db8:60b:c000::1", "2001:db8:60b:c000::/64"},
		{"vazio não inventa prefixo", "", ""},
		{"lixo não inventa prefixo", "não-é-endereço", ""},
		// Wrong prefix is WORSE than no prefix at all: the consumer has the full address and
		// can decide, but a firewall rule written with the wrong /64 opens what it shouldn't
		// (or closes what it should open) silently.
		{"IPv4 não vira prefixo v6", "192.168.1.1", ""},
		{"hextetos de menos", "2001:db8", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ipv6Prefix(c.addr); got != c.want {
				t.Fatalf("ipv6Prefix(%q) = %q, expected %q", c.addr, got, c.want)
			}
		})
	}
}

// The `data` block is a MACHINE contract (#147): if its shape changes, the automation on
// the other side breaks silently. This test locks the shape.
func TestAlertTemplateIPv6ChangedBuildsDateBlock(t *testing.T) {
	m := Message{Lang: "pt-BR"}
	applyAlertTemplate(&m, alertCodeNodeIPv6Changed, map[string]any{
		"hostname":      "seedgrid-node-01",
		"node_id":       "b4f312bb-cd99-4463-9fd3-276e626b17ff",
		"previous_ipv6": "2001:db8:601:d300:be24:11ff:fe12:dee1",
		"current_ipv6":  "2001:db8:60b:c000:be24:11ff:fe12:dee1",
		"bt_port":       6881,
	})

	if m.Schema != SchemaVersion {
		t.Fatalf("schema = %d, expected %d", m.Schema, SchemaVersion)
	}
	if m.Data == nil {
		t.Fatal("missing data block")
	}
	if m.Data["event"] != "node_ipv6_changed" {
		t.Fatalf("event = %v", m.Data["event"])
	}
	ipv6, ok := m.Data["ipv6"].(map[string]any)
	if !ok {
		t.Fatalf("data.ipv6 is not an object: %T", m.Data["ipv6"])
	}
	// The prefix comes CALCULATED by SeedGrid: letting the consumer derive it with string
	// manipulation would plant the same bug from the incident on the other side of the integration.
	if ipv6["prefix_previous"] != "2001:db8:601:d300::/64" {
		t.Fatalf("prefix_previous = %v", ipv6["prefix_previous"])
	}
	if ipv6["prefix_current"] != "2001:db8:60b:c000::/64" {
		t.Fatalf("prefix_current = %v", ipv6["prefix_current"])
	}
	node, ok := m.Data["node"].(map[string]any)
	if !ok || node["hostname"] != "seedgrid-node-01" {
		t.Fatalf("unexpected data.node: %v", m.Data["node"])
	}

	// Fields still exists for humans: Telegram and Discord render text.
	if len(m.Fields) == 0 {
		t.Fatal("Fields empty — the text channel would have no body")
	}
}

// Alerts WITHOUT a structured template should not get a data block or schema: the webhook for
// whoever already consumes it today goes out byte-by-byte identical to what it always did.
func TestAlertTemplateWithoutDateDoesNotInventSchema(t *testing.T) {
	m := Message{Lang: "pt-BR"}
	applyAlertTemplate(&m, alertCodeDiskIOSaturated, map[string]any{"util_pct": 95.0})
	if m.Schema != 0 || m.Data != nil {
		t.Fatalf("alert without structured contract got schema=%d data=%v", m.Schema, m.Data)
	}
}
