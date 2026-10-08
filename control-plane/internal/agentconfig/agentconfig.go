// Package agentconfig resolves and delivers the behavior configuration of the Agent and the
// Engine of each Node (ADR-039, #144).
//
// The tunables catalog (internal/settings, ADR-023) provides the CLUSTER value of the keys
// marked PerNode; this package adds the PER NODE override (node_settings table) and produces the
// EFFECTIVE value that the Agent receives in the heartbeat response:
//
//	Node override > cluster value > (Node env) > (code default)
//
// The last two steps are the Agent's — they only apply when the Control Plane sends nothing.
// Here live the first two, the set's version (hash), what the Agent says it has applied,
// and the one-time seeding of overrides from the env of Nodes that already existed.
package agentconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/settings"
)

// Origin of the effective value of a key.
const (
	SourceNode    = "node"    // override for this Node
	SourceCluster = "cluster" // cluster value (database override or catalog default)
)

// State of a Node regarding today's effective configuration.
const (
	StatusInSync      = "in_sync"     // the Agent has applied the current effective version
	StatusPending     = "pending"     // it changed and the Agent hasn't confirmed yet (will apply on next heartbeat)
	StatusUnsupported = "unsupported" // the Agent has never reported: pre-dates ADR-039, remains in env
)

// SeededBy is the updated_by of the overrides created by seeding (ADR-039 §7).
const SeededBy = "seeding (Node env)"

// Package errors.
var (
	ErrNodeNotFound = errors.New("node not found")
	ErrUnknownKey   = errors.New("unknown tunable")
	ErrNotPerNode   = errors.New("this tunable does not accept per-Node values")
)

// InvalidValueError is the catalog Spec rejecting a value (type, range, option). The
// message is from the Spec itself, already readable by the operator.
type InvalidValueError struct{ Err error }

func (e *InvalidValueError) Error() string { return e.Err.Error() }
func (e *InvalidValueError) Unwrap() error { return e.Err }

// Service resolves and persists the configuration per Node.
type Service struct {
	pool   *pgxpool.Pool
	store  *settings.Store
	events *events.Emitter // nil = seeding does not emit Event
}

// New creates the service. store is the tunables catalog (cluster value).
func New(pool *pgxpool.Pool, store *settings.Store) *Service {
	return &Service{pool: pool, store: store}
}

// SetEvents connects the NODE_CONFIG_SEEDED Event from seeding. Optional and nil-safe, in the same
// pattern as node.Service.SetSettings.
func (s *Service) SetEvents(e *events.Emitter) { s.events = e }

// Item is a resolved PerNode key for a Node.
type Item struct {
	Spec            settings.Spec
	Value           string  // effective value
	Source          string  // SourceNode ou SourceCluster
	ClusterValue    string  // cluster value (what the Node would have without override)
	ClusterExplicit bool    // the operator defined the cluster value (there is a row in `settings`)
	Override        *string // the Node's override, when there is one
}

// NodeState is the configuration state of a Node: what is effective, where it comes from, and if the Agent
// is up to date.
type NodeState struct {
	NodeID           string
	Hostname         string
	DisplayName      *string
	Items            []Item
	EffectiveVersion string
	AppliedVersion   *string
	AppliedAt        *time.Time
	SeededAt         *time.Time
}

// Label is the name the operator knows (nickname, otherwise hostname).
func (n NodeState) Label() string {
	if n.DisplayName != nil && *n.DisplayName != "" {
		return *n.DisplayName
	}
	return n.Hostname
}

// Status compares the version the Agent applied with today's effective one.
func (n NodeState) Status() string {
	switch {
	case n.AppliedVersion == nil || *n.AppliedVersion == "":
		return StatusUnsupported
	case *n.AppliedVersion == n.EffectiveVersion:
		return StatusInSync
	default:
		return StatusPending
	}
}

// Values is the key→effective value map (what goes to the Agent).
func (n NodeState) Values() map[string]string { return valuesOf(n.Items) }

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// effective resolves the PerNode keys of a Node: the override (when it exists and is still
// valid per the Spec — a limit the catalog tightened later cannot crash the Agent)
// over the cluster value. Pure: does not touch the database.
func effective(store *settings.Store, overrides map[string]string) []Item {
	specs := settings.NodeSpecs()
	items := make([]Item, 0, len(specs))
	for _, spec := range specs {
		cv, explicit := store.Cluster(spec.Key)
		it := Item{Spec: spec, Value: cv, Source: SourceCluster, ClusterValue: cv, ClusterExplicit: explicit}
		if ov, ok := overrides[spec.Key]; ok && spec.Validate(ov) == nil {
			v := ov
			it.Value, it.Source, it.Override = ov, SourceNode, &v
		}
		items = append(items, it)
	}
	return items
}

