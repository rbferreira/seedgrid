// Package rtorrent adapts rTorrent's XML-RPC to SeedGrid's operations (ADR-045): the second
// adapter behind engine.Engine. rTorrent 0.9.8 or later.
package rtorrent

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/rbferreira/seedgrid/agent/internal/engine"
	"github.com/rbferreira/seedgrid/agent/internal/pathmap"
)

// queuedMark is the value of the d.custom key customKey on a torrent the Agent's queue stopped
// (ADR-045 §4): stopped and incomplete with it is QUEUED, and the queue only ever starts those.
const (
	customKey  = "seedgrid"
	queuedMark = "queued"
)

// etaInfinite is the ETA of a torrent that is not downloading, the value qBittorrent uses.
const etaInfinite = 8640000

// Adapter is the rTorrent XML-RPC client.
type Adapter struct {
	rpc *client
	// paths translates between the Engine's paths and the host's (ADR-043); the zero value is
	// the identity.
	paths pathmap.Map
}

// New creates the adapter. rawURL is SEEDGRID_ENGINE_RTORRENT_URL: scgi:///path/to/rpc.socket,
// scgi://host:port or http(s)://[user:pass@]host/RPC2.
func New(rawURL string) (*Adapter, error) {
	c, err := newClient(rawURL)
	if err != nil {
		return nil, err
	}
	return &Adapter{rpc: c}, nil
}

// WithPaths sets the translation between the Engine's paths and the host's (ADR-043).
func (a *Adapter) WithPaths(m pathmap.Map) *Adapter {
	a.paths = m
	return a
}

// fields are what d.multicall2 reads of each torrent, in the order row parses them.
var fields = []string{
	"d.hash=", "d.name=", "d.state=", "d.is_active=", "d.complete=", "d.hashing=", "d.message=",
	"d.size_bytes=", "d.completed_bytes=", "d.down.rate=", "d.up.rate=", "d.ratio=",
	"d.down.total=", "d.up.total=", "d.peers_complete=", "d.peers_connected=",
	"d.directory=", "d.is_multi_file=", "d.custom=" + customKey,
}

// row is one torrent as rTorrent reports it.
type row struct {
	hash, name                  string
	started, active, complete   bool
	hashing                     int64
	message                     string
	size, done                  int64
	downRate, upRate            int64
	ratio                       int64 // per mille
	downTotal, upTotal          int64
	seedsConnected, connected   int64
	directory                   string
	multiFile                   bool
	custom                      string
	scrapeSeeds, scrapeLeechers int64
	trackers                    trackerInfo
}

// trackerInfo sums what the torrent's enabled trackers report.
type trackerInfo struct {
	enabled, succeeded bool
}

// rows lists the torrents. withTrackers also reads each torrent's trackers (one more round trip
// for all of them): the swarm counts and the tracker verdict come from there.
func (a *Adapter) rows(ctx context.Context, withTrackers bool) ([]row, error) {
	params := append([]any{"", "main"}, toAny(fields)...)
	v, err := a.rpc.call(ctx, "d.multicall2", params...)
	if err != nil {
		return nil, err
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("rtorrent: d.multicall2 returned %T", v)
	}
	out := make([]row, 0, len(list))
	for _, item := range list {
		f, ok := item.([]any)
		if !ok || len(f) != len(fields) {
			return nil, fmt.Errorf("rtorrent: d.multicall2 row with %d fields, want %d", len(f), len(fields))
		}
		out = append(out, row{
			hash: strings.ToLower(str(f[0])), name: str(f[1]),
			started: num(f[2]) != 0, active: num(f[3]) != 0, complete: num(f[4]) != 0,
			hashing: num(f[5]), message: str(f[6]),
			size: num(f[7]), done: num(f[8]), downRate: num(f[9]), upRate: num(f[10]), ratio: num(f[11]),
			downTotal: num(f[12]), upTotal: num(f[13]), seedsConnected: num(f[14]), connected: num(f[15]),
			directory: str(f[16]), multiFile: num(f[17]) != 0, custom: str(f[18]),
		})
	}
	if !withTrackers || len(out) == 0 {
		return out, nil
	}
	calls := make([]rpcCall, len(out))
	for i, r := range out {
		calls[i] = rpcCall{"t.multicall", []any{strings.ToUpper(r.hash), "",
			"t.is_enabled=", "t.scrape_complete=", "t.scrape_incomplete=", "t.success_counter="}}
	}
	res, err := a.rpc.multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	for i, tv := range res {
		trs, _ := tv.([]any) // a fault: the torrent left between the two calls
		for _, t := range trs {
			tf, ok := t.([]any)
			if !ok || len(tf) != 4 || num(tf[0]) == 0 {
				continue
			}
			out[i].trackers.enabled = true
			out[i].scrapeSeeds = max(out[i].scrapeSeeds, num(tf[1]))
			out[i].scrapeLeechers = max(out[i].scrapeLeechers, num(tf[2]))
			if num(tf[3]) > 0 {
				out[i].trackers.succeeded = true
			}
		}
	}
	return out, nil
}

