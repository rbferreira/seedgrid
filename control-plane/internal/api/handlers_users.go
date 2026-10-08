// Identity and operator handlers (ADR-035): login, password change, authenticated
// profile and user/role management (admin).

package api

import (
	"errors"
	"net/http"

	"github.com/rbferreira/seedgrid/control-plane/internal/auth"
	"github.com/rbferreira/seedgrid/control-plane/internal/httpx"
	"github.com/rbferreira/seedgrid/control-plane/internal/users"
)

func handleLogin(d Deps) http.HandlerFunc {
	type request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		if req.Username == "" || req.Password == "" {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "username and password are required")
			return
		}
		res, err := d.Auth.Login(r.Context(), req.Username, req.Password, httpx.ClientIP(r), r.UserAgent())
		if err != nil {
			// Audit of failed attempt (ADR-025) — security/accountability.
			if d.Audit != nil {
				d.Audit.RecordAction(r.Context(), req.Username, "Failed login attempt", "login_failed", nil, "POST", "/api/v1/auth/login", http.StatusUnauthorized, httpx.ClientIP(r))
			}
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
			return
		}
		if d.Audit != nil {
			d.Audit.RecordAction(r.Context(), req.Username, "Logged in to Portal", "login", nil, "POST", "/api/v1/auth/login", http.StatusOK, httpx.ClientIP(r))
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"token":               res.Token,
			"expires_at":          res.Expires.Format("2006-01-02T15:04:05Z07:00"),
			"default_credentials": res.DefaultCredentials,
		})
	}
}

func handleChangePassword(d Deps) http.HandlerFunc {
	type request struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		err := d.Auth.ChangePassword(r.Context(), user.ID, req.CurrentPassword, req.NewPassword)
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			httpx.WriteError(w, http.StatusBadRequest, "wrong_password", "incorrect current password")
		case errors.Is(err, auth.ErrWeakPassword):
			httpx.WriteError(w, http.StatusBadRequest, "weak_password", "new password too short (min. 6)")
		case err != nil:
			d.Logger.Error("change password", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to change password")
		default:
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		}
	}
}

// handleMe returns the authenticated operator's profile, including roles and
// permissions (ADR-035) — the UI uses it to hide actions without permission.
func handleMe(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpx.OperatorFromContext(r.Context())
		if !ok || user == nil {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "unauthenticated")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, user)
	}
}

// handleListUsers (admin): lists operators with roles (ADR-035).
func handleListUsers(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := d.Users.List(r.Context())
		if err != nil {
			d.Logger.Error("list users", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list users")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": list})
	}
}

// handleGetUser (admin): returns an operator with roles.
func handleGetUser(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := d.Users.Get(r.Context(), r.PathValue("id"))
		if errors.Is(err, users.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "user_not_found", "user not found")
			return
		}
		if err != nil {
			d.Logger.Error("fetch user", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to fetch user")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, u)
	}
}

// handleCreateUser (admin): creates an operator with roles (ADR-035).
func handleCreateUser(d Deps) http.HandlerFunc {
	type request struct {
		Username    string   `json:"username"`
		DisplayName string   `json:"display_name"`
		Email       string   `json:"email"`
		Password    string   `json:"password"`
		Roles       []string `json:"roles"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		u, err := d.Users.Create(r.Context(), users.CreateInput{
			Username:    req.Username,
			DisplayName: req.DisplayName,
			Email:       req.Email,
			Password:    req.Password,
			Roles:       req.Roles,
		})
		if errors.Is(err, users.ErrInvalidInput) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
			return
		}
		if errors.Is(err, users.ErrRoleNotFound) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
			return
		}
		if err != nil {
			d.Logger.Error("create user", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create user")
			return
		}
		httpx.SetAudit(r.Context(), "user_created", map[string]any{"username": u.Username}, "Created user "+u.Username)
		httpx.WriteJSON(w, http.StatusCreated, u)
	}
}

// handleUpdateUser (admin): updates the name/email/status/roles of an operator.
func handleUpdateUser(d Deps) http.HandlerFunc {
	type request struct {
		DisplayName *string  `json:"display_name"`
		Email       *string  `json:"email"`
		Status      *string  `json:"status"`
		Roles       []string `json:"roles"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		// Roles: absent field (nil) = do not change; present (even if empty) = set.
		var roles *[]string
		if req.Roles != nil {
			roles = &req.Roles
		}
		u, err := d.Users.Update(r.Context(), r.PathValue("id"), users.UpdateInput{
			DisplayName: req.DisplayName,
			Email:       req.Email,
			Status:      req.Status,
			Roles:       roles,
		})
		if errors.Is(err, users.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "user_not_found", "user not found")
			return
		}
		if errors.Is(err, users.ErrLastAdmin) {
			httpx.WriteError(w, http.StatusConflict, "last_admin", "cannot remove the administrator role from the last admin")
			return
		}
		if errors.Is(err, users.ErrInvalidInput) || errors.Is(err, users.ErrRoleNotFound) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
			return
		}
		if err != nil {
			d.Logger.Error("update user", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to update user")
			return
		}
		httpx.SetAudit(r.Context(), "user_updated", map[string]any{"username": u.Username}, "Updated user "+u.Username)
		httpx.WriteJSON(w, http.StatusOK, u)
	}
}

// handleDeleteUser (admin): disables an operator (soft delete) and invalidates sessions.
func handleDeleteUser(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := httpx.OperatorFromContext(r.Context())
		actorID := ""
		if actor != nil {
			actorID = actor.ID
		}
		err := d.Users.Delete(r.Context(), r.PathValue("id"), actorID)
		if errors.Is(err, users.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "user_not_found", "user not found")
			return
		}
		if errors.Is(err, users.ErrLastAdmin) {
			httpx.WriteError(w, http.StatusConflict, "last_admin", "cannot disable the last administrator")
			return
		}
		if err != nil {
			d.Logger.Error("disable user", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to disable user")
			return
		}
		httpx.SetAudit(r.Context(), "user_disabled", map[string]any{"user": r.PathValue("id")}, "Disabled user "+r.PathValue("id"))
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// handleResetUserPassword (admin): resets the password of an operator.
func handleResetUserPassword(d Deps) http.HandlerFunc {
	type request struct {
		NewPassword string `json:"new_password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "invalid body")
			return
		}
		err := d.Users.ResetPassword(r.Context(), r.PathValue("id"), req.NewPassword)
		if errors.Is(err, users.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "user_not_found", "user not found")
			return
		}
		if errors.Is(err, users.ErrInvalidInput) {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_input", err.Error())
			return
		}
		if err != nil {
			d.Logger.Error("reset password", "error", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to reset password")
			return
		}
		httpx.SetAudit(r.Context(), "user_password_reset", map[string]any{"user": r.PathValue("id")}, "Reset password for user "+r.PathValue("id"))
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
