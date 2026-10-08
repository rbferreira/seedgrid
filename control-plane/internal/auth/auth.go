// Package auth handles operator authentication (session via Bearer token) and the
// validation of Agent registration tokens. The Control Plane is the identity
// authority of the Cluster (Authentication.md §15).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is returned when login or token are invalid.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Insecure default credentials (test environment). When the operator logs in with
// them, the API signals that the default password must be changed (Deployment.md §35).
const (
	defaultAdminUsername = "admin"
	defaultAdminPassword = "admin"
)

// User represents an authenticated operator (ADR-035: loads roles and permissions).
type User struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
}

// HasPermission checks if the operator has the `resource:action` permission
// (ADR-035). A `*` wildcard on either side covers the entire resource or action:
// `nodes:*` authorizes `nodes:read`, and `*:*` authorizes any permission.
// The `Administrator` system role resolves to full access, even if the seed
// of `role_permissions` is incomplete (defense in depth).
func (u *User) HasPermission(perm string) bool {
	for _, r := range u.Roles {
		if r == "Administrator" {
			return true
		}
	}
	for _, p := range u.Permissions {
		if permissionMatches(p, perm) {
			return true
		}
	}
	return false
}

// permissionMatches compares a user's permission (got) with the one required by the
// route (want). Wildcards apply ONLY on the user's side: `*:*`, `*:read`,
// user's `nodes:*` cover the requested permission; a wildcard on the route side
// (want) does NOT grant anything on its own (ADR-035 review — a route declaring `*:*` by
// mistake cannot become full access).
func permissionMatches(got, want string) bool {
	gr, ga := splitPermission(got)
	wr, wa := splitPermission(want)
	// A route requesting a wildcard (poorly declared) only matches if the user also has a
	// wildcard in that position — `torrents:read` does not cover `*:*`.
	if wr == "*" && gr != "*" {
		return false
	}
	if wa == "*" && ga != "*" {
		return false
	}
	if gr != "*" && wr != "*" && gr != wr {
		return false
	}
	if ga != "*" && wa != "*" && ga != wa {
		return false
	}
	return true
}

func splitPermission(p string) (string, string) {
	for i := 0; i < len(p); i++ {
		if p[i] == ':' {
			return p[:i], p[i+1:]
		}
	}
	return p, ""
}

// Service centralizes authentication operations.
type Service struct {
	pool          *pgxpool.Pool
	agentRegToken string
	sessionTTL    time.Duration
}

// NewService creates the authentication service.
func NewService(pool *pgxpool.Pool, agentRegToken string) *Service {
	return &Service{pool: pool, agentRegToken: agentRegToken, sessionTTL: 24 * time.Hour}
}

// HashPassword generates the bcrypt hash of a password.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// LoginResult is the outcome of a successful login.
type LoginResult struct {
	Token   string
	Expires time.Time
	// DefaultCredentials indicates that the operator logged in with the insecure
	// default credentials (admin/admin) and must change the password.
	DefaultCredentials bool
}

// Login validates user/password and creates a session.
func (s *Service) Login(ctx context.Context, username, password, ip, userAgent string) (LoginResult, error) {
	var id, hash, status string
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, password_hash, status FROM users WHERE username = $1 AND deleted_at IS NULL`,
		username).Scan(&id, &hash, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("query user: %w", err)
	}
	if status != "ACTIVE" {
		return LoginResult{}, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	token, err := generateToken()
	if err != nil {
		return LoginResult{}, err
	}
	expires := time.Now().UTC().Add(s.sessionTTL)
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (user_id, session_token, ip_address, user_agent, expires_at)
		 VALUES ($1, $2, NULLIF($3, '')::inet, NULLIF($4, ''), $5)`,
		id, token, ip, userAgent, expires); err != nil {
		return LoginResult{}, fmt.Errorf("create session: %w", err)
	}
	_, _ = s.pool.Exec(ctx, `UPDATE users SET last_login_at = now() WHERE id = $1`, id)

	defaultCreds := username == defaultAdminUsername && password == defaultAdminPassword
	return LoginResult{Token: token, Expires: expires, DefaultCredentials: defaultCreds}, nil
}

// ErrWeakPassword indicates the new password is below the minimum.
var ErrWeakPassword = errors.New("password too short (minimum 6 characters)")

// ChangePassword changes the operator's password, requiring the correct current password.
func (s *Service) ChangePassword(ctx context.Context, userID, current, newPassword string) error {
	if len(newPassword) < 6 {
		return ErrWeakPassword
	}
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidCredentials
	}
	if err != nil {
		return fmt.Errorf("query user: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) != nil {
		return ErrInvalidCredentials
	}
	nh, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, userID, nh); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// Authenticate validates a session token and returns the corresponding operator,
