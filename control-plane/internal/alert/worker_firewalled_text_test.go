package alert

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// fwTestIPv6 is the Node address in tests (documentation range, RFC 3849).
const fwTestIPv6 = "2001:db8:0:1:be24:11ff:fe12:dee1"

// fwOpenAlert returns the message and the payload of the Node's open Alert.
func fwOpenAlert(t *testing.T, pool *pgxpool.Pool, nodeID string) (string, map[string]any) {
	t.Helper()
	var msg, raw string
	if err := pool.QueryRow(context.Background(),
		`SELECT COALESCE(message, ''), COALESCE(payload::text, '{}') FROM alerts
		  WHERE code = 'NODE_FIREWALLED' AND status = 'OPEN' AND node_id = $1`,
		nodeID).Scan(&msg, &raw); err != nil {
		t.Fatalf("read open alert: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	return msg, payload
}

// TestEvaluateFirewalled_TextDoesNotAge (#223): the Alert is logged once and can
// stay open for days. Address and duration in the text aged — the Node changed
// address with the Alert open and the dashboard kept asking to allow an address that
// no longer existed. The text now has no changing parts; what varies goes in the payload.
func TestEvaluateFirewalled_TextDoesNotAge(t *testing.T) {
	w, pool, nodeID := setupFirewalledWorker(t, "firewalled", "2 hours", "30m")
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE nodes SET public_ipv6 = $2 WHERE id = $1`, nodeID, fwTestIPv6); err != nil {
		t.Fatalf("set the Node to the test address: %v", err)
	}

	w.evaluateFirewalled(ctx)
	msg, payload := fwOpenAlert(t, pool, nodeID)

	if msg != firewalledMessage {
		t.Fatalf("message = %q, expected package constant", msg)
	}
	// No fragments that age: address, port, duration. No digits, in fact — if
	// anyone interpolates a value in the text again, this test breaks.
	for _, frag := range []string{fwTestIPv6, "2001:", "[", "6881", " ago ", "2h"} {
		if strings.Contains(msg, frag) {
			t.Errorf("text contains %q, which goes stale: %q", frag, msg)
		}
	}
	if strings.ContainsAny(msg, "0123456789") {
		t.Errorf("text contains a number, which goes stale: %q", msg)
	}

	// What the operator needs stays in the payload.
	if payload["public_ipv6"] != fwTestIPv6 {
		t.Errorf("payload.public_ipv6 = %v, expected %s", payload["public_ipv6"], fwTestIPv6)
	}
	if got, _ := payload["bt_port"].(float64); got != 6881 {
		t.Errorf("payload.bt_port = %v, expected 6881", payload["bt_port"])
	}
	if payload["hostname"] != "fw-itest-node" {
		t.Errorf("payload.hostname = %v", payload["hostname"])
	}
	if s, _ := payload["connectable_since"].(string); s == "" {
		t.Errorf("payload.connectable_since missing: the verdict start is what the duration no longer indicates")
	}

	// The Node changes address with the Alert open: the text remains true.
	if _, err := pool.Exec(ctx,
		`UPDATE nodes SET public_ipv6 = '2001:db8:0:2::1' WHERE id = $1`, nodeID); err != nil {
		t.Fatalf("change address: %v", err)
	}
	w.evaluateFirewalled(ctx)
	if got, _ := fwOpenAlert(t, pool, nodeID); got != msg {
		t.Fatalf("text changed with the alert open: %q -> %q", msg, got)
	}
}
