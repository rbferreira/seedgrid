package settings

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store resolves the effective value of each tunable (database override → catalog
// default) and applies changes at runtime. Reads (getters used in the loops of the
// workers) are lock-free via an atomic pointer to the overrides map; the
// writes (Set/Reset) are serialized and swap the pointer (copy-on-write), so
// the change takes effect on the next iteration of any worker — hot-reload without redeploy.
type Store struct {
	pool      *pgxpool.Pool // nil for in-memory test Stores (Set/Reset unavailable)
	overrides atomic.Pointer[map[string]string]
	writeMu   sync.Mutex // serializes writes to avoid lost updates during copy-on-write
}

// New creates an empty Store (no overrides). Call Load to hydrate it from the database.
func New(pool *pgxpool.Pool) *Store {
	s := &Store{pool: pool}
	empty := map[string]string{}
	s.overrides.Store(&empty)
	return s
}

// NewForTest creates an in-memory Store with the given overrides (no database). Useful for
// testing workers without Postgres. Set/Reset return an error (no pool).
func NewForTest(overrides map[string]string) *Store {
	s := &Store{}
	cp := make(map[string]string, len(overrides))
	for k, v := range overrides {
		cp[k] = v
	}
	s.overrides.Store(&cp)
	return s
}

// Load reads the settings table into the cache and then seeds the database: for each
// spec without a row whose EnvVar is set and valid, it writes that value as
// initial seed (ADR-023 precedence: the database rules; env only seeds on 1st boot).
// Invalid envs are ignored (the catalog default remains).
func (s *Store) Load(ctx context.Context) error {
	if s.pool == nil {
		return fmt.Errorf("settings: Store without pool")
	}
	m, err := s.readAll(ctx)
	if err != nil {
		return err
	}
	for _, spec := range catalog {
		if _, ok := m[spec.Key]; ok || spec.EnvVar == "" {
			continue
		}
		v := os.Getenv(spec.EnvVar)
		if v == "" || spec.Validate(v) != nil {
			continue
		}
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO settings (key, value, updated_by) VALUES ($1, $2, 'env-seed')
			 ON CONFLICT (key) DO NOTHING`, spec.Key, v); err != nil {
			return fmt.Errorf("settings: seed %s: %w", spec.Key, err)
		}
		m[spec.Key] = v
	}
	s.overrides.Store(&m)
	return nil
}

func (s *Store) readAll(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, fmt.Errorf("settings: read overrides: %w", err)
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

// raw returns the effective value of key (database override → catalog default) and
// panics if the key does not exist in the catalog (programming error).
func (s *Store) raw(key string) string {
	spec, ok := specByKey[key]
	if !ok {
		panic("settings: unknown key: " + key)
	}
	if ov := *s.overrides.Load(); ov != nil {
		if v, ok := ov[key]; ok {
			return v
		}
	}
	return spec.Default
}

// Cluster returns the CLUSTER value of key — the database override or, if absent, the catalog
// default — and whether the operator defined it (`explicit`: row exists in `settings`). It is the
// "cluster value" step of the ADR-039 precedence; the Node override lives in
// node_settings and is resolved by whoever knows the Node (internal/agentconfig).
func (s *Store) Cluster(key string) (value string, explicit bool) {
	spec, ok := specByKey[key]
	if !ok {
		panic("settings: unknown key: " + key)
	}
	if ov := *s.overrides.Load(); ov != nil {
		if v, ok := ov[key]; ok && spec.Validate(v) == nil {
			return v, true
		}
	}
	return spec.Default, false
}

// Duration reads a duration tunable. A corrupted effective value falls back to the catalog
// default (which is guaranteed to be valid).
func (s *Store) Duration(key string) time.Duration {
	if d, err := time.ParseDuration(s.raw(key)); err == nil {
		return d
	}
	d, _ := time.ParseDuration(specByKey[key].Default)
	return d
}

// String reads a text tunable (KindString) — the raw value, without parsing.
func (s *Store) String(key string) string { return s.raw(key) }

// Bool reads a toggle tunable (KindBool). A corrupted value falls back to the default.
func (s *Store) Bool(key string) bool {
	if b, err := strconv.ParseBool(s.raw(key)); err == nil {
		return b
	}
	b, _ := strconv.ParseBool(specByKey[key].Default)
	return b
}

// Int reads an integer tunable.
func (s *Store) Int(key string) int { return int(s.Int64(key)) }

// Int64 reads a 64-bit integer tunable.
func (s *Store) Int64(key string) int64 {
	if n, err := strconv.ParseInt(s.raw(key), 10, 64); err == nil {
		return n
	}
	n, _ := strconv.ParseInt(specByKey[key].Default, 10, 64)
	return n
}

// Float reads a floating-point tunable.
func (s *Store) Float(key string) float64 {
	if f, err := strconv.ParseFloat(s.raw(key), 64); err == nil {
		return f
	}
	f, _ := strconv.ParseFloat(specByKey[key].Default, 64)
	return f
}

// Set validates and writes an override to the database, updating the cache (hot-reload).
func (s *Store) Set(ctx context.Context, key, value, updatedBy string) error {
	spec, ok := specByKey[key]
	if !ok {
		return fmt.Errorf("unknown key: %s", key)
	}
	if err := spec.Validate(value); err != nil {
		return err
	}
	if s.pool == nil {
		return fmt.Errorf("settings: Store without pool")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO settings (key, value, updated_at, updated_by) VALUES ($1, $2, now(), $3)
		 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = now(), updated_by = $3`,
		key, value, updatedBy); err != nil {
		return fmt.Errorf("settings: write %s: %w", key, err)
	}
	next := s.clone()
	next[key] = value
	s.overrides.Store(&next)
	return nil
}

// Reset removes the override for key, reverting to the catalog default (hot-reload).
func (s *Store) Reset(ctx context.Context, key, updatedBy string) error {
	if _, ok := specByKey[key]; !ok {
		return fmt.Errorf("unknown key: %s", key)
	}
	if s.pool == nil {
		return fmt.Errorf("settings: Store without pool")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.pool.Exec(ctx, `DELETE FROM settings WHERE key = $1`, key); err != nil {
		return fmt.Errorf("settings: reset %s: %w", key, err)
	}
	next := s.clone()
	delete(next, key)
	s.overrides.Store(&next)
	return nil
}

// clone copies the current overrides map (for copy-on-write under writeMu).
func (s *Store) clone() map[string]string {
	cur := *s.overrides.Load()
	cp := make(map[string]string, len(cur)+1)
	for k, v := range cur {
		cp[k] = v
	}
	return cp
}

// Effective is the effective value of a tunable + its source, for the dashboard.
type Effective struct {
	Spec
	Value  string // effective value (override or default)
	Source string // "db" (database override) or "default" (catalog)
}

// Effective returns the current state of all tunables, in catalog order.
func (s *Store) Effective() []Effective {
	ov := *s.overrides.Load()
	out := make([]Effective, 0, len(catalog))
	for _, spec := range catalog {
		e := Effective{Spec: spec, Value: spec.Default, Source: "default"}
		if v, ok := ov[spec.Key]; ok {
			e.Value, e.Source = v, "db"
		}
		out = append(out, e)
	}
	return out
}
