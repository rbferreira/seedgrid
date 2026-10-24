package rtorrent

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxResponse bounds what the Agent reads from rTorrent: a d.multicall2 over a few thousand
// torrents is a few MB of XML.
const maxResponse = 64 << 20

// client calls rTorrent's XML-RPC, over SCGI (a Unix socket or a TCP port) or HTTP (ADR-045 §2):
//
//	scgi:///home/user/.rtorrent/rpc.socket
//	scgi://127.0.0.1:5000
//	https://user:pass@seedbox.example/RPC2
type client struct {
	network, addr string // SCGI
	httpURL       string // HTTP: the URL without the credentials
	user, pass    string
	http          *http.Client
	timeout       time.Duration
}

func newClient(rawURL string) (*client, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("rtorrent: bad URL: %w", err)
	}
	c := &client{timeout: 30 * time.Second}
	switch u.Scheme {
	case "scgi":
		if u.Host == "" {
			if u.Path == "" {
				return nil, fmt.Errorf("rtorrent: scgi URL without a socket path or host:port")
			}
			c.network, c.addr = "unix", u.Path
		} else {
			c.network, c.addr = "tcp", u.Host
		}
	case "http", "https":
		if u.User != nil {
			c.user = u.User.Username()
			c.pass, _ = u.User.Password()
			u.User = nil
		}
		c.httpURL = u.String()
		c.http = &http.Client{Timeout: c.timeout}
	default:
		return nil, fmt.Errorf("rtorrent: URL scheme %q (want scgi, http or https)", u.Scheme)
	}
	return c, nil
}

// call runs one XML-RPC method and returns its decoded value.
func (c *client) call(ctx context.Context, method string, params ...any) (any, error) {
	body, err := encodeCall(method, params...)
	if err != nil {
		return nil, err
	}
	var resp io.Reader
	if c.http != nil {
		data, err := c.postHTTP(ctx, body)
		if err != nil {
			return nil, err
		}
		resp = bytes.NewReader(data)
	} else {
		data, err := c.postSCGI(ctx, body)
		if err != nil {
			return nil, err
		}
		resp = bytes.NewReader(data)
	}
	return decodeResponse(resp)
}

func (c *client) postHTTP(ctx context.Context, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.httpURL, bytes.NewReader(body)) //nolint:gosec // G704: the URL is the operator's SEEDGRID_ENGINE_RTORRENT_URL
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml") // rTorrent 0.16 tells XML-RPC from JSON-RPC by it
	if c.user != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	res, err := c.http.Do(req) //nolint:gosec // G704: the URL is the operator's SEEDGRID_ENGINE_RTORRENT_URL
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(res.Body, maxResponse))
	if err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rtorrent: HTTP status %d", res.StatusCode)
	}
	return data, nil
}

// postSCGI sends one request on a new connection, as SCGI does: a netstring of headers, the
// body, then rTorrent answers with CGI-style headers and closes the connection.
func (c *client) postSCGI(ctx context.Context, body []byte) ([]byte, error) {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, c.network, c.addr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)

	// CONTENT_TYPE matters: rTorrent 0.16 also speaks JSON-RPC and drops a request that does not
	// say it is XML.
	headers := "CONTENT_LENGTH\x00" + strconv.Itoa(len(body)) + "\x00SCGI\x001\x00REQUEST_METHOD\x00POST\x00REQUEST_URI\x00/RPC2\x00CONTENT_TYPE\x00text/xml\x00"
	req := make([]byte, 0, len(headers)+len(body)+16)
	req = append(req, strconv.Itoa(len(headers))+":"+headers+","...)
	req = append(req, body...)
	if _, err := conn.Write(req); err != nil {
		return nil, err
	}

	r := bufio.NewReader(io.LimitReader(conn, maxResponse))
	h, err := textproto.NewReader(r).ReadMIMEHeader()
	if err != nil {
		return nil, fmt.Errorf("rtorrent: SCGI response: %w", err)
	}
	if st := h.Get("Status"); st != "" && !strings.HasPrefix(st, "200") {
		return nil, fmt.Errorf("rtorrent: SCGI status %s", st)
	}
	return io.ReadAll(r)
}

// multicall runs calls in one round trip (system.multicall). Each result is the call's value, or
// a *Fault for a call rTorrent refused; the others still come back.
func (c *client) multicall(ctx context.Context, calls []rpcCall) ([]any, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	list := make([]any, len(calls))
	for i, cl := range calls {
		params := cl.params
		if params == nil {
			params = []any{}
		}
		list[i] = map[string]any{"methodName": cl.method, "params": params}
	}
	v, err := c.call(ctx, "system.multicall", list)
	if err != nil {
		return nil, err
	}
	res, ok := v.([]any)
	if !ok || len(res) != len(calls) {
		return nil, fmt.Errorf("rtorrent: system.multicall returned %d results for %d calls", len(res), len(calls))
	}
	out := make([]any, len(res))
	for i, r := range res {
		switch x := r.(type) {
		case []any: // success: the value wrapped in a one-element array
			if len(x) == 1 {
				out[i] = x[0]
			}
		case map[string]any: // a fault
			code, _ := x["faultCode"].(int64)
			msg, _ := x["faultString"].(string)
			out[i] = &Fault{Code: int(code), Message: msg}
		}
	}
	return out, nil
}

// rpcCall is one entry of a multicall.
type rpcCall struct {
	method string
	params []any
}
