// Package scheduler decides on which Seed Node an operation will be executed
// (Scheduler.md). Decisions are deterministic, auditable, and reproducible.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/capacity"
)

// AlgorithmVersion identifies the version of the decision algorithm (Scheduler.md §23).
const AlgorithmVersion = "v1"

// ErrNoEligibleNode indicates that no Node is eligible for the operation.
var ErrNoEligibleNode = errors.New("no eligible Node")

// NoEligibleNodeError says WHY there was no eligible Node when the cause is failed storage.
//
// It wraps ErrNoEligibleNode instead of replacing it: every `errors.Is(err, ErrNoEligibleNode)`
// that already exists in the handlers still works, and whoever wants the detail uses errors.As. Without this
// the operator saw "no eligible Seed Node at the moment" with all Nodes online in the
// Portal, with nothing connecting one thing to the other (#180).
type NoEligibleNodeError struct {
	BlockedByStorage []string // hostnames excluded due to unmounted/unwritable /data
}

func (e *NoEligibleNodeError) Error() string {
	if len(e.BlockedByStorage) == 1 {
		return fmt.Sprintf("no eligible Node — %s is out due to failing storage", e.BlockedByStorage[0])
	}
	return fmt.Sprintf("no eligible Node — %d Nodes out due to failing storage (%s)",
		len(e.BlockedByStorage), strings.Join(e.BlockedByStorage, ", "))
}

func (e *NoEligibleNodeError) Unwrap() error { return ErrNoEligibleNode }

// ErrNoNodeWithSpace indicates that there are eligible Nodes, but none fits the torrent
// size with the disk margin (ADR-032) — distinct from "no eligible Node".
var ErrNoNodeWithSpace = errors.New("no Node with disk space for the torrent")

// NoNodeWithSpaceError says the NUMBERS of the space refusal.
//
// It wraps ErrNoNodeWithSpace, just like NoEligibleNodeError does with ErrNoEligibleNode: the
// `errors.Is` in the handlers still work. Without the numbers the operator read "no Node with
// space" for a 756 MiB torrent on a Node with 3.5 GiB free — what blocked it was the
// 5 GiB margin, which the message didn't mention, and "free up space" didn't solve it (#22).
type NoNodeWithSpaceError struct {
	NeedBytes     int64 // total torrent size
	MinFreeBytes  int64 // free disk margin in effect (admission.disk_min_free)
	BestFreeBytes int64 // largest projected free space among the eligible Nodes
}

func (e *NoNodeWithSpaceError) Error() string {
	return fmt.Sprintf("no Node with disk space for the torrent: it has %s and the margin is %s, but the Node with the most free space has %s free",
		capacity.HumanBytes(e.NeedBytes), capacity.HumanBytes(e.MinFreeBytes), capacity.HumanBytes(e.BestFreeBytes))
}

func (e *NoNodeWithSpaceError) Unwrap() error { return ErrNoNodeWithSpace }

// Decision is the outcome of a Scheduler decision.
type Decision struct {
	NodeID string
	Score  int
	Reason string
	// ReasonCode is the Reason as a stable code (ADR-041): the text above remains stored in the
	// database and the Feed; the code is what the Portal uses for the phrase in the reader's language.
	ReasonCode string
	Candidates int
	Algorithm  string
}

