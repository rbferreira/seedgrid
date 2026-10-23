package commands

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/client"
)

type fakeEngine struct {
	added, stopped, started, rechecked, deleted string
	addedFile                                   []byte
	addedFileTo                                 []byte
	addedFileToSavePath                         string
	deleteFiles                                 bool
	err                                         error
	state                                       string  // state returned by State
	progress                                    float64 // progress returned by State
	shutdown                                    bool
	contentPath                                 string // path returned by ContentPathOf
	// stopErr fails ONLY the Stop (the `err` above would also fail the State, preventing
	// testing the migration pause in isolation).
	stopErr error
	// stateAfterStop is the state that State starts returning after a Stop —
	// simulating the Engine taking a moment to confirm the pause. Empty = "PAUSED".
	stateAfterStop string
	// stopCalls counts the stops (the migration pause cannot re-stop what is already stopped).
	stopCalls int
}

func (f *fakeEngine) AddURL(_ context.Context, s string) error  { f.added = s; return f.err }
func (f *fakeEngine) AddFile(_ context.Context, d []byte) error { f.addedFile = d; return f.err }
func (f *fakeEngine) AddFileTo(_ context.Context, d []byte, savePath string) error {
	f.addedFileTo, f.addedFileToSavePath = d, savePath
	return f.err
}
func (f *fakeEngine) Stop(_ context.Context, h string) error {
	f.stopped, f.stopCalls = h, f.stopCalls+1
	if f.stopErr != nil {
		return f.stopErr
	}
	if f.err != nil {
		return f.err
	}
	if f.state != "" {
		f.state = f.stateAfterStop
		if f.state == "" {
			f.state = "PAUSED"
		}
	}
	return nil
}
func (f *fakeEngine) Start(_ context.Context, h string) error   { f.started = h; return f.err }
func (f *fakeEngine) Recheck(_ context.Context, h string) error { f.rechecked = h; return f.err }
func (f *fakeEngine) Delete(_ context.Context, h string, df bool) error {
	f.deleted = h
	f.deleteFiles = df
	return f.err
}
func (f *fakeEngine) State(_ context.Context, _ string) (float64, string, error) {
	return f.progress, f.state, f.err
}
func (f *fakeEngine) Shutdown(_ context.Context) error { f.shutdown = true; return f.err }
func (f *fakeEngine) ContentPathOf(_ context.Context, _ string) (string, error) {
	return f.contentPath, f.err
}

type fakeMigrator struct {
	authToken, authHash  string
	authTTL              time.Duration
	pulledHost, pulledTo string
	pullErr              error
	uploadToken          string
	uploadTTL            time.Duration
}

func (m *fakeMigrator) Authorize(token, hash string, ttl time.Duration) {
	m.authToken, m.authHash, m.authTTL = token, hash, ttl
}
func (m *fakeMigrator) Pull(_ context.Context, host string, _ int, _ string, destPath string) error {
	m.pulledHost, m.pulledTo = host, destPath
	return m.pullErr
}
func (m *fakeMigrator) AuthorizeUpload(token string, ttl time.Duration) {
	m.uploadToken, m.uploadTTL = token, ttl
}

func TestExecute_AddTorrent(t *testing.T) {
	fe := &fakeEngine{}
	res := NewExecutor(fe).Execute(context.Background(), client.Command{
		Type:    "add_torrent",
		Payload: []byte(`{"magnet":"magnet:?xt=urn:btih:abc"}`),
	})
	if res.Status != "DONE" {
		t.Fatalf("status = %q (%s)", res.Status, res.Error)
	}
	if fe.added != "magnet:?xt=urn:btih:abc" {
		t.Errorf("added = %q", fe.added)
	}
}

