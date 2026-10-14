// Package metrics exposes Control Plane metrics in Prometheus format
// (Monitoring.md). A collector queries the Cluster state on each scrape, without
// duplicating state in memory.
package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	nodesDesc = prometheus.NewDesc(
		"seedgrid_nodes", "Number of Seed Nodes by status.", []string{"status"}, nil)
	torrentsDesc = prometheus.NewDesc(
		"seedgrid_torrents", "Number of torrents by status.", []string{"status"}, nil)
	alertsOpenDesc = prometheus.NewDesc(
		"seedgrid_alerts_open", "Number of open alerts (OPEN status).", nil, nil)
	storageOKDesc = prometheus.NewDesc(
		"seedgrid_node_storage_ok", "Node storage (/data) mounted and writable: 1=ok, 0=failed. Only emitted when known.", []string{"node"}, nil)
	admissionHoldDesc = prometheus.NewDesc(
		"seedgrid_admission_hold", "Number of torrents on hold by the download admission controller.", nil, nil)
	connectableDesc = prometheus.NewDesc(
		"seedgrid_node_connectable",
		"Node receives inbound BitTorrent connections: 1=yes, 0=firewalled. Only emitted when known (Agent reporting connection_status).",
		[]string{"node"}, nil)
)

// collector queries the database on each scrape and emits the Cluster metrics.
type collector struct {
	pool *pgxpool.Pool
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- nodesDesc
	ch <- torrentsDesc
	ch <- alertsOpenDesc
	ch <- storageOKDesc
	ch <- admissionHoldDesc
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c.collectGroup(ctx, ch, nodesDesc,
		`SELECT status, count(*) FROM nodes WHERE deleted_at IS NULL GROUP BY status`)
	c.collectGroup(ctx, ch, torrentsDesc,
		`SELECT status, count(*) FROM torrents WHERE deleted_at IS NULL GROUP BY status`)

	var open float64
	if err := c.pool.QueryRow(ctx, `SELECT count(*) FROM alerts WHERE status = 'OPEN'`).Scan(&open); err == nil {
		ch <- prometheus.MustNewConstMetric(alertsOpenDesc, prometheus.GaugeValue, open)
	}

	var held float64
	if err := c.pool.QueryRow(ctx,
		`SELECT count(*) FROM torrents WHERE admission_paused_at IS NOT NULL AND deleted_at IS NULL`).Scan(&held); err == nil {
		ch <- prometheus.MustNewConstMetric(admissionHoldDesc, prometheus.GaugeValue, held)
	}

	// Gauge per Node for storage (only when known; NULL = old agent → omitted).
	if srows, err := c.pool.Query(ctx,
		`SELECT hostname, storage_ok FROM nodes WHERE deleted_at IS NULL AND storage_ok IS NOT NULL`); err == nil {
		defer srows.Close()
		for srows.Next() {
			var host string
			var ok bool
			if err := srows.Scan(&host, &ok); err != nil {
				break
			}
			v := 0.0
			if ok {
				v = 1.0
			}
			ch <- prometheus.MustNewConstMetric(storageOKDesc, prometheus.GaugeValue, v, host)
		}
	}

	// Gauge per Node for inbound connectability (#141). Only when the Engine gave a
	// verdict: NULL (old Agent) is left out — absence of metric is "don't know",
	// and emitting 0 there would be asserting "firewalled" without basis.
	if crows, err := c.pool.Query(ctx,
		`SELECT hostname, engine_connectable FROM nodes
		  WHERE deleted_at IS NULL AND engine_connectable IS NOT NULL AND engine_connectable <> ''`); err == nil {
		defer crows.Close()
		for crows.Next() {
			var host, st string
			if err := crows.Scan(&host, &st); err != nil {
				break
			}
			v := 0.0
			if st == "connected" {
				v = 1.0
			}
			ch <- prometheus.MustNewConstMetric(connectableDesc, prometheus.GaugeValue, v, host)
		}
	}
}

// collectGroup emits a metric with a label per row of a query (status, count).
func (c *collector) collectGroup(ctx context.Context, ch chan<- prometheus.Metric, desc *prometheus.Desc, query string) {
	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return // scrape must not fail due to temporary database unavailability
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n float64
		if err := rows.Scan(&status, &n); err != nil {
			return
		}
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, n, status)
	}
}

// NewHandler creates the HTTP handler for /metrics with the Cluster metrics and the
// default Go runtime/process metrics.
//
// extra receives collectors from those who keep counters in MEMORY (not in the database) — the registry
// here is private, so whoever has process metrics must provide them. This is the case of
// notify, which counts what the per-minute cap discarded.
func NewHandler(pool *pgxpool.Pool, extra ...prometheus.Collector) http.Handler {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		&collector{pool: pool},
	)
	for _, c := range extra {
		reg.MustRegister(c)
	}
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}
