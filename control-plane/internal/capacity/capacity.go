// Package capacity centralizes the projected disk space calculation per Seed Node
// (ADR-032). It is the SINGLE source of the formula, consumed by the Scheduler (where to place a
// new torrent), by migration (whether the destination fits) and by the Alert Worker (warn
// when a Node has committed beyond what the disk concludes). Keeping the expression in one
// place prevents the three from diverging.
//
// Projected free space = disk_free_bytes (heartbeat, bytes ALREADY written deducted)
//
//	− Σ (live torrents on the Node) size_bytes × (1 − progress)
//
// that is: the current free minus what the torrents already present STILL need to write until
// completion. A complete torrent contributes 0 (it is already in free). Measuring by TOTAL
// size (not downloaded) is the ADR-032 decision: the torrent will download in full.
package capacity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// projectedFreeExpr is the SQL expression for the projected free space of a Node, assuming
// the aliases `n` (nodes) and `h` (heartbeats) in scope. `committedRemainingExpr` is the
// sum of what the Node's torrents still need to write.
const committedRemainingExpr = `
	COALESCE((
		SELECT sum(t.size_bytes * (1 - LEAST(GREATEST(COALESCE(t.progress, 0), 0), 1)))
		  FROM torrents t
		 WHERE t.node_id = n.id AND t.deleted_at IS NULL AND t.size_bytes IS NOT NULL
	), 0)`

// projectedFreeExpr computes disk_free_bytes − committedRemaining. It only makes sense when
// the heartbeat has disk data (disk_total_bytes > 0).
const projectedFreeExpr = `(h.disk_free_bytes - ` + committedRemainingExpr + `)`

// ProjectedFree returns the projected free space (bytes) of a Node and whether the data is
// KNOWN. known=false when there is no heartbeat with disk data (new Agent has not yet
// reported, or disk_total=0) — in that case the caller must NOT block (there is no way to
// judge). An offline Node / one without heartbeat also falls into known=false.
func ProjectedFree(ctx context.Context, pool *pgxpool.Pool, nodeID string) (freeBytes int64, known bool, err error) {
	err = pool.QueryRow(ctx,
		`SELECT `+projectedFreeExpr+`::bigint, TRUE
		   FROM nodes n JOIN heartbeats h ON h.node_id = n.id
		  WHERE n.id = $1 AND n.deleted_at IS NULL AND h.disk_total_bytes > 0`,
		nodeID).Scan(&freeBytes, &known)
	if err == pgx.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return freeBytes, known, nil
}

// Fits reports whether a torrent of needBytes fits on the Node while preserving the minFree margin.
// needBytes<=0 (unknown size) → fits=true, known=false: do NOT block without
// knowing the size. Node without disk data → known=false (same). Only returns
// (fits=false, known=true) when there is certainty that it does NOT fit.
func Fits(ctx context.Context, pool *pgxpool.Pool, nodeID string, needBytes, minFree int64) (fits, known bool, freeBytes int64, err error) {
	if needBytes <= 0 {
		return true, false, 0, nil
	}
	free, ok, err := ProjectedFree(ctx, pool, nodeID)
	if err != nil {
		return false, false, 0, err
	}
	if !ok {
		return true, false, 0, nil
	}
	return free >= needBytes+minFree, true, free, nil
}

// ProjectedFreeExpr exposes the SQL expression (aliases n/h) for anyone needing to embed it
// in a larger query — Scheduler (ranking/filtering by Node) and Alert Worker (scanning
// all Nodes). Keeps the single formula.
func ProjectedFreeExpr() string { return projectedFreeExpr }

// HumanBytes formats bytes into a readable binary unit ("3.5 GiB") for the messages that
// explain a space decision — the disk alert and the Scheduler refusal. Accepts
// negative (deficit) without a sign: the surrounding text already provides context for "missing".
func HumanBytes(b int64) string {
	if b < 0 {
		b = -b
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
