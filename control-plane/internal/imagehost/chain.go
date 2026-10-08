package imagehost

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Closed set reasons (ADR-040). The reason is NEVER raw err.Error() — the raw
// text can carry the URL (in imgbb, the key in the query — defect (i) closed in ADR-040)
// or third-party message. The chain only produces one of these reasons.
const (
	ReasonKeyMismatch = "key registered for another host"
	ReasonRedirect    = "redirect rejected"
	ReasonTimeout     = "timed out"
	ReasonConnect     = "connection failure"
	ReasonNoURL       = "response without URL"
	ReasonInvalidURL  = "invalid URL in response"
)

// ReasonCode returns the stable code for a reason (ADR-041): the text above is what the Feed
// records as detail; the code is what the Portal uses to build the phrase in the reader's language.
// "HTTP 503" becomes http_status (the text, same in any language, goes in the `reason`).
func ReasonCode(reason string) string {
	switch reason {
	case ReasonKeyMismatch:
		return "key_mismatch"
	case ReasonRedirect:
		return "redirect"
	case ReasonTimeout:
		return "timeout"
	case ReasonConnect:
		return "connect"
	case ReasonNoURL:
		return "no_url"
	case ReasonInvalidURL:
		return "invalid_url"
	}
	if strings.HasPrefix(reason, "HTTP ") {
		return "http_status"
	}
	return "other"
}

// Failure records an attempt that failed or was skipped. It goes to the Feed (ADR-022)
// and to the API response.
type Failure struct {
	Host   string `json:"host"`
	Reason string `json:"reason"`
}

// Candidate is a candidate host in the chain. A non-empty Skip means the host is
// skipped even before the attempt (e.g.: key registered for another host). Upload
// receives the context already with the adjusted deadline and complete frames; if it fails on
// any frame, the chain moves to the next candidate, which restarts from frame 1.
type Candidate struct {
	Host   string
	Skip   string
	Upload func(ctx context.Context, frames [][]byte) ([]Image, error)
}

// ChainError signals that no candidate was able to host all frames.
// The API response returns this to the operator with the list of reasons per host.
type ChainError struct {
	Failures []Failure
}

func (e *ChainError) Error() string {
	if len(e.Failures) == 0 {
		return "all image hosts failed"
	}
	var parts []string
	for _, f := range e.Failures {
		parts = append(parts, fmt.Sprintf("%s: %s", f.Host, f.Reason))
	}
	return fmt.Sprintf("all image hosts failed (%s)", strings.Join(parts, "; "))
}

// ValidateURL checks the URL returned by any host before entering BBCode.
// The URL is interpolated into `[url=…][img]…[/img][/url]`; without validation, a host's
// response would inject markup into the description the site publishes.
func ValidateURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("empty URL")
	}
	if len(raw) > 2048 {
		return fmt.Errorf("URL with %d characters (maximum 2048)", len(raw))
	}
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		switch {
		case b == ' ':
			return fmt.Errorf("URL contains space")
		case b <= 0x1F || b == 0x7F:
			return fmt.Errorf("URL contains control character")
		case b == '[' || b == ']' || b == '"' || b == '\'':
			return fmt.Errorf("URL contains forbidden character (%q)", string(b))
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	if !u.IsAbs() {
		return fmt.Errorf("relative URL")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("URL with scheme %q (https only)", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("URL without host")
	}
	return nil
}

// Chain tries each candidate in order with a total time budget. Each candidate
// gets the fair share of what remains (what is left divided by the candidates that
// haven't been tried yet), so a hanging host doesn't consume the time of the
// fallback. The first host to host ALL frames wins; failure on any
// frame makes the chain move to the next, which restarts from frame 1.
func Chain(ctx context.Context, total time.Duration, candidates []Candidate, frames [][]byte) ([]Image, string, []Failure, error) {
	if len(frames) == 0 {
		return nil, "", nil, fmt.Errorf("no frames to send")
	}
	deadline := time.Now().Add(total)
	failures := []Failure{}

	for i, c := range candidates {
		if c.Skip != "" {
			failures = append(failures, Failure{Host: c.Host, Reason: c.Skip})
			continue
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			failures = append(failures, Failure{Host: c.Host, Reason: ReasonTimeout})
			continue
		}

		// Fair share: remainder divided by the remaining active candidates (including i).
		divisor := 0
		for j := i; j < len(candidates); j++ {
			if candidates[j].Skip == "" {
				divisor++
			}
		}
		if divisor == 0 {
			divisor = 1
		}
		share := remaining / time.Duration(divisor)

		attemptCtx, cancel := context.WithDeadline(ctx, time.Now().Add(share))
		imgs, err := c.Upload(attemptCtx, frames)
		cancel()

		if err == nil {
			if len(imgs) != len(frames) {
				failures = append(failures, Failure{Host: c.Host, Reason: ReasonNoURL})
				continue
			}
			// All or nothing: validates ALL URLs before accepting the host.
			if valid := validateImages(imgs); !valid {
				failures = append(failures, Failure{Host: c.Host, Reason: ReasonInvalidURL})
				continue
			}
			return imgs, c.Host, failures, nil
		}

		failures = append(failures, Failure{Host: c.Host, Reason: reasonFor(err)})
	}

	return nil, "", failures, &ChainError{Failures: failures}
}

// validateImages checks Full and Thumb for each image. If any URL is invalid,
// it returns false — the chain treats it as a failure (ReasonInvalidURL) and tries the next host.
func validateImages(imgs []Image) bool {
	for _, im := range imgs {
		if err := ValidateURL(im.Full); err != nil {
			return false
		}
		if err := ValidateURL(im.Thumb); err != nil {
			return false
		}
	}
	return true
}

// reasonFor translates an Upload error into a closed set reason. It never leaks
// the raw error text.
func reasonFor(err error) string {
	var se *StatusError
	if errors.As(err, &se) {
		return fmt.Sprintf("HTTP %d", se.Status)
	}
	if errors.Is(err, ErrRedirect) {
		return ReasonRedirect
	}
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return ReasonTimeout
	}
	if errors.Is(err, ErrNoURL) {
		return ReasonNoURL
	}
	// Safe fallback within the closed set (ErrConnect, *url.Error, etc.).
	return ReasonConnect
}
