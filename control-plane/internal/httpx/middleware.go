package httpx

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/auth"
)

type ctxKey string

const (
	userCtxKey ctxKey = "operator"
	nodeCtxKey ctxKey = "node"
)

// AgentAuthenticator resolves an Agent token to the Node id.
type AgentAuthenticator interface {
	AuthenticateAgent(ctx context.Context, token string) (string, error)
}

// registrationTokenValidator validates the registration token of Agents.
type registrationTokenValidator interface {
	AgentRegistrationTokenValid(token string) bool
}

// AgentRegistrationHeader is the header that carries the Agent registration token.
const AgentRegistrationHeader = "X-Seedgrid-Register-Token"

// Middleware is a function that wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain applies the middlewares in the order they are passed (the first is the outermost).
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// RequireOperator validates the operator's session token (Bearer) and injects the
// authenticated user into the context.
func RequireOperator(a *auth.Service) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "missing authentication token")
				return
			}
			user, err := a.Authenticate(r.Context(), token)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
				return
			}
			ctx := context.WithValue(r.Context(), userCtxKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OperatorFromContext retrieves the authenticated operator, if any.
func OperatorFromContext(ctx context.Context) (*auth.User, bool) {
	u, ok := ctx.Value(userCtxKey).(*auth.User)
	return u, ok
}

// RequirePermission requires the authenticated operator to have the permission
// `resource:action` (ADR-035). Must run INSIDE RequireOperator — the
// user needs to be in the context. Returns 403 when the permission is missing.
func RequirePermission(perm string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := OperatorFromContext(r.Context())
			if !ok || user == nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated operator")
				return
			}
			if !user.HasPermission(perm) {
				WriteErrorParams(w, http.StatusForbidden, "forbidden", "permission denied: "+perm, map[string]any{"permission": perm})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AuditSink receives audited actions (ADR-025). Implemented by *audit.Recorder;
// interface here so httpx doesn't depend on the audit domain.
type AuditSink interface {
	RecordAction(ctx context.Context, actor, action, code string, params map[string]any, method, path string, status int, ip string)
}

// auditInfo carries the friendly description that a handler can set for the
// current action (ADR-025). The middleware puts a pointer in the context; the handler
// fills it via SetAudit; the middleware reads it after the handler.
type auditInfo struct {
	action string
	code   string
	params map[string]any
}

type auditCtxKeyType struct{}

var auditCtxKey = auditCtxKeyType{}

// SetAudit describes the current action for the audit trail (ADR-025): the code and data
// from which the reader builds the phrase in their language (#57, ADR-041 §3), and the Portuguese
// text saved as a fallback — e.g.: "torrent_paused", {"name": "Debian"}, "Paused the
// torrent «Debian»". The code is literal at the call site (messages.Registry, audit trail
// checks it). Without this, the middleware uses the action derived from the route. No-op outside an
// audited request.
func SetAudit(ctx context.Context, code string, params map[string]any, text string) {
	if ai, ok := ctx.Value(auditCtxKey).(*auditInfo); ok {
		ai.code, ai.params, ai.action = code, params, text
	}
}

// AuditMutations logs every MUTATING request (POST/PUT/PATCH/DELETE) to the audit log
// from an authenticated operator (ADR-025). Must run INSIDE RequireOperator (the
// operator already injected in context). GET is not audited (read noise). It logs the
// action set by the handler (SetAudit: code + data + text) or, if missing, the route-derived action
// derived from the route — `method`/`path` are kept logged for technical detail. The sink is
// best-effort and does not block the response. Reuses the statusWriter (with Unwrap) to not
// break streaming responses.
func AuditMutations(sink AuditSink) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMutatingMethod(r.Method) || skipAuditPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			ai := &auditInfo{}
			ctx := context.WithValue(r.Context(), auditCtxKey, ai)
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r.WithContext(ctx))
			user, ok := OperatorFromContext(ctx)
			if !ok || user == nil {
				return
			}
			code, params, action := ai.code, ai.params, ai.action
			if action == "" {
				code, action = routeActionFor(r.Method, r.URL.Path)
			}
			sink.RecordAction(ctx, user.Username, action, code, params, r.Method, r.URL.Path, sw.status, ClientIP(r))
		})
	}
}

