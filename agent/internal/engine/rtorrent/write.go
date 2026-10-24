package rtorrent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/engine"
	"github.com/rbferreira/seedgrid/agent/internal/torrentfile"
)

// queueState holds the engine.* settings the Agent's queue applies for rTorrent (ADR-045 §4).
// applied: the queue acts only once the settings came through Configure or ConfigureQueue — the
// env baseline alone must change nothing on an rTorrent that just joined (ADR-043).
type queueState struct {
	mu      sync.Mutex
	cfg     engine.Config
	applied bool
}

// The Adapter is an engine.Engine.
var _ engine.Engine = (*Adapter)(nil)

// AddURL adds a torrent from a magnet or a .torrent URL; rTorrent fetches it. With the Agent's
// queue on, a .torrent URL comes in queued and the queue starts it when a slot is free; a magnet
// starts, because rTorrent only fetches its metadata while it runs, and the queue then holds it.
func (a *Adapter) AddURL(ctx context.Context, magnetOrURL string) error {
	if a.queueing() && !strings.HasPrefix(magnetOrURL, "magnet:") {
		_, err := a.rpc.call(ctx, "load.normal", "", magnetOrURL, markQueued)
		return err
	}
	_, err := a.rpc.call(ctx, "load.start", "", magnetOrURL)
	return err
}

// AddFile adds a torrent from the contents of a .torrent file, into rTorrent's default folder:
// queued when the Agent's queue is on, so that adding many at once does not start them all.
func (a *Adapter) AddFile(ctx context.Context, data []byte) error {
	if a.queueing() {
		return a.loadRaw(ctx, "load.raw", data, markQueued)
	}
	return a.loadRaw(ctx, "load.raw_start", data)
}

// loadRaw loads a .torrent and confirms rTorrent took it: rTorrent answers 0 and drops, in silence,
// one it does not accept (0.9.8 drops a .torrent without a tracker).
func (a *Adapter) loadRaw(ctx context.Context, method string, data []byte, cmds ...string) error {
	hash, err := torrentfile.InfoHash(data)
	if err != nil {
		return err
	}
	params := []any{"", data}
	for _, c := range cmds {
		params = append(params, c)
	}
	if _, err := a.rpc.call(ctx, method, params...); err != nil {
		return err
	}
	for i := 0; ; i++ {
		_, err := a.rpc.call(ctx, "d.hash", strings.ToUpper(hash))
		if err == nil {
			return nil
		}
		var f *Fault
		if !errors.As(err, &f) {
			return err
		}
		if i == 10 {
			return fmt.Errorf("rtorrent: the .torrent %s was not taken (rTorrent drops one it refuses, such as a .torrent without a tracker on 0.9.8)", hash)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// markQueued is the load command that marks a torrent as the queue's.
const markQueued = "d.custom.set=" + customKey + "," + queuedMark

func (a *Adapter) queueing() bool {
	a.queue.mu.Lock()
	defer a.queue.mu.Unlock()
	return a.queue.applied && a.queue.cfg.QueueingEnabled
}

// AddFileTo adds a torrent whose data is already at savePath (a host path), laid out as the
// .torrent says (migration, ADR-018): d.directory is the folder holding a single file and the
// parent of a multi-file torrent's own folder, which is how Pull extracted it. It starts, queue or
// not, so that rTorrent checks the data the migration waits on; the queue counts it from then on.
func (a *Adapter) AddFileTo(ctx context.Context, data []byte, savePath string) error {
	return a.loadRaw(ctx, "load.raw_start", data, "d.directory.set="+a.paths.ToEngine(savePath))
}

// Stop stops a torrent and closes its files, as ruTorrent's stop does. It clears the queue mark:
// a torrent stopped by the operator or by admission (ADR-021) is not the Agent queue's to start.
func (a *Adapter) Stop(ctx context.Context, hash string) error {
	h := strings.ToUpper(hash)
	return a.all(ctx, []rpcCall{
		{"d.custom.set", []any{h, customKey, ""}},
		{"d.stop", []any{h}},
		{"d.close", []any{h}},
	})
}

// Start starts a torrent, stopped or paused, and clears the queue mark.
func (a *Adapter) Start(ctx context.Context, hash string) error {
	h := strings.ToUpper(hash)
	return a.all(ctx, []rpcCall{
		{"d.custom.set", []any{h, customKey, ""}},
		{"d.open", []any{h}},
		{"d.start", []any{h}},
		{"d.resume", []any{h}},
	})
}

// Recheck checks a torrent's data.
func (a *Adapter) Recheck(ctx context.Context, hash string) error {
	_, err := a.rpc.call(ctx, "d.check_hash", strings.ToUpper(hash))
	return err
}

// Delete removes a torrent. rTorrent never deletes data, so with deleteFiles the Agent removes it
// after rTorrent has let go of the torrent.
func (a *Adapter) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	var data string
	if deleteFiles {
		p, err := a.ContentPathOf(ctx, hash)
		if err != nil {
			return err
		}
		// Checked before rTorrent lets go: a refusal leaves everything as it was.
		if p != "" {
			data = filepath.Clean(p)
			if !filepath.IsAbs(data) || strings.Count(data, string(filepath.Separator)) < 2 {
				return fmt.Errorf("rtorrent: refusing to delete %q: not a torrent's data path", p)
			}
		}
	}
	if _, err := a.rpc.call(ctx, "d.erase", strings.ToUpper(hash)); err != nil {
		return err
	}
	if data == "" {
		return nil
	}
	return os.RemoveAll(data) //nolint:gosec // G703: the torrent's data path as rTorrent reports it, checked above
}

// Export returns the .torrent of a torrent rTorrent holds (backfill, ADR-018). rTorrent keeps it
// in its session folder with its own keys added; removing them gives the original back (ADR-045 §6).
func (a *Adapter) Export(ctx context.Context, hash string) ([]byte, error) {
	v, err := a.rpc.call(ctx, "session.path")
	if err != nil {
		return nil, err
	}
	dir := str(v)
	if dir == "" {
		return nil, errors.New("rtorrent: no session folder (session.path is empty), nowhere to read the .torrent from")
	}
	f, err := os.Open(filepath.Join(a.paths.ToHost(dir), strings.ToUpper(hash)+".torrent")) //nolint:gosec // G304: rTorrent's session folder and a hash
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 64<<20))
	if err != nil {
		return nil, err
	}
	return torrentfile.StripKeys(data, "rtorrent", "libtorrent_resume")
}

