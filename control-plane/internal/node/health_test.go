package node

import (
	"testing"
	"time"
)

func boolPtr(b bool) *bool { return &b }

func TestComputeHealth(t *testing.T) {
	// failingFor: how long /data has already been failing BEFORE this heartbeat. Zero (the
	// table default) is the FIRST bad reading, which must not drop the score (#180).
	const grace = time.Minute
	cases := []struct {
		name                     string
		in                       HeartbeatInput
		failingFor               time.Duration
		overall, storage, memory int
	}{
		{
			name:    "no resource data assumes 100 (compat)",
			in:      HeartbeatInput{},
			overall: 100, storage: 100, memory: 100,
		},
		{
			name:    "ample disk and memory",
			in:      HeartbeatInput{DiskTotalBytes: 100, DiskFreeBytes: 50, MemTotalBytes: 100, MemAvailBytes: 80},
			overall: 100, storage: 100, memory: 100,
		},
		{
			name:    "disco 10% livre penaliza storage",
			in:      HeartbeatInput{DiskTotalBytes: 100, DiskFreeBytes: 10, MemTotalBytes: 100, MemAvailBytes: 60},
			overall: 70, storage: 50, memory: 100, // 0.6*50 + 0.4*100
		},
		{
			name:    "full disk zeroes storage",
			in:      HeartbeatInput{DiskTotalBytes: 100, DiskFreeBytes: 0, MemTotalBytes: 100, MemAvailBytes: 100},
			overall: 40, storage: 0, memory: 100, // 0.6*0 + 0.4*100
		},
		{
			name:       "sustained storage_ok=false zeros storage even with ample disk",
			in:         HeartbeatInput{DiskTotalBytes: 100, DiskFreeBytes: 50, MemTotalBytes: 100, MemAvailBytes: 80, StorageOK: boolPtr(false)},
			failingFor: 5 * time.Minute,
			overall:    40, storage: 0, memory: 100, // storage forced to 0 (HD crashed) → 0.6*0 + 0.4*100
		},
		{
			// The #177 false positive seen on the dashboard: a bad read doesn't drop the
			// Node from 100 to 40. Same grace period as the alert, for the same reason.
			name:       "first bad read DOES NOT drop the score",
			in:         HeartbeatInput{DiskTotalBytes: 100, DiskFreeBytes: 50, MemTotalBytes: 100, MemAvailBytes: 80, StorageOK: boolPtr(false)},
			failingFor: 0,
			overall:    100, storage: 100, memory: 100,
		},
		{
			name:       "failure still within grace period continues without penalizing",
			in:         HeartbeatInput{DiskTotalBytes: 100, DiskFreeBytes: 50, MemTotalBytes: 100, MemAvailBytes: 80, StorageOK: boolPtr(false)},
			failingFor: 30 * time.Second,
			overall:    100, storage: 100, memory: 100,
		},
		{
			// Full disk is a direct measurement, not an Agent verdict: it keeps penalizing
			// right away, no grace period. Only storage_ok goes through the wait.
			name:       "full disk penalizes immediately, no grace period",
			in:         HeartbeatInput{DiskTotalBytes: 100, DiskFreeBytes: 0, MemTotalBytes: 100, MemAvailBytes: 100, StorageOK: boolPtr(false)},
			failingFor: 0,
			overall:    40, storage: 0, memory: 100,
		},
		{
			name:    "storage_ok=true does not change (ample disk)",
			in:      HeartbeatInput{DiskTotalBytes: 100, DiskFreeBytes: 50, MemTotalBytes: 100, MemAvailBytes: 80, StorageOK: boolPtr(true)},
			overall: 100, storage: 100, memory: 100,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := computeHealth(c.in, c.failingFor, grace)
			if h.Overall != c.overall || h.Storage != c.storage || h.Memory != c.memory {
				t.Errorf("got overall=%d storage=%d memory=%d; want %d/%d/%d",
					h.Overall, h.Storage, h.Memory, c.overall, c.storage, c.memory)
			}
		})
	}
}
