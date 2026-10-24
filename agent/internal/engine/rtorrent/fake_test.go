package rtorrent

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// fakeRTorrent answers XML-RPC like rTorrent does, over SCGI or HTTP, from a handler per method.
// system.multicall is resolved here, call by call, as rTorrent does.
type fakeRTorrent struct {
	mu       sync.Mutex
	handlers map[string]func(params []any) (any, error)
	calls    []string // methods called, in order (system.multicall expanded)
}

func newFake() *fakeRTorrent {
	return &fakeRTorrent{handlers: map[string]func([]any) (any, error){}}
}

func (f *fakeRTorrent) on(method string, h func(params []any) (any, error)) {
	f.handlers[method] = h
}

func (f *fakeRTorrent) dispatch(method string, params []any) (any, error) {
	f.mu.Lock()
	f.calls = append(f.calls, method)
	h := f.handlers[method]
	f.mu.Unlock()
	if method == "system.multicall" {
		list, _ := params[0].([]any)
		out := make([]any, 0, len(list))
		for _, c := range list {
			m := c.(map[string]any)
			v, err := f.dispatch(m["methodName"].(string), m["params"].([]any))
			if err != nil {
				out = append(out, map[string]any{"faultCode": -501, "faultString": err.Error()})
				continue
			}
			out = append(out, []any{v})
		}
		return out, nil
	}
	if h == nil {
		return nil, fmt.Errorf("method %s not defined", method)
	}
	return h(params)
}

// respond turns a request body into a methodResponse body.
func (f *fakeRTorrent) respond(t *testing.T, body []byte) []byte {
	method, params := decodeCall(t, body)
	v, err := f.dispatch(method, params)
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0"?><methodResponse>`)
	if err != nil {
		b.WriteString(`<fault>`)
		_ = encodeValue(&b, map[string]any{"faultCode": -506, "faultString": err.Error()})
		b.WriteString(`</fault>`)
	} else {
		b.WriteString(`<params><param>`)
		if eerr := encodeValue(&b, fakeValue(v)); eerr != nil {
			t.Errorf("fake: encode %T: %v", v, eerr)
		}
		b.WriteString(`</param></params>`)
	}
	b.WriteString(`</methodResponse>`)
	return b.Bytes()
}

// fakeValue makes handler results encodable (ints of any width become int64).
func fakeValue(v any) any {
	switch x := v.(type) {
	case int:
		return int64(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = fakeValue(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = fakeValue(e)
		}
		return out
	}
	return v
}

// decodeCall reads a methodCall with the package's own value decoder.
func decodeCall(t *testing.T, body []byte) (string, []any) {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(body))
	if err := expectPath(d, "methodCall", "methodName"); err != nil {
		t.Fatalf("fake: %v", err)
	}
	method, err := text(d)
	if err != nil {
		t.Fatalf("fake: %v", err)
	}
	if err := expectPath(d, "params"); err != nil {
		t.Fatalf("fake: %v", err)
	}
	var params []any
	for {
		if _, err := nextStart(d); err != nil { // <param>, or </params>
			break
		}
		if err := expectPath(d, "value"); err != nil {
			t.Fatalf("fake: %v", err)
		}
		v, err := decodeValue(d)
		if err != nil {
			t.Fatalf("fake: %v", err)
		}
		params = append(params, v)
		if err := skipToEnd(d); err != nil { // </param>
			t.Fatalf("fake: %v", err)
		}
	}
	return method, params
}

// serveSCGI listens on a Unix socket and returns its scgi:// URL.
func (f *fakeRTorrent) serveSCGI(t *testing.T) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "rpc.socket")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.handleSCGI(t, conn)
		}
	}()
	return "scgi://" + sock
}

func (f *fakeRTorrent) handleSCGI(t *testing.T, conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	lenStr, err := r.ReadString(':')
	if err != nil {
		return
	}
	n, _ := strconv.Atoi(strings.TrimSuffix(lenStr, ":"))
	hdr := make([]byte, n+1) // the headers and the trailing comma
	if _, err := io.ReadFull(r, hdr); err != nil {
		return
	}
	kv := strings.Split(string(hdr[:n]), "\x00")
	var clen int
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i] == "CONTENT_LENGTH" {
			clen, _ = strconv.Atoi(kv[i+1])
		}
	}
	body := make([]byte, clen)
	if _, err := io.ReadFull(r, body); err != nil {
		return
	}
	resp := f.respond(t, body)
	_, _ = fmt.Fprintf(conn, "Status: 200 OK\r\nContent-Type: text/xml\r\nContent-Length: %d\r\n\r\n", len(resp))
	_, _ = conn.Write(resp)
}

// serveHTTP serves /RPC2 behind basic auth and returns the URL with the credentials.
func (f *fakeRTorrent) serveHTTP(t *testing.T, user, pass string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != user || p != pass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write(f.respond(t, body))
	}))
	t.Cleanup(srv.Close)
	return strings.Replace(srv.URL, "http://", "http://"+user+":"+pass+"@", 1) + "/RPC2"
}