// CreateTorrent creates the .torrent of sourcePath (a host path) with the Agent's creator, since
// rTorrent has none (ADR-045 §5), and loads it started and pointing at the data: rTorrent checks
// it and seeds, which is what publishing (ADR-019) gets from qBittorrent's creator. d.directory is
// the folder holding the file, or the parent of the torrent's own folder, as in AddFileTo.
func (a *Adapter) CreateTorrent(ctx context.Context, sourcePath string, trackers []string, private bool, source string) ([]byte, error) {
	data, err := torrentfile.Create(ctx, sourcePath, torrentfile.Options{Trackers: trackers, Private: private, Source: source})
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(filepath.Clean(sourcePath))
	if err := a.loadRaw(ctx, "load.raw_start", data, "d.directory.set="+a.paths.ToEngine(dir)); err != nil {
		return nil, err
	}
	return data, nil
}

// Configure applies what belongs to the machine — the listening port and the default folder — and
// keeps the queue for the Agent's queue. On a Node that joined with its rTorrent both were read
// from it (ADR-043), so this changes nothing there. rTorrent opens its port at start.
func (a *Adapter) Configure(ctx context.Context, c engine.Config) error {
	calls := []rpcCall{{"network.port_random.set", []any{"", 0}}}
	if c.BTPort > 0 {
		p := strconv.Itoa(c.BTPort)
		calls = append(calls, rpcCall{"network.port_range.set", []any{"", p + "-" + p}})
	}
	if c.SavePath != "" {
		calls = append(calls, rpcCall{"directory.default.set", []any{"", a.paths.ToEngine(c.SavePath)}})
	}
	if err := a.all(ctx, calls); err != nil {
		return err
	}
	return a.ConfigureQueue(ctx, c)
}

// ConfigureQueue keeps the queue settings; the Agent's queue applies them (ADR-045 §4).
func (a *Adapter) ConfigureQueue(_ context.Context, c engine.Config) error {
	a.queue.mu.Lock()
	a.queue.cfg, a.queue.applied = c, true
	a.queue.mu.Unlock()
	return nil
}

// ReadQueue: rTorrent has no queue, so the Node starts with the queue off (ADR-045 §8) and the
// limits the Agent would use, ready for when the operator turns it on.
func (a *Adapter) ReadQueue(context.Context) (engine.Config, error) {
	a.queue.mu.Lock()
	defer a.queue.mu.Unlock()
	c := a.queue.cfg
	c.QueueingEnabled = false
	return c, nil
}

// Shutdown asks rTorrent to quit; whatever supervises it brings it back (ADR-045 §9). The
// connection may drop as it goes, which is the expected outcome.
func (a *Adapter) Shutdown(ctx context.Context) error {
	_, err := a.rpc.call(ctx, "system.shutdown.normal")
	var f *Fault
	if errors.As(err, &f) {
		return err
	}
	return nil
}

// all runs calls in one round trip and fails on the first refused one, except d.open and
// d.resume: rTorrent refuses to open what is open and to resume what is not paused.
func (a *Adapter) all(ctx context.Context, calls []rpcCall) error {
	res, err := a.rpc.multicall(ctx, calls)
	if err != nil {
		return err
	}
	for i, r := range res {
		if f, ok := r.(*Fault); ok && calls[i].method != "d.open" && calls[i].method != "d.resume" {
			return fmt.Errorf("%s: %w", calls[i].method, f)
		}
	}
	return nil
}
