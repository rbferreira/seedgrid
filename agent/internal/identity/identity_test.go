package identity

import "testing"

func TestSaveLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()

	if _, exists, err := Load(dir); err != nil || exists {
		t.Fatalf("Load in empty directory: exists=%v err=%v", exists, err)
	}

	want := Identity{NodeID: "node-123", AgentToken: "tok-abc"}
	if err := Save(dir, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, exists, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !exists {
		t.Fatal("exists = false after Save")
	}
	if got != want {
		t.Errorf("identity = %+v, want %+v", got, want)
	}
}

func TestLoadIncomplete(t *testing.T) {
	dir := t.TempDir()
	// Identity without a token must not be considered valid.
	if err := Save(dir, Identity{NodeID: "only-id"}); err != nil {
		t.Fatal(err)
	}
	if _, exists, _ := Load(dir); exists {
		t.Error("incomplete identity was considered existing")
	}
}
