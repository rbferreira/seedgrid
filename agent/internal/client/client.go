// Package client is the Agent's HTTP client for the Control Plane API. All
// communication is initiated by the Agent (ADR-012) via HTTPS/HTTP.
package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrUnauthorized indicates rejected credential (e.g.: revoked Node) — requires new registration.
var ErrUnauthorized = errors.New("unauthorized")

// Client talks to the Control Plane.
type Client struct {
	baseURL string
	http    *http.Client
}

// New creates the client for the Control Plane base URL.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// RegisterRequest is the registration body.
type RegisterRequest struct {
	Hostname      string `json:"hostname"`
	DisplayName   string `json:"display_name,omitempty"`
	OSName        string `json:"os_name,omitempty"`
	OSVersion     string `json:"os_version,omitempty"`
	KernelVersion string `json:"kernel_version,omitempty"`
	Architecture  string `json:"architecture,omitempty"`
	AgentVersion  string `json:"agent_version,omitempty"`
	Engine        string `json:"engine,omitempty"`
	EngineVersion string `json:"engine_version,omitempty"`
}

// RegisterResponse is the registration response.
type RegisterResponse struct {
	NodeID     string `json:"node_id"`
	NodeUUID   string `json:"node_uuid"`
	AgentToken string `json:"agent_token"`
}

