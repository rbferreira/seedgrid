package imagehost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// validSpec returns a valid Spec for tests. Copying and adjusting fields instead
// of building from scratch prevents forgetting any required field.
func validSpec() Spec {
	return Spec{
		Name:          "Host do site",
		UploadURL:     "https://img.tracker.example.org/api/v1/sharex/upload",
		FileField:     "file",
		AuthHeader:    "Authorization",
		AuthPrefix:    "Bearer ",
		URLPath:       "url",
		ThumbnailPath: "thumbnail_url",
	}
}

func TestValidateRulesOfADR(t *testing.T) {
	base := validSpec()

	// Helper: copies the base and applies the mutation.
	with := func(mut func(*Spec)) Spec {
		s := base
		mut(&s)
		return s
	}

	cases := []struct {
		name string
		spec Spec
		err  bool
	}{
		// Cases that MUST pass.
		{"ADR-040 example", base, false},
		{"empty thumbnail_path", with(func(s *Spec) { s.ThumbnailPath = "" }), false},
		{"url_path with multiple segments", with(func(s *Spec) { s.URLPath = "data.link" }), false},
		{"numeric segment in url_path", with(func(s *Spec) { s.URLPath = "images.0.url" }), false},

		// Cases that MUST fail — one per rule.
		{"empty name", with(func(s *Spec) { s.Name = "" }), true},
		{"name with 65 runes", with(func(s *Spec) { s.Name = strings.Repeat("a", 65) }), true},
		{"name with control character", with(func(s *Spec) { s.Name = "test\x00" }), true},
		{"name with U+202E (direction override)", with(func(s *Spec) { s.Name = "test\u202E" }), true},
		{"upload_url http", with(func(s *Spec) { s.UploadURL = "http://img.example.org/up" }), true},
		{"relative upload_url", with(func(s *Spec) { s.UploadURL = "/api/upload" }), true},
		{"upload_url > 2048 characters", with(func(s *Spec) {
			s.UploadURL = "https://img.example.org/" + strings.Repeat("a", 2050)
		}), true},
		{"upload_url with IP literal", with(func(s *Spec) { s.UploadURL = "https://203.0.113.7/up" }), true},
		{"upload_url localhost", with(func(s *Spec) { s.UploadURL = "https://localhost/up" }), true},
		{"upload_url IPv6 literal", with(func(s *Spec) { s.UploadURL = "https://[::1]/x" }), true},
		{"upload_url localhost uppercase", with(func(s *Spec) { s.UploadURL = "https://LOCALHOST/up" }), true},
		{"upload_url suffix localhost", with(func(s *Spec) { s.UploadURL = "https://evil.localhost/up" }), true},
		{"upload_url localhost with trailing dot", with(func(s *Spec) { s.UploadURL = "https://localhost./up" }), true},
		{"abbreviated IPv4 upload_url", with(func(s *Spec) { s.UploadURL = "https://127.1/up" }), true},
		{"upload_url IPv4 hexadecimal", with(func(s *Spec) { s.UploadURL = "https://0x7f.1/up" }), true},
		{"upload_url explicit port", with(func(s *Spec) { s.UploadURL = "https://img.example.org:8443/up" }), true},
		{"upload_url userinfo", with(func(s *Spec) { s.UploadURL = "https://u:p@img.example.org/up" }), true},
		{"upload_url fragment", with(func(s *Spec) { s.UploadURL = "https://img.example.org/up#frag" }), true},
		{"empty file_field", with(func(s *Spec) { s.FileField = "" }), true},
		{"file_field with space", with(func(s *Spec) { s.FileField = "my field" }), true},
		{"file_field with 65 characters", with(func(s *Spec) { s.FileField = strings.Repeat("a", 65) }), true},
		{"auth_header with underscore", with(func(s *Spec) { s.AuthHeader = "X_Api" }), true},
		{"auth_header Cookie", with(func(s *Spec) { s.AuthHeader = "Cookie" }), true},
		{"auth_header content-type (lowercase)", with(func(s *Spec) { s.AuthHeader = "content-type" }), true},
		{"auth_header Proxy-Authorization", with(func(s *Spec) { s.AuthHeader = "Proxy-Authorization" }), true},
		{"auth_prefix with 33 characters", with(func(s *Spec) { s.AuthPrefix = strings.Repeat("x", 33) }), true},
		{"auth_prefix with non-ASCII character", with(func(s *Spec) { s.AuthPrefix = "Bearer ç" }), true},
		{"empty url_path", with(func(s *Spec) { s.URLPath = "" }), true},
		{"url_path with 6 segments", with(func(s *Spec) { s.URLPath = "a.b.c.d.e.f" }), true},
		{"url_path segment with hyphen", with(func(s *Spec) { s.URLPath = "da-ta.link" }), true},
		{"thumbnail_path with invalid character", with(func(s *Spec) { s.ThumbnailPath = "thumb-url" }), true},
	}
	for _, header := range []string{"Host", "Content-Type", "Content-Length", "Transfer-Encoding", "Connection", "Cookie", "Upgrade", "TE", "Trailer", "Proxy-Authorization"} {
		for _, name := range []string{strings.ToUpper(header), strings.ToLower(header)} {
			cases = append(cases, struct {
				name string
				spec Spec
				err  bool
			}{"auth_header " + name, with(func(s *Spec) { s.AuthHeader = name }), true})
		}
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.spec.Validate()
			if c.err && err == nil {
				t.Errorf("expected error, passed")
			}
			if !c.err && err != nil {
				t.Errorf("did not expect error, got: %v", err)
			}
		})
	}
}