// routeAction is the label of an action derived from method+route, for when the handler hasn't set
// one (SetAudit) — e.g.: the request failed earlier. code is what the Portal uses for the phrase in
// the reader's language (#57, ADR-041); text is the fallback, in Portuguese. Matches by segments of the
// path (with ids); the first rule that matches wins.
type routeAction struct {
	code, text string
	match      func(method, p string) bool
}

var routeActions = []routeAction{
	{code: "route_torrent_pause", text: "Paused a torrent", match: func(method, p string) bool { return strings.HasSuffix(p, "/pause") }},
	{code: "route_torrent_resume", text: "Resumed a torrent", match: func(method, p string) bool { return strings.HasSuffix(p, "/resume") }},
	{code: "route_torrent_recheck", text: "Rechecked a torrent", match: func(method, p string) bool { return strings.HasSuffix(p, "/recheck") }},
	{code: "route_torrent_migrate", text: "Migrated a torrent", match: func(method, p string) bool { return strings.HasSuffix(p, "/migrate") }},
	{code: "route_migration_cancel", text: "Canceled a migration", match: func(method, p string) bool {
		return strings.HasPrefix(p, "migrations/") && strings.HasSuffix(p, "/cancel")
	}},
	{code: "route_publish_tracker", text: "Published a torrent to the tracker", match: func(method, p string) bool { return strings.HasSuffix(p, "/publish-tracker") }},
	{code: "route_download_ticket", text: "Generated a download link", match: func(method, p string) bool { return strings.HasSuffix(p, "/download-ticket") }},
	{code: "route_engine_restart", text: "Restarted a Node's Engine", match: func(method, p string) bool { return strings.HasSuffix(p, "/engine/restart") }},
	{code: "route_node_drain", text: "Drained a Node", match: func(method, p string) bool { return strings.HasSuffix(p, "/drain") }},
	{code: "route_node_command", text: "Sent a command to a Node", match: func(method, p string) bool { return strings.HasSuffix(p, "/commands") }},
	{code: "route_torrent_add", text: "Added a torrent", match: func(method, p string) bool { return p == "torrents" && method == http.MethodPost }},
	{code: "route_torrent_create", text: "Created a torrent", match: func(method, p string) bool { return p == "torrents/create" }},
	{code: "route_torrent_remove", text: "Removed a torrent", match: func(method, p string) bool { return strings.HasPrefix(p, "torrents/") && method == http.MethodDelete }},
	{code: "route_node_remove", text: "Removed a Node", match: func(method, p string) bool { return strings.HasPrefix(p, "nodes/") && method == http.MethodDelete }},
	{code: "route_enroll_token", text: "Generated a join token", match: func(method, p string) bool { return p == "enroll-tokens" }},
	{code: "route_password_change", text: "Changed password", match: func(method, p string) bool { return p == "auth/password" }},
	{code: "route_cluster_mode", text: "Changed cluster mode", match: func(method, p string) bool { return p == "cluster/operation-mode" }},
	{code: "route_node_mode", text: "Changed a Node's mode", match: func(method, p string) bool {
		return strings.HasPrefix(p, "nodes/") && strings.HasSuffix(p, "/operation-mode")
	}},
	{code: "route_node_rename", text: "Renamed a Node", match: func(method, p string) bool {
		return strings.HasPrefix(p, "nodes/") && strings.HasSuffix(p, "/display-name")
	}},
	{code: "route_setting_change", text: "Changed a setting", match: func(method, p string) bool { return strings.HasPrefix(p, "settings/") && method == http.MethodPut }},
	{code: "route_setting_reset", text: "Reset a setting to default", match: func(method, p string) bool { return strings.HasPrefix(p, "settings/") && method == http.MethodDelete }},
	{code: "route_tracker_seed", text: "Seeded a torrent from the tracker", match: func(method, p string) bool { return strings.HasSuffix(p, "/seed") }},
	{code: "route_tracker_default", text: "Set the default tracker", match: func(method, p string) bool { return strings.HasSuffix(p, "/default") }},
	{code: "route_taxonomy_apply", text: "Applied the discovered taxonomy to a tracker", match: func(method, p string) bool { return strings.HasSuffix(p, "/apply-taxonomy") }},
	{code: "route_profile_edit", text: "Edited a tracker's profile", match: func(method, p string) bool {
		return strings.HasPrefix(p, "trackers/") && strings.HasSuffix(p, "/profile") && method == http.MethodPut
	}},
	{code: "route_profile_import", text: "Imported a tracker profile", match: func(method, p string) bool {
		return strings.HasPrefix(p, "trackers/") && strings.HasSuffix(p, "/profile/import")
	}},
	{code: "route_description_edit", text: "Edited a tracker's description composition", match: func(method, p string) bool {
		return strings.HasPrefix(p, "trackers/") && strings.HasSuffix(p, "/profile/description") && method == http.MethodPut
	}},
	{code: "route_description_reset", text: "Reset a tracker's description composition to default", match: func(method, p string) bool {
		return strings.HasPrefix(p, "trackers/") && strings.HasSuffix(p, "/profile/description") && method == http.MethodDelete
	}},
	{code: "route_image_host_edit", text: "Edited a tracker's image host", match: func(method, p string) bool {
		return strings.HasPrefix(p, "trackers/") && strings.HasSuffix(p, "/profile/image-host") && method == http.MethodPut
	}},
	{code: "route_image_host_remove", text: "Removed a tracker's image host", match: func(method, p string) bool {
		return strings.HasPrefix(p, "trackers/") && strings.HasSuffix(p, "/profile/image-host") && method == http.MethodDelete
	}},
	{code: "route_image_host_key_set", text: "Registered a tracker's image host key", match: func(method, p string) bool {
		return strings.HasPrefix(p, "trackers/") && strings.HasSuffix(p, "/image-host-key") && method == http.MethodPut
	}},
	{code: "route_image_host_key_remove", text: "Removed a tracker's image host key", match: func(method, p string) bool {
		return strings.HasPrefix(p, "trackers/") && strings.HasSuffix(p, "/image-host-key") && method == http.MethodDelete
	}},
	{code: "route_tracker_add", text: "Registered a tracker", match: func(method, p string) bool { return p == "trackers" && method == http.MethodPost }},
	{code: "route_tracker_edit", text: "Edited a tracker", match: func(method, p string) bool { return strings.HasPrefix(p, "trackers/") && method == http.MethodPut }},
	{code: "route_tracker_remove", text: "Removed a tracker", match: func(method, p string) bool { return strings.HasPrefix(p, "trackers/") && method == http.MethodDelete }},
}

