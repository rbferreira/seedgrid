package commands

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/client"
)

// orderEngine records the ORDER of calls to the Engine. The standard fakeEngine only keeps the
// last hash of each operation — it doesn't prove that Start comes after Recheck, which is the
// order that #79 depends on.
type orderEngine struct {
	fakeEngine
	calls      []string
	stateCalls int
}

func (o *orderEngine) AddFileTo(ctx context.Context, d []byte, sp string) error {
	o.calls = append(o.calls, "AddFileTo")
	return o.fakeEngine.AddFileTo(ctx, d, sp)
}
func (o *orderEngine) Recheck(ctx context.Context, h string) error {
	o.calls = append(o.calls, "Recheck:"+h)
	return o.fakeEngine.Recheck(ctx, h)
}
func (o *orderEngine) Start(ctx context.Context, h string) error {
	o.calls = append(o.calls, "Start:"+h)
	return o.fakeEngine.Start(ctx, h)
}
func (o *orderEngine) Stop(ctx context.Context, h string) error {
	o.calls = append(o.calls, "Stop:"+h)
	return o.fakeEngine.Stop(ctx, h)
}
func (o *orderEngine) State(ctx context.Context, h string) (float64, string, error) {
	o.stateCalls++
	return o.fakeEngine.State(ctx, h)
}

// pullCmdPayload builds the payload for a migrate_pull; async enables the CP notification (#87).
func pullCmdPayload(hash string, async bool) []byte {
	tf := base64.StdEncoding.EncodeToString([]byte("d4:infod4:name3:fooee"))
	extra := ""
	if async {
		extra = `,"async_recheck":true`
	}
	return []byte(`{"hash":"` + hash + `","token":"t","source_host":"10.0.0.9","source_port":6900,"min_progress":1,"torrent_file":"` + tf + `"` + extra + `}`)
}

// With async_recheck, migrate_pull does NOT wait for the recheck: it responds DONE as soon as the
// copy finishes and the recheck is triggered. The engine here would never bottom out (it stays
// in CHECKING at 0%) — if the command waited, it would only return via the watchdog.
func TestMigratePull_AsyncRecheckReturnsWithoutWaitingForRecheck(t *testing.T) {
	e := &orderEngine{fakeEngine: fakeEngine{state: "CHECKING", progress: 0}}
	ex := fastExecutor(e).EnableMigration(&fakeMigrator{}, "/data")

	res := ex.Execute(context.Background(), client.Command{Type: "migrate_pull", Payload: pullCmdPayload("h1", true)})

	if res.Status != "DONE" {
		t.Fatalf("status = %q (%s), want DONE — the recheck wait belongs to CP", res.Status, res.Error)
	}
	if string(res.Result) != `{"recheck":"dispatched"}` {
		t.Errorf("result = %s, want {\"recheck\":\"dispatched\"} (trace showing DONE precedes verification)", res.Result)
	}
	if e.stateCalls != 0 {
		t.Errorf("State called %d time(s); Agent should not track recheck", e.stateCalls)
	}
	// What the command MUST do before responding, in order: add, recheck, and
	// only then start (starting before Recheck would make the Engine download what is already on
	// disk — #79).
	want := []string{"AddFileTo", "Recheck:h1", "Start:h1"}
	if strings.Join(e.calls, ",") != strings.Join(want, ",") {
		t.Errorf("calls to Engine = %v, want %v", e.calls, want)
	}
}

// Without the notification (CP prior to #87) the behavior is the usual: the Agent waits for the recheck
// and only responds DONE with the destination verified. Responding DONE earlier would make that CP
// remove the source without proof — which is the reason the field exists.
func TestMigratePull_WithoutAsyncRecheckKeepsWaiting(t *testing.T) {
	e := &orderEngine{fakeEngine: fakeEngine{state: "CHECKING", progress: 0}}
	ex := fastExecutor(e).EnableMigration(&fakeMigrator{}, "/data")

	res := ex.Execute(context.Background(), client.Command{Type: "migrate_pull", Payload: pullCmdPayload("h1", false)})

	if res.Status != "FAILED" || !strings.Contains(res.Error, "stalled") {
		t.Fatalf("res = %+v; without async_recheck the stalled recheck should fail due to stagnation", res)
	}
	if e.stateCalls == 0 {
		t.Error("the Agent should have tracked the recheck on the legacy path")
	}
}

