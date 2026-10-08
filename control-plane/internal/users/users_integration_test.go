package users_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/auth"
	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/users"
)

func setup(t *testing.T) (*users.Service, *auth.Service, string) {
	t.Helper()
	// OWN database for the package: auth_test also drops the schema in it, and the
	// packages run in parallel — sharing the same database would break each other.
	dsn := os.Getenv("SEEDGRID_USERS_TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("SEEDGRID_USERS_TEST_DATABASE_URL/SEEDGRID_TEST_DATABASE_URL not defined; skipping integration test")
	}
	// Clean database per execution: the last admin test requires deterministic state
	// (exactly 1 active Administrator). Extensions return in migration 00001.
	poolRaw, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := poolRaw.Exec(context.Background(), `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		poolRaw.Close()
		t.Fatalf("clean schema: %v", err)
	}
	poolRaw.Close()

	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	authSvc := auth.NewService(pool, "reg-token")
	admin := fmt.Sprintf("admin_%d", time.Now().UnixNano())
	if err := authSvc.Bootstrap(context.Background(), admin, "strong-password"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return users.NewService(pool), authSvc, admin
}

// uniqueUsername returns a unique username per call (parallelism between tests).
func uniqueUsername(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
}

func TestCreateListGetUpdateDelete(t *testing.T) {
	svc, authSvc, admin := setup(t)
	ctx := context.Background()
	username := uniqueUsername("maria")

	// Creates a viewer.
	u, err := svc.Create(ctx, users.CreateInput{
		Username: username,
		Password: "secure-password",
		Roles:    []string{"Viewer"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.Username != username || u.Status != "ACTIVE" {
		t.Errorf("incorrectly created user: %+v", u)
	}
	if len(u.Roles) != 1 || u.Roles[0] != "Viewer" {
		t.Errorf("roles = %v, want [Viewer]", u.Roles)
	}

	// maria's login and loaded permission verification.
	res, err := authSvc.Login(ctx, username, "secure-password", "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	authed, err := authSvc.Authenticate(ctx, res.Token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if !authed.HasPermission("torrents:read") {
		t.Error("viewer should have torrents:read")
	}
	if authed.HasPermission("torrents:write") {
		t.Error("viewer SHOULD NOT have torrents:write")
	}
	if authed.HasPermission("users:write") {
		t.Error("viewer SHOULD NOT have users:write")
	}

	// Admin login: full permission via Administrator role.
	adminRes, err := authSvc.Login(ctx, admin, "strong-password", "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("login admin: %v", err)
	}
	adminAuthed, err := authSvc.Authenticate(ctx, adminRes.Token)
	if err != nil {
		t.Fatalf("authenticate admin: %v", err)
	}
	if !adminAuthed.HasPermission("users:write") {
		t.Error("admin should have users:write")
	}
	if !adminAuthed.HasPermission("any:thing") {
		t.Error("admin should have any permission")
	}

	// List includes admin + maria.
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) < 2 {
		t.Errorf("list should have >= 2 users, has %d", len(list))
	}

	// Promotes maria to publisher.
	rolesPub := []string{"Publisher"}
	up, err := svc.Update(ctx, u.ID, users.UpdateInput{Roles: &rolesPub})
	if err != nil {
		t.Fatalf("update roles: %v", err)
	}
	if len(up.Roles) != 1 || up.Roles[0] != "Publisher" {
		t.Errorf("roles after update = %v, want [Publisher]", up.Roles)
	}

	// roles: [] clears the roles (valid state — ADR-035 review, bug #4).
	rolesEmpty := []string{}
	cleared, err := svc.Update(ctx, u.ID, users.UpdateInput{Roles: &rolesEmpty})
	if err != nil {
		t.Fatalf("empty roles update: %v", err)
	}
	if len(cleared.Roles) != 0 {
		t.Errorf("roles after clearing = %v, want []", cleared.Roles)
	}
	// Assigns a role again for the following steps.
	rolesPub2 := []string{"Publisher"}
	if _, err := svc.Update(ctx, u.ID, users.UpdateInput{Roles: &rolesPub2}); err != nil {
		t.Fatalf("reassign roles: %v", err)
	}

	// Password reset works.
	if err := svc.ResetPassword(ctx, u.ID, "new-password-6"); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if _, err := authSvc.Login(ctx, username, "secure-password", "", ""); err == nil {
		t.Error("old password should fail after reset")
	}
	if _, err := authSvc.Login(ctx, username, "new-password-6", "", ""); err != nil {
		t.Errorf("new password should work: %v", err)
	}

	// Delete (soft) and login starts failing.
	if err := svc.Delete(ctx, u.ID, ""); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(ctx, u.ID); err != users.ErrNotFound {
		t.Errorf("Get after delete should be ErrNotFound, got %v", err)
	}
	if _, err := authSvc.Login(ctx, username, "new-password-6", "", ""); err == nil {
		t.Error("disabled user login should fail")
	}
}

func TestCreateValidations(t *testing.T) {
	svc, _, _ := setup(t)
	ctx := context.Background()

	if _, err := svc.Create(ctx, users.CreateInput{Username: "x", Password: "123"}); err == nil {
		t.Error("short password should fail")
	}
	if _, err := svc.Create(ctx, users.CreateInput{Username: "", Password: "strong-password"}); err == nil {
		t.Error("empty username should fail")
	}
	if _, err := svc.Create(ctx, users.CreateInput{Username: "duplicate", Password: "strong-password", Roles: []string{"Nonexistent"}}); !errors.Is(err, users.ErrRoleNotFound) {
		t.Errorf("non-existent role should fail with ErrRoleNotFound, got %v", err)
	}
}

func TestLastAdminProtection(t *testing.T) {
	svc, _, admin := setup(t)
	ctx := context.Background()

	// admin (created in setup) is the only active Administrator: removing the role must fail.
	adminUser, err := svc.GetByUsername(ctx, admin)
	if err != nil {
		t.Fatalf("get admin: %v", err)
	}
	viewerRoles := []string{"Viewer"}
	_, err = svc.Update(ctx, adminUser.ID, users.UpdateInput{Roles: &viewerRoles})
	if !errors.Is(err, users.ErrLastAdmin) {
		t.Errorf("removing Administrator from last admin should fail with ErrLastAdmin, got %v", err)
	}
	// Disabling by status must also be blocked (ADR-035 review, bug #3).
	disabled := "DISABLED"
	_, err = svc.Update(ctx, adminUser.ID, users.UpdateInput{Status: &disabled})
	if !errors.Is(err, users.ErrLastAdmin) {
		t.Errorf("disabling last admin by status should fail with ErrLastAdmin, got %v", err)
	}
	if err := svc.Delete(ctx, adminUser.ID, ""); !errors.Is(err, users.ErrLastAdmin) {
		t.Errorf("deleting last admin should fail with ErrLastAdmin, got %v", err)
	}
}

// TestLastAdminIgnoringSoftDeleted reproduces review bug #2: an admin
// soft-deleted in history MUST NOT count towards the last admin protection.
func TestLastAdminIgnoringSoftDeleted(t *testing.T) {
	svc, _, admin := setup(t)
	ctx := context.Background()

	// Creates a second admin.
	admin2Name := uniqueUsername("admin2")
	admin2, err := svc.Create(ctx, users.CreateInput{
		Username: admin2Name,
		Password: "strong-password",
		Roles:    []string{"Administrator"},
	})
	if err != nil {
		t.Fatalf("create admin2: %v", err)
	}

	// Soft-deletes admin2: it is NO LONGER an active administrator.
	if err := svc.Delete(ctx, admin2.ID, ""); err != nil {
		t.Fatalf("delete admin2: %v", err)
	}

	// Now the only active admin is the one from setup: removing the role must fail (409).
	adminUser, err := svc.GetByUsername(ctx, admin)
	if err != nil {
		t.Fatalf("get admin: %v", err)
	}
	viewerRoles := []string{"Viewer"}
	_, err = svc.Update(ctx, adminUser.ID, users.UpdateInput{Roles: &viewerRoles})
	if !errors.Is(err, users.ErrLastAdmin) {
		t.Errorf("protection ignored soft-deleted admin: removing role from last active admin should fail, got %v", err)
	}
	if err := svc.Delete(ctx, adminUser.ID, ""); !errors.Is(err, users.ErrLastAdmin) {
		t.Errorf("deleting last active admin with soft-deleted in history should fail, got %v", err)
	}
}

// TestRemoveRoleFromDisabledAdmin covers #82: removing the Administrator role from an
// operator ALREADY disabled is safe (it doesn't count as an active administrator), and
// must not trigger the last admin protection — even if only one active remains.
func TestRemoveRoleFromDisabledAdmin(t *testing.T) {
	svc, _, _ := setup(t)
	ctx := context.Background()

	// Second admin, then disabled by status (not soft delete).
	admin2, err := svc.Create(ctx, users.CreateInput{
		Username: uniqueUsername("admin2"),
		Password: "strong-password",
		Roles:    []string{"Administrator"},
	})
	if err != nil {
		t.Fatalf("create admin2: %v", err)
	}
	disabled := "DISABLED"
	if _, err := svc.Update(ctx, admin2.ID, users.UpdateInput{Status: &disabled}); err != nil {
		t.Fatalf("disable admin2: %v", err)
	}

	// The setup admin is the only ACTIVE one. Removing the role from admin2 (disabled)
	// doesn't leave the cluster without an administrator, so it must pass.
	viewerRoles := []string{"Viewer"}
	got, err := svc.Update(ctx, admin2.ID, users.UpdateInput{Roles: &viewerRoles})
	if err != nil {
		t.Fatalf("removing role from disabled admin should work, got %v", err)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "Viewer" {
		t.Errorf("roles = %v, want [Viewer]", got.Roles)
	}
}

// TestDeleteDisabledAdmin covers #82 on the Delete path: soft-deleting an
// administrator ALREADY disabled is safe — it doesn't count as an active admin, so
// the last admin guard must not trigger.
func TestDeleteDisabledAdmin(t *testing.T) {
	svc, _, _ := setup(t)
	ctx := context.Background()

	admin2 := createDisabledAdmin(t, svc)
	if err := svc.Delete(ctx, admin2.ID, ""); err != nil {
		t.Fatalf("deleting disabled admin should work, got %v", err)
	}
	if _, err := svc.Get(ctx, admin2.ID); !errors.Is(err, users.ErrNotFound) {
		t.Errorf("admin2 should be soft-deleted, got %v", err)
	}
}

// TestLockDisabledAdmin covers #82 on the Update-status path: changing the status
// of an already disabled admin (DISABLED → LOCKED) must not trigger the guard.
func TestLockDisabledAdmin(t *testing.T) {
	svc, _, _ := setup(t)
	ctx := context.Background()

	admin2 := createDisabledAdmin(t, svc)
	locked := "LOCKED"
	got, err := svc.Update(ctx, admin2.ID, users.UpdateInput{Status: &locked})
	if err != nil {
		t.Fatalf("locking disabled admin should work, got %v", err)
	}
	if got.Status != "LOCKED" {
		t.Errorf("status = %q, want LOCKED", got.Status)
	}
}

// createDisabledAdmin creates an Administrator and disables it by status — the setup
// admin remains as the only ACTIVE administrator.
func createDisabledAdmin(t *testing.T, svc *users.Service) *users.User {
	t.Helper()
	ctx := context.Background()
	u, err := svc.Create(ctx, users.CreateInput{
		Username: uniqueUsername("admin2"),
		Password: "strong-password",
		Roles:    []string{"Administrator"},
	})
	if err != nil {
		t.Fatalf("create admin2: %v", err)
	}
	disabled := "DISABLED"
	if _, err := svc.Update(ctx, u.ID, users.UpdateInput{Status: &disabled}); err != nil {
		t.Fatalf("disable admin2: %v", err)
	}
	return u
}

// TestStatusChangeRevokesSessions reproduces review bug #1: changing the status
// from ACTIVE to DISABLED immediately invalidates the user's sessions.
func TestStatusChangeRevokesSessions(t *testing.T) {
	svc, authSvc, _ := setup(t)
	ctx := context.Background()

	username := uniqueUsername("joao")
	u, err := svc.Create(ctx, users.CreateInput{
		Username: username,
		Password: "secure-password",
		Roles:    []string{"Viewer"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Login works before.
	res, err := authSvc.Login(ctx, username, "secure-password", "127.0.0.1", "go-test")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := authSvc.Authenticate(ctx, res.Token); err != nil {
		t.Fatalf("authenticate before: %v", err)
	}

	// Disables by status.
	disabled := "DISABLED"
	if _, err := svc.Update(ctx, u.ID, users.UpdateInput{Status: &disabled}); err != nil {
		t.Fatalf("update status: %v", err)
	}

	// Live token must be rejected (session invalidated AND Authenticate filters status).
	if _, err := authSvc.Authenticate(ctx, res.Token); err == nil {
		t.Error("old token should fail after DISABLED status")
	}
	// New login also fails.
	if _, err := authSvc.Login(ctx, username, "secure-password", "", ""); err == nil {
		t.Error("DISABLED user login should fail")
	}
}
