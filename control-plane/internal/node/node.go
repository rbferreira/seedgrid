// Package node implements the Control Plane Node Service: registration, heartbeat,
// Agent authentication, and Seed Node querying.
package node

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// ErrUnknownAgent indicates an invalid or untrusted Agent token.
var ErrUnknownAgent = errors.New("unknown or untrusted Agent")

// Node removal errors.
var (
	ErrNodeNotFound    = errors.New("node not found")
	ErrNodeHasTorrents = errors.New("Node still has torrents; migrate/remove before or use force")
	// ErrNodeRemoved (ADR-044): the machine of a removed Node registering again with the shared
	// registration token — an Agent retrying on its own. The removal sticks; a join token brings it back.
	ErrNodeRemoved = errors.New("this Node was removed from the Cluster")
)

// Node is the representation of a Seed Node exposed by the API.
type Node struct {
	ID           string  `json:"id"`
	UUID         string  `json:"uuid"`
	Hostname     string  `json:"hostname"`
	DisplayName  *string `json:"display_name,omitempty"`
	Status       string  `json:"status"`
	HealthScore  *int    `json:"health_score,omitempty"`
	AgentVersion *string `json:"agent_version,omitempty"`
	// OperationMode is the per-Node "sg"|"manual" mode (ADR-024). It only takes effect under
	// the global Flex gate; in Full auto the controller ignores it and treats every Node as SG.
	OperationMode string `json:"operation_mode"`
	// EngineAdopted: the Node joined with the qBittorrent it already ran (ADR-043). Its Engine
	// is the operator's: SeedGrid does not write its preferences or restart it.
	EngineAdopted bool       `json:"engine_adopted"`
	Engine        *string    `json:"engine,omitempty"`
	LastSeenAt    *time.Time `json:"last_seen_at,omitempty"`
	RegisteredAt  *time.Time `json:"registered_at,omitempty"`
	// Basic inventory (OS/kernel/architecture) collected during registration.
	OSName             *string    `json:"os_name,omitempty"`
	OSVersion          *string    `json:"os_version,omitempty"`
	KernelVersion      *string    `json:"kernel_version,omitempty"`
	Architecture       *string    `json:"architecture,omitempty"`
	InventoryUpdatedAt *time.Time `json:"inventory_updated_at,omitempty"`
	// Current resources (snapshot of the last heartbeat).
	DiskTotalBytes *int64   `json:"disk_total_bytes,omitempty"`
	DiskFreeBytes  *int64   `json:"disk_free_bytes,omitempty"`
	MemTotalBytes  *int64   `json:"mem_total_bytes,omitempty"`
	MemAvailBytes  *int64   `json:"mem_avail_bytes,omitempty"`
	Load1          *float64 `json:"load1,omitempty"`
	CPUCores       *int     `json:"cpu_cores,omitempty"`
	// Storage disk I/O utilization (busy%, last heartbeat) — saturation.
	DiskIOUtilPct *float64 `json:"disk_io_util_pct,omitempty"`
	// CPU utilization (%, last heartbeat). Measured via /proc/stat, not derived from
	// load — load counts I/O wait and would lie precisely on a Node with a saturated disk.
	CPUUtilPct *float64 `json:"cpu_util_pct,omitempty"`
	// Storage health (/data) at the last heartbeat: true=mounted+writable, false=failure,
	// nil (omitted)=unknown. Basis for the NODE_STORAGE alert and the Portal badge.
	StorageOK     *bool   `json:"storage_ok,omitempty"`
	StorageDetail *string `json:"storage_detail,omitempty"`
	// BitTorrent endpoint for the operator to configure port forwarding.
	BTPort *int    `json:"bt_port,omitempty"`
	LastIP *string `json:"last_ip,omitempty"`
	// INBOUND Connectability (ADR-034). PublicIPv6 is the address to open in the
	// firewall — under CGNAT it's the only inbound path. EngineConnectable is the
	// Engine's verdict ("connected" = has received an outside connection; "firewalled" =
	// never has); nil/omitted = unknown, never "closed". ConnectableSince marks
	// how long the current verdict has held — it's what distinguishes "just came up" from
	// "has been unreachable for hours".
	PublicIPv6        *string    `json:"public_ipv6,omitempty"`
	EngineConnectable *string    `json:"engine_connectable,omitempty"`
	ConnectableSince  *time.Time `json:"connectable_since,omitempty"`
	// Status for troubleshooting (last heartbeat): uptime of the service (Agent),
	// the host, and the Torrent Engine (container) + whether it's reachable.
	UptimeSeconds     *int64     `json:"uptime_seconds,omitempty"`
	HostUptimeSeconds *int64     `json:"host_uptime_seconds,omitempty"`
	EngineReachable   *bool      `json:"engine_reachable,omitempty"`
	EngineStartedAt   *time.Time `json:"engine_started_at,omitempty"`
	EngineVersion     *string    `json:"engine_version,omitempty"`
}

// RegisterInput is the data sent by the Agent during registration.
type RegisterInput struct {
	Hostname      string `json:"hostname"`
	DisplayName   string `json:"display_name"`
	OSName        string `json:"os_name"`
	OSVersion     string `json:"os_version"`
	KernelVersion string `json:"kernel_version"`
	Architecture  string `json:"architecture"`
	AgentVersion  string `json:"agent_version"`
	Engine        string `json:"engine"`
	EngineVersion string `json:"engine_version"`
	// EngineAdopted: the Agent runs against the qBittorrent the machine already had (ADR-043).
	// A Node that becomes adopted starts in Manual.
	EngineAdopted bool `json:"engine_adopted"`
	// MachineID and PreviousNodeID identify the machine (ADR-044): /etc/machine-id, and the Node id
	// of the Agent's identity file when it still has one. They tell a removed Node coming back from
	// a different machine with the same hostname.
	MachineID      string `json:"machine_id"`
	PreviousNodeID string `json:"node_id"`
	// BTPort is the port assigned by the CP via join token (ADR-017); populated by
	// the handler from the enroll token, never from the Agent's JSON.
	BTPort *int `json:"-"`
	// ViaJoinToken: the registration carries a join token, which only the operator issues
	// (ADR-044). Set by the handler.
	ViaJoinToken bool `json:"-"`
}

// RegisterResult is returned to the Agent after registration. The token is displayed only once.
type RegisterResult struct {
	NodeID     string `json:"node_id"`
	NodeUUID   string `json:"node_uuid"`
	AgentToken string `json:"agent_token"`
}

