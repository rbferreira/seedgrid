package imagehost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestChainHost1FailsHost2TakesOver(t *testing.T) {
	var framesReceived [][]byte
	c1 := Candidate{
		Host: "host1",
		Upload: func(ctx context.Context, frames [][]byte) ([]Image, error) {
			return nil, &StatusError{Status: 500}
		},
	}
	c2 := Candidate{
		Host: "host2",
		Upload: func(ctx context.Context, frames [][]byte) ([]Image, error) {
			framesReceived = frames
			return []Image{{Full: "https://i.example.org/1.jpg", Thumb: "https://i.example.org/1.jpg"}}, nil
		},
	}

	frames := [][]byte{[]byte("frame1")}
	imgs, host, failures, err := Chain(context.Background(), 10*time.Second, []Candidate{c1, c2}, frames)
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	if host != "host2" {
		t.Errorf("host = %q, want host2", host)
	}
	if len(imgs) != 1 {
		t.Fatalf("images = %d, want 1", len(imgs))
	}
	if imgs[0].Full != "https://i.example.org/1.jpg" {
		t.Errorf("Full = %q, want https://i.example.org/1.jpg", imgs[0].Full)
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if failures[0].Host != "host1" || failures[0].Reason != "HTTP 500" {
		t.Errorf("failure = %+v, want {host1, HTTP 500}", failures[0])
	}
	if len(framesReceived) != 1 || string(framesReceived[0]) != "frame1" {
		t.Errorf("frames received by host2 = %v, want [frame1]", framesReceived)
	}
}

func TestChainAllFail(t *testing.T) {
	c1 := Candidate{
		Host: "host1",
		Upload: func(ctx context.Context, frames [][]byte) ([]Image, error) {
			return nil, ErrConnect
		},
	}
	c2 := Candidate{
		Host: "host2",
		Upload: func(ctx context.Context, frames [][]byte) ([]Image, error) {
			return nil, ErrNoURL
		},
	}

	frames := [][]byte{[]byte("frame1")}
	_, host, failures, err := Chain(context.Background(), 10*time.Second, []Candidate{c1, c2}, frames)

	var ce *ChainError
	if !errors.As(err, &ce) {
		t.Fatalf("error = %v, want *ChainError", err)
	}
	if host != "" {
		t.Errorf("host = %q, want empty", host)
	}
	if len(failures) != 2 {
		t.Fatalf("failures = %d, want 2", len(failures))
	}
	if failures[0].Host != "host1" || failures[0].Reason != ReasonConnect {
		t.Errorf("failure[0] = %+v, want {host1, %s}", failures[0], ReasonConnect)
	}
	if failures[1].Host != "host2" || failures[1].Reason != ReasonNoURL {
		t.Errorf("failure[1] = %+v, want {host2, %s}", failures[1], ReasonNoURL)
	}
}

func TestChainSkipByOtherHostKey(t *testing.T) {
	c1 := Candidate{
		Host: "host1",
		Skip: ReasonKeyMismatch,
		Upload: func(ctx context.Context, frames [][]byte) ([]Image, error) {
			t.Error("Upload from host1 should not be called")
			return nil, nil
		},
	}
	c2 := Candidate{
		Host: "host2",
		Upload: func(ctx context.Context, frames [][]byte) ([]Image, error) {
			return []Image{{Full: "https://i.example.org/2.jpg", Thumb: "https://i.example.org/2.jpg"}}, nil
		},
	}

	frames := [][]byte{[]byte("frame1")}
	imgs, host, failures, err := Chain(context.Background(), 10*time.Second, []Candidate{c1, c2}, frames)
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	if host != "host2" {
		t.Errorf("host = %q, want host2", host)
	}
	if len(imgs) != 1 {
		t.Fatalf("images = %d, want 1", len(imgs))
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if failures[0].Host != "host1" || failures[0].Reason != ReasonKeyMismatch {
		t.Errorf("failure = %+v, want {host1, %s}", failures[0], ReasonKeyMismatch)
	}
}

func TestChain_Host1HangsDoesNotConsumeHost2Part(t *testing.T) {
	// Host 1: Fake upload that blocks until the context cancels. Doesn't use HTTP
	// because httptest.Server doesn't cancel r.Context() on client disconnect.
	c1 := Candidate{
		Host: "host1",
		Upload: func(ctx context.Context, _ [][]byte) ([]Image, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	var host2Called bool
	c2 := Candidate{Host: "host2"}

	frames := [][]byte{[]byte("frame1")}
	start := time.Now()
	c2.Upload = func(ctx context.Context, frames [][]byte) ([]Image, error) {
		host2Called = true
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("host2 context without deadline")
		} else if remaining := time.Until(deadline); remaining < 500*time.Millisecond {
			t.Errorf("host2 received only %v of deadline, want at least 500ms", remaining)
		}
		return []Image{{Full: "https://i.example.org/ok.jpg", Thumb: "https://i.example.org/ok.jpg"}}, nil
	}
	_, host, failures, err := Chain(context.Background(), 2*time.Second, []Candidate{c1, c2}, frames)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	if host != "host2" {
		t.Errorf("host = %q, want host2", host)
	}
	if !host2Called {
		t.Error("host2 was not called")
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if failures[0].Reason != ReasonTimeout {
		t.Errorf("failure.Reason = %q, want %s", failures[0].Reason, ReasonTimeout)
	}
	if elapsed > 1500*time.Millisecond {
		t.Errorf("elapsed = %v, host1 should have consumed only its share of the deadline", elapsed)
	}
}

func TestChain_Host1FailsOnFrame2Host2ReceivesFromFrame1(t *testing.T) {
	// Host 1: responds 200 on the 1st request and 500 on the 2nd.
	callCount := 0
	srv := newTLSServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(200)
			_, _ = io.WriteString(w, `{"url":"https://i.example.org/1.jpg"}`)
		} else {
			w.WriteHeader(500)
		}
	})

	spec := validSpec()
	spec.UploadURL = srv.URL

	c1 := Candidate{
		Host: "host1",
		Upload: func(ctx context.Context, frames [][]byte) ([]Image, error) {
			return Upload(ctx, spec, "K", frames)
		},
	}

	var framesReceived [][]byte
	c2 := Candidate{
		Host: "host2",
		Upload: func(ctx context.Context, frames [][]byte) ([]Image, error) {
			framesReceived = frames
			result := make([]Image, len(frames))
			for i := range frames {
				result[i] = Image{
					Full:  fmt.Sprintf("https://i.example.org/%d.jpg", i),
					Thumb: fmt.Sprintf("https://i.example.org/%d.jpg", i),
				}
			}
			return result, nil
		},
	}

	frames := [][]byte{[]byte("frameA"), []byte("frameB"), []byte("frameC")}
	_, host, failures, err := Chain(context.Background(), 10*time.Second, []Candidate{c1, c2}, frames)
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	if host != "host2" {
		t.Errorf("host = %q, want host2", host)
	}
	if callCount != 2 {
		t.Errorf("host1 received %d frames, want 2", callCount)
	}
	if len(failures) != 1 || failures[0].Reason != "HTTP 500" {
		t.Errorf("failures = %+v, want reason HTTP 500", failures)
	}
	// Host 2 must receive BOTH frames, starting from the first.
	if len(framesReceived) != len(frames) {
		t.Fatalf("framesReceived = %d, want %d", len(framesReceived), len(frames))
	}
	if string(framesReceived[0]) != "frameA" {
		t.Errorf("framesReceived[0] = %q, want frameA (restart of the first frame)", framesReceived[0])
	}
	if string(framesReceived[1]) != "frameB" {
		t.Errorf("framesReceived[1] = %q, want frameB", framesReceived[1])
	}
	if string(framesReceived[2]) != "frameC" {
		t.Errorf("framesReceived[2] = %q, want frameC", framesReceived[2])
	}
}

func TestChainInvalidURL(t *testing.T) {
	c1 := Candidate{
		Host: "host1",
		Upload: func(ctx context.Context, frames [][]byte) ([]Image, error) {
			return []Image{
				{Full: "https://ok.example.org/a[1].jpg", Thumb: "https://ok.example.org/a[1].jpg"},
			}, nil
		},
	}
	c2 := Candidate{
		Host: "host2",
		Upload: func(ctx context.Context, frames [][]byte) ([]Image, error) {
			return []Image{{Full: "https://i.example.org/ok.jpg", Thumb: "https://i.example.org/ok.jpg"}}, nil
		},
	}

	frames := [][]byte{[]byte("frame1")}
	imgs, host, failures, err := Chain(context.Background(), 10*time.Second, []Candidate{c1, c2}, frames)
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	if host != "host2" {
		t.Errorf("host = %q, want host2", host)
	}
	if len(imgs) != 1 {
		t.Fatalf("images = %d, want 1", len(imgs))
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	if failures[0].Host != "host1" || failures[0].Reason != ReasonInvalidURL {
		t.Errorf("failure = %+v, want {host1, %s}", failures[0], ReasonInvalidURL)
	}
}

func TestChain_ReasonNeverContainsKey(t *testing.T) {
	secretKey := "secret-key-12345"
	sensitiveURL := "https://host1.ex/upload?api_key=secret"
	candidate := Candidate{Host: "host1", Upload: func(context.Context, [][]byte) ([]Image, error) {
		return nil, fmt.Errorf("error with URL %s and key %s", sensitiveURL, secretKey)
	}}
	_, _, failures, err := Chain(context.Background(), time.Second, []Candidate{candidate}, [][]byte{[]byte("frame")})
	if err == nil {
		t.Fatal("Chain should fail")
	}
	allowed := map[string]bool{ReasonConnect: true, ReasonTimeout: true, ReasonRedirect: true, ReasonNoURL: true}
	if len(failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(failures))
	}
	reason := failures[0].Reason
	if !allowed[reason] {
		t.Errorf("reason %q outside closed set", reason)
	}
	if strings.Contains(reason, secretKey) || strings.Contains(reason, sensitiveURL) {
		t.Errorf("reason contains secret: %q", reason)
	}
}

func TestChain_ResponseWithIncorrectQuantity(t *testing.T) {
	candidate := Candidate{Host: "host1", Upload: func(context.Context, [][]byte) ([]Image, error) {
		return []Image{{Full: "https://i.example.org/1.jpg", Thumb: "https://i.example.org/1.jpg"}}, nil
	}}
	_, _, failures, err := Chain(context.Background(), time.Second, []Candidate{candidate}, [][]byte{[]byte("1"), []byte("2")})
	if err == nil {
		t.Fatal("Chain should fail when image is missing")
	}
	if len(failures) != 1 || failures[0].Reason != ReasonNoURL {
		t.Errorf("failures = %+v, want reason %q", failures, ReasonNoURL)
	}
}

func TestChainRejectsEmptyFrames(t *testing.T) {
	_, _, _, err := Chain(context.Background(), time.Second, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "frames") {
		t.Errorf("error = %v, want clear error about frames", err)
	}
}

func TestValidateURL(t *testing.T) {
	cases := []struct {
		url  string
		err  bool
		desc string
	}{
		{"https://i.example.org/a.jpg", false, "valid URL"},
		{"", true, "empty"},
		{strings.Repeat("a", 2049), true, "> 2048 bytes"},
		{"http://i.example.org/a.jpg", true, "http"},
		{"https://i.example.org/a b.jpg", true, "with space"},
		{"https://i.example.org/a[1].jpg", true, "with ["},
		{"https://i.example.org/a]1[.jpg", true, "with ]"},
		{`https://i.example.org/a"b.jpg`, true, "with \""},
		{"https://i.example.org/a'b.jpg", true, "with '"},
		{"https://i.example.org/a\x00b.jpg", true, "with control character"},
		{"/a/b", true, "relative"},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			err := ValidateURL(c.url)
			if c.err && err == nil {
				t.Errorf("expected error for %q, passed", c.url)
			}
			if !c.err && err != nil {
				t.Errorf("did not expect error for %q, got: %v", c.url, err)
			}
		})
	}
}

