// Package clusterstats periodically samples the cluster's ACCUMULATED throughput (F2): the
// sum of the cumulative download/upload totals of all torrents ever managed, recorded
// in cluster_stat_samples for the evolution charts on the Metrics page.
//
// Why a dedicated table and not torrent_stat_samples or Prometheus: torrent_stat_samples
// is per-torrent, stores SESSION bytes and has short retention (7d); Prometheus retains 30d and
// does not export download/upload bytes. Neither covers 6 months / 1 year. Here it's 1 row per
// interval for the entire cluster, with long retention (the table is tiny) — which makes
// long period filters viable. ALL torrents are summed (including those removed by
// soft-delete) to keep the curve monotonic.
package clusterstats

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// Worker records periodic snapshots of the accumulated cluster throughput.
type Worker struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
	cfg    *settings.Store
}

// NewWorker creates the worker. The tunables (interval/retention) come from the Store (ADR-023) and are
// read per pass → hot-reload.
func NewWorker(pool *pgxpool.Pool, logger *slog.Logger, cfg *settings.Store) *Worker {
	return &Worker{pool: pool, logger: logger, cfg: cfg}
}

func (w *Worker) interval() time.Duration {
	return w.cfg.Duration("metrics.cluster_sample_interval")
}
func (w *Worker) retention() time.Duration {
	return w.cfg.Duration("metrics.cluster_sample_retention")
}

// Run samples every interval until the context is canceled. beat (variadic) marks each
// cycle for the health API (see internal/health), following the same pattern as the other workers.
func (w *Worker) Run(ctx context.Context, beat ...func()) {
	interval := w.interval()
	w.logger.Info("cluster statistics worker started", "interval", interval.String())
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	mark := func() {
		for _, b := range beat {
			if b != nil {
				b()
			}
		}
	}
	w.Sample(ctx) // first immediate sample
	mark()
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("cluster statistics worker stopped")
			return
		case <-ticker.C:
			w.Sample(ctx)
			mark()
			if cur := w.interval(); cur != interval {
				interval = cur
				ticker.Reset(cur)
			}
		}
	}
}

// Sample records a snapshot of the accumulated cluster throughput and purges expired ones. Failures
// are logged, not propagated (controlled degradation). The sum includes removed torrents
// (without filtering by deleted_at) on purpose: it keeps the curve monotonic.
func (w *Worker) Sample(ctx context.Context) {
	if _, err := w.pool.Exec(ctx,
		`INSERT INTO cluster_stat_samples (downloaded_total, uploaded_total)
		 SELECT COALESCE(SUM(downloaded_total), 0), COALESCE(SUM(uploaded_total), 0) FROM torrents`); err != nil {
		w.logger.Error("record Cluster throughput sample", "error", err)
		return
	}
	if ret := w.retention(); ret > 0 { // 0 = keep forever (tiny table)
		if _, err := w.pool.Exec(ctx,
			`DELETE FROM cluster_stat_samples WHERE sampled_at < now() - make_interval(secs => $1)`,
			ret.Seconds()); err != nil {
			w.logger.Error("purge Cluster throughput samples", "error", err)
		}
	}
}

// Point is a point in the cluster's aggregated series: instant (epoch s) + cumulative totals.
type Point struct {
	T    int64
	Down int64
	Up   int64
}

// Throughput reads the cluster's accumulated throughput series in the rng window, with DOWNSAMPLING
// per bucket (date_bin) to fit in a lightweight payload even in 1-year windows: the bucket is
// chosen to render ~at most `maxPoints` points, never smaller than the sampling
// interval. MAX per bucket because the total is monotonic (the last in the bucket ≈ the maximum).
func Throughput(ctx context.Context, pool *pgxpool.Pool, rng time.Duration) ([]Point, error) {
	const maxPoints = 400
	bucket := rng / maxPoints
	if bucket < time.Minute {
		bucket = time.Minute
	}
	rows, err := pool.Query(ctx,
		`SELECT extract(epoch from date_bin(make_interval(secs => $1), sampled_at, to_timestamp(0)))::bigint AS t,
		        MAX(downloaded_total), MAX(uploaded_total)
		   FROM cluster_stat_samples
		  WHERE sampled_at >= now() - make_interval(secs => $2)
		  GROUP BY 1
		  ORDER BY 1`,
		bucket.Seconds(), rng.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Point, 0)
	for rows.Next() {
		var p Point
		if err := rows.Scan(&p.T, &p.Down, &p.Up); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