// routeActionFor returns the code and text of the action by route; a route without a rule gets no code
// and "METHOD path" as text.
func routeActionFor(method, path string) (code, text string) {
	p := strings.TrimPrefix(path, "/api/v1/")
	for _, a := range routeActions {
		if a.match(method, p) {
			return a.code, a.text
		}
	}
	return "", method + " " + p
}

// friendlyAction is the text (fallback) of the action by route.
func friendlyAction(method, path string) string {
	_, text := routeActionFor(method, path)
	return text
}

func isMutatingMethod(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// skipAuditPath excludes endpoints that are POST for convenience (take a body) but are
// QUERIES/inference — they don't mutate state, so auditing them is just noise (ADR-025).
func skipAuditPath(path string) bool {
	p := strings.TrimPrefix(path, "/api/v1/")
	// publish/infer and publish/description only COMPOSE text from the body: they don't change state.
	if p == "publish/infer" || p == "publish/description" {
		return true
	}
	// Discovering the taxonomy only PROPOSES (#196): reads the tracker catalog and saves nothing. The
	// operator's action is to apply the proposal, and that one gets logged in the trail. Same goes for the
	// profile import preview (#197): the action is the import.
	return strings.HasPrefix(p, "trackers/") &&
		(strings.HasSuffix(p, "/discover-taxonomy") || strings.HasSuffix(p, "/profile/import/preview"))
}

// ClientIP extracts the source IP of the request. The header is treated as untrusted
// input: it is only accepted if it parses as a valid IP (net.ParseIP), otherwise it falls
// back to the next in the chain — an X-Real-IP "unknown" or "ip:port" (common in
// proxies/CDNs) cannot break the login (INET column) or pollute the last_ip.
//
// Order: X-Real-IP (nginx overwrites it with the real $remote_addr), then
// X-Forwarded-For (first item), and finally the RemoteAddr host.
//
// WARNING: validation does NOT prevent spoofing — a header with a syntactically
// valid IP is accepted and prevails over RemoteAddr. What makes the value
// trustworthy is nginx overwriting X-Real-IP with the real $remote_addr; this
// only applies to requests arriving through the proxy. Directly on the CP (without
// nginx), the header is forgeable — do not use ClientIP as a basis for authorization
// decisions (e.g.: IP allowlist) without first validating the connection source.
func ClientIP(r *http.Request) string {
	if ip := parseIP(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first := xff
		if i := strings.IndexByte(xff, ','); i >= 0 {
			first = xff[:i]
		}
		if ip := parseIP(first); ip != "" {
			return ip
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// parseIP normalizes and validates an IP candidate. Returns "" if it is not a
// valid IP address (v4 or v6) after trimming.
func parseIP(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if net.ParseIP(s) == nil {
		return ""
	}
	return s
}

// RequireAgentRegistration validates the registration token (dedicated header) used
// by an Agent that still lacks its own credential.
func RequireAgentRegistration(v registrationTokenValidator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !v.AgentRegistrationTokenValid(r.Header.Get(AgentRegistrationHeader)) {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid registration token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAgent validates the credential (Bearer) of an already registered Agent and injects the
// Node id into the context.
func RequireAgent(a AgentAuthenticator) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "agent credential missing")
				return
			}
			nodeID, err := a.AuthenticateAgent(r.Context(), token)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid Agent credential")
				return
			}
			ctx := context.WithValue(r.Context(), nodeCtxKey, nodeID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// NodeIDFromContext retrieves the id of the authenticated Node (Agent), if any.
func NodeIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(nodeCtxKey).(string)
	return id, ok
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// RequestLogger logs every HTTP request with method, route, status and duration.
func RequestLogger(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the underlying ResponseWriter so that http.NewResponseController
// can reach the real connection (SetReadDeadline/SetWriteDeadline). Without this, long
// streaming uploads (content publishing) die on the server's timeouts.
func (s *statusWriter) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

// integrationWarnOnce prevents spam: the open webhook warning is issued once per process.
var integrationWarnOnce sync.Once

// RequireIntegrationToken protects internal webhooks (/integrations/*, #116) with a
// shared secret. Accepts the dedicated X-Seedgrid-Integration-Token header or Authorization
// Bearer (Alertmanager's http_config only knows how to send Authorization). Constant-time
// comparison. Empty token (not configured) lets it pass — compatibility with
// deploys still without the token — but warns in the log; scripts/bootstrap-controller.sh
// generates and injects the token.
func RequireIntegrationToken(token string, logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				integrationWarnOnce.Do(func() {
					if logger != nil {
						logger.Warn("integration webhooks WITHOUT token — configure SEEDGRID_INTEGRATION_TOKEN (#116)")
					}
				})
				next.ServeHTTP(w, r)
				return
			}
			got := r.Header.Get("X-Seedgrid-Integration-Token")
			if got == "" {
				got = bearerToken(r)
			}
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid integration token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
