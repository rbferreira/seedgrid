package notify

import (
	"strings"
	"testing"
	"time"
)

// The NODE_INBOUND_SILENT text does not include the address (#203): it is logged once and ages.
// The notification, which is an instant, retrieves it from the payload as a structured field.
func TestAlertTemplateInboundSilentUsesPayloadAddress(t *testing.T) {
	m := Message{Lang: "pt-BR", At: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	applyAlertTemplate(&m, alertCodeNodeInboundSilent, map[string]any{
		"hostname":         "node-a",
		"public_ipv6":      "2001:db8:0:1:be24:11ff:fe12:dee1",
		"bt_port":          6881, // int: directly from the worker
		"last_incoming_at": "2026-10-01T03:19:46.706569Z",
	})

	fields := map[string]string{}
	for _, f := range m.Fields {
		fields[f.Label] = f.Value
	}
	if got := fields["Endereço do Node"]; got != "[2001:db8:0:1:be24:11ff:fe12:dee1]:6881" {
		t.Fatalf("Node address = %q", got)
	}
	if got := fields["Última entrada"]; got != "01/10 03:19:46" {
		t.Fatalf("Last entry = %q", got)
	}
	// No machine contract: this alert does not trigger automation, so the webhook continues
	// byte by byte as before.
	if m.Schema != 0 || m.Data != nil {
		t.Fatalf("alert without structured contract got schema=%d data=%v", m.Schema, m.Data)
	}
}

// No address in the payload, no invented field: "[]:6881" would look like an instruction.
func TestAlertTemplateInboundSilentWithoutAddressDoesNotInventField(t *testing.T) {
	m := Message{Lang: "pt-BR"}
	applyAlertTemplate(&m, alertCodeNodeInboundSilent, map[string]any{
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

func TestBtEndpoint(t *testing.T) {
	cases := []struct {
		name    string
		payload map[string]any
		want    string
	}{
		{"int do worker", map[string]any{"public_ipv6": "2001:db8::1", "bt_port": 6881}, "[2001:db8::1]:6881"},
		{"int64", map[string]any{"public_ipv6": "2001:db8::1", "bt_port": int64(6881)}, "[2001:db8::1]:6881"},
		{"float64 depois de JSON", map[string]any{"public_ipv6": "2001:db8::1", "bt_port": float64(6881)}, "[2001:db8::1]:6881"},
		{"porta zero devolve só o endereço", map[string]any{"public_ipv6": "2001:db8::1", "bt_port": 0}, "2001:db8::1"},
		{"porta ausente devolve só o endereço", map[string]any{"public_ipv6": "2001:db8::1"}, "2001:db8::1"},
		{"sem endereço", map[string]any{"bt_port": 6881}, ""},
		{"endereço vazio", map[string]any{"public_ipv6": "", "bt_port": 6881}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := btEndpoint(c.payload); got != c.want {
				t.Fatalf("btEndpoint = %q, expected %q", got, c.want)
			}
		})
	}
}
