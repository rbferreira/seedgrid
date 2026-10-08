package settings

import "testing"

// The watchdog for the migration recheck (#87) is tunable with hot-reload (ADR-023): the catalog
// limits are what prevents an absurd value in the panel from disabling it in practice.
func TestRecheckWatchdogSettings(t *testing.T) {
	cases := []struct {
		key, value string
		ok         bool
	}{
		{"migration.recheck_stall_window", "10m", true}, // the default inherited from #79
		{"migration.recheck_stall_window", "30m", true}, // very slow disk
		{"migration.recheck_stall_window", "30s", false},
		{"migration.recheck_stall_window", "0s", false}, // never disables: without watchdog, a stuck recheck occupies the slot forever
		{"migration.recheck_stall_window", "banana", false},
		{"migration.recheck_max_wait", "6h", true}, // the default
		{"migration.recheck_max_wait", "48h", true},
		{"migration.recheck_max_wait", "5m", false},
		{"migration.recheck_max_wait", "0s", false},
		{"migration.recheck_max_wait", "999h", false},
	}
	for _, c := range cases {
		spec, ok := Lookup(c.key)
		if !ok {
			t.Fatalf("key %s missing from catalog", c.key)
		}
		if err := spec.Validate(c.value); (err == nil) != c.ok {
			t.Errorf("Validate(%s=%q) err=%v, want ok=%v", c.key, c.value, err, c.ok)
		}
	}
}

// The catalog defaults are what the CP uses without configuration: the ceiling must be GREATER
// than the stagnation window, otherwise stagnation would never be able to fail first.
func TestRecheckWatchdogDefaults(t *testing.T) {
	s := NewForTest(nil)
	stall, ceiling := s.Duration("migration.recheck_stall_window"), s.Duration("migration.recheck_max_wait")
	if stall <= 0 || ceiling <= stall {
		t.Errorf("inconsistent patterns: stall=%s ceiling=%s", stall, ceiling)
	}
}