func TestExecute_AddTorrentFile(t *testing.T) {
	fe := &fakeEngine{}
	raw := []byte("d4:infod4:name3:fooee") // any content; the Agent just passes the bytes through
	payload := `{"magnet":"magnet:?xt=urn:btih:abc","torrent_file":"` + base64.StdEncoding.EncodeToString(raw) + `"}`
	res := NewExecutor(fe).Execute(context.Background(), client.Command{
		Type: "add_torrent", Payload: []byte(payload),
	})
	if res.Status != "DONE" {
		t.Fatalf("status = %q (%s)", res.Status, res.Error)
	}
	// bytes take precedence over the magnet
	if string(fe.addedFile) != string(raw) {
		t.Errorf("addedFile = %q, want %q", fe.addedFile, raw)
	}
	if fe.added != "" {
		t.Errorf("should not use AddURL when torrent_file is present; added = %q", fe.added)
	}
}

func TestExecute_AddTorrentWithoutSource(t *testing.T) {
	res := NewExecutor(&fakeEngine{}).Execute(context.Background(), client.Command{
		Type: "add_torrent", Payload: []byte(`{}`),
	})
	if res.Status != "FAILED" {
		t.Errorf("status = %q, want FAILED", res.Status)
	}
}

func TestExecute_PauseResumeRecheck(t *testing.T) {
	fe := &fakeEngine{}
	ex := NewExecutor(fe)
	ctx := context.Background()

	if res := ex.Execute(ctx, client.Command{Type: "pause_torrent", Payload: []byte(`{"hash":"h1"}`)}); res.Status != "DONE" || fe.stopped != "h1" {
		t.Errorf("pause: status=%q stopped=%q", res.Status, fe.stopped)
	}
	if res := ex.Execute(ctx, client.Command{Type: "resume_torrent", Payload: []byte(`{"hash":"h2"}`)}); res.Status != "DONE" || fe.started != "h2" {
		t.Errorf("resume: status=%q started=%q", res.Status, fe.started)
	}
	if res := ex.Execute(ctx, client.Command{Type: "recheck_torrent", Payload: []byte(`{"hash":"h3"}`)}); res.Status != "DONE" || fe.rechecked != "h3" {
		t.Errorf("recheck: status=%q rechecked=%q", res.Status, fe.rechecked)
	}
	if res := ex.Execute(ctx, client.Command{Type: "remove_torrent", Payload: []byte(`{"hash":"h4","delete_files":true}`)}); res.Status != "DONE" || fe.deleted != "h4" || !fe.deleteFiles {
		t.Errorf("remove: status=%q deleted=%q deleteFiles=%v", res.Status, fe.deleted, fe.deleteFiles)
	}
}

func TestExecute_HashRequired(t *testing.T) {
	res := NewExecutor(&fakeEngine{}).Execute(context.Background(), client.Command{Type: "pause_torrent", Payload: []byte(`{}`)})
	if res.Status != "FAILED" {
		t.Errorf("status = %q, want FAILED", res.Status)
	}
}

func TestExecute_UnknownType(t *testing.T) {
	res := NewExecutor(&fakeEngine{}).Execute(context.Background(), client.Command{Type: "fly"})
	if res.Status != "FAILED" {
		t.Errorf("status = %q, want FAILED", res.Status)
	}
}

func TestExecute_EngineError(t *testing.T) {
	fe := &fakeEngine{err: errors.New("engine down")}
	res := NewExecutor(fe).Execute(context.Background(), client.Command{Type: "add_torrent", Payload: []byte(`{"magnet":"m"}`)})
	if res.Status != "FAILED" || res.Error == "" {
		t.Errorf("res = %+v", res)
	}
}

func TestExecute_RestartEngine(t *testing.T) {
	fe := &fakeEngine{}
	res := NewExecutor(fe).Execute(context.Background(), client.Command{Type: "restart_engine"})
	if res.Status != "DONE" || !fe.shutdown {
		t.Errorf("restart_engine: status=%q shutdown=%v", res.Status, fe.shutdown)
	}
}