// async_recheck only dispenses with the WAIT. Failures before it remain FAILED: the CP
// needs to know that the copy or the recheck trigger didn't happen, otherwise it would be left
// watching a torrent that will never appear at the destination.
func TestMigratePull_AsyncRecheckStillFailsIfCopyOrTriggerFails(t *testing.T) {
	t.Run("pull fails", func(t *testing.T) {
		fm := &fakeMigrator{pullErr: errors.New("connection refused")}
		ex := fastExecutor(&orderEngine{}).EnableMigration(fm, "/data")
		ex.pullRetryFor = 0
		res := ex.Execute(context.Background(), client.Command{Type: "migrate_pull", Payload: pullCmdPayload("h1", true)})
		if res.Status != "FAILED" {
			t.Errorf("status = %q, want FAILED", res.Status)
		}
	})
	t.Run("engine rejects recheck", func(t *testing.T) {
		e := &orderEngine{fakeEngine: fakeEngine{err: errors.New("engine down")}}
		ex := fastExecutor(e).EnableMigration(&fakeMigrator{}, "/data")
		res := ex.Execute(context.Background(), client.Command{Type: "migrate_pull", Payload: pullCmdPayload("h1", true)})
		if res.Status != "FAILED" {
			t.Errorf("status = %q, want FAILED (AddFileTo/Recheck/Start failed)", res.Status)
		}
	})
}

// cpFake is a minimal Control Plane for the Poller: it delivers ONE batch of commands and records,
// in order, what the Agent has confirmed.
type cpFake struct {
	mu        sync.Mutex
	batch     []client.Command
	delivered bool
	acks      []string // "<id>:<status>"
}

func (c *cpFake) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agent/commands":
			batch := c.batch
			if c.delivered {
				batch = nil
			}
			c.delivered = true
			_ = json.NewEncoder(w).Encode(map[string]any{"data": batch})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/agent/commands/"):
			var res client.CommandResult
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &res)
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/agent/commands/"), "/result")
			c.acks = append(c.acks, id+":"+res.Status)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// The #87 scenario: a migrate_pull with the recheck still running in the Engine MUST NOT
// hold up the Node. The pause/resume/remove of OTHER torrents that come in the same batch (and the
// next batch) need to be executed and confirmed without waiting.
func TestPoller_MigratePullDoesNotHoldNodeQueue(t *testing.T) {
	// Engine with the recheck eternally in progress: if any command waited for it, the tick
	// would only finish via the watchdog (3 s here) — the test requires much less than that.
	e := &orderEngine{fakeEngine: fakeEngine{state: "CHECKING", progress: 0.2}}
	ex := fastExecutor(e).EnableMigration(&fakeMigrator{}, "/data")
	ex.recheckStall = time.Hour

	cp := &cpFake{batch: []client.Command{
		{ID: "c1", Type: "migrate_pull", Payload: pullCmdPayload("h1", true)},
		{ID: "c2", Type: "pause_torrent", Payload: []byte(`{"hash":"other"}`)},
		{ID: "c3", Type: "remove_torrent", Payload: []byte(`{"hash":"other2","delete_files":true}`)},
	}}
	srv := httptest.NewServer(cp.handler(t))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := NewPoller(client.New(srv.URL), func() string { return "tok" }, ex, time.Hour, logger)

	start := time.Now()
	p.tick(context.Background())
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("tick took %s; the in-progress recheck cannot hold up the batch", d)
	}

	want := []string{"c1:DONE", "c2:DONE", "c3:DONE"}
	if strings.Join(cp.acks, ",") != strings.Join(want, ",") {
		t.Errorf("confirmations = %v, want %v (batch order preserved, all executed)", cp.acks, want)
	}
	// Commands for other torrents reached the Engine AFTER the pull and were executed
	// in fact, not just confirmed.
	if e.stopped != "other" || e.deleted != "other2" {
		t.Errorf("stopped=%q deleted=%q; pause/remove of other torrents should have run", e.stopped, e.deleted)
	}
}

// Contention for the SAME torrent (#87, point 3): with the pull returning early, a command for
// the hash still in recheck might arrive right behind it. The Agent keeps the batch order
// (serial) — the pause runs AFTER the pull's Start, never in the middle of it. What prevents
// this command from existing is the CP (torrent.ErrMigrationInFlight); here we only enforce the guarantee
// of order that the Agent provides.
func TestPoller_CommandOnSameHashRunsAfterPull(t *testing.T) {
	e := &orderEngine{fakeEngine: fakeEngine{state: "CHECKING", progress: 0.2}}
	ex := fastExecutor(e).EnableMigration(&fakeMigrator{}, "/data")

	cp := &cpFake{batch: []client.Command{
		{ID: "c1", Type: "migrate_pull", Payload: pullCmdPayload("h1", true)},
		{ID: "c2", Type: "pause_torrent", Payload: []byte(`{"hash":"h1"}`)},
	}}
	srv := httptest.NewServer(cp.handler(t))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	p := NewPoller(client.New(srv.URL), func() string { return "tok" }, ex, time.Hour, logger)
	p.tick(context.Background())

	want := []string{"AddFileTo", "Recheck:h1", "Start:h1", "Stop:h1"}
	if strings.Join(e.calls, ",") != strings.Join(want, ",") {
		t.Errorf("calls to Engine = %v, want %v", e.calls, want)
	}
}
