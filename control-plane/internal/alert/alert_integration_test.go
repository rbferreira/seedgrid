package alert_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/alert"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
)

const testCode = "TEST_ALERT_ITEST"

func setup(t *testing.T) (*alert.Service, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	// cleans up leftovers from previous runs and at the end.
	clean := func() { _, _ = pool.Exec(context.Background(), `DELETE FROM alerts WHERE code = $1`, testCode) }
	clean()
	t.Cleanup(clean)
	return alert.NewService(pool), pool
}

func TestList(t *testing.T) {
	svc, _ := setup(t)
	alerts, err := svc.List(context.Background(), false)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if alerts == nil {
		t.Error("List returned nil slice; expected empty slice")
	}
}

// The listing returns the payload: the Portal uses it to build the phrase in the reader's language
// (ADR-041 §3); the message is just the fallback.
func TestListDevolveOPayload(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()
	if _, err := svc.Open(ctx, testCode, "WARNING", "", "test condition", map[string]any{"hostname": "node-x"}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	open, err := svc.List(ctx, true)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, a := range open {
		if a.Code != testCode {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal(a.Payload, &p); err != nil || p["hostname"] != "node-x" {
			t.Fatalf("payload = %s (err %v), want {\"hostname\":\"node-x\"}", a.Payload, err)
		}
		return
	}
	t.Fatal("the open alert did not appear in the list")
}

func TestOpenDedupResolveReopen(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()

	// 1) First opening creates the Alert.
	opened, err := svc.Open(ctx, testCode, "WARNING", "", "test condition", map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("Open (1): %v", err)
	}
	if !opened {
		t.Fatal("first Open should open a new alert")
	}

	// 2) Repeating the same condition does not duplicate (dedup).
	opened, err = svc.Open(ctx, testCode, "WARNING", "", "test condition", nil)
	if err != nil {
		t.Fatalf("Open (2): %v", err)
	}
	if opened {
		t.Error("second Open should not open another alert (dedup)")
	}

	// 3) Must appear among the OPEN.
	open, err := svc.List(ctx, true)
	if err != nil {
		t.Fatalf("List(open): %v", err)
	}
	if countCode(open, testCode) != 1 {
		t.Errorf("expected 1 OPEN alert with code %s, got %d", testCode, countCode(open, testCode))
	}

	// 4) Resolving closes the Alert.
	n, err := svc.Resolve(ctx, testCode, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if n != 1 {
		t.Errorf("expected to resolve 1 alert, resolved %d", n)
	}

	// 5) There are no more OPEN.
	open, _ = svc.List(ctx, true)
	if countCode(open, testCode) != 0 {
		t.Errorf("after resolving there should be no OPEN alert, got %d", countCode(open, testCode))
	}

	// 6) The condition can reopen after being resolved.
	opened, err = svc.Open(ctx, testCode, "WARNING", "", "reopened", nil)
	if err != nil {
		t.Fatalf("Open (reopen): %v", err)
	}
	if !opened {
		t.Error("resolved condition should be able to reopen")
	}
}

// spyNotifier records what the Alert Service sent to telemetry (ADR-028).
type spyNotifier struct {
	opened   []string // code|severity|message|hostname
	resolved []string
}

func (s *spyNotifier) AlertOpened(code, severity, nodeID, message string, payload map[string]any) {
	s.opened = append(s.opened, code+"|"+severity+"|"+message+"|"+host(payload))
}
func (s *spyNotifier) AlertResolved(code, severity, nodeID, message string, payload map[string]any) {
	s.resolved = append(s.resolved, code+"|"+severity+"|"+message+"|"+host(payload))
}
func host(p map[string]any) string {
	if p != nil {
		if h, ok := p["hostname"].(string); ok {
			return h
		}
	}
	return ""
}

// Telemetry (ADR-028) must go out on NEW FACTS and on RESOLVES — and the
// Open deduplication must apply to it too, otherwise the Alert Worker would notify the
// same condition every pass (30s) and the operator's phone would turn into a machine gun.
func TestNotifierOnOpenAndResolve(t *testing.T) {
	svc, _ := setup(t)
	spy := &spyNotifier{}
	svc.SetNotifier(spy)
	ctx := context.Background()

	if _, err := svc.Open(ctx, testCode, "CRITICAL", "", "Node offline",
		map[string]any{"hostname": "seedgrid-node-03"}); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(spy.opened) != 1 {
		t.Fatalf("open notifications = %d, want 1", len(spy.opened))
	}
	if spy.opened[0] != testCode+"|CRITICAL|Node offline|seedgrid-node-03" {
		t.Errorf("wrong open payload: %q", spy.opened[0])
	}

	// Same condition again (the worker re-evaluates every cycle): MUST NOT notify.
	if _, err := svc.Open(ctx, testCode, "CRITICAL", "", "Node offline", nil); err != nil {
		t.Fatalf("Open (dup): %v", err)
	}
	if len(spy.opened) != 1 {
		t.Errorf("repeated condition notified again (%d): Open dedupe must apply to telemetry", len(spy.opened))
	}

	n, err := svc.Resolve(ctx, testCode, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if n != 1 {
		t.Errorf("Resolve returned %d, want 1 (Exec→RETURNING transition must not change count)", n)
	}
	if len(spy.resolved) != 1 {
		t.Fatalf("resolution notifications = %d, want 1", len(spy.resolved))
	}
	if spy.resolved[0] != testCode+"|CRITICAL|Node offline|seedgrid-node-03" {
		t.Errorf("wrong resolution payload: %q (severity/message/hostname come from the alert that closed)", spy.resolved[0])
	}

	// Resolving again has nothing to close → nothing notified.
	if _, err := svc.Resolve(ctx, testCode, ""); err != nil {
		t.Fatalf("Resolve (empty): %v", err)
	}
	if len(spy.resolved) != 1 {
		t.Errorf("resolving without an open alert notified (%d)", len(spy.resolved))
	}
}

// Without a notifier, everything works the same: telemetry is optional (nil-safe).
func TestNoNotifier(t *testing.T) {
	svc, _ := setup(t)
	ctx := context.Background()
	if _, err := svc.Open(ctx, testCode, "WARNING", "", "no telemetry", nil); err != nil {
		t.Fatalf("Open without notifier: %v", err)
	}
	if _, err := svc.Resolve(ctx, testCode, ""); err != nil {
		t.Fatalf("Resolve without notifier: %v", err)
	}
}

func countCode(alerts []alert.Alert, code string) int {
	n := 0
	for _, a := range alerts {
		if a.Code == code {
			n++
		}
	}
	return n
}