func TestExecute_MigrateServe(t *testing.T) {
	fe := &fakeEngine{}
	fm := &fakeMigrator{}
	ex := NewExecutor(fe).EnableMigration(fm, "/data")
	exp := time.Now().Add(20 * time.Minute).UTC().Format(time.RFC3339)
	res := ex.Execute(context.Background(), client.Command{
		Type:    "migrate_serve",
		Payload: []byte(`{"hash":"h1","token":"tok","expires_at":"` + exp + `"}`),
	})
	if res.Status != "DONE" {
		t.Fatalf("status = %q (%s)", res.Status, res.Error)
	}
	if fm.authToken != "tok" || fm.authHash != "h1" || fm.authTTL <= 0 {
		t.Errorf("wrong Authorize: %+v", fm)
	}
}

// serveCmd builds the migrate_serve command with a valid token (used by the
// source pause tests, issue #8).
func serveCmd() client.Command {
	exp := time.Now().Add(20 * time.Minute).UTC().Format(time.RFC3339)
	return client.Command{
		Type:    "migrate_serve",
		Payload: []byte(`{"hash":"h1","token":"tok","expires_at":"` + exp + `"}`),
	}
}

// Issue #8: migrating copies disk content — a still-downloading torrent has
// in-flight writes. migrate_serve STOPS the torrent before authorizing the source.
func TestExecute_MigrateServeStopsActiveTorrent(t *testing.T) {
	fe := &fakeEngine{state: "DOWNLOADING"}
	fm := &fakeMigrator{}
	res := NewExecutor(fe).EnableMigration(fm, "/data").Execute(context.Background(), serveCmd())

	if res.Status != "DONE" {
		t.Fatalf("status = %q (%s)", res.Status, res.Error)
	}
	if fe.stopped != "h1" {
		t.Errorf("torrent was not stopped at the source: stopped=%q", fe.stopped)
	}
	if fm.authHash != "h1" {
		t.Error("should authorize serving after stopping")
	}
	if string(res.Result) != `{"paused":true}` {
		t.Errorf("result = %s, want {\"paused\":true}", res.Result)
	}
}

// An already stopped Torrent (or a hash the Engine doesn't even know) is not re-stopped —
// and the migration proceeds normally.
func TestExecute_MigrateServeDoesNotRePauseAlreadyStopped(t *testing.T) {
	for _, state := range []string{"PAUSED", ""} {
		fe := &fakeEngine{state: state}
		fm := &fakeMigrator{}
		res := NewExecutor(fe).EnableMigration(fm, "/data").Execute(context.Background(), serveCmd())

		if res.Status != "DONE" {
			t.Fatalf("state %q: status = %q (%s)", state, res.Status, res.Error)
		}
		if fe.stopCalls != 0 {
			t.Errorf("state %q: Stop called %d time(s); nothing to stop", state, fe.stopCalls)
		}
		if fm.authHash != "h1" {
			t.Errorf("state %q: should authorize serving anyway", state)
		}
		if string(res.Result) != `{"paused":false}` {
			t.Errorf("state %q: result = %s, want {\"paused\":false}", state, res.Result)
		}
	}
}

// The core guarantee of issue #8: without a confirmed pause, serving is NOT authorized.
// An aborted migration is recoverable (the torrent stays at the source); corrupted data is not.
func TestExecute_MigrateServeDoesNotAuthorizeWithoutPause(t *testing.T) {
	cases := map[string]*fakeEngine{
		"Stop fails":        {state: "DOWNLOADING", stopErr: errors.New("engine refused")},
		"does not confirm":  {state: "DOWNLOADING", stateAfterStop: "DOWNLOADING"},
		"State unavailable": {state: "DOWNLOADING", err: errors.New("engine down")},
	}
	for name, fe := range cases {
		fm := &fakeMigrator{}
		ex := NewExecutor(fe).EnableMigration(fm, "/data")
		ex.pauseSettle = 50 * time.Millisecond // not to hit the actual 10s timeout
		res := ex.Execute(context.Background(), serveCmd())

		if res.Status != "FAILED" {
			t.Errorf("%s: status = %q, want FAILED", name, res.Status)
		}
		if fm.authToken != "" {
			t.Errorf("%s: authorized to serve without confirmed pause — this is bug #8", name)
		}
	}
}

