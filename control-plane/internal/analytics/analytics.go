// Package analytics answers the catalog questions that the Portal didn't know how to ask (#160).
//
// All of them originated from a MANUAL cleanup on 06-07/09/2026: to figure out what to remove
// it was necessary to access the four Nodes via SSH, checking 983 torrents one by one in
// qBittorrent and cross-referencing the results by hand. What was found: 3 dead torrents on the tracker,
// 36 duplicate title groups and 464 torrents (47% of the catalog, 2,40 TiB) that never
// sent a single byte — with three Nodes above 90% disk usage.
package analytics

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/release"
)

// Service queries the catalog. Read-only: nothing here removes or enqueues a command —
// removal remains the torrent DELETE, with the same audited path.
type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Torrent is the minimal projection used by all dashboards.
type Torrent struct {
	ID         string  `json:"id"`
	Hash       string  `json:"hash"`
	Name       string  `json:"name"`
	NodeID     string  `json:"node_id"`
	Hostname   string  `json:"hostname"`
	SizeBytes  int64   `json:"size_bytes"`
	NumSeeds   int     `json:"num_seeds"`
	Uploaded   int64   `json:"uploaded_total"`
	Downloaded int64   `json:"downloaded_total"`
	Ratio      float64 `json:"ratio"`
	AgeDays    float64 `json:"age_days"`
	// Node alias (#145); empty = the Portal shows the hostname.
	DisplayName string `json:"display_name,omitempty"`
	// Tracker verdict; TrackerStatus nil = unknown, never "dead".
	TrackerStatus  *int   `json:"tracker_status,omitempty"`
	TrackerMessage string `json:"tracker_message,omitempty"`
	Resolution     string `json:"resolution,omitempty"`

	// Flags that classify this torrent. They exist so that EVERY on-screen counter
	// has a corresponding list (#169): previously the response returned full counters and
	// truncated example lists, with nothing linking them together — the operator saw "157
	// with dead swarm" and had no way to reach the 157.
	NeverUploaded bool `json:"never_uploaded"`
	DeadSwarm     bool `json:"dead_swarm"`
	TrackerDead   bool `json:"tracker_dead"`
	// DuplicateExtra: it's the SURPLUS copy of a group (not the largest). It's the one that would free
	// disk space if removed.
	DuplicateExtra bool   `json:"duplicate_extra"`
	DuplicateOf    string `json:"duplicate_of,omitempty"`
}

// Report is the full response of GET /analytics.
type Report struct {
	Total          int            `json:"total"`
	TotalBytes     int64          `json:"total_bytes"`
	TrackerUnknown int            `json:"tracker_unknown"`
	Duplicates     []DuplicateSet `json:"duplicates"`
	Idle           IdleSummary    `json:"idle"`
	Nodes          []NodeUsage    `json:"nodes"`

	// Flagged brings ALL torrents that fall into any category, without truncating. This is what
	// allows the Portal to filter by card and reach the item (#169): previously the response
	// returned full counters and example lists cut off at 20, and the operator saw
	// "157 with dead swarm" with no way to reach the 157.
	//
	// Same pattern as the Downloaded screen, which already loads the entire list and filters on the client:
	// at this order of magnitude (hundreds) the cost is irrelevant compared to a route
	// paginated by filter, which would multiply the queries to build ONE screen.
	Flagged []Torrent `json:"flagged"`
}

// DuplicateSet is a title with more than one copy in the cluster.
type DuplicateSet struct {
	Title string `json:"title"`
	Year  int    `json:"year,omitempty"`
	SE    string `json:"se,omitempty"`
	// Part separates split works ("Phase 1" x "Phase 2"). Without it both would fall into the same
	// group and the smaller one would be marked as a surplus copy to delete (#172).
	Part string `json:"part,omitempty"`
	// Identical marks the case of PURE WASTE: same release, same size, on
	// different Nodes. It's a trivial decision, unlike different resolution — which the
	// tracker accepts as two torrents and yields ratio on both.
	Identical    bool      `json:"identical"`
	Copies       []Torrent `json:"copies"`
	ReclaimBytes int64     `json:"reclaim_bytes"` // sum of all except the largest
}

