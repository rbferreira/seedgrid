package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{
			name:    "valid message",
			content: "feat: add feature\n",
			want:    0,
		},
		{
			name:    "valid message with body",
			content: "fix: fix bug\n\nMessage body.\n",
			want:    0,
		},
		{
			name:    "Merge exempt message",
			content: "Merge branch 'main' into feature\n",
			want:    0,
		},
		{
			name:    "Revert exempt message",
			content: `Revert "feat: something"` + "\n",
			want:    0,
		},
		{
			name:    "invalid message",
			content: "Added a feature\n",
			want:    1,
		},
		{
			name:    "Co-Authored-By fails",
			content: "feat: something\n\nCo-authored-by: X <x@y>\n",
			want:    1,
		},
		{
			name:    "git comments are removed",
			content: "feat: something\n# git comment\n# other\n",
			want:    0,
		},
		{
			name:    "Co-Authored-By in comment is removed",
			content: "feat: something\n# Co-authored-by: X <x@y>\n",
			want:    0,
		},
		{
			name:    "empty message fails",
			content: "",
			want:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "COMMIT_EDITMSG")
			if err := os.WriteFile(path, []byte(tt.content), 0600); err != nil {
				t.Fatal(err)
			}
			got := checkFile(path)
			if got != tt.want {
				t.Errorf("checkFile(%q) = %d, want %d", tt.content, got, tt.want)
			}
		})
	}
}

func TestParseLog(t *testing.T) {
	// Simulates the output of git log --format=%H%x1f%B%x1e.
	// TrimSpace in parseLog removes trailing line breaks; the message does not have a trailing \n preserved.
	out := "abc123def456\x1ffeat: first message\n\x1e" +
		"789012345678\x1ffix: second message\n\x1e"

	commits := parseLog(out)
	if len(commits) != 2 {
		t.Fatalf("parseLog() returned %d commits, want 2", len(commits))
	}
	if commits[0].sha != "abc123def456" {
		t.Errorf("commits[0].sha = %q, want %q", commits[0].sha, "abc123def456")
	}
	if commits[0].msg != "feat: first message" {
		t.Errorf("commits[0].msg = %q, want %q", commits[0].msg, "feat: first message")
	}
	if commits[1].sha != "789012345678" {
		t.Errorf("commits[1].sha = %q, want %q", commits[1].sha, "789012345678")
	}
}

func TestRangePattern(t *testing.T) {
	valid := []string{
		"origin/main..HEAD",
		"a61d488..e68ac3c",
		"0123456789abcdef0123456789abcdef01234567..fedcba9876543210fedcba9876543210fedcba98",
		"e68ac3c~1..e68ac3c", // CI fallback to a new branch and force-push
		"main...feature/x",
		"HEAD^..HEAD",
	}
	for _, r := range valid {
		if !rangePattern.MatchString(r) {
			t.Errorf("rangePattern rejected %q, which is a valid range", r)
		}
	}
	invalid := []string{"", "--output=/tmp/x", "-p", "a b", "a;rm -rf /", "$(id)", "a..b\nc"}
	for _, r := range invalid {
		if rangePattern.MatchString(r) {
			t.Errorf("rangePattern accepted %q", r)
		}
	}
}