// HeartbeatRequest is the heartbeat body. The Agent reports raw resources; the
// Health Score is calculated by the Control Plane (ADR-003).
type HeartbeatRequest struct {
	Status         string  `json:"status"`
	Hostname       string  `json:"hostname,omitempty"`
	AgentVersion   string  `json:"agent_version,omitempty"`
	EngineVersion  string  `json:"engine_version,omitempty"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
	DiskTotalBytes uint64  `json:"disk_total_bytes,omitempty"`
	DiskFreeBytes  uint64  `json:"disk_free_bytes,omitempty"`
	MemTotalBytes  uint64  `json:"mem_total_bytes,omitempty"`
	MemAvailBytes  uint64  `json:"mem_avail_bytes,omitempty"`
	Load1          float64 `json:"load1,omitempty"`
	CPUCores       int     `json:"cpu_cores,omitempty"`
	// DiskIOUtilPct is the storage disk utilization (busy%) since the last
	// heartbeat — an I/O saturation signal (feeds the DISK_IO_SATURATED alert).
	DiskIOUtilPct float64 `json:"disk_io_util_pct,omitempty"`
	// CPUUtilPct is the CPU utilization (%) since the last heartbeat (#151). Measured via
	// /proc/stat, not derived from load: load counts I/O wait, busy CPU does not.
	CPUUtilPct float64 `json:"cpu_util_pct,omitempty"`
	BTPort     int     `json:"bt_port,omitempty"`
	// Status for troubleshooting without SSH: host uptime, engine reachable, and
	// Engine container start time (best-effort).
	HostUptimeSeconds int64  `json:"host_uptime_seconds,omitempty"`
	EngineReachable   bool   `json:"engine_reachable"`
	EngineStartedAt   string `json:"engine_started_at,omitempty"` // RFC3339
	// Torrent storage health (StoragePath): mounted and writable. Allows the
	// operator to know, without SSH, whether /data went down (e.g.: the Node's USB HD
	// disconnected and the mount got stuck on a dead device). StorageDetail is the reason
	// in PT-BR when StorageOK is false ("" when ok).
	StorageOK     bool   `json:"storage_ok"`
	StorageDetail string `json:"storage_detail"`
	// INBOUND Connectability (ADR-034). PublicIPv6 is the Node's global address —
	// what the operator needs to allow in the firewall, and whose CHANGE (provider
	// prefix rotation) currently goes unnoticed. EngineConnectable is the Engine's own
	// verdict: "connected" = has received an outside connection, "firewalled" = never has.
	// Both best-effort: "" when unavailable (does not become a false negative in the CP).
	PublicIPv6        string `json:"public_ipv6,omitempty"`
	EngineConnectable string `json:"engine_connectable,omitempty"`
	// IncomingPeers is the count of INBOUND connections currently open (#155). Pointer
	// because nil = not sampled (old Agent, or first sample not yet taken):
	// UNKNOWN, which the CP never reads as zero. 0 is a legitimate and different reading.
	IncomingPeers *int `json:"incoming_peers,omitempty"`

	// Behavior config negotiation (ADR-039). The heartbeat body is
	// decoded by the Control Plane with DisallowUnknownFields: a new field sent to an
	// old CP becomes a 400 and the Node goes OFFLINE. That's why these fields are only
	// populated AFTER the CP has proven, in a prior response, that it delivers `agent_config`
	// (the decision is remotecfg.Manager.Prepare's) — never by default.
	//
	// ConfigVersion is the `version` of the agent_config the Agent successfully applied.
	ConfigVersion string `json:"config_version,omitempty"`
	// LocalConfig is the explicit behavior env for this Node, reported when the CP asks
	// (`report_local`). Pointer on purpose: an EMPTY report (`{}`) needs to reach the CP — it
	// is what marks the seeding as done —, and `omitempty` on a map would omit the empty.
	LocalConfig *map[string]string `json:"local_config,omitempty"`
}

// AgentConfig is the configuration block the Control Plane returns in the heartbeat
// response (ADR-039): the EFFECTIVE values for this Node, with precedence already resolved.
type AgentConfig struct {
	// Version identifies the set of Values; the Agent returns it in ConfigVersion after
	// successfully applying everything.
	Version string `json:"version"`
	// Values are the per-key values from the catalog ("engine.max_active_downloads": "10").
	Values map[string]string `json:"values"`
	// ReportLocal asks the Agent to report the explicit env (LocalConfig) in the next heartbeat
	// and NOT apply anything before: the seeding of the overrides has not happened yet (ADR-039 §7).
	ReportLocal bool `json:"report_local"`
}

// HeartbeatResponse is the heartbeat response. AgentConfig is nil when the Control Plane
// predates ADR-039 (or failed to resolve): the Agent continues with the env.
type HeartbeatResponse struct {
	Status      string       `json:"status"`
	AgentConfig *AgentConfig `json:"agent_config,omitempty"`
}

// Register registers the Node using the registration token (dedicated header).
func (c *Client) Register(ctx context.Context, regToken string, in RegisterRequest) (RegisterResponse, error) {
	var out RegisterResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/agent/register", in, &out, func(r *http.Request) {
		r.Header.Set("X-Seedgrid-Register-Token", regToken)
	})
	return out, err
}

// Heartbeat sends a heartbeat authenticated by the Agent credential.
//
// The response body is read with tolerance: a CP that replies something the Agent doesn't
// understand (proxy returning HTML, new format) still counts as a SUCCESSFUL heartbeat — the
// Node is alive —, just without configuration. It has always been this way (the response
// wasn't even read); reading the configuration cannot turn this into a failure.
func (c *Client) Heartbeat(ctx context.Context, agentToken string, in HeartbeatRequest) (HeartbeatResponse, error) {
	var resp HeartbeatResponse
	b, err := c.roundTrip(ctx, http.MethodPost, "/api/v1/agent/heartbeat", in, bearer(agentToken))
	if err != nil {
		return resp, err
	}
	if jerr := json.Unmarshal(b, &resp); jerr != nil {
		return HeartbeatResponse{}, nil
	}
	return resp, nil
}

// Command is a command received from the Control Plane for local execution.
type Command struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// CommandResult is the outcome reported to the Control Plane.
type CommandResult struct {
	Status string          `json:"status"` // DONE ou FAILED
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Commands queries the queue of pending commands for this Node (polling).
func (c *Client) Commands(ctx context.Context, agentToken string) ([]Command, error) {
	var wrap struct {
		Data []Command `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/agent/commands", nil, &wrap, bearer(agentToken)); err != nil {
		return nil, err
	}
	return wrap.Data, nil
}

