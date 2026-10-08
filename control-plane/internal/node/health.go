package node

import (
	"math"
	"time"
)

// healthResult is the Health Score calculated with its dimensions (0-100).
type healthResult struct {
	Overall int
	Storage int
	Memory  int
}

// computeHealth derives the Health Score from resources reported by the Agent. The
// policy lives in the Control Plane (ADR-003: Agent stateless). Deterministic.
//
// Storage: 20% or more free → 100; drops linearly to 0 when the disk fills up.
// Memory:  30% or more available → 100; same.
// Overall: higher weight on storage (it's a seedbox). With no resource data (old
// Agent/no info) assumes 100, maintaining compatibility.
func computeHealth(in HeartbeatInput, failingFor, grace time.Duration) healthResult {
	storage := 100
	if in.DiskTotalBytes > 0 {
		freePct := float64(in.DiskFreeBytes) / float64(in.DiskTotalBytes) * 100
		storage = clampScore(freePct * 5) // 20% -> 100
	}
	// Unmounted/unwritable Storage (HD died, mount stuck on dead device) is the worst
	// scenario for a seedbox: zeroes the dimension (→ Overall plummets). nil = unknown
	// (old agent) → does not penalize.
	//
	// It only zeroes when the failure is SUSTAINED (#180), with the same grace as the
	// NODE_STORAGE alert (#177): no point turning the panel red over an I/O hiccup
	// that the alert learned to ignore. `failingFor` is the age of storage_failing_since
	// BEFORE this heartbeat — on the first bad read it's 0, and the Node stays green.
	//
	// Unlike the Scheduler, which keeps excluding on the first read: there is protection
	// to preserve (not dispatching work to a failing disk); here it is an indicator.
	if in.StorageOK != nil && !*in.StorageOK && failingFor >= grace {
		storage = 0
	}
	memory := 100
	if in.MemTotalBytes > 0 {
		availPct := float64(in.MemAvailBytes) / float64(in.MemTotalBytes) * 100
		memory = clampScore(availPct * (100.0 / 30.0)) // 30% -> 100
	}
	overall := int(math.Round(0.6*float64(storage) + 0.4*float64(memory)))
	return healthResult{Overall: overall, Storage: storage, Memory: memory}
}

func clampScore(v float64) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return int(math.Round(v))
}