// HeartbeatInput is the summarized data sent periodically by the Agent.
type HeartbeatInput struct {
	Status         string  `json:"status"`
	Hostname       string  `json:"hostname"`
	AgentVersion   string  `json:"agent_version"`
	EngineVersion  string  `json:"engine_version"`
	UptimeSeconds  int64   `json:"uptime_seconds"`
	DiskTotalBytes uint64  `json:"disk_total_bytes"`
	DiskFreeBytes  uint64  `json:"disk_free_bytes"`
	MemTotalBytes  uint64  `json:"mem_total_bytes"`
	MemAvailBytes  uint64  `json:"mem_avail_bytes"`
	Load1          float64 `json:"load1"`
	CPUCores       int     `json:"cpu_cores"`
	DiskIOUtilPct  float64 `json:"disk_io_util_pct"` // Storage disk busy% (saturation signal)
	CPUUtilPct     float64 `json:"cpu_util_pct"`     // CPU utilization (%), measured via /proc/stat (#151)
	// Storage health (/data): true = mounted AND writable. Pointer to distinguish
	// "unreported" (old agent → nil → unknown, no alert) from false (real failure).
	StorageOK     *bool  `json:"storage_ok"`
	StorageDetail string `json:"storage_detail"` // reason when not-ok (e.g.: "not a mountpoint")
	// Inbound Connectability (ADR-034): Node's global IPv6 address and the Engine's
	// verdict on whether it has received an outside connection. Both "" when the Agent doesn't report
	// (old version) — "" is UNKNOWN, never "firewalled".
	PublicIPv6        string `json:"public_ipv6"`
	EngineConnectable string `json:"engine_connectable"`
	// IncomingPeers: INBOUND connections open right now (#155). Unlike
	// EngineConnectable (sticky), this signal DECAYS. nil = unreported =
	// unknown: does not zero the history or alert.
	IncomingPeers *int `json:"incoming_peers"`
	BTPort        int  `json:"bt_port"`
	// Status for troubleshooting: host uptime, engine reachability, and container start time.
	HostUptimeSeconds int64  `json:"host_uptime_seconds"`
	EngineReachable   bool   `json:"engine_reachable"`
	EngineStartedAt   string `json:"engine_started_at"` // RFC3339, best-effort ("" if unknown)
	// Behavior configuration coming from the Control Plane (ADR-039, #144). NEGOTIATION
	// fields: the Agent only sends them after the CP has proven, in a previous response, that
	// it delivers `agent_config` — because this body is decoded with DisallowUnknownFields and
	// an unknown field on an old CP would turn into a 400 (Node OFFLINE). They are not read by
	// Heartbeat(): the consumer is internal/agentconfig, called by the handler.
	//
	// ConfigVersion is the `version` of the agent_config that the Agent successfully applied; "" = not
	// reported. LocalConfig is the Node's explicit behavior env (seeding, §7);
	// nil = Agent didn't report, {} = reported and there was nothing.
	ConfigVersion string            `json:"config_version"`
	LocalConfig   map[string]string `json:"local_config"`
	// SourceIP is populated by the handler from the request's source IP
	// (does not come from the Agent's JSON).
	SourceIP string `json:"-"`
}

// Service centralizes operations on Seed Nodes.
type Service struct {
	pool             *pgxpool.Pool
	events           *events.Emitter
	defaultClusterID string
	cfg              *settings.Store // nil = uses the compiled default (see storageGrace)
}

// NewService creates the Node Service.
func NewService(pool *pgxpool.Pool, emitter *events.Emitter) *Service {
	return &Service{pool: pool, events: emitter}
}

// SetSettings turns on the tunables catalog (ADR-023). Optional and nil-safe, following the same
// pattern as activity.Recorder.SetNotifier — so callers that don't need
// tunables (integration tests for other packages) keep building the Service with
// two arguments.
func (s *Service) SetSettings(cfg *settings.Store) { s.cfg = cfg }

// defaultStorageGrace mirrors `alerting.storage_grace` from the catalog, which is the source of
// truth. Only applies when no one has called SetSettings.
const defaultStorageGrace = time.Minute

// storageGrace: how long /data needs to be failing to count as a real
// failure. The Health Score uses the SAME grace period as the NODE_STORAGE alert (#177) — it wouldn't make
// sense for the dashboard to turn red over a hiccup that the alert has already learned to ignore.
func (s *Service) storageGrace() time.Duration {
	if s.cfg == nil {
		return defaultStorageGrace
	}
	return s.cfg.Duration("alerting.storage_grace")
}