func TestExecute_PrepareUpload(t *testing.T) {
	fm := &fakeMigrator{}
	ex := NewExecutor(&fakeEngine{}).EnableMigration(fm, "/data")
	exp := time.Now().Add(20 * time.Minute).UTC().Format(time.RFC3339)
	res := ex.Execute(context.Background(), client.Command{
		Type:    "prepare_upload",
		Payload: []byte(`{"token":"uptok","expires_at":"` + exp + `"}`),
	})
	if res.Status != "DONE" {
		t.Fatalf("status = %q (%s)", res.Status, res.Error)
	}
	if fm.uploadToken != "uptok" || fm.uploadTTL <= 0 {
		t.Errorf("wrong AuthorizeUpload: token=%q ttl=%v", fm.uploadToken, fm.uploadTTL)
	}
}

func TestExecute_PrepareUploadDisabled(t *testing.T) {
	res := NewExecutor(&fakeEngine{}).Execute(context.Background(), client.Command{
		Type: "prepare_upload", Payload: []byte(`{"token":"t"}`),
	})
	if res.Status != "FAILED" {
		t.Errorf("status = %q, want FAILED (publication disabled)", res.Status)
	}
}

func TestExecute_MigrateServeDisabled(t *testing.T) {
	res := NewExecutor(&fakeEngine{}).Execute(context.Background(), client.Command{
		Type: "migrate_serve", Payload: []byte(`{"hash":"h1","token":"t"}`),
	})
	if res.Status != "FAILED" {
		t.Errorf("status = %q, want FAILED (migration disabled)", res.Status)
	}
}

func TestExecute_MigratePullSeeding(t *testing.T) {
	fe := &fakeEngine{state: "SEEDING", progress: 1.0}
	fm := &fakeMigrator{}
	ex := NewExecutor(fe).EnableMigration(fm, "/data")
	tf := base64.StdEncoding.EncodeToString([]byte("d4:infod4:name3:fooee"))
	res := ex.Execute(context.Background(), client.Command{
		Type:    "migrate_pull",
		Payload: []byte(`{"hash":"h1","token":"tok","source_host":"10.0.0.9","source_port":6900,"torrent_file":"` + tf + `"}`),
	})
	if res.Status != "DONE" {
		t.Fatalf("status = %q (%s)", res.Status, res.Error)
	}
	if fm.pulledHost != "10.0.0.9" || fm.pulledTo != "/data" {
		t.Errorf("wrong Pull: %+v", fm)
	}
	if fe.addedFileTo == nil || fe.rechecked != "h1" {
		t.Errorf("expected AddFileTo + Recheck; addedFileTo=%v rechecked=%q", fe.addedFileTo != nil, fe.rechecked)
	}
	// savepath must match where Pull() extracted the data — otherwise the Engine
	// might apply a different layout and fail to find the already copied data on recheck.
	if fe.addedFileToSavePath != "/data" {
		t.Errorf("AddFileTo savePath = %q, want %q (same destination as Pull)", fe.addedFileToSavePath, "/data")
	}
}

// TestExecute_MigratePullPartialCompletes is the case ADR-031 unlocks: torrent at 60%
// migrated to a Node with space. Previously the command required SEEDING and failed any
// partial migration, even with a perfect copy — and the destination, already with the .torrent
// added, would still download everything from the swarm (burns ratio).
func TestExecute_MigratePullPartialCompletes(t *testing.T) {
	fe := &fakeEngine{state: "DOWNLOADING", progress: 0.6}
	fm := &fakeMigrator{}
	ex := NewExecutor(fe).EnableMigration(fm, "/data")
	tf := base64.StdEncoding.EncodeToString([]byte("d4:infod4:name3:fooee"))
	res := ex.Execute(context.Background(), client.Command{
		Type:    "migrate_pull",
		Payload: []byte(`{"hash":"h1","token":"tok","source_host":"10.0.0.9","source_port":6900,"min_progress":0.6,"torrent_file":"` + tf + `"}`),
	})
	if res.Status != "DONE" {
		t.Fatalf("status = %q (%s), want DONE — partially migrated is success", res.Status, res.Error)
	}
}