// uploadServer spins up a fake TLS server to test the Upload contract.
// It logs per request: auth header, field name, filename, Content-Type
// of the part, and received bytes. The handler returns the JSON the test asks for.
func newTLSServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	old := transport
	transport = srv.Client().Transport
	t.Cleanup(func() { transport = old; srv.Close() })
	return srv
}

func uploadServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return newTLSServer(t, handler)
}

func TestUploadContract(t *testing.T) {
	// Logs what the server received per request.
	type reqInfo struct {
		authHeader  string
		fieldName   string
		filename    string
		contentType string
		bytes       []byte
	}
	var reqs []reqInfo

	srv := uploadServer(t, func(w http.ResponseWriter, r *http.Request) {
		info := reqInfo{authHeader: r.Header.Get("Authorization")}
		_ = r.ParseMultipartForm(1 << 20) //nolint:gosec // G120: 1MB is already the explicit limit
		if r.MultipartForm != nil && len(r.MultipartForm.File) > 0 {
			for fieldName, files := range r.MultipartForm.File {
				info.fieldName = fieldName
				if len(files) > 0 {
					f := files[0]
					info.filename = f.Filename
					info.contentType = f.Header.Get("Content-Type")
					rc, _ := f.Open()
					info.bytes, _ = io.ReadAll(rc)
					_ = rc.Close()
				}
				break
			}
		}
		reqs = append(reqs, info)
		w.WriteHeader(200)
		_, _ = fmt.Fprintf(w, `{"url":"https://i.example.org/img%d.jpg","thumbnail_url":"https://i.example.org/thumb%d.jpg"}`, len(reqs), len(reqs))
	})

	spec := validSpec()
	spec.UploadURL = srv.URL
	frames := [][]byte{[]byte("frameA"), []byte("frameB"), []byte("frameC")}

	imgs, err := Upload(context.Background(), spec, "SECRET-KEY", frames)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if len(imgs) != 3 {
		t.Fatalf("images = %d, want 3", len(imgs))
	}

	// Order preserved, full + thumb.
	for i, im := range imgs {
		if want := fmt.Sprintf("https://i.example.org/img%d.jpg", i+1); im.Full != want {
			t.Errorf("full[%d] = %q, want %q", i, im.Full, want)
		}
		if want := fmt.Sprintf("https://i.example.org/thumb%d.jpg", i+1); im.Thumb != want {
			t.Errorf("thumb[%d] = %q, want %q", i, im.Thumb, want)
		}
	}

	// Contract: auth header, field, filename, content-type, bytes in order.
	for i, info := range reqs {
		if want := "Bearer SECRET-KEY"; info.authHeader != want {
			t.Errorf("req[%d] authHeader = %q, want %q", i, info.authHeader, want)
		}
		if want := "file"; info.fieldName != want {
			t.Errorf("req[%d] fieldName = %q, want %q", i, info.fieldName, want)
		}
		if want := fmt.Sprintf("frame-%02d.jpg", i+1); info.filename != want {
			t.Errorf("req[%d] filename = %q, want %q", i, info.filename, want)
		}
		if want := "image/jpeg"; info.contentType != want {
			t.Errorf("req[%d] contentType = %q, want %q", i, info.contentType, want)
		}
		if want := frames[i]; string(info.bytes) != string(want) {
			t.Errorf("req[%d] bytes = %q, want %q", i, info.bytes, want)
		}
	}
}