// SubmitResult confirms the outcome of a command.
func (c *Client) SubmitResult(ctx context.Context, agentToken, commandID string, res CommandResult) error {
	return c.do(ctx, http.MethodPost, "/api/v1/agent/commands/"+commandID+"/result", res, nil, bearer(agentToken))
}

// TorrentReport is the state of a torrent reported to the Control Plane
// (status already mapped to the official SeedGrid state).
//
// num_seeds/num_leechs = SWARM (tracker scrape); num_seeds_connected/
// num_leechs_connected = actually CONNECTED peers (ADR-021 refinement — the CP uses
// the connected ones to mark a torrent as dead). Additive fields: a CP predating them
// simply ignores them on decode.
type TorrentReport struct {
	Hash               string  `json:"hash"`
	Name               string  `json:"name"`
	Status             string  `json:"status"`
	Progress           float64 `json:"progress"`
	NumSeeds           int     `json:"num_seeds"`
	NumLeechs          int     `json:"num_leechs"`
	NumSeedsConnected  int     `json:"num_seeds_connected"`
	NumLeechsConnected int     `json:"num_leechs_connected"`
	DlSpeed            int64   `json:"dl_speed"`
	UpSpeed            int64   `json:"up_speed"`
	Size               int64   `json:"size"`
	Ratio              float64 `json:"ratio"`
	ETA                int64   `json:"eta"`
	Downloaded         int64   `json:"downloaded"`
	Uploaded           int64   `json:"uploaded"`
	// Tracker verdict on announce (#160). Sampled on its own cadence, much slower
	// than the sync: a torrent deleted on the tracker remains SEEDING and green in the
	// Portal, and nothing in the product measured that. 0 = not yet sampled.
	TrackerStatus  int    `json:"tracker_status,omitempty"`
	TrackerMessage string `json:"tracker_message,omitempty"`
}

// ReportTorrents sends the current state of the Node's torrents to the Control Plane.
// dlSlotLimit, when non-nil and positive, reports the per-account DL slot limit
// that the Agent extracted from the tracker message (#21) — the CP uses it in the
// admission controller.
func (c *Client) ReportTorrents(ctx context.Context, agentToken string, reports []TorrentReport, dlSlotLimit *int) error {
	body := map[string]any{"torrents": reports}
	if dlSlotLimit != nil && *dlSlotLimit > 0 {
		body["dl_slot_limit"] = *dlSlotLimit
	}
	return c.do(ctx, http.MethodPost, "/api/v1/agent/torrents", body, nil, bearer(agentToken))
}

// PendingTorrentFiles asks the CP which hashes for this Node still need the
// .torrent exported from the Engine (backfill, ADR-018).
func (c *Client) PendingTorrentFiles(ctx context.Context, agentToken string) ([]string, error) {
	var wrap struct {
		Data []string `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/agent/torrent-files/pending", nil, &wrap, bearer(agentToken)); err != nil {
		return nil, err
	}
	return wrap.Data, nil
}

// UploadTorrentFile sends the .torrent exported from the Engine (base64) to the CP.
func (c *Client) UploadTorrentFile(ctx context.Context, agentToken, hash string, data []byte) error {
	body := map[string]string{"hash": hash, "data": base64.StdEncoding.EncodeToString(data)}
	return c.do(ctx, http.MethodPost, "/api/v1/agent/torrent-files", body, nil, bearer(agentToken))
}

func bearer(token string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

func (c *Client) do(ctx context.Context, method, path string, body, out any, decorate func(*http.Request)) error {
	b, err := c.roundTrip(ctx, method, path, body, decorate)
	if err != nil {
		return err
	}
	if out != nil {
		// Decoder, not Unmarshal: reads the first JSON value and ignores whatever comes after, as
		// it always has.
		return json.NewDecoder(bytes.NewReader(b)).Decode(out)
	}
	return nil
}

// roundTrip makes the request, translates the status (401 → ErrUnauthorized; non-2xx → error with the
// start of the body) and returns the response body.
func (c *Client) roundTrip(ctx context.Context, method, path string, body any, decorate func(*http.Request)) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if decorate != nil {
		decorate(req)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}