func valuesOf(items []Item) map[string]string {
	m := make(map[string]string, len(items))
	for _, it := range items {
		m[it.Spec.Key] = it.Value
	}
	return m
}

// Version is the version of a set of effective values: deterministic hash (12 hex) of
// ordered "key=value". The Agent returns the same text in the heartbeat after applying —
// it doesn't need to know how to calculate it.
func Version(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s=%s\n", k, values[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// load reads the Node and its overrides in a single database call.
func (s *Service) load(ctx context.Context, nodeID string) (NodeState, map[string]string, error) {
	var st NodeState
	if !uuidRe.MatchString(nodeID) {
		return st, nil, ErrNodeNotFound
	}
	rows, err := s.pool.Query(ctx,
		`SELECT n.hostname, n.display_name, n.agent_config_version, n.agent_config_applied_at,
		        n.agent_config_seeded_at, ns.key, ns.value
		   FROM nodes n
		   LEFT JOIN node_settings ns ON ns.node_id = n.id
		  WHERE n.id = $1::uuid AND n.deleted_at IS NULL`, nodeID)
	if err != nil {
		return st, nil, fmt.Errorf("read node configuration: %w", err)
	}
	defer rows.Close()
	overrides := map[string]string{}
	found := false
	for rows.Next() {
		var key, value *string
		if err := rows.Scan(&st.Hostname, &st.DisplayName, &st.AppliedVersion, &st.AppliedAt,
			&st.SeededAt, &key, &value); err != nil {
			return st, nil, err
		}
		found = true
		if key != nil && value != nil {
			overrides[*key] = *value
		}
	}
	if err := rows.Err(); err != nil {
		return st, nil, err
	}
	if !found {
		return st, nil, ErrNodeNotFound
	}
	st.NodeID = nodeID
	return st, overrides, nil
}

// Node returns the configuration state of a Node (Portal: "Settings of this Node").
func (s *Service) Node(ctx context.Context, nodeID string) (NodeState, error) {
	st, overrides, err := s.load(ctx, nodeID)
	if err != nil {
		return st, err
	}
	st.Items = effective(s.store, overrides)
	st.EffectiveVersion = Version(valuesOf(st.Items))
	return st, nil
}

// Delivery is the `agent_config` block of the heartbeat response (ADR-039 §4).
type Delivery struct {
	// Version identifies the set of Values; the Agent returns it in `config_version`
	// after applying everything.
	Version string `json:"version"`
	// Values are the EFFECTIVE values of this Node (precedence already resolved), per catalog
	// key, serialized as in the catalog ("10s", "true", "3").
	Values map[string]string `json:"values"`
	// ReportLocal asks the Agent to report its explicit env (`local_config`) on the next
	// heartbeat and NOT apply anything before: seeding hasn't happened yet (ADR-039 §7).
	ReportLocal bool `json:"report_local,omitempty"`
}

// Report is what the Agent brings in the heartbeat (additive fields — see ADR-039 §6).
type Report struct {
	// Version is the `version` the Agent successfully applied; "" = not reported.
	Version string
	// Local is the Node's explicit behavior env (catalog key → value); nil =
	// the Agent didn't report (only reports when the CP asks), {} = reported and there was nothing.
	Local map[string]string
}

// Exchange is what the Control Plane does on every heartbeat of a new Agent: records what it
// applied, seeds the overrides the first time the Agent reports the env, and returns what it
// should apply. Any error here MUST NOT crash the heartbeat — the caller records and
// responds without the block (the Agent then keeps what it already has).
func (s *Service) Exchange(ctx context.Context, nodeID string, rep Report) (Delivery, error) {
	if rep.Version != "" {
		if err := s.recordApplied(ctx, nodeID, rep.Version); err != nil {
			return Delivery{}, err
		}
	}
	if rep.Local != nil {
		if _, err := s.Seed(ctx, nodeID, rep.Local); err != nil {
			return Delivery{}, err
		}
	}
	st, err := s.Node(ctx, nodeID)
	if err != nil {
		return Delivery{}, err
	}
	return Delivery{
		Version:     st.EffectiveVersion,
		Values:      st.Values(),
		ReportLocal: st.SeededAt == nil,
	}, nil
}

// recordApplied stores the version the Agent says it has applied. It only writes when it changes — the
// heartbeat repeats the same version every cycle and the `applied_at` column marks WHEN it was
// applied, not when it was repeated.
func (s *Service) recordApplied(ctx context.Context, nodeID, version string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE nodes SET agent_config_version = $2, agent_config_applied_at = now()
		  WHERE id = $1::uuid AND agent_config_version IS DISTINCT FROM $2`, nodeID, version)
	if err != nil {
		return fmt.Errorf("record applied config version: %w", err)
	}
	return nil
}

// Change describes an override change for Audit (ADR-025): the before and after
// are all the trail keeps, because the old value no longer exists anywhere.
type Change struct {
	NodeLabel string
	Spec      settings.Spec
	Before    *string // previous override (nil = there was none: the cluster one applied)
	After     *string // new override (nil = removed)
	// Effective is the value the Node will have after the change.
	Effective string
}

// SetOverride persists the override of key for the Node, validated by the catalog Spec.
func (s *Service) SetOverride(ctx context.Context, nodeID, key, value, actor string) (Change, error) {
	spec, err := perNodeSpec(key)
	if err != nil {
		return Change{}, err
	}
	if err := spec.Validate(value); err != nil {
		return Change{}, &InvalidValueError{Err: err}
	}
	st, overrides, err := s.load(ctx, nodeID)
	if err != nil {
		return Change{}, err
	}
	c := Change{NodeLabel: st.Label(), Spec: spec, After: &value, Effective: value}
	if prev, ok := overrides[key]; ok {
		c.Before = &prev
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO node_settings (node_id, key, value, updated_by)
		 VALUES ($1::uuid, $2, $3, $4)
		 ON CONFLICT (node_id, key) DO UPDATE
		    SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by`,
		nodeID, key, value, actor); err != nil {
		return Change{}, fmt.Errorf("write node override: %w", err)
	}
	return c, nil
}