// Torrents lists the torrents rTorrent holds, with the SeedGrid state (mapState).
func (a *Adapter) Torrents(ctx context.Context) ([]engine.Torrent, error) {
	rows, err := a.rows(ctx, true)
	if err != nil {
		return nil, err
	}
	out := make([]engine.Torrent, 0, len(rows))
	for _, r := range rows {
		out = append(out, a.torrent(r))
	}
	return out, nil
}

func (a *Adapter) torrent(r row) engine.Torrent {
	leechsConnected := max(r.connected-r.seedsConnected, 0)
	t := engine.Torrent{
		Hash:   r.hash,
		Name:   r.name,
		Status: mapState(r),
		// The scrape is 0 until a tracker answers it; the swarm is never smaller than what is
		// connected.
		NumSeeds:           int(max(r.scrapeSeeds, r.seedsConnected)),
		NumLeechs:          int(max(r.scrapeLeechers, leechsConnected)),
		NumSeedsConnected:  int(r.seedsConnected),
		NumLeechsConnected: int(leechsConnected),
		DlSpeed:            r.downRate,
		UpSpeed:            r.upRate,
		Size:               r.size,
		Ratio:              float64(r.ratio) / 1000,
		ETA:                etaInfinite,
		Downloaded:         r.downTotal,
		Uploaded:           r.upTotal,
		ContentPath:        a.paths.ToHost(contentPath(r)),
	}
	if r.size > 0 {
		t.Progress = float64(r.done) / float64(r.size)
	}
	if t.Status == "DOWNLOADING" && r.downRate > 0 {
		t.ETA = (r.size - r.done) / r.downRate
	}
	return t
}

// contentPath is where the data is: d.directory is the torrent's own folder for a multi-file
// torrent and the folder holding the file for a single-file one.
func contentPath(r row) string {
	if r.directory == "" || r.multiFile {
		return r.directory
	}
	return path.Join(r.directory, r.name)
}

// mapState translates rTorrent's state to the official SeedGrid state (Database.md §36,
// ADR-045 §3). rTorrent has no single state field:
//   - d.hashing set: CHECKING;
//   - d.message set by anything but a tracker (a storage error, a failed hash check): ERROR. A
//     tracker's answer is the tracker verdict (#160), as with qBittorrent, not the torrent's state;
//   - stopped (d.state 0) or paused (started, not active): PAUSED, except an incomplete torrent the
//     Agent's queue stopped, which is QUEUED;
//   - started: SEEDING when complete, DOWNLOADING otherwise.
func mapState(r row) string {
	switch {
	case r.hashing != 0:
		return "CHECKING"
	case r.message != "" && !strings.HasPrefix(r.message, "Tracker:"):
		return "ERROR"
	case !r.started && !r.complete && r.custom == queuedMark:
		return "QUEUED"
	case !r.started || !r.active:
		return "PAUSED"
	case r.complete:
		return "SEEDING"
	default:
		return "DOWNLOADING"
	}
}

// State returns the progress (0..1) and the SeedGrid state of a torrent by hash; ("", 0) when
// rTorrent does not hold it.
func (a *Adapter) State(ctx context.Context, hash string) (float64, string, error) {
	rows, err := a.rows(ctx, false)
	if err != nil {
		return 0, "", err
	}
	for _, r := range rows {
		if strings.EqualFold(r.hash, hash) {
			t := a.torrent(r)
			return t.Progress, t.Status, nil
		}
	}
	return 0, "", nil
}

// ContentPathOf returns where a torrent's data is, as the host sees it; "" when rTorrent does
// not hold it.
func (a *Adapter) ContentPathOf(ctx context.Context, hash string) (string, error) {
	rows, err := a.rows(ctx, false)
	if err != nil {
		return "", err
	}
	for _, r := range rows {
		if strings.EqualFold(r.hash, hash) {
			return a.paths.ToHost(contentPath(r)), nil
		}
	}
	return "", nil
}

