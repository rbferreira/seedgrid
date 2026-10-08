package api

import (
	"fmt"
	"strings"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/scheduler"
)

// The message to the operator states the size, the margin and the best space, and points out the margin
// as something adjustable — that was what was missing for a small disk Node (#22).
func TestNoNodeWithSpaceMessage(t *testing.T) {
	const mib, gib = int64(1) << 20, int64(1) << 30
	err := fmt.Errorf("create torrent: %w", &scheduler.NoNodeWithSpaceError{
		NeedBytes: 756 * mib, MinFreeBytes: 5 * gib, BestFreeBytes: 3*gib + 512*mib,
	})
	msg := noNodeWithSpaceMessage(err)
	for _, want := range []string{"756.0 MiB", "5.0 GiB", "3.5 GiB", "Free disk margin", "Settings"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message without %q: %s", want, msg)
		}
	}

	// Without the numbers (raw error), it falls back to generic text instead of printing zeros.
	generic := noNodeWithSpaceMessage(scheduler.ErrNoNodeWithSpace)
	if strings.Contains(generic, "0 B") || !strings.Contains(generic, "no Seed Node with disk space") {
		t.Errorf("unexpected generic message: %s", generic)
	}
}