// ClearOverride removes the override of key: the Node reverts to the cluster value.
func (s *Service) ClearOverride(ctx context.Context, nodeID, key string) (Change, error) {
	spec, err := perNodeSpec(key)
	if err != nil {
		return Change{}, err
	}
	st, overrides, err := s.load(ctx, nodeID)
	if err != nil {
		return Change{}, err
	}
	cv, _ := s.store.Cluster(key)
	c := Change{NodeLabel: st.Label(), Spec: spec, Effective: cv}
	if prev, ok := overrides[key]; ok {
		c.Before = &prev
	}
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM node_settings WHERE node_id = $1::uuid AND key = $2`, nodeID, key); err != nil {
		return Change{}, fmt.Errorf("remove node override: %w", err)
	}
	return c, nil
}

func perNodeSpec(key string) (settings.Spec, error) {
	spec, ok := settings.Lookup(key)
	if !ok {
		return spec, ErrUnknownKey
	}
	if !spec.PerNode {
		return spec, ErrNotPerNode
	}
	return spec, nil
}

// OverrideRow is a Node's override, for the Settings screen list.
type OverrideRow struct {
	NodeID       string
	Hostname     string
	DisplayName  *string
	Key          string
	Value        string
	ClusterValue string
	UpdatedAt    time.Time
	UpdatedBy    *string
}

// Label is the name the operator knows for the Node (nickname, otherwise hostname).
func (r OverrideRow) Label() string {
	if r.DisplayName != nil && *r.DisplayName != "" {
		return *r.DisplayName
	}
	return r.Hostname
}

// Overrides lists all overrides of active Nodes — so the operator, when changing a cluster
// value, can see WHO won't be affected.
func (s *Service) Overrides(ctx context.Context) ([]OverrideRow, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT n.id::text, n.hostname, n.display_name, ns.key, ns.value, ns.updated_at, ns.updated_by
		   FROM node_settings ns
		   JOIN nodes n ON n.id = ns.node_id
		  WHERE n.deleted_at IS NULL
		  ORDER BY n.hostname, ns.key`)
	if err != nil {
		return nil, fmt.Errorf("list overrides: %w", err)
	}
	defer rows.Close()
	out := make([]OverrideRow, 0)
	for rows.Next() {
		var r OverrideRow
		if err := rows.Scan(&r.NodeID, &r.Hostname, &r.DisplayName, &r.Key, &r.Value, &r.UpdatedAt, &r.UpdatedBy); err != nil {
			return nil, err
		}
		if _, ok := settings.Lookup(r.Key); !ok {
			continue // key that left the catalog: there is nothing to show
		}
		r.ClusterValue, _ = s.store.Cluster(r.Key)
		out = append(out, r)
	}
	return out, rows.Err()
}

// SeedResult is the outcome of a seeding attempt.
type SeedResult struct {
	// AlreadySeeded: the Node had already gone through seeding; nothing was read or written.
	AlreadySeeded bool
	// Seeded are the keys that became overrides, with the value.
	Seeded map[string]string
	// Skipped are the reported keys that DID NOT become overrides, with the reason.
	Skipped map[string]string
}