// loading roles and permissions (ADR-035) for the authorization middleware.
// Blocks non-ACTIVE users (immediate access revocation, even with a live
// token) and soft-deleted ones — the defense applies to any path that changes status.
func (s *Service) Authenticate(ctx context.Context, token string) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT u.id::text, u.username,
		        COALESCE((
		            SELECT array_agg(DISTINCT r.name ORDER BY r.name)
		            FROM user_roles ur
		            JOIN roles r ON r.id = ur.role_id
		            WHERE ur.user_id = u.id
		        ), '{}') AS roles,
		        COALESCE((
		            SELECT array_agg(DISTINCT p.resource || ':' || p.action)
		            FROM user_roles ur
		            JOIN role_permissions rp ON rp.role_id = ur.role_id
		            JOIN permissions p ON p.id = rp.permission_id
		            WHERE ur.user_id = u.id
		        ), '{}') AS permissions
		   FROM sessions s
		   JOIN users u ON u.id = s.user_id
		  WHERE s.session_token = $1 AND s.expires_at > now() AND u.deleted_at IS NULL
		    AND u.status = 'ACTIVE'`,
		token).Scan(&u.ID, &u.Username, &u.Roles, &u.Permissions)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	// last_activity with throttle (#114): the Portal authenticates on EVERY request (polling
	// every 8s × N tabs) and writing a timestamp per request is worthless WAL/bloat — no one
	// needs second-level precision. It only writes if the mark is outdated; within the
	// window the UPDATE matches 0 rows and writes nothing.
	_, _ = s.pool.Exec(ctx,
		`UPDATE sessions SET last_activity = now()
		  WHERE session_token = $1
		    AND (last_activity IS NULL OR last_activity < now() - interval '60 seconds')`, token)
	return &u, nil
}

// AgentRegistrationTokenValid compares, in constant time, the registration token
// presented by an Agent with the configured token.
func (s *Service) AgentRegistrationTokenValid(token string) bool {
	if s.agentRegToken == "" || token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.agentRegToken)) == 1
}

// SystemRoles are the system roles guaranteed by Bootstrap (ADR-035).
var SystemRoles = []struct{ Name, Description string }{
	{"Administrator", "Full administrative access"},
	{"Publisher", "Manages content: torrents, trackers, publishing"},
	{"Downloader", "Consumes content: reads and downloads torrents"},
	{"Viewer", "Read-only: observes the cluster without mutating anything"},
}

// EnsureSystemRoles idempotently guarantees the system roles (ADR-035).
func (s *Service) EnsureSystemRoles(ctx context.Context) error {
	for _, r := range SystemRoles {
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO roles (name, description, system)
			 VALUES ($1, $2, true)
			 ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description`,
			r.Name, r.Description); err != nil {
			return fmt.Errorf("ensure role %s: %w", r.Name, err)
		}
	}
	return nil
}

// RoleIDByName resolves a role's id by name (system or custom roles).
func (s *Service) RoleIDByName(ctx context.Context, name string) (string, error) {
	var id string
	if err := s.pool.QueryRow(ctx,
		`SELECT id::text FROM roles WHERE name = $1`, name).Scan(&id); err != nil {
		return "", fmt.Errorf("query role %s: %w", name, err)
	}
	return id, nil
}

// Bootstrap idempotently guarantees the system roles and the initial
// administrator user (Deployment.md §35: default credentials must be changed).
func (s *Service) Bootstrap(ctx context.Context, username, password string) error {
	if err := s.EnsureSystemRoles(ctx); err != nil {
		return err
	}

	roleID, err := s.RoleIDByName(ctx, "Administrator")
	if err != nil {
		return err
	}

	var userID string
	err = s.pool.QueryRow(ctx,
		`SELECT id::text FROM users WHERE username = $1`, username).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		hash, herr := HashPassword(password)
		if herr != nil {
			return fmt.Errorf("admin password hash: %w", herr)
		}
		if err := s.pool.QueryRow(ctx,
			`INSERT INTO users (username, display_name, password_hash, status)
			 VALUES ($1, $2, $3, 'ACTIVE')
			 RETURNING id::text`,
			username, username, hash).Scan(&userID); err != nil {
			return fmt.Errorf("create admin user: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("query admin user: %w", err)
	}

	if _, err := s.pool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, roleID); err != nil {
		return fmt.Errorf("assign role to admin: %w", err)
	}
	return nil
}