func TestUploadErrors(t *testing.T) {
	t.Run("missing thumbnail in response", func(t *testing.T) {
		srv := uploadServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			_, _ = io.WriteString(w, `{"url":"https://i.example.org/a.jpg"}`)
		})

		spec := validSpec()
		spec.UploadURL = srv.URL
		imgs, err := Upload(context.Background(), spec, "K", [][]byte{[]byte("x")})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if imgs[0].Thumb != imgs[0].Full {
			t.Errorf("Thumb = %q, should be equal to Full %q", imgs[0].Thumb, imgs[0].Full)
		}
	})

	t.Run("thumbnail_path pointing to empty string", func(t *testing.T) {
		srv := uploadServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			_, _ = io.WriteString(w, `{"url":"https://i.example.org/a.jpg","thumbnail_url":""}`)
		})

		spec := validSpec()
		spec.UploadURL = srv.URL
		imgs, err := Upload(context.Background(), spec, "K", [][]byte{[]byte("x")})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if imgs[0].Thumb != imgs[0].Full {
			t.Errorf("Thumb = %q, should be equal to Full %q", imgs[0].Thumb, imgs[0].Full)
		}
	})

	t.Run("nested path data.link", func(t *testing.T) {
		srv := uploadServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			_, _ = io.WriteString(w, `{"data":{"link":"https://i.example.org/nested.jpg"}}`)
		})

		spec := validSpec()
		spec.UploadURL = srv.URL
		spec.URLPath = "data.link"
		spec.ThumbnailPath = ""
		imgs, err := Upload(context.Background(), spec, "K", [][]byte{[]byte("x")})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if imgs[0].Full != "https://i.example.org/nested.jpg" {
			t.Errorf("Full = %q, want https://i.example.org/nested.jpg", imgs[0].Full)
		}
	})

	t.Run("numeric segment data.0.url", func(t *testing.T) {
		srv := uploadServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			_, _ = io.WriteString(w, `{"data":[{"url":"https://i.example.org/arr0.jpg"}]}`)
		})

		spec := validSpec()
		spec.UploadURL = srv.URL
		spec.URLPath = "data.0.url"
		spec.ThumbnailPath = ""
		imgs, err := Upload(context.Background(), spec, "K", [][]byte{[]byte("x")})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if imgs[0].Full != "https://i.example.org/arr0.jpg" {
			t.Errorf("Full = %q, want https://i.example.org/arr0.jpg", imgs[0].Full)
		}
	})

	t.Run("302 response with Location", func(t *testing.T) {
		srv := uploadServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "https://other.example.org/")
			w.WriteHeader(http.StatusFound)
		})

		spec := validSpec()
		spec.UploadURL = srv.URL
		_, err := Upload(context.Background(), spec, "K", [][]byte{[]byte("x")})
		if !errors.Is(err, ErrRedirect) {
			t.Errorf("error = %v, want ErrRedirect", err)
		}
	})

	t.Run("status 400", func(t *testing.T) {
		srv := uploadServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"error":"bad request"}`)
		})

		spec := validSpec()
		spec.UploadURL = srv.URL
		_, err := Upload(context.Background(), spec, "K", [][]byte{[]byte("x")})
		var se *StatusError
		if !errors.As(err, &se) {
			t.Fatalf("error = %v, want *StatusError", err)
		}
		if se.Status != 400 {
			t.Errorf("Status = %d, want 400", se.Status)
		}
	})

	t.Run("status 201 accepted", func(t *testing.T) {
		srv := uploadServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(201)
			_, _ = io.WriteString(w, `{"url":"https://i.example.org/ok.jpg"}`)
		})

		spec := validSpec()
		spec.UploadURL = srv.URL
		imgs, err := Upload(context.Background(), spec, "K", [][]byte{[]byte("x")})
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if imgs[0].Full != "https://i.example.org/ok.jpg" {
			t.Errorf("Full = %q, want https://i.example.org/ok.jpg", imgs[0].Full)
		}
	})

	t.Run("200 with JSON without URL in path", func(t *testing.T) {
		srv := uploadServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			_, _ = io.WriteString(w, `{"data":{"other":"value"}}`)
		})

		spec := validSpec()
		spec.UploadURL = srv.URL
		_, err := Upload(context.Background(), spec, "K", [][]byte{[]byte("x")})
		if !errors.Is(err, ErrNoURL) {
			t.Errorf("error = %v, want ErrNoURL", err)
		}
	})

	t.Run("200 with non-JSON body", func(t *testing.T) {
		srv := uploadServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(200)
			_, _ = io.WriteString(w, "<html>not json</html>")
		})

		spec := validSpec()
		spec.UploadURL = srv.URL
		_, err := Upload(context.Background(), spec, "K", [][]byte{[]byte("x")})
		if !errors.Is(err, ErrNoURL) {
			t.Errorf("error = %v, want ErrNoURL", err)
		}
	})
}

func TestUploadTimeoutReal(t *testing.T) {
	release := make(chan struct{})
	srv := uploadServer(t, func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = io.WriteString(w, `{"url":"https://i.example.org/a.jpg"}`)
	})
	t.Cleanup(func() { close(release) })
	spec := validSpec()
	spec.UploadURL = srv.URL
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := Upload(ctx, spec, "K", [][]byte{[]byte("x")})
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
}

func TestUploadLimitResponse64KiB(t *testing.T) {
	srv := uploadServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat(" ", 64<<10)+`{"url":"https://i.example.org/a.jpg"}`)
	})
	spec := validSpec()
	spec.UploadURL = srv.URL
	_, err := Upload(context.Background(), spec, "K", [][]byte{[]byte("x")})
	if !errors.Is(err, ErrNoURL) {
		t.Errorf("error = %v, want ErrNoURL for response truncated at the 64 KiB limit", err)
	}
}

func TestHost(t *testing.T) {
	cases := []struct {
		url, want string
	}{
		{"https://img.example.org/api/upload", "img.example.org"},
		{"https://sub.domain.com/path", "sub.domain.com"},
		{"invalid", ""},
	}
	for _, c := range cases {
		s := Spec{UploadURL: c.url}
		if got := s.Host(); got != c.want {
			t.Errorf("Host(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}
