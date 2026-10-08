package imagehost

import "testing"

// The code is what the Portal uses for the phrase (ADR-041); the set must match the
// variants of screenshot_host_failed in messages.Registry.
func TestReasonCode(t *testing.T) {
	cases := map[string]string{
		ReasonKeyMismatch: "key_mismatch",
		ReasonRedirect:    "redirect",
		ReasonTimeout:     "timeout",
		ReasonConnect:     "connect",
		ReasonNoURL:       "no_url",
		ReasonInvalidURL:  "invalid_url",
		"HTTP 503":        "http_status",
		"something new":   "other",
	}
	for reason, want := range cases {
		if got := ReasonCode(reason); got != want {
			t.Errorf("ReasonCode(%q) = %q, want %q", reason, got, want)
		}
	}
}
