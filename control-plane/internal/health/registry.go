// Package health maintains the heartbeat of internal Control Plane Workers. Each
// loop marks its last execution and the health API exposes this to the Portal, giving the
// operator confidence that the loops are alive (not just "existing"). The state is
// in memory — the semantics are "the loop ran", no need to survive a restart
// of the CP (after restart, each Worker marks again on the first cycle).
package health

import (
	"sort"
	"sync"
	"time"
)

// staleFactor: a Worker is considered late (unhealthy) if it hasn't run for more
// than N nominal intervals. Absorbs jitter/latency of a cycle without false alarm.
const staleFactor = 3

// Registry tracks the last execution of each Worker. Safe for concurrent use.
type Registry struct {
	mu sync.Mutex
	m  map[string]*workerState
}

type workerState struct {
	interval time.Duration
	lastRun  time.Time // zero = hasn't run yet in this process lifetime
}

// WorkerReport is the snapshot of a Worker for the health API.
type WorkerReport struct {
	Name     string     `json:"name"`
	LastRun  *time.Time `json:"last_run"`         // nil = hasn't run yet
	Interval int        `json:"interval_seconds"` // nominal loop period
	Healthy  bool       `json:"healthy"`          // ran within the tolerance window
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{m: make(map[string]*workerState)}
}

// Register announces a Worker (it starts appearing in health even before the 1st
// execution) and returns the heartbeat callback that the loop must call every cycle.
// interval is the nominal loop period, base of the tolerance window.
func (r *Registry) Register(name string, interval time.Duration) func() {
	r.mu.Lock()
	r.m[name] = &workerState{interval: interval}
	r.mu.Unlock()
	return func() { r.mark(name) }
}

func (r *Registry) mark(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if st, ok := r.m[name]; ok {
		st.lastRun = time.Now()
	}
}

// Snapshot returns the current state of all Workers, sorted by name.
func (r *Registry) Snapshot() []WorkerReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	out := make([]WorkerReport, 0, len(r.m))
	for name, st := range r.m {
		rep := WorkerReport{Name: name, Interval: int(st.interval.Seconds())}
		if !st.lastRun.IsZero() {
			lr := st.lastRun
			rep.LastRun = &lr
			tol := st.interval * staleFactor
			if tol <= 0 {
				tol = time.Minute // stores for unconfigured intervals
			}
			rep.Healthy = now.Sub(lr) <= tol
		}
		out = append(out, rep)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
