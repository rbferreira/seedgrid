package torrent

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/rbferreira/seedgrid/control-plane/internal/activity"
	"github.com/rbferreira/seedgrid/control-plane/internal/imagehost"
	"github.com/rbferreira/seedgrid/control-plane/internal/imgbb"
)

// The screenshot chain assembly (#193, ADR-040), no database: fake providers.

type fakeProfiles struct {
	spec *imagehost.Spec
	err  error
}

func (f fakeProfiles) ImageHostSpec(context.Context, string) (*imagehost.Spec, error) {
	return f.spec, f.err
}

type fakeKeys struct {
	key, host string
	err       error
}

func (f fakeKeys) ImageHostKeyForChain(context.Context, string) (string, string, error) {
	return f.key, f.host, f.err
}

var chainSpec = &imagehost.Spec{
	Name:       "Host do site",
	UploadURL:  "https://img.tracker.example.org/api/upload",
	FileField:  "file",
	AuthHeader: "Authorization",
	AuthPrefix: "Bearer ",
	URLPath:    "url",
}

func hosts(cs []imagehost.Candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Host
		if c.Skip != "" {
			out[i] += " (skipped: " + c.Skip + ")"
		}
	}
	return out
}

func TestScreenshotCandidates(t *testing.T) {
	const site = "img.tracker.example.org"
	cases := []struct {
		name      string
		profiles  TrackerProfileProvider
		keys      ImageHostKeyProvider
		trackerID string
		imgbbKey  string
		want      []string
	}{
		{"tracker host with key from same host, then imgbb",
			fakeProfiles{spec: chainSpec}, fakeKeys{key: "K", host: site}, "t1", "IMGBB",
			[]string{site, "imgbb"}},
		{"no key: tracker host not included",
			fakeProfiles{spec: chainSpec}, fakeKeys{}, "t1", "IMGBB",
			[]string{"imgbb"}},
		{"key registered for another host: skipped with reason",
			fakeProfiles{spec: chainSpec}, fakeKeys{key: "K", host: "img.old.example.org"}, "t1", "IMGBB",
			[]string{site + " (skipped: key registered for another host)", "imgbb"}},
		{"key without recorded host: skipped (strict binding)",
			fakeProfiles{spec: chainSpec}, fakeKeys{key: "K"}, "t1", "IMGBB",
			[]string{site + " (skipped: key registered for another host)", "imgbb"}},
		{"profile without image_host section",
			fakeProfiles{}, fakeKeys{key: "K", host: site}, "t1", "IMGBB",
			[]string{"imgbb"}},
		{"no imgbb: only tracker host",
			fakeProfiles{spec: chainSpec}, fakeKeys{key: "K", host: site}, "t1", "  ",
			[]string{site}},
		{"no tracker_id: only imgbb, doesn't even query providers",
			fakeProfiles{err: errors.New("should not be called")}, fakeKeys{}, "", "IMGBB",
			[]string{"imgbb"}},
		{"nothing configured: empty list",
			fakeProfiles{}, fakeKeys{}, "t1", "",
			[]string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{profiles: tc.profiles, imageHostKey: tc.keys}
			got, err := s.screenshotCandidates(context.Background(), tc.trackerID, tc.imgbbKey)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if fmt.Sprint(hosts(got)) != fmt.Sprint(tc.want) {
				t.Errorf("chain = %v, want %v", hosts(got), tc.want)
			}
		})
	}
}

func TestScreenshotCandidates_ProviderErrorReturnedAsIs(t *testing.T) {
	sentinel := errors.New("tracker not found")
	s := &Service{profiles: fakeProfiles{err: sentinel}, imageHostKey: fakeKeys{}}
	if _, err := s.screenshotCandidates(context.Background(), "t1", "IMGBB"); !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want the profile provider's", err)
	}
	s = &Service{profiles: fakeProfiles{spec: chainSpec}, imageHostKey: fakeKeys{err: sentinel}}
	if _, err := s.screenshotCandidates(context.Background(), "t1", "IMGBB"); !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want the one from the key provider", err)
	}
}

// One Feed entry per failure, with the torrent, and `next` = the next candidate IN THE ORDER OF THE
// CHAIN — even when it is the winner (the common case: the site host fails and imgbb saves it).
func TestScreenshotFailureEntries(t *testing.T) {
	cands := []imagehost.Candidate{{Host: "img.a.org"}, {Host: "img.b.org", Skip: imagehost.ReasonKeyMismatch}, {Host: "imgbb"}}

	t.Run("last one wins", func(t *testing.T) {
		fails := []imagehost.Failure{{Host: "img.a.org", Reason: "HTTP 500"}, {Host: "img.b.org", Reason: imagehost.ReasonKeyMismatch}}
		got := screenshotFailureEntries(cands, fails, "trk", "tor", "Movie (2024)")
		if len(got) != 2 {
			t.Fatalf("%d entries, want 2", len(got))
		}
		wantNext := []string{"img.b.org", "imgbb"}
		for i, e := range got {
			if e.Kind != activity.KindScreenshotHostFailed || e.TorrentID != "tor" || e.TorrentName != "Movie (2024)" {
				t.Errorf("entry %d: %+v", i, e)
			}
			if e.Detail["next"] != wantNext[i] || e.Detail["host"] != fails[i].Host || e.Detail["reason"] != fails[i].Reason || e.Detail["tracker_id"] != "trk" {
				t.Errorf("detail %d = %v", i, e.Detail)
			}
		}
	})

	t.Run("all fail: last one without next", func(t *testing.T) {
		fails := []imagehost.Failure{{Host: "img.a.org", Reason: "HTTP 500"}, {Host: "img.b.org", Reason: imagehost.ReasonKeyMismatch}, {Host: "imgbb", Reason: imagehost.ReasonTimeout}}
		got := screenshotFailureEntries(cands, fails, "", "", "")
		if got[2].Detail["next"] != "" {
			t.Errorf("last next = %q, want empty", got[2].Detail["next"])
		}
	})

	t.Run("no failures: no entries", func(t *testing.T) {
		if got := screenshotFailureEntries(cands, nil, "", "", ""); len(got) != 0 {
			t.Errorf("%d entries, want 0", len(got))
		}
	})
}

// The imgbb client error becomes the chain vocabulary: the reason in the Feed outputs "HTTP <status>"
// or "response without URL", not "connection failure".
func TestImgbbChainError(t *testing.T) {
	status := imgbbChainError(fmt.Errorf("frame 0: %w", &imgbb.StatusError{Status: 400, Message: "Invalid API v1 key"}))
	var se *imagehost.StatusError
	if !errors.As(status, &se) || se.Status != 400 {
		t.Errorf("status: %v", status)
	}
	if !errors.Is(imgbbChainError(fmt.Errorf("frame 1: %w", imgbb.ErrNoURL)), imagehost.ErrNoURL) {
		t.Error("ErrNoURL not translated")
	}
	other := errors.New("connect to imgbb: dial tcp: refused")
	if imgbbChainError(other) != other {
		t.Error("connection error should pass through as is")
	}
}