// TestExecute_MigratePullPartialInsufficient: if the destination ends up BELOW what the source
// had, the migration fails — it's the "no loss" guarantee of ADR-018 under the new criteria.
func TestExecute_MigratePullPartialInsufficient(t *testing.T) {
	fe := &fakeEngine{state: "DOWNLOADING", progress: 0.10}
	fm := &fakeMigrator{}
	ex := NewExecutor(fe).EnableMigration(fm, "/data")
	tf := base64.StdEncoding.EncodeToString([]byte("d4:infod4:name3:fooee"))
	// The command context is canceled early (the destination never reaches the floor); the
	// test wait is longer, otherwise the ctx itself would win the race against the return.
	done := make(chan client.CommandResult, 1)
	cmdCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go func() {
		done <- ex.Execute(cmdCtx, client.Command{
			Type:    "migrate_pull",
			Payload: []byte(`{"hash":"h1","token":"tok","source_host":"h","source_port":6900,"min_progress":0.9,"torrent_file":"` + tf + `"}`),
		})
	}()
	select {
	case res := <-done:
		if res.Status != "FAILED" {
			t.Errorf("status = %q, want FAILED (destination has less than source)", res.Status)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Execute did not return — waitProgress should respect context cancellation")
	}
}

// TestMinProgressOf locks backward compatibility: old CP doesn't send the field, and in
// that case the Agent must require a COMPLETE torrent (ADR-018 behavior). If this
// regresses to 0, an empty migration would pass as successful.
func TestMinProgressOf(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"missing (old CP) requires complete", 0, 0.999},
		{"out of range requires complete", 1.5, 0.999},
		{"negative requires complete", -0.2, 0.999},
		{"partial deducts tolerance", 0.6, 0.595},
		{"complete deducts tolerance", 1.0, 0.995},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := minProgressOf(payload{MinProgress: c.in})
			if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("minProgressOf(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestExecute_MigratePullFailsOnPull(t *testing.T) {
	fe := &fakeEngine{}
	fm := &fakeMigrator{pullErr: errors.New("connection refused")}
	ex := NewExecutor(fe).EnableMigration(fm, "/data")
	ex.pullRetryFor = 0 // do not repeat in the failure test
	tf := base64.StdEncoding.EncodeToString([]byte("x"))
	res := ex.Execute(context.Background(), client.Command{
		Type:    "migrate_pull",
		Payload: []byte(`{"hash":"h1","token":"t","source_host":"h","source_port":6900,"torrent_file":"` + tf + `"}`),
	})
	if res.Status != "FAILED" {
		t.Errorf("status = %q, want FAILED", res.Status)
	}
	if fe.addedFileTo != nil {
		t.Errorf("should not add .torrent if pull failed")
	}
}

// TestExecute_MigrateServeForADownloadKeepsSeeding (#61): a download from the Portal reuses
// migrate_serve, and nothing starts the torrent again afterwards. With serve_only the torrent
// is not stopped; the token is still authorized.
func TestExecute_MigrateServeForADownloadKeepsSeeding(t *testing.T) {
	fe := &fakeEngine{state: "SEEDING"}
	fm := &fakeMigrator{}
	exp := time.Now().Add(20 * time.Minute).UTC().Format(time.RFC3339)
	res := NewExecutor(fe).EnableMigration(fm, "/data").Execute(context.Background(), client.Command{
		Type:    "migrate_serve",
		Payload: []byte(`{"hash":"h1","token":"tok","expires_at":"` + exp + `","serve_only":true}`),
	})
	if res.Status != "DONE" || fm.authHash != "h1" {
		t.Fatalf("status = %q (%s), authorized %q", res.Status, res.Error, fm.authHash)
	}
	if fe.stopCalls != 0 {
		t.Errorf("a download stopped the torrent (%d stops)", fe.stopCalls)
	}
	if string(res.Result) != `{"paused":false}` {
		t.Errorf("result = %s, want {\"paused\":false}", res.Result)
	}
}