// EnsureDefaultCluster ensures a default Cluster and memorizes its id. Called on boot.
func (s *Service) EnsureDefaultCluster(ctx context.Context) error {
	var id string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text FROM clusters WHERE name = 'default' AND deleted_at IS NULL`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := s.pool.QueryRow(ctx,
			`INSERT INTO clusters (name, description) VALUES ('default', 'SeedGrid default cluster')
			 RETURNING id::text`).Scan(&id); err != nil {
			return fmt.Errorf("create default cluster: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("query default cluster: %w", err)
	}
	s.defaultClusterID = id
	return nil
}

// Register creates (or updates, by hostname) a Seed Node, issues the Agent
// credential, and logs events. In V1 the Node is automatically trusted upon registration.
func (s *Service) Register(ctx context.Context, in RegisterInput) (RegisterResult, error) {
	if in.Hostname == "" {
		return RegisterResult{}, errors.New("hostname is required")
	}
	if s.defaultClusterID == "" {
		return RegisterResult{}, errors.New("default cluster not initialized")
	}

	// The nickname (#145) comes from the join token; registrationName checks it again because the
	// world changes between issuing and using the token. An unusable name doesn't abort the registration.
	displayName, nameDropped := s.registrationName(ctx, in.Hostname, in.DisplayName)

	// A removed Node of this machine comes back as itself (ADR-044) before the upsert finds it live.
	returned, err := s.returnRemoved(ctx, in)
	if err != nil {
		return RegisterResult{}, err
	}

	var nodeID, nodeUUID string
	// wasAdopted is the row's engine_adopted BEFORE this registration (false for a new Node):
	// the CTE reads the snapshot taken before the INSERT. A Node that becomes adopted here —
	// new, or reinstalled with the Engine it already had — starts in Manual (ADR-043).
	var wasAdopted bool
	upsert := func(name string) error {
		return s.pool.QueryRow(ctx,
			`WITH prev AS (SELECT engine_adopted FROM nodes WHERE cluster_id = $1 AND hostname = $2 AND deleted_at IS NULL)
			 INSERT INTO nodes (cluster_id, hostname, display_name, os_name, os_version,
			                    kernel_version, architecture, agent_version, engine, engine_version,
			                    bt_port, engine_adopted, machine_id, status, registered_at, last_seen_at)
			 VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''),
			         NULLIF($8,''), NULLIF($9,''), NULLIF($10,''), $11, $12, NULLIF($13,''), 'ONLINE', now(), now())
			 ON CONFLICT (cluster_id, hostname) WHERE deleted_at IS NULL DO UPDATE SET
			    display_name   = COALESCE(NULLIF(EXCLUDED.display_name,''), nodes.display_name),
			    os_name        = COALESCE(NULLIF(EXCLUDED.os_name,''), nodes.os_name),
			    os_version     = COALESCE(NULLIF(EXCLUDED.os_version,''), nodes.os_version),
			    kernel_version = COALESCE(NULLIF(EXCLUDED.kernel_version,''), nodes.kernel_version),
			    architecture   = COALESCE(NULLIF(EXCLUDED.architecture,''), nodes.architecture),
			    agent_version  = COALESCE(NULLIF(EXCLUDED.agent_version,''), nodes.agent_version),
			    engine         = COALESCE(NULLIF(EXCLUDED.engine,''), nodes.engine),
			    engine_version = COALESCE(NULLIF(EXCLUDED.engine_version,''), nodes.engine_version),
			    bt_port        = COALESCE(EXCLUDED.bt_port, nodes.bt_port),
			    engine_adopted = EXCLUDED.engine_adopted,
			    machine_id     = COALESCE(EXCLUDED.machine_id, nodes.machine_id),
			    status         = 'ONLINE',
			    last_seen_at   = now(),
			    updated_at     = now()
			 RETURNING id::text, uuid::text, COALESCE((SELECT engine_adopted FROM prev), false)`,
			s.defaultClusterID, in.Hostname, name, in.OSName, in.OSVersion,
			in.KernelVersion, in.Architecture, in.AgentVersion, in.Engine, in.EngineVersion, in.BTPort,
			in.EngineAdopted, in.MachineID,
		).Scan(&nodeID, &nodeUUID, &wasAdopted)
	}
	err = upsert(displayName)
	if err != nil && displayName != "" && isDisplayNameViolation(err) {
		// Race condition: another Node took the nickname between the check and the INSERT. The Node joins
		// without a nickname (the operator renames it later) instead of failing the registration.
		displayName, nameDropped = "", true
		err = upsert("")
	}
	if err != nil {
		return RegisterResult{}, fmt.Errorf("registrar node: %w", err)
	}

	token, err := generateToken()
	if err != nil {
		return RegisterResult{}, err
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO node_credentials (node_id, token_sha256, state) VALUES ($1, $2, 'TRUSTED')`,
		nodeID, sha256hex(token)); err != nil {
		return RegisterResult{}, fmt.Errorf("issue credential: %w", err)
	}

	if s.events != nil {
		msg := fmt.Sprintf("Seed Node %q registered", in.Hostname)
		payload := map[string]any{"hostname": in.Hostname, "agent_version": in.AgentVersion, "name_dropped": nameDropped}
		if displayName != "" {
			payload["display_name"] = displayName
		}
		if nameDropped {
			// The Node joined, but the requested name couldn't be applied (already in use, or
			// invalid): without this note the operator would only find out by looking at the list.
			msg += " — requested alias is already in use or invalid and was not applied; rename via Portal"
		}
		_ = s.events.Emit(ctx, "NODE_REGISTERED", "INFO", nodeID, msg, payload)
		if returned != nil {
			_ = s.events.Emit(ctx, "NODE_RETURNED", "INFO", nodeID,
				fmt.Sprintf("Seed Node %q came back as itself: its history, mode and settings are kept, %d torrent(s) restored",
					in.Hostname, returned.torrents),
				map[string]any{"hostname": in.Hostname, "torrents_restored": returned.torrents})
		}
	}

	if in.EngineAdopted && !wasAdopted {
		if err := s.startAdoptedInManual(ctx, nodeID, in.Hostname); err != nil {
			return RegisterResult{}, err
		}
	}

	return RegisterResult{NodeID: nodeID, NodeUUID: nodeUUID, AgentToken: token}, nil
}

// returnedNode is a removed Node brought back by returnRemoved.
type returnedNode struct {
	id       string
	torrents int64
}

