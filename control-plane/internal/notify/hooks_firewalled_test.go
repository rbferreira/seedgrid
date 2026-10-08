package notify

import (
	"strings"
	"testing"
	"time"
)

// The NODE_FIREWALLED text does not take the address (#223): it is written once and ages.
// The notification, which is instantaneous, retrieves it from the payload as a structured field.
func TestAlertTemplateFirewalledUsesPayloadAddress(t *testing.T) {
	m := Message{Lang: "pt-BR", At: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)}
	applyAlertTemplate(&m, alertCodeNodeFirewalled, map[string]any{
		"hostname":          "node-a",
		"public_ipv6":       "2001:db8:0:1:be24:11ff:fe12:dee1",
		"bt_port":           6881, // int: directly from the worker
		"connectable_since": "2026-10-01T03:19:46.706569Z",
	})

	fields := map[string]string{}
	for _, f := range m.Fields {
		fields[f.Label] = f.Value
	}
	if got := fields["Endereço do Node"]; got != "[2001:db8:0:1:be24:11ff:fe12:dee1]:6881" {
		t.Fatalf("Node address = %q", got)
	}
	if got := fields["Firewalled desde"]; got != "01/10 03:19:46" {
		t.Fatalf("Firewalled since = %q", got)
	}
	// No machine contract: this alert doesn't trigger automation, so the webhook follows
	// byte by byte as before.
	if m.Schema != 0 || m.Data != nil {
		t.Fatalf("alert without structured contract got schema=%d data=%v", m.Schema, m.Data)
	}
}

// No address in the payload, no invented fields: "[]:6881" would look like an instruction.
func TestAlertTemplateFirewalledWithoutAddressDoesNotInventField(t *testing.T) {
	m := Message{Lang: "pt-BR"}
	applyAlertTemplate(&m, alertCodeNodeFirewalled, map[string]any{
		"public_ipv6": "",
		"bt_port":     6881,
	})
	for _, f := range m.Fields {
		if strings.Contains(f.Value, "[]") || f.Label == "Endereço do Node" {
			t.Fatalf("made-up field without address: %+v", f)
		}
	}
	if len(m.Fields) != 0 {
		t.Fatalf("expected no fields, got %+v", m.Fields)
	}
}