// The raw text of the actual errors holds the secret: the *url.Error from http.Client contains the URL
// with the key in the query, and an authentication error might echo the header. Neither can
// become the reason.
func TestChain_ReasonDoesNotLeakRealError(t *testing.T) {
	const secret = "SECRET-123"
	errs := []error{
		&url.Error{Op: "Post", URL: "https://api.example.org/1/upload?key=" + secret, Err: errors.New("dial tcp: connection refused")},
		fmt.Errorf("frame 0: %w", &url.Error{Op: "Post", URL: "https://img.example.org/up?key=" + secret, Err: context.DeadlineExceeded}),
		fmt.Errorf("auth refused: Authorization: Bearer %s", secret),
	}
	for i, e := range errs {
		c := Candidate{Host: "host1", Upload: func(context.Context, [][]byte) ([]Image, error) { return nil, e }}
		_, _, failures, err := Chain(context.Background(), time.Second, []Candidate{c}, [][]byte{[]byte("f")})
		if err == nil || len(failures) != 1 {
			t.Fatalf("case %d: failures = %+v, error = %v", i, failures, err)
		}
		r := failures[0].Reason
		if r != ReasonConnect && r != ReasonTimeout {
			t.Errorf("case %d: reason %q outside expected", i, r)
		}
		if strings.Contains(r, secret) || strings.Contains(r, "example.org") || strings.Contains(err.Error(), secret) {
			t.Errorf("case %d: secret or URL in reason/error: %q / %q", i, r, err.Error())
		}
	}
}

// A candidate skipped AFTER the active one doesn't count in the time allocation: the active one gets the
// entire budget, not half.
func TestChain_SkippedAfterActiveDoesNotReducePart(t *testing.T) {
	var got time.Duration
	active := Candidate{Host: "host1", Upload: func(ctx context.Context, _ [][]byte) ([]Image, error) {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Error("context without deadline")
		}
		got = time.Until(dl)
		return []Image{{Full: "https://i.example.org/a.jpg", Thumb: "https://i.example.org/a.jpg"}}, nil
	}}
	skipped := Candidate{Host: "host2", Skip: ReasonKeyMismatch}
	if _, _, _, err := Chain(context.Background(), 2*time.Second, []Candidate{active, skipped}, [][]byte{[]byte("f")}); err != nil {
		t.Fatal(err)
	}
	if got < 1500*time.Millisecond {
		t.Errorf("the active one received %v, want close to 2s (the skipped one does not split the time)", got)
	}
}