// Version is rTorrent's version, e.g. "0.9.8"; an error means it is unreachable.
func (a *Adapter) Version(ctx context.Context) (string, error) {
	v, err := a.rpc.call(ctx, "system.client_version")
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("rtorrent: system.client_version returned %v", v)
	}
	return s, nil
}

// ConnectionStatus: rTorrent has no verdict on its inbound connectability (ADR-045 §7). "" is
// unknown to the Control Plane (ADR-034); IncomingPeers is what the inbound alert reads.
func (a *Adapter) ConnectionStatus(context.Context) (string, error) {
	return "", nil
}

// IncomingPeers counts the inbound connections open now, over all torrents (#155): the peers
// rTorrent marks p.is_incoming. One round trip for every torrent with a peer connected.
func (a *Adapter) IncomingPeers(ctx context.Context) (int, error) {
	rows, err := a.rows(ctx, false)
	if err != nil {
		return 0, err
	}
	var calls []rpcCall
	for _, r := range rows {
		if r.connected > 0 {
			calls = append(calls, rpcCall{"p.multicall", []any{strings.ToUpper(r.hash), "", "p.is_incoming="}})
		}
	}
	res, err := a.rpc.multicall(ctx, calls)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, pv := range res {
		peers, _ := pv.([]any) // a fault: the torrent left between the two calls
		for _, p := range peers {
			if pf, ok := p.([]any); ok && len(pf) == 1 && num(pf[0]) != 0 {
				total++
			}
		}
	}
	return total, nil
}

// Tracker statuses as libtorrent numbers them, which is what the Control Plane reads (#160):
// ≥ 4 is a tracker that is not answering or refuses.
const (
	trackerDisabled     = 0
	trackerNotContacted = 1
	trackerWorking      = 2
	trackerNotWorking   = 4
)

// TrackerVerdicts returns, by lowercase hash, the tracker verdict of each torrent (#160). rTorrent
// keeps the last tracker answer in d.message ("Tracker: [...]") until an announce succeeds: that is
// the "not working" with its message; otherwise a tracker that has answered is "working".
func (a *Adapter) TrackerVerdicts(ctx context.Context) (map[string]engine.TrackerVerdict, error) {
	rows, err := a.rows(ctx, true)
	if err != nil {
		return nil, err
	}
	out := make(map[string]engine.TrackerVerdict, len(rows))
	for _, r := range rows {
		out[r.hash] = verdict(r)
	}
	return out, nil
}

func verdict(r row) engine.TrackerVerdict {
	if msg, ok := trackerMessage(r.message); ok {
		return engine.TrackerVerdict{Status: trackerNotWorking, Message: msg}
	}
	switch {
	case !r.trackers.enabled:
		return engine.TrackerVerdict{Status: trackerDisabled}
	case r.trackers.succeeded:
		return engine.TrackerVerdict{Status: trackerWorking}
	default:
		return engine.TrackerVerdict{Status: trackerNotContacted}
	}
}

// trackerMessage takes the tracker's own words out of rTorrent's message:
// `Tracker: [Failure reason "Torrent has been deleted."]` → `Torrent has been deleted.`.
func trackerMessage(msg string) (string, bool) {
	rest, ok := strings.CutPrefix(msg, "Tracker: [")
	if !ok {
		return "", false
	}
	rest = strings.TrimSuffix(rest, "]")
	if reason, ok := strings.CutPrefix(rest, `Failure reason "`); ok {
		rest = strings.TrimSuffix(reason, `"`)
	}
	return strings.TrimSpace(rest), true
}

// SlotLimit looks for the tracker's per-account download limit in these torrents' messages (#21).
func (a *Adapter) SlotLimit(ctx context.Context, hashes []string) (int, bool) {
	rows, err := a.rows(ctx, false)
	if err != nil {
		return 0, false
	}
	want := make(map[string]bool, len(hashes))
	for _, h := range hashes {
		want[strings.ToLower(h)] = true
	}
	for _, r := range rows {
		if want[r.hash] {
			if n, ok := engine.SlotLimitIn(r.message); ok {
				return n, true
			}
		}
	}
	return 0, false
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// str and num read a decoded value, tolerating the type rTorrent chose for it.
func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	}
	return ""
}

func num(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case bool:
		if x {
			return 1
		}
	case float64:
		return int64(x)
	}
	return 0
}
