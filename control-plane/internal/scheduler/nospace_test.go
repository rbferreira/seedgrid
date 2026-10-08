package scheduler

import (
	"errors"
	"strings"
	"testing"
)

// The space rejection loads the three numbers and remains an ErrNoNodeWithSpace for
// the already existing errors.Is (#22).
func TestNoNodeWithSpaceError(t *testing.T) {
	const mib, gib = int64(1) << 20, int64(1) << 30
	var err error = &NoNodeWithSpaceError{NeedBytes: 756 * mib, MinFreeBytes: 5 * gib, BestFreeBytes: 3*gib + 512*mib}

	if !errors.Is(err, ErrNoNodeWithSpace) {
		t.Fatal("NoNodeWithSpaceError is no longer an ErrNoNodeWithSpace")
	}
	if errors.Is(err, ErrNoEligibleNode) {
		t.Error("rejection due to space cannot masquerade as \"no eligible Node\"")
	}
	msg := err.Error()
	for _, want := range []string{"756.0 MiB", "5.0 GiB", "3.5 GiB"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message without %q: %s", want, msg)
		}
	}
}
