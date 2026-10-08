// Package users implements the management of Control Plane operators (ADR-035):
// creation, listing, updating, disabling, role assignment, and password
// reset. The Control Plane is the identity authority of the Cluster
// (Authentication.md §15) — no other component creates users.
package users

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/auth"
)

// ErrNotFound indicates a non-existent (or soft-deleted) user.
var ErrNotFound = errors.New("user not found")

// ErrInvalidInput indicates invalid data on creation/update.
var ErrInvalidInput = errors.New("invalid user data")

// ErrRoleNotFound indicates a non-existent role during assignment.
var ErrRoleNotFound = errors.New("role not found")

// ErrLastAdmin protects against disabling/removing the last Administrator user.
var ErrLastAdmin = errors.New("cannot disable the last administrator")

// User is the management view of an operator (without credentials).
type User struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email"`
	Status      string    `json:"status"`
	Roles       []string  `json:"roles"`
	LastLoginAt *string   `json:"last_login_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Service centralizes the management of operators.
type Service struct {
	pool *pgxpool.Pool
}

// NewService creates the user management service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// CreateInput is the data to create an operator.
type CreateInput struct {
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	Email       string   `json:"email"`
	Password    string   `json:"password"`
	Roles       []string `json:"roles"`
}

// Create creates an operator with bcrypt hash and associated roles (ADR-035).
func (s *Service) Create(ctx context.Context, in CreateInput) (*User, error) {
	if strings.TrimSpace(in.Username) == "" {
		return nil, fmt.Errorf("%w: username is required", ErrInvalidInput)
	}
	if len(in.Password) < 6 {
		return nil, fmt.Errorf("%w: password must be at least 6 characters", ErrInvalidInput)
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID string
	err = tx.QueryRow(ctx,
		`INSERT INTO users (username, display_name, email, password_hash, status)
		 VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, 'ACTIVE')
		 RETURNING id::text`,
		in.Username, in.DisplayName, in.Email, hash).Scan(&userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("%w: username or email already in use", ErrInvalidInput)
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	if err := s.setRolesTx(ctx, tx, userID, in.Roles); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return s.Get(ctx, userID)
}

// Get returns an operator by id (including roles).
func (s *Service) Get(ctx context.Context, id string) (*User, error) {
	u, err := s.getByQuery(ctx, `u.id = $1`, id)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetByUsername returns an operator by username (for the Portal to display the profile).
func (s *Service) GetByUsername(ctx context.Context, username string) (*User, error) {
	u, err := s.getByQuery(ctx, `u.username = $1`, username)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Service) getByQuery(ctx context.Context, where string, arg any) (*User, error) {
	var u User
	var lastLogin *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT u.id::text, u.username, COALESCE(u.display_name, ''), COALESCE(u.email, ''),
		        u.status,
		        u.last_login_at, u.created_at, u.updated_at,
		        COALESCE((
		            SELECT array_agg(r.name ORDER BY r.name)
		            FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		            WHERE ur.user_id = u.id
		        ), '{}') AS roles
		   FROM users u
		  WHERE `+where+` AND u.deleted_at IS NULL`,
		arg).Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Status,
		&lastLogin, &u.CreatedAt, &u.UpdatedAt, &u.Roles)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if lastLogin != nil {
		s := lastLogin.Format(time.RFC3339)
		u.LastLoginAt = &s
	}
	return &u, nil
}