// IdleSummary summarizes the idle catalog — the biggest candidate for release measured in the cluster.
type IdleSummary struct {
	NeverUploaded      int   `json:"never_uploaded"`
	NeverUploadedBytes int64 `json:"never_uploaded_bytes"`
	// DeadSwarm: never sent AND no one else has it (0 seeds). Different decision from "never
	// sent but there are seeds": here the content died, there it just had no demand.
	DeadSwarm      int   `json:"dead_swarm"`
	DeadSwarmBytes int64 `json:"dead_swarm_bytes"`
}

// NodeUsage cross-references occupation with what is recoverable on that Node.
type NodeUsage struct {
	NodeID       string `json:"node_id"`
	Hostname     string `json:"hostname"`
	DisplayName  string `json:"display_name,omitempty"` // Node alias (#145)
	Torrents     int    `json:"torrents"`
	Bytes        int64  `json:"bytes"`
	IdleBytes    int64  `json:"idle_bytes"`
	ReclaimBytes int64  `json:"reclaim_bytes"`
}

const baseQuery = `
	SELECT t.id::text, t.hash, COALESCE(t.name, t.hash), t.node_id::text,
	       COALESCE(n.hostname, ''), COALESCE(n.display_name, ''),
	       COALESCE(t.size_bytes, 0), COALESCE(t.num_seeds, 0),
	       COALESCE(t.uploaded_total, 0), COALESCE(t.downloaded_total, 0),
	       EXTRACT(EPOCH FROM (now() - t.created_at)) / 86400.0,
	       t.tracker_status, COALESCE(t.tracker_message, '')
	  FROM torrents t
	  LEFT JOIN nodes n ON n.id = t.node_id
	 WHERE t.deleted_at IS NULL`

// Build assembles the entire report from a SINGLE read of the catalog.
//
// Just one query, and the rest in memory, on purpose: the dashboards overlap (duplicate
// that is also idle, idle that is also dead on the tracker) and recalculating each one
// in the database would mean scanning the same table four times. At the order of magnitude of this
// cluster — hundreds of torrents — the cost is irrelevant.
func (s *Service) Build(ctx context.Context) (Report, error) {
	rows, err := s.pool.Query(ctx, baseQuery)
	if err != nil {
		return Report{}, fmt.Errorf("read collection: %w", err)
	}
	defer rows.Close()

	var all []Torrent
	for rows.Next() {
		var t Torrent
		if err := rows.Scan(&t.ID, &t.Hash, &t.Name, &t.NodeID, &t.Hostname, &t.DisplayName,
			&t.SizeBytes, &t.NumSeeds, &t.Uploaded, &t.Downloaded, &t.AgeDays,
			&t.TrackerStatus, &t.TrackerMessage); err != nil {
			return Report{}, err
		}
		if t.Downloaded > 0 {
			t.Ratio = float64(t.Uploaded) / float64(t.Downloaded)
		}
		t.Resolution = release.ParseResolution(t.Name)
		all = append(all, t)
	}
	if err := rows.Err(); err != nil {
		return Report{}, err
	}

	rep := Report{Total: len(all)}
	for _, t := range all {
		rep.TotalBytes += t.SizeBytes
	}
	rep.TrackerUnknown = markTracker(all)
	rep.Duplicates = duplicates(all)
	markDuplicates(all, rep.Duplicates)
	rep.Idle = markIdle(all)
	rep.Nodes = nodeUsage(all, rep.Duplicates)

	// Just one pass at the end: the flags are all calculated already, and the counter and the list
	// now come from the SAME source — before nothing guaranteed they matched.
	for _, t := range all {
		if t.NeverUploaded || t.DeadSwarm || t.TrackerDead || t.DuplicateExtra {
			rep.Flagged = append(rep.Flagged, t)
		}
	}
	sort.Slice(rep.Flagged, func(i, j int) bool { return rep.Flagged[i].SizeBytes > rep.Flagged[j].SizeBytes })
	return rep, nil
}

// markTracker marks who the tracker is REFUSING and counts who hasn't been sampled yet.
// Status >= 4 is "not working/refused" in libtorrent; NULL is unknown and is never
// marked — unknown cannot turn into an accusation.
//
// Marks instead of returning a list so that the flag travels along with the torrent: it's what
// links the on-screen counter to the item (#169).
func markTracker(all []Torrent) int {
	unknown := 0
	for i := range all {
		switch {
		case all[i].TrackerStatus == nil:
			unknown++
		case *all[i].TrackerStatus >= 4:
			all[i].TrackerDead = true
		}
	}
	return unknown
}
