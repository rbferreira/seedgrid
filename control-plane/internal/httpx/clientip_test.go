package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestClientIP covers #84: the source IP must come from the proxy header
// (X-Real-IP first — single, non-spoofable value, then XFF), never from
// RemoteAddr when behind nginx. Without headers, fall back to RemoteAddr.
func TestClientIP(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{
			name:    "X-Real-IP takes precedence over XFF and RemoteAddr",
			headers: map[string]string{"X-Real-IP": "192.168.0.114", "X-Forwarded-For": "192.168.0.114, 172.18.0.4"},
			remote:  "172.18.0.4:56789",
			want:    "192.168.0.114",
		},
		{
			name:    "XFF is used when there is no X-Real-IP (first in the chain)",
			headers: map[string]string{"X-Forwarded-For": "192.168.0.114, 172.18.0.4"},
			remote:  "172.18.0.4:56789",
			want:    "192.168.0.114",
		},
		{
			name:    "XFF without comma",
			headers: map[string]string{"X-Forwarded-For": "192.168.0.114"},
			remote:  "172.18.0.4:56789",
			want:    "192.168.0.114",
		},
		{
			name:    "no headers → RemoteAddr (split host:port)",
			headers: map[string]string{},
			remote:  "192.168.0.114:9100",
			want:    "192.168.0.114",
		},
		{
			name:    "no headers and RemoteAddr without port",
			headers: map[string]string{},
			remote:  "192.168.0.114",
			want:    "192.168.0.114",
		},
		{
			name:    "empty X-Real-IP is ignored (falls back to XFF)",
			headers: map[string]string{"X-Real-IP": "", "X-Forwarded-For": "10.0.0.5"},
			remote:  "172.18.0.4:1234",
			want:    "10.0.0.5",
		},
		{
			name:    "X-Real-IP 'unknown' is rejected → RemoteAddr (review #85)",
			headers: map[string]string{"X-Real-IP": "unknown"},
			remote:  "172.18.0.4:56789",
			want:    "172.18.0.4",
		},
		{
			name:    "X-Real-IP with port is rejected → RemoteAddr (review #85)",
			headers: map[string]string{"X-Real-IP": "192.168.0.50:9999"},
			remote:  "172.18.0.4:56789",
			want:    "172.18.0.4",
		},
		{
			name:    "invalid X-Real-IP + valid XFF → uses XFF (review #85)",
			headers: map[string]string{"X-Real-IP": "unknown", "X-Forwarded-For": "192.168.0.114, 172.18.0.4"},
			remote:  "172.18.0.4:56789",
			want:    "192.168.0.114",
		},
		{
			name:    "invalid first XFF item → falls back to RemoteAddr (does not use 2nd item)",
			headers: map[string]string{"X-Forwarded-For": "not-an-ip, 192.168.0.114"},
			remote:  "172.18.0.4:56789",
			want:    "172.18.0.4",
		},
		{
			name:    "valid IPv6 in X-Real-IP is accepted",
			headers: map[string]string{"X-Real-IP": "2001:db8::1"},
			remote:  "172.18.0.4:56789",
			want:    "2001:db8::1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
			req.RemoteAddr = tt.remote
			for k, v := range tt.headers {
				if v == "" {
					continue // missing header, not empty
				}
				req.Header.Set(k, v)
			}
			if got := ClientIP(req); got != tt.want {
				t.Errorf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