// maxLocalKeys limits the Agent's report: there are ~12 PerNode keys, so a report with over a
// hundred of them is not a healthy Agent. Without the cap, a giant body would turn into a giant loop.
const maxLocalKeys = 128

// ErrReportTooLarge: the env report has too many keys to be legitimate.
var ErrReportTooLarge = errors.New("local configuration report too large")

// Seed transforms the explicit env that the Agent reported into Node overrides — ONCE per
// Node (ADR-039 §7). Key K only becomes an override if ALL conditions hold: the Node has not yet
// been seeded; K is a PerNode key; the value is valid per the Spec; there is no K override
// for this Node; the cluster value of K was not explicitly set by the operator; and the
// reported value differs from the cluster one. After the attempt — even with no keys — the Node
// is marked as seeded and never touched again, even if the operator deletes what was
// seeded: seeding is migration, not synchronization.
func (s *Service) Seed(ctx context.Context, nodeID string, local map[string]string) (SeedResult, error) {
	res := SeedResult{Seeded: map[string]string{}, Skipped: map[string]string{}}
	if !uuidRe.MatchString(nodeID) {
		return res, ErrNodeNotFound
	}
	if len(local) > maxLocalKeys {
		return res, fmt.Errorf("%w: %d keys (maximum %d)", ErrReportTooLarge, len(local), maxLocalKeys)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("seeding: open transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// FOR UPDATE: two concurrent heartbeats from the same Agent do not seed twice.
	var seededAt *time.Time
	var hostname string
	var displayName *string
	err = tx.QueryRow(ctx,
		`SELECT agent_config_seeded_at, hostname, display_name FROM nodes
		  WHERE id = $1::uuid AND deleted_at IS NULL FOR UPDATE`, nodeID).Scan(&seededAt, &hostname, &displayName)
	if errors.Is(err, pgx.ErrNoRows) {
		return res, ErrNodeNotFound
	}
	if err != nil {
		return res, fmt.Errorf("seeding: read node: %w", err)
	}
	if seededAt != nil {
		res.AlreadySeeded = true
		return res, nil
	}

	keys := make([]string, 0, len(local))
	for k := range local {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := local[key]
		spec, ok := settings.Lookup(key)
		switch {
		case !ok || !spec.PerNode:
			res.Skipped[key] = "unknown key"
			continue
		case spec.Validate(value) != nil:
			res.Skipped[key] = "invalid value"
			continue
		}
		cv, explicit := s.store.Cluster(key)
		switch {
		case explicit:
			res.Skipped[key] = "operator set the cluster value"
			continue
		case spec.Same(value, cv):
			res.Skipped[key] = "same as cluster value"
			continue
		}
		tag, err := tx.Exec(ctx,
			`INSERT INTO node_settings (node_id, key, value, updated_by)
			 VALUES ($1::uuid, $2, $3, $4) ON CONFLICT (node_id, key) DO NOTHING`,
			nodeID, key, value, SeededBy)
		if err != nil {
			return res, fmt.Errorf("seeding: write %s: %w", key, err)
		}
		if tag.RowsAffected() == 0 {
			res.Skipped[key] = "Node already has override"
			continue
		}
		res.Seeded[key] = value
	}
	if _, err := tx.Exec(ctx,
		`UPDATE nodes SET agent_config_seeded_at = now() WHERE id = $1::uuid`, nodeID); err != nil {
		return res, fmt.Errorf("seeding: mark node: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("seeding: commit: %w", err)
	}

	// Event only AFTER the commit and only when there is something to report: it is a domain fact
	// initiated by the system (not an operator action → does not go to Audit, ADR-025).
	if s.events != nil && len(res.Seeded) > 0 {
		label := hostname
		if displayName != nil && *displayName != "" {
			label = *displayName
		}
		parts := make([]string, 0, len(res.Seeded))
		seededKeys := make([]string, 0, len(res.Seeded))
		for k := range res.Seeded {
			seededKeys = append(seededKeys, k)
		}
		sort.Strings(seededKeys)
		for _, k := range seededKeys {
			parts = append(parts, k+"="+res.Seeded[k])
		}
		_ = s.events.Emit(ctx, "NODE_CONFIG_SEEDED", "INFO", nodeID,
			fmt.Sprintf("Node %q: .env values became per-Node overrides (%s)", label, strings.Join(parts, ", ")),
			map[string]any{"hostname": hostname, "overrides": res.Seeded})
	}
	return res, nil
}
