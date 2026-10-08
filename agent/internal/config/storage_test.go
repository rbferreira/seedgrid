package config

import (
	"os"
	"testing"
)

// SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT (#221): missing = true. This keeps the Nodes that
// already exist exactly as they are (none have the variable) and makes any configuration
// error default to the safe side — require the mountpoint and remove the Node, never
// silently accept a disk that didn't mount.
func TestLoad_StorageRequireMount(t *testing.T) {
	cases := []struct {
		name  string
		unset bool
		val   string
		want  bool
	}{
		{"missing", true, "", true},
		{"empty", false, "", true},
		{"true", false, "true", true},
		{"false", false, "false", false},
		{"0", false, "0", false},
		{"1", false, "1", true},
		// ParseBool doesn't recognize these: falls back to the default (true), never to false by accident.
		{"invalid value", false, "maybe", true},
		{"typo de false", false, "flase", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("SEEDGRID_CONTROL_PLANE_URL", "http://cp:8080")
			// t.Setenv registers the restoration of the original value; Unsetenv only applies within the test.
			t.Setenv("SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT", c.val)
			if c.unset {
				if err := os.Unsetenv("SEEDGRID_AGENT_STORAGE_REQUIRE_MOUNT"); err != nil {
					t.Fatalf("unsetenv: %v", err)
				}
			}
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg.StorageRequireMount != c.want {
				t.Errorf("StorageRequireMount = %v, want %v", cfg.StorageRequireMount, c.want)
			}
		})
	}
}