// Service implements the Scheduler.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the Scheduler.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// Select chooses the most suitable Node to receive a new Torrent.
//
// Eligibility (V1): ONLINE, with reachable Engine, healthy storage, recent heartbeat.
// Scoring: Health Score (higher is better); tie-breaker by LARGEST projected free space
// (ADR-032), then fewer torrents and hostname.
// The engine_reachable IS NOT FALSE filter excludes Nodes whose Engine is known to be
// unreachable (e.g.: down storage) — it wouldn't make sense to dispatch work to them; NULL
// (not yet probed) is tolerated.
// The storage_ok IS NOT FALSE filter, symmetrically, excludes Nodes whose /data the Agent reports
// as unmounted or unwritable. Without it the Node writes to the SYSTEM disk: the disk
// measured by the heartbeat becomes the rootfs one, so the Node still "fits" the space
// filter and receives work until the root is full. This is what happened with node-04 on
// 05/09/2026 (two torrents on a 15 GB /data on eMMC, before the HD was mounted).
// NULL (old Agent, which doesn't report the field) remains eligible — blocking it would be a regression.
//
// Disk-aware (ADR-032): when needBytes > 0, EXCLUDE Nodes that cannot accommodate
// the total torrent size + minFree margin. Nodes without disk data (heartbeat
// still without disk_total) are NOT excluded — there's no way to judge, and blocking would be a false
// positive. needBytes <= 0 (unknown size, e.g.: magnet) turns off the filter and the
// node with most free space just wins the tie-breaker. Distinguishes "none eligible" from "eligible,
// but none with space" (ErrNoNodeWithSpace) for the right message to the operator.
func (s *Service) Select(ctx context.Context, needBytes, minFree int64) (Decision, error) {
	pf := capacity.ProjectedFreeExpr()
	rows, err := s.pool.Query(ctx,
		`SELECT n.id::text,
		        COALESCE(n.hostname, '') AS hostname,
		        (n.storage_ok IS FALSE) AS storage_bad,
		        COALESCE(n.health_score, 0) AS health,
		        (SELECT count(*) FROM torrents t WHERE t.node_id = n.id AND t.deleted_at IS NULL) AS torrents,
		        CASE WHEN h.disk_total_bytes > 0 THEN `+pf+`::bigint ELSE NULL END AS proj_free,
		        CASE
		          WHEN $1::bigint <= 0 THEN TRUE                           -- unknown size: do not filter
		          WHEN h.disk_total_bytes IS NULL OR h.disk_total_bytes = 0 THEN TRUE  -- unknown disk: do not exclude
		          ELSE `+pf+` >= $1::bigint + $2::bigint                   -- fits within the margin?
		        END AS fits
		   FROM nodes n
		   LEFT JOIN heartbeats h ON h.node_id = n.id
		  WHERE n.deleted_at IS NULL
		    AND n.status = 'ONLINE'
		    AND n.engine IS NOT NULL
		    AND n.engine_reachable IS NOT FALSE
		    AND n.last_seen_at > now() - interval '90 seconds'
		  ORDER BY health DESC, proj_free DESC NULLS LAST, torrents ASC, n.hostname ASC`,
		needBytes, minFree)
	if err != nil {
		return Decision{}, fmt.Errorf("query eligible nodes: %w", err)
	}
	defer rows.Close()

	var (
		bestID         string
		bestScore      int
		blockedStorage []string // hostnames removed from scheduling due to failed storage (#180)
		eligible       int      // total eligible ONLINE (before the disk filter)
		fitting        int      // of the eligible ones, how many fit the torrent
		haveBest       bool
		bestFree       int64 // largest proj_free among the eligible (to explain the refusal)
	)
	for rows.Next() {
		var id, hostname string
		var storageBad, fits bool
		var health, torrents int
		var projFree *int64
		if err := rows.Scan(&id, &hostname, &storageBad, &health, &torrents, &projFree, &fits); err != nil {
			return Decision{}, err
		}
		// Failed storage keeps excluding on the FIRST read — it's fail-safe, and delaying
		// would open a window dispatching work to a disk with an I/O error. What
		// changes (#180) is that the exclusion stops being invisible: the caller learns
		// WHICH Nodes were dropped and why, instead of just receiving "no eligible Node".
		if storageBad {
			blockedStorage = append(blockedStorage, hostname)
			continue
		}
		eligible++
		if projFree != nil && *projFree > bestFree {
			bestFree = *projFree
		}
		if fits {
			fitting++
			if !haveBest {
				// First one that fits according to the ordering (health desc, most free space, fewer torrents).
				bestID, bestScore, haveBest = id, health, true
			}
		}
	}
	if err := rows.Err(); err != nil {
		return Decision{}, err
	}
	if eligible == 0 {
		// When storage exclusion is the reason there is no one, say this: it was
		// exactly the missing information for the operator to understand why the cluster
		// "has no Node" with all Nodes online (#180).
		if len(blockedStorage) > 0 {
			return Decision{}, &NoEligibleNodeError{BlockedByStorage: blockedStorage}
		}
		return Decision{}, ErrNoEligibleNode
	}
	if !haveBest {
		// There were eligible Nodes, but none fits the torrent — specific message.
		return Decision{}, &NoNodeWithSpaceError{NeedBytes: needBytes, MinFreeBytes: minFree, BestFreeBytes: bestFree}
	}

	reason, reasonCode := "highest health score among eligible Nodes", "best_health"
	if needBytes > 0 {
		reason, reasonCode = "highest health score among Nodes with space for the torrent", "best_health_with_space"
	}
	return Decision{
		NodeID:     bestID,
		Score:      bestScore,
		Reason:     reason,
		ReasonCode: reasonCode,
		Candidates: fitting,
		Algorithm:  AlgorithmVersion,
	}, nil
}