// returnRemoved brings back a removed Node when the machine registering is the same (ADR-044): a
// removed row with this hostname whose machine_id is the one sent, or whose id is the Node id of
// the Agent's identity file. Nothing happens when a live Node has the hostname (the upsert updates
// it, as always), when no removed row matches (a different machine joins as a new Node), or when
// it matches through the shared token (ErrNodeRemoved: the removal sticks against an Agent
// retrying on its own). The Node comes back with its id, history, mode and overrides, and the
// torrents removed with it — Delete stamps them in the Node's transaction; the 10-second window
// also catches Nodes removed before it did — unless the hash is live again on it. A display name another Node took meanwhile is dropped.
func (s *Service) returnRemoved(ctx context.Context, in RegisterInput) (*returnedNode, error) {
	if in.MachineID == "" && in.PreviousNodeID == "" {
		return nil, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("return removed node: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	var deletedAt time.Time
	err = tx.QueryRow(ctx,
		`SELECT id::text, deleted_at FROM nodes
		  WHERE cluster_id = $1 AND hostname = $2 AND deleted_at IS NOT NULL
		    AND NOT EXISTS (SELECT 1 FROM nodes l WHERE l.cluster_id = $1 AND l.hostname = $2 AND l.deleted_at IS NULL)
		    AND ((machine_id IS NOT NULL AND machine_id = NULLIF($3,'')) OR id::text = NULLIF($4,''))
		  ORDER BY deleted_at DESC LIMIT 1
		  FOR UPDATE`,
		s.defaultClusterID, in.Hostname, in.MachineID, in.PreviousNodeID).Scan(&id, &deletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("return removed node: %w", err)
	}
	if !in.ViaJoinToken {
		return nil, ErrNodeRemoved
	}
	if _, err := tx.Exec(ctx,
		`UPDATE nodes n SET deleted_at = NULL, updated_at = now(),
		        display_name = CASE WHEN EXISTS (
		            SELECT 1 FROM nodes o WHERE o.cluster_id = n.cluster_id AND o.deleted_at IS NULL
		               AND lower(o.display_name) = lower(n.display_name)) THEN NULL ELSE n.display_name END
		  WHERE n.id = $1`, id); err != nil {
		return nil, fmt.Errorf("return removed node: %w", err)
	}
	tag, err := tx.Exec(ctx,
		`UPDATE torrents t SET deleted_at = NULL, updated_at = now()
		  WHERE t.node_id = $1 AND t.deleted_at BETWEEN $2::timestamptz - interval '10 seconds' AND $2
		    AND NOT EXISTS (SELECT 1 FROM torrents l WHERE l.node_id = t.node_id AND l.hash = t.hash AND l.deleted_at IS NULL)`,
		id, deletedAt)
	if err != nil {
		return nil, fmt.Errorf("return removed node: restore torrents: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("return removed node: %w", err)
	}
	return &returnedNode{id: id, torrents: tag.RowsAffected()}, nil
}

// startAdoptedInManual puts a Node that joined with the operator's own Engine in Manual
// (ADR-043): its torrents show in the Cluster as they are, and the admission controller does
// not pause, start or rotate any of them until the operator moves the Node to SG. Manual only
// exists under the Flex gate (ADR-024), so a Cluster in Full auto moves to Flex; the other
// Nodes stay SG and behave as before. The gate row is locked so a concurrent SetGate cannot
// interleave between reading the gate and setting the Node.
func (s *Service) startAdoptedInManual(ctx context.Context, nodeID, hostname string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("adopted node mode: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var gate string
	if err := tx.QueryRow(ctx,
		`SELECT operation_mode FROM clusters WHERE id = $1 FOR UPDATE`, s.defaultClusterID).Scan(&gate); err != nil {
		return fmt.Errorf("adopted node mode: read gate: %w", err)
	}
	gateMoved := gate == GateFullAuto
	if gateMoved {
		if _, err := tx.Exec(ctx,
			`UPDATE clusters SET operation_mode = $2, updated_at = now() WHERE id = $1`,
			s.defaultClusterID, GateFlex); err != nil {
			return fmt.Errorf("adopted node mode: move gate to flex: %w", err)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE nodes SET operation_mode = $2, updated_at = now() WHERE id = $1`,
		nodeID, NodeModeManual); err != nil {
		return fmt.Errorf("adopted node mode: set manual: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("adopted node mode: %w", err)
	}
	if s.events != nil {
		msg := fmt.Sprintf("Seed Node %q joined with the qBittorrent it already ran and starts in Manual: review its settings, then move it to SG; until then the admission controller leaves its torrents alone", hostname)
		if gateMoved {
			msg += " — the Cluster moved from Full auto to Flex for it; the other Nodes stay SG"
		}
		_ = s.events.Emit(ctx, "NODE_ADOPTED_MANUAL", "INFO", nodeID, msg,
			map[string]any{"hostname": hostname, "gate_moved": gateMoved})
	}
	return nil
}

// AuthenticateAgent resolves an Agent's token to the corresponding Node id.
func (s *Service) AuthenticateAgent(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", ErrUnknownAgent
	}
	var nodeID string
	err := s.pool.QueryRow(ctx,
		`SELECT node_id::text FROM node_credentials
		  WHERE token_sha256 = $1 AND state = 'TRUSTED' AND revoked_at IS NULL`,
		sha256hex(token)).Scan(&nodeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUnknownAgent
	}
	if err != nil {
		return "", err
	}
	// last_used_at with throttling (#114): each Agent authenticates 4+ times per cycle
	// (heartbeat + command poll + sync + backfill), 24/7 — per-request timestamp
	// is a hot write with no value. Window larger than the operator's session: the data only
	// serves for "credential still in use", not for fine-grained telemetry.
	_, _ = s.pool.Exec(ctx,
		`UPDATE node_credentials SET last_used_at = now()
		  WHERE token_sha256 = $1
		    AND (last_used_at IS NULL OR last_used_at < now() - interval '5 minutes')`, sha256hex(token))
	return nodeID, nil
}

// Heartbeat updates a Node's last contact, state, and health.
func (s *Service) Heartbeat(ctx context.Context, nodeID string, in HeartbeatInput) error {
	status := in.Status
	if status == "" {
		status = "ONLINE"
	}
	// How long this Node's /data had already been failing BEFORE this heartbeat. This is what
	// allows the Health Score to require sustained failure (#180) with the same grace period as the
	// alert (#177). Best-effort: errors here (new Node, null column) yield 0 = "started
	// now", which is the safe side — it doesn't zero the score by mistake.
	var failingFor time.Duration
	if in.StorageOK != nil && !*in.StorageOK {
		var since *time.Time
		if err := s.pool.QueryRow(ctx,
			`SELECT storage_failing_since FROM nodes WHERE id = $1`, nodeID).Scan(&since); err == nil && since != nil {
			failingFor = time.Since(*since)
		}
	}
	// The Control Plane calculates the Health Score from the reported resources.
	h := computeHealth(in, failingFor, s.storageGrace())

	if _, err := s.pool.Exec(ctx,
		`UPDATE nodes SET
		    last_seen_at        = now(),
		    status              = $2,
		    health_score        = $3,
		    agent_version       = COALESCE(NULLIF($4,''), agent_version),
		    engine_version      = COALESCE(NULLIF($5,''), engine_version),
		    bt_port             = COALESCE($6, bt_port),
		    last_ip             = COALESCE(NULLIF($7,''), last_ip),
		    host_uptime_seconds = $8,
		    engine_reachable    = $9,
		    engine_started_at   = COALESCE(NULLIF($10,'')::timestamptz, engine_started_at),
		    disk_io_util_pct    = $11,
		    cpu_util_pct        = $16,
		    -- storage_failing_since stamps since when /data has been failing, so the
		    -- Alert Worker can require a SUSTAINED failure instead of alerting on the first
		    -- I/O hiccup (#177). Same feature that connectable_since gives to
		    -- NODE_FIREWALLED. Assignments see the OLD row, so the test
		    -- below compares with the previous storage_ok, not with $12.
		    -- The cast is mandatory: $12 appears here only in IS NULL/IS TRUE, and Postgres
		    -- infers the parameter type by its FIRST use — without it, 42P08
		    -- ("could not determine data type of parameter $12").
		    storage_failing_since = CASE
		                              WHEN $12::boolean IS NULL THEN storage_failing_since
		                              WHEN $12::boolean IS TRUE THEN NULL
		                              WHEN storage_ok IS DISTINCT FROM FALSE THEN now()
		                              ELSE storage_failing_since
		                            END,
		    storage_ok          = $12,
		    storage_detail      = NULLIF($13,''),
		    -- Provider prefix rotation (#147): saves the PREVIOUS address when the
		    -- value changes from one non-empty to another non-empty. NULL -> address is the
		    -- first boot (discovery), not rotation, and should not alert.
		    previous_public_ipv6 = CASE
		                             WHEN NULLIF($14,'') IS NOT NULL
		                              AND public_ipv6 IS NOT NULL AND public_ipv6 <> ''
		                              AND public_ipv6 <> $14
		                             THEN public_ipv6
		                             ELSE previous_public_ipv6
		                           END,
		    public_ipv6_changed_at = CASE
		                               WHEN NULLIF($14,'') IS NOT NULL
		                                AND public_ipv6 IS NOT NULL AND public_ipv6 <> ''
		                                AND public_ipv6 <> $14
		                               THEN now()
		                               ELSE public_ipv6_changed_at
		                             END,
		    public_ipv6         = COALESCE(NULLIF($14,''), public_ipv6),
		    -- connectable_since only resets when the VERDICT changes: this is what allows
		    -- requiring "sustained firewalled" instead of alerting on the first cycle, since
		    -- every newly-spun-up Node is firewalled just because no one has hit the
		    -- port yet. Empty value (old Agent) does not clear the history.
		    connectable_since   = CASE
		                            WHEN NULLIF($15,'') IS NULL THEN connectable_since
		                            WHEN engine_connectable IS DISTINCT FROM $15 THEN now()
		                            ELSE connectable_since
		                          END,
		    engine_connectable  = COALESCE(NULLIF($15,''), engine_connectable),
		    -- Signal that DECAYS (#155), unlike engine_connectable just above.
		    -- Agent NULL = unknown: does not overwrite the count nor the timestamp.
		    incoming_peers      = COALESCE($17, incoming_peers),
		    -- last_incoming_at only ADVANCES, and only on an actual inbound connection.
		    -- It's the age of this timestamp the Alert Worker reads; count 0 just stops
		    -- renewing it, and the aging itself is what constitutes the alert.
		    last_incoming_at    = CASE
		                            WHEN $17 IS NOT NULL AND $17 > 0 THEN now()
		                            ELSE last_incoming_at
		                          END,
		    updated_at          = now()
		  WHERE id = $1`,
		nodeID, status, h.Overall, in.AgentVersion, in.EngineVersion,
		nzInt(in.BTPort), in.SourceIP,
		nzInt64(in.HostUptimeSeconds), in.EngineReachable, in.EngineStartedAt, in.DiskIOUtilPct,
		in.StorageOK, in.StorageDetail, in.PublicIPv6, in.EngineConnectable,
		in.CPUUtilPct, in.IncomingPeers); err != nil {
		return fmt.Errorf("update node: %w", err)
	}

	// Hostname can change (operator renames the Node) — reflects in the CP. Best-effort:
	// a collision with another Node's hostname (unique constraint) MUST NOT take down
	// the heartbeat, so we ignore this update's error.
	if in.Hostname != "" {
		_, _ = s.pool.Exec(ctx,
			`UPDATE nodes SET hostname = $2, updated_at = now() WHERE id = $1 AND hostname <> $2`,
			nodeID, in.Hostname)
	}

	if _, err := s.pool.Exec(ctx,
		`INSERT INTO heartbeats (node_id, received_at, agent_version, engine_version, status, uptime_seconds, health_score,
		                         disk_total_bytes, disk_free_bytes, mem_total_bytes, mem_avail_bytes, load1, cpu_cores)
		 VALUES ($1, now(), NULLIF($2,''), NULLIF($3,''), $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT (node_id) DO UPDATE SET
		    received_at = now(),
		    agent_version = EXCLUDED.agent_version,
		    engine_version = EXCLUDED.engine_version,
		    status = EXCLUDED.status,
		    uptime_seconds = EXCLUDED.uptime_seconds,
		    health_score = EXCLUDED.health_score,
		    disk_total_bytes = EXCLUDED.disk_total_bytes,
		    disk_free_bytes = EXCLUDED.disk_free_bytes,
		    mem_total_bytes = EXCLUDED.mem_total_bytes,
		    mem_avail_bytes = EXCLUDED.mem_avail_bytes,
		    load1 = EXCLUDED.load1,
		    cpu_cores = EXCLUDED.cpu_cores,
		    updated_at = now()`,
		nodeID, in.AgentVersion, in.EngineVersion, status, in.UptimeSeconds, h.Overall,
		nzUint(in.DiskTotalBytes), nzUint(in.DiskFreeBytes), nzUint(in.MemTotalBytes), nzUint(in.MemAvailBytes),
		in.Load1, nzInt(in.CPUCores)); err != nil {
		return fmt.Errorf("write heartbeat: %w", err)
	}

	if _, err := s.pool.Exec(ctx,
		`INSERT INTO health_scores (node_id, overall_score, memory_score, storage_score, calculated_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (node_id) DO UPDATE SET
		    overall_score = EXCLUDED.overall_score,
		    memory_score = EXCLUDED.memory_score,
		    storage_score = EXCLUDED.storage_score,
		    calculated_at = now()`,
		nodeID, h.Overall, h.Memory, h.Storage); err != nil {
		return fmt.Errorf("write health score: %w", err)
	}
	return nil
}

// nzUint/nzInt convert 0 to NULL (unreported resource) to avoid writing false zeros.
func nzUint(v uint64) any {
	if v == 0 {
		return nil
	}
	return int64(v) //nolint:gosec // G115: resource metric (RAM/disk bytes), never near math.MaxInt64
}

func nzInt(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func nzInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// List returns all non-removed Seed Nodes, sorted by the name the Portal displays
// (alias, or hostname when none) — the list only looks sorted if it follows the label.
func (s *Service) List(ctx context.Context) ([]Node, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT n.id::text, n.uuid::text, n.hostname, n.display_name, n.status,
		        n.health_score, n.agent_version, n.engine, n.last_seen_at, n.registered_at,
		        n.os_name, n.os_version, n.kernel_version, n.architecture, n.inventory_updated_at,
		        n.bt_port, n.last_ip, n.operation_mode, n.engine_adopted,
		        h.disk_total_bytes, h.disk_free_bytes, h.mem_total_bytes, h.mem_avail_bytes, h.load1, h.cpu_cores,
		        h.uptime_seconds, n.host_uptime_seconds, n.engine_reachable, n.engine_started_at, n.engine_version,
		        n.disk_io_util_pct, n.storage_ok, n.storage_detail,
		        n.public_ipv6, n.engine_connectable, n.connectable_since,
		        n.cpu_util_pct
		   FROM nodes n
		   LEFT JOIN heartbeats h ON h.node_id = n.id
		  WHERE n.deleted_at IS NULL
		  ORDER BY COALESCE(NULLIF(n.display_name, ''), n.hostname), n.hostname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nodes := make([]Node, 0)
	for rows.Next() {
		var n Node
		if err := rows.Scan(
			&n.ID, &n.UUID, &n.Hostname, &n.DisplayName, &n.Status,
			&n.HealthScore, &n.AgentVersion, &n.Engine, &n.LastSeenAt, &n.RegisteredAt,
			&n.OSName, &n.OSVersion, &n.KernelVersion, &n.Architecture, &n.InventoryUpdatedAt,
			&n.BTPort, &n.LastIP, &n.OperationMode, &n.EngineAdopted,
			&n.DiskTotalBytes, &n.DiskFreeBytes, &n.MemTotalBytes, &n.MemAvailBytes, &n.Load1, &n.CPUCores,
			&n.UptimeSeconds, &n.HostUptimeSeconds, &n.EngineReachable, &n.EngineStartedAt, &n.EngineVersion,
			&n.DiskIOUtilPct, &n.StorageOK, &n.StorageDetail,
			&n.PublicIPv6, &n.EngineConnectable, &n.ConnectableSince,
			&n.CPUUtilPct,
		); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// btPortBase is the start of the BitTorrent port range assigned to the Nodes, and
// btPortSlots is its size: AssignPort looks for the lowest free port in
// [btPortBase, btPortBase+btPortSlots).
const (
	btPortBase  = 6881
	btPortSlots = 1000
)

// IndexForPort returns the Node's index — its position in the BitTorrent port
// range (port 6881 → 1, 6882 → 2…) — and ok=false for a port outside of it.
//
// It's the source of the Node's deterministic IPv6 suffix, `::5eed:<index>` (ADR-038). The index
// inherits the port's uniqueness, which AssignPort guarantees among active Nodes and live
// reservations, and returns to the pool with it when the Node is removed — so the Node that
// replaces another inherits the same address, and the operator's firewall rules remain
// valid. This function is the only place that knows the rule; bootstrap only mirrors it
// when running without enroll (`--network-only`, from the Node's own port).
func IndexForPort(port int) (index int, ok bool) {
	if port < btPortBase || port >= btPortBase+btPortSlots {
		return 0, false
	}
	return port - btPortBase + 1, true
}

// promTarget is a file-based service discovery entry for Prometheus (ADR-017).
type promTarget struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

// WritePrometheusTargets regenerates the Prometheus targets file (file_sd) with
// the active Seed Nodes (one <ip>:9100 target per Node, label instance=hostname).
// Atomic write (tmp + rename). Self-registration in Prometheus (ADR-017).
func (s *Service) WritePrometheusTargets(ctx context.Context, path string) error {
	rows, err := s.pool.Query(ctx,
		`SELECT hostname, last_ip FROM nodes
		  WHERE deleted_at IS NULL AND last_ip IS NOT NULL AND last_ip <> ''
		  ORDER BY hostname`)
	if err != nil {
		return err
	}
	defer rows.Close()
	targets := make([]promTarget, 0)
	for rows.Next() {
		var hostname, ip string
		if err := rows.Scan(&hostname, &ip); err != nil {
			return err
		}
		targets = append(targets, promTarget{
			Targets: []string{ip + ":9100"},
			Labels:  map[string]string{"instance": hostname},
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(targets, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil { //nolint:gosec // G306: read by the Prometheus container via shared volume (different UID, needs to be readable by "other")
		return fmt.Errorf("write targets: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("publish targets: %w", err)
	}
	return nil
}

// RunPrometheusTargetsWriter keeps the targets file updated periodically
// (Prometheus reloads file_sd on its own). Terminates when the context is canceled.
// interval is a PROVIDER read every cycle (ADR-023): the tunable
// workers.prom_writer_interval can change in the panel and applies without redeploy.
// Optional beat (variadic to avoid breaking callers/tests): heartbeat marked every
// cycle, exposed in the health API (see internal/health).
func (s *Service) RunPrometheusTargetsWriter(ctx context.Context, path string, interval func() time.Duration, logger *slog.Logger, beat ...func()) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // G301: same reason as WriteFile below — shared volume with the Prometheus container
		logger.Error("create targets directory", "error", err)
	}
	cur := interval()
	if cur <= 0 {
		cur = 30 * time.Second
	}
	ticker := time.NewTicker(cur)
	defer ticker.Stop()
	mark := func() {
		for _, b := range beat {
			if b != nil {
				b()
			}
		}
	}
	write := func() {
		if err := s.WritePrometheusTargets(ctx, path); err != nil {
			logger.Error("write prometheus targets", "error", err)
		}
	}
	write() // immediate
	mark()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			write()
			mark()
			if next := interval(); next > 0 && next != cur {
				cur = next
				ticker.Reset(cur)
				logger.Info("prometheus targets writer interval updated", "interval", cur.String())
			}
		}
	}
}

// ErrInvalidEnrollToken indicates a non-existent, expired, or already consumed join token.
var ErrInvalidEnrollToken = errors.New("invalid, expired or already used join token")

// MintEnrollToken generates a one-time join token (ADR-017), stores only the hash, and returns
// the plaintext value a single time (for the operator to hand to the Node).
//
// displayName is the optional alias for the Node that will use the token (#145): it is stored in the
// token and applied on registration. Empty = no alias (the Node shows up by hostname).
// Invalid name returns ErrInvalidDisplayName; name already in use by a Node returns
// ErrDisplayNameTaken, so the operator knows immediately and not after the Node comes up.
//
// Two open tokens with the same name are allowed on purpose: reserving the name in the
// token would leave the operator stuck (closed the modal without copying the command and the same name
// would be refused for up to 1h). The dispute is resolved on registration — the first Node takes the
// alias, the other enters without it and the registration event warns.
func (s *Service) MintEnrollToken(ctx context.Context, ttl time.Duration, displayName string) (token string, expiresAt time.Time, err error) {
	name, err := NormalizeDisplayName(displayName)
	if err != nil {
		return "", time.Time{}, err
	}
	if name != "" {
		taken, err := s.nameTaken(ctx, name, "")
		if err != nil {
			return "", time.Time{}, err
		}
		if taken {
			return "", time.Time{}, ErrDisplayNameTaken
		}
	}
	token, err = generateToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt = time.Now().Add(ttl)
	if _, err = s.pool.Exec(ctx,
		`INSERT INTO enroll_tokens (token_sha256, expires_at, display_name) VALUES ($1, $2, NULLIF($3, ''))`,
		sha256hex(token), expiresAt, name); err != nil {
		return "", time.Time{}, fmt.Errorf("write enroll token: %w", err)
	}
	return token, expiresAt, nil
}

// AssignPort validates a join token and reserves (idempotent) the lowest free BT port from
// btPortBase, considering active Nodes and other live reservations. Ports from
// removed Nodes return to the pool (reuse). Returns the reserved port.
func (s *Service) AssignPort(ctx context.Context, token string) (int, error) {
	return s.AssignPortFor(ctx, token, Machine{})
}

// Machine identifies the machine behind an enroll (ADR-044): its hostname, /etc/machine-id, and
// the Node id of the Agent's identity file, when it has one. Empty fields match nothing.
type Machine struct {
	Hostname  string
	MachineID string
	NodeID    string
}

// AssignPortFor is AssignPort for a known machine: when it is a removed Node coming back (same
// hostname and the same machine_id or Node id, ADR-044), it gets its old port back if no live Node
// and no pending join token holds it — the operator's port forward still points at it.
func (s *Service) AssignPortFor(ctx context.Context, token string, m Machine) (int, error) {
	var id string
	var reserved *int
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, bt_port FROM enroll_tokens
		  WHERE token_sha256 = $1 AND used_at IS NULL AND expires_at > now()`,
		sha256hex(token)).Scan(&id, &reserved)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrInvalidEnrollToken
	}
	if err != nil {
		return 0, err
	}
	if reserved != nil {
		return *reserved, nil // already reserved — idempotent
	}
	var port int
	if m.Hostname != "" && (m.MachineID != "" || m.NodeID != "") {
		err := s.pool.QueryRow(ctx,
			`SELECT n.bt_port FROM nodes n
			  WHERE n.cluster_id = $1 AND n.hostname = $2 AND n.deleted_at IS NOT NULL AND n.bt_port IS NOT NULL
			    AND ((n.machine_id IS NOT NULL AND n.machine_id = NULLIF($3,'')) OR n.id::text = NULLIF($4,''))
			    AND NOT EXISTS (SELECT 1 FROM nodes l WHERE l.deleted_at IS NULL AND l.bt_port = n.bt_port)
			    AND NOT EXISTS (SELECT 1 FROM enroll_tokens e
			                     WHERE e.bt_port = n.bt_port AND e.used_at IS NULL AND e.expires_at > now())
			  ORDER BY n.deleted_at DESC LIMIT 1`,
			s.defaultClusterID, m.Hostname, m.MachineID, m.NodeID).Scan(&port)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("previous port: %w", err)
		}
	}
	if port != 0 {
		if _, err := s.pool.Exec(ctx, `UPDATE enroll_tokens SET bt_port = $2 WHERE id = $1`, id, port); err != nil {
			return 0, fmt.Errorf("reserve port: %w", err)
		}
		return port, nil
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT min(p) FROM generate_series($1::int, $1::int + $2::int - 1) AS p
		  WHERE p NOT IN (
		     SELECT bt_port FROM nodes WHERE deleted_at IS NULL AND bt_port IS NOT NULL
		     UNION
		     SELECT bt_port FROM enroll_tokens WHERE bt_port IS NOT NULL AND used_at IS NULL AND expires_at > now()
		  )`, btPortBase, btPortSlots).Scan(&port); err != nil {
		return 0, fmt.Errorf("calculate free port: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE enroll_tokens SET bt_port = $2 WHERE id = $1`, id, port); err != nil {
		return 0, fmt.Errorf("reserve port: %w", err)
	}
	return port, nil
}

// ErrInvalidPort: a requested BitTorrent port outside 1-65535.
var ErrInvalidPort = errors.New("invalid BitTorrent port")

// ReserveEnginePort validates a join token and reserves for the Node the port its Engine
// already listens on (ADR-043), instead of the next free one in the sequence: the operator's
// port forward and firewall rule point at it. The port may be outside the sequence (the Node
// then has no index, see IndexForPort). It may also be the port of another active Node, which
// is accepted: two Nodes behind the same IPv4 cannot share a port forward, but two on different
// networks can, and the Control Plane cannot tell. sharedWith names that Node ("" when none),
// so the bootstrap can warn. hostname is the joining machine, left out of that check: running
// the bootstrap again on a Node must not warn about itself.
func (s *Service) ReserveEnginePort(ctx context.Context, token string, port int, hostname string) (sharedWith string, err error) {
	if port < 1 || port > 65535 {
		return "", ErrInvalidPort
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE enroll_tokens SET bt_port = $2
		  WHERE token_sha256 = $1 AND used_at IS NULL AND expires_at > now()`,
		sha256hex(token), port)
	if err != nil {
		return "", fmt.Errorf("reserve port: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", ErrInvalidEnrollToken
	}
	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(NULLIF(display_name, ''), hostname) FROM nodes
		  WHERE deleted_at IS NULL AND bt_port = $1 AND hostname <> $2
		  ORDER BY registered_at LIMIT 1`, port, hostname).Scan(&sharedWith)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("check port in use: %w", err)
	}
	return sharedWith, nil
}

// EnrollTokenInfo is what a still-open join token carries for registration.
type EnrollTokenInfo struct {
	BTPort      *int   // port reserved on enroll (nil if not yet reserved)
	DisplayName string // alias chosen when issuing the token ("" = no alias)
}

// EnrollToken validates (without consuming) a join token and returns what it carries — the reserved
// port and the requested alias — or ok=false if the token is not valid/open.
func (s *Service) EnrollToken(ctx context.Context, token string) (info EnrollTokenInfo, ok bool) {
	var name *string
	err := s.pool.QueryRow(ctx,
		`SELECT bt_port, display_name FROM enroll_tokens
		  WHERE token_sha256 = $1 AND used_at IS NULL AND expires_at > now()`,
		sha256hex(token)).Scan(&info.BTPort, &name)
	if err != nil {
		return EnrollTokenInfo{}, false
	}
	if name != nil {
		info.DisplayName = *name
	}
	return info, true
}

// MarkEnrollTokenUsed consumes a join token (called after Node registration).
func (s *Service) MarkEnrollTokenUsed(ctx context.Context, token string) {
	_, _ = s.pool.Exec(ctx,
		`UPDATE enroll_tokens SET used_at = now() WHERE token_sha256 = $1 AND used_at IS NULL`,
		sha256hex(token))
}

// Delete removes a Seed Node from the Cluster (soft-delete), revokes the Agent credential
// (heartbeats stop being accepted), and emits an event. If the Node still has active
// torrents, it blocks (ErrNodeHasTorrents) unless force=true — in which case the
// torrents are also soft-deleted. The BT port returns to the pool (the port calculation
// only considers non-removed Nodes) and Prometheus stops scraping (the targets
// writer also filters deleted_at IS NULL).
func (s *Service) Delete(ctx context.Context, id string, force bool) error {
	var hostname string
	err := s.pool.QueryRow(ctx,
		`SELECT hostname FROM nodes WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&hostname)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNodeNotFound
	}
	if err != nil {
		return err
	}

	var torrents int
	if err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM torrents WHERE node_id = $1 AND deleted_at IS NULL`, id).Scan(&torrents); err != nil {
		return err
	}
	if torrents > 0 && !force {
		return ErrNodeHasTorrents
	}
	// One transaction, so the torrents and the Node share deleted_at (now() is the transaction's):
	// it is how a Node that comes back finds the torrents removed with it (ADR-044).
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("remove node: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if torrents > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE torrents SET deleted_at = now(), updated_at = now() WHERE node_id = $1 AND deleted_at IS NULL`, id); err != nil {
			return fmt.Errorf("remove torrents from node: %w", err)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE node_credentials SET revoked_at = now() WHERE node_id = $1 AND revoked_at IS NULL`, id); err != nil {
		return fmt.Errorf("revoke credential: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE nodes SET deleted_at = now(), updated_at = now() WHERE id = $1`, id); err != nil {
		return fmt.Errorf("remover node: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("remove node: %w", err)
	}
	if s.events != nil {
		// WARNING (not INFO): removing a Node from the Cluster is too serious to be blue —
		// seeding capacity disappears permanently, unlike a routine event.
		_ = s.events.Emit(ctx, "NODE_DELETED", "WARNING", id,
			fmt.Sprintf("Seed Node %q removed from Cluster", hostname),
			map[string]any{"hostname": hostname, "force": force, "torrents_removed": torrents})
	}
	return nil
}

// Label returns the Node's hostname (readable label for Audit/UI
// descriptions — same field the Portal already displays). "" when not found.
func (s *Service) Label(ctx context.Context, id string) string {
	var hostname string
	if err := s.pool.QueryRow(ctx,
		`SELECT hostname FROM nodes WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&hostname); err != nil {
		return ""
	}
	return hostname
}

// Operation modes (ADR-024).
const (
	GateFullAuto   = "full_auto" // global gate: SG controls everything (per-Node modes ignored)
	GateFlex       = "flex"      // global gate: per-Node modes apply
	NodeModeSG     = "sg"        // per-Node: system has priority (automatic admission)
	NodeModeManual = "manual"    // per-Node: sovereign operator (the controller does not touch)
)

// Operation mode errors.
var (
	ErrInvalidMode = errors.New("invalid operation mode")
	ErrGateLocked  = errors.New("per-Node mode can only be changed when the cluster is in Flex")
)

// Gate returns the global operation mode (gate) of the default Cluster (ADR-024).
func (s *Service) Gate(ctx context.Context) (string, error) {
	var mode string
	err := s.pool.QueryRow(ctx,
		`SELECT operation_mode FROM clusters WHERE name = 'default' AND deleted_at IS NULL`).Scan(&mode)
	if err != nil {
		return "", err
	}
	return mode, nil
}

// SetGate sets the global gate (full_auto|flex). When returning to full_auto, all
// Nodes are reset to 'sg' — "full-auto is full-auto": no Manual Node is left
// forgotten to reappear when the operator returns to Flex (ADR-024).
func (s *Service) SetGate(ctx context.Context, mode string) error {
	if mode != GateFullAuto && mode != GateFlex {
		return ErrInvalidMode
	}
	if s.defaultClusterID == "" {
		return errors.New("default cluster not initialized")
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE clusters SET operation_mode = $2, updated_at = now() WHERE id = $1`,
		s.defaultClusterID, mode); err != nil {
		return fmt.Errorf("update gate: %w", err)
	}
	if mode == GateFullAuto {
		if _, err := s.pool.Exec(ctx,
			`UPDATE nodes SET operation_mode = 'sg', updated_at = now()
			  WHERE cluster_id = $1 AND operation_mode <> 'sg' AND deleted_at IS NULL`,
			s.defaultClusterID); err != nil {
			return fmt.Errorf("reset node modes: %w", err)
		}
	}
	// Mode change is an operator action → logged only in Audit (ADR-025), not
	// in Events (domain facts). The admission Worker reads the mode from the DB on each pass.
	return nil
}

// SetNodeMode sets the per-Node mode (sg|manual). It is only allowed with the Cluster in Flex
// (in Full auto the per-Node toggles are locked — ADR-024).
func (s *Service) SetNodeMode(ctx context.Context, nodeID, mode string) error {
	if mode != NodeModeSG && mode != NodeModeManual {
		return ErrInvalidMode
	}
	gate, err := s.Gate(ctx)
	if err != nil {
		return fmt.Errorf("query gate: %w", err)
	}
	if gate != GateFlex {
		return ErrGateLocked
	}
	var hostname string
	err = s.pool.QueryRow(ctx,
		`UPDATE nodes SET operation_mode = $2, updated_at = now()
		  WHERE id = $1 AND deleted_at IS NULL RETURNING hostname`, nodeID, mode).Scan(&hostname)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNodeNotFound
	}
	if err != nil {
		return fmt.Errorf("update node mode: %w", err)
	}
	_ = hostname // RETURNING hostname detects non-existent Node (ErrNoRows); action goes to Audit (ADR-025), not Events.
	return nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
