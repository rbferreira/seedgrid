package node

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeDisplayName(t *testing.T) {
	ok := []struct{ name, in, want string }{
		{"no nickname", "", ""},
		{"spaces only equals no nickname", "  \t \n ", ""},
		{"trim", "  Living Room  ", "Living Room"},
		{"collapses internal whitespace", "Living   Room\t", "Living Room"},
		{"newline becomes space", "Living\nRoom", "Living Room"},
		{"accent and emoji", "Lô's Rack 🌱", "Lô's Rack 🌱"},
		{"exactly at limit", strings.Repeat("a", MaxDisplayNameLen), strings.Repeat("a", MaxDisplayNameLen)},
		{"limit counts characters, not bytes", strings.Repeat("ç", MaxDisplayNameLen), strings.Repeat("ç", MaxDisplayNameLen)},
	}
	for _, c := range ok {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalizeDisplayName(c.in)
			if err != nil {
				t.Fatalf("NormalizeDisplayName(%q): unexpected error: %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("NormalizeDisplayName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}

	bad := []struct{ name, in string }{
		{"one character above limit", strings.Repeat("a", MaxDisplayNameLen+1)},
		{"above limit in multibyte characters", strings.Repeat("ç", MaxDisplayNameLen+1)},
		{"control character", "node\x00um"},
		{"escape ANSI", "node\x1b[31mvermelho"},
		// Zero-width: two visually identical strings, but different to the database —
		// it's like fabricating a "duplicate" that the uniqueness check wouldn't catch.
		{"zero-width space", "node" + string(rune(0x200b)) + "one"},
		{"invalid UTF-8", "node\xff\xfeum"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if got, err := NormalizeDisplayName(c.in); !errors.Is(err, ErrInvalidDisplayName) {
				t.Errorf("NormalizeDisplayName(%q) = (%q, %v), want ErrInvalidDisplayName", c.in, got, err)
			}
		})
	}
}

// The error message is what the operator reads on the Portal: it must state the limit.
func TestNormalizeDisplayName_MessageNamesTheLimit(t *testing.T) {
	_, err := NormalizeDisplayName(strings.Repeat("a", MaxDisplayNameLen+5))
	if err == nil || !strings.Contains(err.Error(), "64") {
		t.Fatalf("the message should mention the 64 limit, got: %v", err)
	}
}
