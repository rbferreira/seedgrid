package imgbb

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeImgbb returns predictable URLs and logs what it receives, to verify the contract:
// key in the query and `image` in base64 in the body.
func fakeImgbb(t *testing.T, status int) (*httptest.Server, *[]string) {
	t.Helper()
	var gotImages []string
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "K" {
			t.Errorf("key in query = %q, want K", r.URL.Query().Get("key"))
		}
		_ = r.ParseMultipartForm(1 << 20) //nolint:gosec // G120: 1MB is already the explicit limit
		gotImages = append(gotImages, r.FormValue("image"))
		w.WriteHeader(status)
		if status == 200 {
			_, _ = fmt.Fprintf(w, `{"data":{"url":"https://i.ibb.co/img%d.jpg","thumb":{"url":"https://i.ibb.co/thumb%d.jpg"}},"success":true,"status":200}`, n, n)
		} else {
			_, _ = fmt.Fprint(w, `{"status":400,"error":{"message":"Invalid API v1 key"},"success":false}`)
		}
		n++
	}))
	t.Cleanup(srv.Close)
	return srv, &gotImages
}

func TestUploadOrderAndContract(t *testing.T) {
	srv, gotImages := fakeImgbb(t, 200)
	old := uploadURL
	uploadURL = srv.URL
	defer func() { uploadURL = old }()

	frames := [][]byte{[]byte("frameA"), []byte("frameB"), []byte("frameC")}
	imgs, err := Upload(context.Background(), "K", frames)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(imgs) != 3 {
		t.Fatalf("images = %d, want 3", len(imgs))
	}
	// Order preserved (screenshots appear in sequence in the description), full + thumb.
	for i, im := range imgs {
		if want := fmt.Sprintf("https://i.ibb.co/img%d.jpg", i); im.Full != want {
			t.Errorf("full[%d] = %q, want %q (order swapped?)", i, im.Full, want)
		}
		if want := fmt.Sprintf("https://i.ibb.co/thumb%d.jpg", i); im.Thumb != want {
			t.Errorf("thumb[%d] = %q, want %q", i, im.Thumb, want)
		}
	}
	// The body took the frame in base64, not raw.
	for i, f := range frames {
		if want := base64.StdEncoding.EncodeToString(f); (*gotImages)[i] != want {
			t.Errorf("image[%d] is not the base64 of the frame", i)
		}
	}
}

func TestUploadNoKey(t *testing.T) {
	if _, err := Upload(context.Background(), "  ", [][]byte{[]byte("x")}); err == nil {
		t.Error("Upload without api_key should fail")
	}
}

func TestUploadNoFrames(t *testing.T) {
	if _, err := Upload(context.Background(), "K", nil); err == nil {
		t.Error("Upload without frames should fail")
	}
}

// Thumbnail: without `thumb`, falls back to `medium`; without both, uses the full image (F1).
func TestUploadThumbnailFallback(t *testing.T) {
	cases := []struct {
		name, body, wantThumb string
	}{
		{"medium only", `{"data":{"url":"https://i.ibb.co/a.jpg","medium":{"url":"https://i.ibb.co/am.jpg"}},"success":true}`, "https://i.ibb.co/am.jpg"},
		{"uses full if no thumb or medium", `{"data":{"url":"https://i.ibb.co/a.jpg"},"success":true}`, "https://i.ibb.co/a.jpg"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, c.body)
			}))
			defer srv.Close()
			old := uploadURL
			uploadURL = srv.URL
			defer func() { uploadURL = old }()
			imgs, err := Upload(context.Background(), "K", [][]byte{[]byte("x")})
			if err != nil {
				t.Fatalf("Upload: %v", err)
			}
			if imgs[0].Thumb != c.wantThumb {
				t.Errorf("thumb = %q, want %q", imgs[0].Thumb, c.wantThumb)
			}
		})
	}
}

// imgbb error (invalid key) aborts and does NOT return partial URLs.
func TestUploadErrorAborts(t *testing.T) {
	srv, _ := fakeImgbb(t, 400)
	old := uploadURL
	uploadURL = srv.URL
	defer func() { uploadURL = old }()

	imgs, err := Upload(context.Background(), "K", [][]byte{[]byte("a"), []byte("b")})
	if err == nil {
		t.Fatal("expected imgbb error")
	}
	if imgs != nil {
		t.Error("should not return images when a frame fails")
	}
	if !strings.Contains(err.Error(), "Invalid API v1 key") {
		t.Errorf("error should carry the imgbb message: %v", err)
	}
}

// Non-JSON response (e.g.: maintenance HTML) with bad status → status error, no panic.
func TestUploadNonJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		_, _ = io.WriteString(w, "<html>service unavailable</html>")
	}))
	defer srv.Close()
	old := uploadURL
	uploadURL = srv.URL
	defer func() { uploadURL = old }()

	if _, err := Upload(context.Background(), "K", [][]byte{[]byte("a")}); err == nil {
		t.Error("non-JSON response with status 503 should fail")
	}
}