// List returns active operators (not soft-deleted), with roles.
func (s *Service) List(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT u.id::text, u.username, COALESCE(u.display_name, ''), COALESCE(u.email, ''),
		        u.status,
		        u.last_login_at, u.created_at, u.updated_at,
		        COALESCE((
		            SELECT array_agg(r.name ORDER BY r.name)
		            FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		            WHERE ur.user_id = u.id
		        ), '{}') AS roles
		   FROM users u
		  WHERE u.deleted_at IS NULL
		  ORDER BY u.username`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		var lastLogin *time.Time
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.Status,
			&lastLogin, &u.CreatedAt, &u.UpdatedAt, &u.Roles); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		if lastLogin != nil {
			s := lastLogin.Format(time.RFC3339)
			u.LastLoginAt = &s
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UpdateInput contains the editable fields of an operator.
// Roles *[]string: nil = do not touch; [] = clear all roles (ADR-035 review).
type UpdateInput struct {
	DisplayName *string   `json:"display_name"`
	Email       *string   `json:"email"`
	Status      *string   `json:"status"`
	Roles       *[]string `json:"roles"`
}

// Update applies the allowed changes (name, email, status, roles).
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*User, error) {
	if in.Status != nil && !validStatus(*in.Status) {
		return nil, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Current state of the target (exists? current status?).
	var one int
	var currentStatus string
	if err := tx.QueryRow(ctx,
		`SELECT 1, status FROM users u WHERE u.id = $1 AND u.deleted_at IS NULL`,
		id).Scan(&one, &currentStatus); err != nil {
		return nil, ErrNotFound
	}

	// Protection of the last active admin (ADR-035 review, bug #3): disabling/locking
	// an administrator when they are the last active → irreversible lockout. It only applies
	// if the target is an ACTIVE administrator (#82) — those already disabled do not count.
	if in.Status != nil && *in.Status != "ACTIVE" {
		isAdmin, err := s.isActiveAdmin(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if isAdmin {
			count, err := s.activeAdminCountTx(ctx, tx)
			if err != nil {
				return nil, err
			}
			if count <= 1 {
				return nil, ErrLastAdmin
			}
		}
	}

	if in.DisplayName != nil || in.Email != nil || in.Status != nil {
		if _, err := tx.Exec(ctx,
			`UPDATE users SET
			   display_name = COALESCE($2, display_name),
			   email        = COALESCE($3, email),
			   status       = COALESCE($4, status),
			   updated_at   = now()
			 WHERE id = $1`,
			id, in.DisplayName, in.Email, in.Status); err != nil {
			return nil, fmt.Errorf("update user: %w", err)
		}
		// Immediate revocation (ADR-035 review, bug #1): status changed from ACTIVE →
		// invalidates the operator's sessions. Authenticate also blocks by
		// status (defense in depth), but clearing the session is the state correction.
		if in.Status != nil && *in.Status != "ACTIVE" && currentStatus == "ACTIVE" {
			if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id); err != nil {
				return nil, fmt.Errorf("invalidate sessions: %w", err)
			}
		}
	}

	if in.Roles != nil {
		if err := s.setRolesTx(ctx, tx, id, *in.Roles); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return s.Get(ctx, id)
}

// rowQuerier abstracts *pgxpool.Pool and pgx.Tx — the administrator checks
// run both inside a transaction and directly on the pool.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// isActiveAdmin answers whether the operator is an ACTIVE administrator — the single source of
// the question "does taking this from them leave the cluster without an admin?" (#82). Used by the three
// paths that protect the last administrator (roles, status, and delete); previously,
// each measured the target in its own way and an already disabled admin triggered the guard.
func (s *Service) isActiveAdmin(ctx context.Context, q rowQuerier, userID string) (bool, error) {
	var ok bool
	if err := q.QueryRow(ctx,
		`SELECT EXISTS(
		    SELECT 1 FROM user_roles ur
		      JOIN roles r ON r.id = ur.role_id
		      JOIN users u ON u.id = ur.user_id
		     WHERE ur.user_id = $1 AND r.name = 'Administrator'
		       AND u.deleted_at IS NULL AND u.status = 'ACTIVE')`, userID).Scan(&ok); err != nil {
		return false, fmt.Errorf("check active administrator: %w", err)
	}
	return ok, nil
}

// activeAdminCountTx counts ACTIVE administrators (not soft-deleted and status
// ACTIVE) — ignores admins from history (ADR-035 review, bug #2: the raw count
// of user_roles counted soft-deleted and broke the last admin protection).
func (s *Service) activeAdminCountTx(ctx context.Context, q rowQuerier) (int, error) {
	var count int
	if err := q.QueryRow(ctx,
		`SELECT count(DISTINCT ur.user_id)
		   FROM user_roles ur
		   JOIN roles r ON r.id = ur.role_id
		   JOIN users u ON u.id = ur.user_id
		  WHERE r.name = 'Administrator'
		    AND u.deleted_at IS NULL AND u.status = 'ACTIVE'`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count administrators: %w", err)
	}
	return count, nil
}

// setRolesTx replaces the user's roles within a transaction. Validates that
// all roles exist and protects the last active administrator. Empty `roles`
// CLEARS the roles (valid state: operator without any role/permission).
func (s *Service) setRolesTx(ctx context.Context, tx pgx.Tx, userID string, roles []string) error {
	for _, name := range roles {
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT true FROM roles WHERE name = $1`, name).Scan(&exists); err != nil {
			return fmt.Errorf("%w: %s", ErrRoleNotFound, name)
		}
	}
	// Protection: if we are removing Administrator from a user who is the last
	// active administrator (ADR-035 review, bug #2 — count ignores soft-deleted).
	// The target is also measured by "active" (#82): removing the role from an already
	// disabled admin does not leave the cluster without anyone, so it does not trigger the guard.
	if !contains(roles, "Administrator") {
		isAdmin, err := s.isActiveAdmin(ctx, tx, userID)
		if err != nil {
			return err
		}
		if isAdmin {
			admins, err := s.activeAdminCountTx(ctx, tx)
			if err != nil {
				return err
			}
			if admins <= 1 {
				return ErrLastAdmin
			}
		}
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("clear roles: %w", err)
	}
	for _, name := range roles {
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_roles (user_id, role_id)
			 SELECT $1, id FROM roles WHERE name = $2`,
			userID, name); err != nil {
			return fmt.Errorf("assign role %s: %w", name, err)
		}
	}
	return nil
}

// ResetPassword resets an operator's password (bcrypt).
func (s *Service) ResetPassword(ctx context.Context, id, newPassword string) error {
	if len(newPassword) < 6 {
		return fmt.Errorf("%w: password must be at least 6 characters", ErrInvalidInput)
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = now()
		  WHERE id = $1 AND deleted_at IS NULL`, id, hash)
	if err != nil {
		return fmt.Errorf("reset password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete disables an operator (soft delete) and invalidates active sessions.
func (s *Service) Delete(ctx context.Context, id, deletedBy string) error {
	// Protects the last ACTIVE administrator (ADR-035 review, bug #2: count
	// ignores soft-deleted and non-ACTIVE users). The target is also measured by
	// "active" (#82): deleting someone who is already disabled does not remove anyone from the post.
	isAdmin, err := s.isActiveAdmin(ctx, s.pool, id)
	if err != nil {
		return err
	}
	if isAdmin {
		admins, err := s.activeAdminCountTx(ctx, s.pool)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE users SET deleted_at = now(), deleted_by = NULLIF($2, '')::uuid, status = 'DISABLED'
		  WHERE id = $1 AND deleted_at IS NULL`, id, deletedBy)
	if err != nil {
		return fmt.Errorf("disable user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	_, _ = s.pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id)
	return nil
}

func validStatus(s string) bool {
	switch s {
	case "ACTIVE", "DISABLED", "LOCKED", "PENDING":
		return true
	}
	return false
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
