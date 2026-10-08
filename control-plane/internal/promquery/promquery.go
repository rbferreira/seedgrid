// Package promquery queries Prometheus (PromQL) on the Control Plane side and
// returns already normalized time series. The Portal never talks directly to
// Prometheus (ADR-005/ADR-016): the CP is the gateway.
package promquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrUnknownMetric indicates a metric key outside the curated list.
var ErrUnknownMetric = errors.New("unknown metric")

// curated maps safe keys exposed by the API to PromQL expressions. The client
// never accepts raw PromQL — only these keys (prevents injection and keeps the contract stable).
var curated = map[string]string{
	"host_cpu_pct":         `100 - (avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`,
	"host_mem_used_bytes":  `node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes`,
	"host_mem_total_bytes": `node_memory_MemTotal_bytes`,
	"host_fs_avail_bytes":  `node_filesystem_avail_bytes{mountpoint="/"}`,
	// Network rate (bytes/s) on the physical uplink. We exclude virtual interfaces
	// (lo/docker0/veth/br-*/virbr/tap/dummy) because container traffic already leaves
	// via the physical interface — summing them would count it twice.
	//
	// ⚠️ Previously this was `device="eth0"`, and the fixed name ERASED from the graph any Node whose
	// interface has another name: node-04 (WiFi `mlan0`, plus onboard `enp1s0`) vanished
	// entirely from the network series, with no error or empty series — it simply didn't exist.
	// `eth0` is the common name in containers, not a guarantee: bare-metal with systemd uses
	// predictable names (enp1s0/wlp2s0) and out-of-tree drivers invent their own (mlan0).
	// Excluding known virtual interfaces is robust; enumerating expected physical ones is not.
	"host_net_rx_bytes": `sum by (instance) (rate(node_network_receive_bytes_total{device!~"lo|docker.*|veth.*|br-.*|virbr.*|tap.*|dummy.*"}[2m]))`,
	"host_net_tx_bytes": `sum by (instance) (rate(node_network_transmit_bytes_total{device!~"lo|docker.*|veth.*|br-.*|virbr.*|tap.*|dummy.*"}[2m]))`,
	// Disk I/O. read/written: throughput (bytes/s) summed per Node. util: the
	// FRACTION OF TIME the disk was busy (node_disk_io_time_seconds_total
	// is the "busy time"), in %, taking the Node's most saturated disk (max) — it is the signal
	// of SATURATION (pegging at ~100% = disk is the bottleneck; as with the Pi's USB HDD).
	// On a container Node it reflects the host's disks (shared); meaningful
	// on a bare-metal Node (Pi) — same caveat as node-exporter.
	"host_disk_read_bytes":  `sum by (instance) (rate(node_disk_read_bytes_total[2m]))`,
	"host_disk_write_bytes": `sum by (instance) (rate(node_disk_written_bytes_total[2m]))`,
	"host_disk_util_pct":    `max by (instance) (rate(node_disk_io_time_seconds_total[2m]) * 100)`,
	"nodes":                 `seedgrid_nodes`,
	"torrents":              `seedgrid_torrents`,
	"alerts_open":           `seedgrid_alerts_open`,
}

// Metrics returns the available metric keys (order not guaranteed).
func Metrics() []string {
	keys := make([]string, 0, len(curated))
	for k := range curated {
		keys = append(keys, k)
	}
	return keys
}

// Point is a time series data point (t in Unix seconds, v the value).
type Point struct {
	T float64 `json:"t"`
	V float64 `json:"v"`
}

// Series is a labeled series.
type Series struct {
	Labels map[string]string `json:"labels"`
	Points []Point           `json:"points"`
}

// Client talks to the Prometheus HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates the client for the Prometheus base URL (e.g., http://seedgrid-prometheus:9090).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// QueryMetric resolves a curated key and queries the [now-rng, now] window.
func (c *Client) QueryMetric(ctx context.Context, key string, rng time.Duration) ([]Series, error) {
	expr, ok := curated[key]
	if !ok {
		return nil, ErrUnknownMetric
	}
	if rng <= 0 {
		rng = 6 * time.Hour
	}
	step := rng / 120
	if step < 15*time.Second {
		step = 15 * time.Second
	}
	end := time.Now()
	return c.queryRange(ctx, expr, end.Add(-rng), end, step)
}

// queryRange calls /api/v1/query_range and normalizes the response (resultType matrix).
func (c *Client) queryRange(ctx context.Context, expr string, start, end time.Time, step time.Duration) ([]Series, error) {
	q := url.Values{}
	q.Set("query", expr)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", strconv.Itoa(int(step.Seconds())))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/query_range?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query prometheus: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("prometheus status %d", resp.StatusCode)
	}

	var raw struct {
		Status string `json:"status"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
				Values [][2]any          `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode prometheus response: %w", err)
	}
	if raw.Status != "success" {
		return nil, fmt.Errorf("prometheus returned status %q", raw.Status)
	}

	out := make([]Series, 0, len(raw.Data.Result))
	for _, r := range raw.Data.Result {
		s := Series{Labels: r.Metric, Points: make([]Point, 0, len(r.Values))}
		for _, v := range r.Values {
			ts, ok1 := v[0].(float64)
			valStr, ok2 := v[1].(string)
			if !ok1 || !ok2 {
				continue
			}
			f, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				continue // "NaN" etc. — ignores the point
			}
			s.Points = append(s.Points, Point{T: ts, V: f})
		}
		out = append(out, s)
	}
	return out, nil
}
