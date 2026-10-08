// Package identity persists the local Node identity (id + Agent credential),
// so that the same Node reuses its identity across restarts (Agent.md §16).
package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Identity is the persisted Agent identity.
type Identity struct {
	NodeID     string `json:"node_id"`
	AgentToken string `json:"agent_token"`
}

func file(dir string) string { return filepath.Join(dir, "identity.json") }

// Load reads the identity from the data directory. The second return indicates if it exists.
func Load(dir string) (Identity, bool, error) {
	b, err := os.ReadFile(file(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return Identity{}, false, nil
	}
	if err != nil {
		return Identity{}, false, fmt.Errorf("read identity: %w", err)
	}
	var id Identity
	if err := json.Unmarshal(b, &id); err != nil {
		return Identity{}, false, fmt.Errorf("decode identity: %w", err)
	}
	return id, id.NodeID != "" && id.AgentToken != "", nil
}

// Save writes the identity with restricted permissions (contains credential).
func Save(dir string, id Identity) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	b, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(file(dir), b, 0o600); err != nil {
		return fmt.Errorf("write identity: %w", err)
	}
	return nil
}
