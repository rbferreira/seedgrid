package node_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
)

// The package's integration tests share a database that ACCUMULATES state (see
// setup). Thus each Node here is created with a unique hostname and display name per run, and is
// removed (soft delete) in Cleanup — without this, a second run would violate the uniqueness
// constraint of the first run's own data.
func uniq(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }

func registerNode(t *testing.T, svc *node.Service, hostname, displayName string) string {
	t.Helper()
	ctx := context.Background()
	res, err := svc.Register(ctx, node.RegisterInput{Hostname: hostname, DisplayName: displayName, Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("Register(%s): %v", hostname, err)
	}
	t.Cleanup(func() { _ = svc.Delete(context.Background(), res.NodeID, true) })
	return res.NodeID
}

// displayNameOf reads a Node's display name from the listing (what the API exposes). "" = no display name.
func displayNameOf(t *testing.T, svc *node.Service, id string) string {
	t.Helper()
	nodes, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, n := range nodes {
		if n.ID == id {
			if n.DisplayName == nil {
				return ""
			}
			return *n.DisplayName
		}
	}
	t.Fatalf("node %s is not in the listing", id)
	return ""
}

func TestSetDisplayName(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()
	hostA, hostB := uniq("dn-host-a"), uniq("dn-host-b")
	idA := registerNode(t, svc, hostA, "")
	idB := registerNode(t, svc, hostB, "")
	room, bedroom := uniq("Room"), uniq("Bedroom")

	if got := displayNameOf(t, svc, idA); got != "" {
		t.Fatalf("Newly registered Node without alias came with %q", got)
	}

	// Set: the value comes out normalized and the before/after goes back to Audit.
	res, err := svc.SetDisplayName(ctx, idA, "  "+room+"  ")
	if err != nil {
		t.Fatalf("SetDisplayName: %v", err)
	}
	if res.Hostname != hostA || res.Before != "" || res.After != room {
		t.Errorf("RenameResult = %+v, want {%s \"\" %s}", res, hostA, room)
	}
	if got := displayNameOf(t, svc, idA); got != room {
		t.Errorf("nickname = %q, want %q", got, room)
	}

	// Uniqueness among display names — case-insensitive.
	if _, err := svc.SetDisplayName(ctx, idB, strings.ToUpper(room)); !errors.Is(err, node.ErrDisplayNameTaken) {
		t.Errorf("duplicate alias (other box): err = %v, want ErrDisplayNameTaken", err)
	}
	// ...and against the HOSTNAME of another Node: two Nodes can never display the same label.
	if _, err := svc.SetDisplayName(ctx, idB, strings.ToUpper(hostA)); !errors.Is(err, node.ErrDisplayNameTaken) {
		t.Errorf("alias matching hostname of another Node: err = %v, want ErrDisplayNameTaken", err)
	}
	// Renaming to what the Node itself already has is not a conflict.
	if _, err := svc.SetDisplayName(ctx, idA, room); err != nil {
		t.Errorf("reapply own alias: %v", err)
	}
	// Swap: the Before is the previous display name.
	res, err = svc.SetDisplayName(ctx, idA, bedroom)
	if err != nil {
		t.Fatalf("change alias: %v", err)
	}
	if res.Before != room || res.After != bedroom {
		t.Errorf("RenameResult = %+v, want Before=%q After=%q", res, room, bedroom)
	}

	// Remove: "" reverts to the hostname, and the freed name can be taken by another Node.
	res, err = svc.SetDisplayName(ctx, idA, "")
	if err != nil {
		t.Fatalf("remove alias: %v", err)
	}
	if res.Before != bedroom || res.After != "" {
		t.Errorf("RenameResult = %+v, want Before=%q After=\"\"", res, bedroom)
	}
	if got := displayNameOf(t, svc, idA); got != "" {
		t.Errorf("alias after removal = %q, want empty (reverts to hostname)", got)
	}
	if _, err := svc.SetDisplayName(ctx, idB, bedroom); err != nil {
		t.Errorf("released name should be available for another Node: %v", err)
	}
}

func TestSetDisplayName_Errors(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()
	id := registerNode(t, svc, uniq("dn-err"), "")

	if _, err := svc.SetDisplayName(ctx, id, strings.Repeat("x", node.MaxDisplayNameLen+1)); !errors.Is(err, node.ErrInvalidDisplayName) {
		t.Errorf("nickname too long: err = %v, want ErrInvalidDisplayName", err)
	}
	if _, err := svc.SetDisplayName(ctx, id, "a\x00b"); !errors.Is(err, node.ErrInvalidDisplayName) {
		t.Errorf("alias with control character: err = %v, want ErrInvalidDisplayName", err)
	}
	// Invalid cannot have saved anything.
	if got := displayNameOf(t, svc, id); got != "" {
		t.Errorf("invalid alias was saved: %q", got)
	}
	if _, err := svc.SetDisplayName(ctx, "00000000-0000-0000-0000-000000000000", "Anything"); !errors.Is(err, node.ErrNodeNotFound) {
		t.Errorf("nonexistent Node: err = %v, want ErrNodeNotFound", err)
	}
}

// A removed Node's name returns to the pool — otherwise the display name of a retired machine
// would be stuck forever.
func TestSetDisplayName_DeletedNodeReleasesName(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()
	idA := registerNode(t, svc, uniq("dn-del-a"), "")
	idB := registerNode(t, svc, uniq("dn-del-b"), "")
	name := uniq("Retired")

	if _, err := svc.SetDisplayName(ctx, idA, name); err != nil {
		t.Fatalf("SetDisplayName: %v", err)
	}
	if err := svc.Delete(ctx, idA, false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.SetDisplayName(ctx, idB, name); err != nil {
		t.Errorf("removed Node name should be free: %v", err)
	}
	// And renaming a removed Node is "not found", not silent success.
	if _, err := svc.SetDisplayName(ctx, idA, "Ghost"); !errors.Is(err, node.ErrNodeNotFound) {
		t.Errorf("rename removed Node: err = %v, want ErrNodeNotFound", err)
	}
}

// The name chosen in the "Add Node" modal travels in the join token and becomes the display name on
// registration (#145).
func TestEnrollTokenCarriesDisplayName(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()
	name := uniq("Kitchen")

	tok, _, err := svc.MintEnrollToken(ctx, time.Hour, "  "+name+" ")
	if err != nil {
		t.Fatalf("MintEnrollToken: %v", err)
	}
	info, ok := svc.EnrollToken(ctx, tok)
	if !ok || info.DisplayName != name {
		t.Fatalf("EnrollToken = (%+v, %v), want DisplayName=%q", info, ok, name)
	}

	// A 2nd token with the same name IS allowed: reserving the name in the token would leave the
	// operator with no way out (closed the modal without copying the command). The dispute is resolved on
	// registration — see TestRegister_TakenDisplayNameIsDroppedNotFatal.
	if _, _, err := svc.MintEnrollToken(ctx, time.Hour, strings.ToUpper(name)); err != nil {
		t.Errorf("2nd token with same name should be allowed: %v", err)
	}

	// Registration applies the token's name.
	host := uniq("dn-enroll")
	id := registerNode(t, svc, host, info.DisplayName)
	svc.MarkEnrollTokenUsed(ctx, tok)
	if got := displayNameOf(t, svc, id); got != name {
		t.Errorf("registered Node nickname = %q, want %q", got, name)
	}
	if _, ok := svc.EnrollToken(ctx, tok); ok {
		t.Error("used token should no longer be valid")
	}
	// Now the name belongs to the Node, and remains reserved.
	if _, _, err := svc.MintEnrollToken(ctx, time.Hour, name); !errors.Is(err, node.ErrDisplayNameTaken) {
		t.Errorf("token with alias of an existing Node: err = %v, want ErrDisplayNameTaken", err)
	}
}

func TestEnrollToken_WithoutNameAndInvalidName(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()

	// No name is the original usage: nothing changes.
	tok, _, err := svc.MintEnrollToken(ctx, time.Hour, "")
	if err != nil {
		t.Fatalf("MintEnrollToken without name: %v", err)
	}
	if info, ok := svc.EnrollToken(ctx, tok); !ok || info.DisplayName != "" {
		t.Errorf("EnrollToken without name = (%+v, %v)", info, ok)
	}

	if _, _, err := svc.MintEnrollToken(ctx, time.Hour, strings.Repeat("x", node.MaxDisplayNameLen+1)); !errors.Is(err, node.ErrInvalidDisplayName) {
		t.Errorf("name too long: err = %v, want ErrInvalidDisplayName", err)
	}
}

// Between issuing the token and the Node registering, the name might have been taken. The Node has
// to JOIN anyway — failing onboarding because of a name would be worse than the
// problem — and the current owner of the name cannot lose it.
func TestRegister_TakenDisplayNameIsDroppedNotFatal(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()
	name := uniq("Busy")
	idA := registerNode(t, svc, uniq("dn-owner"), name)
	if got := displayNameOf(t, svc, idA); got != name {
		t.Fatalf("setup: nickname = %q, want %q", got, name)
	}

	idB := registerNode(t, svc, uniq("dn-latecomer"), strings.ToUpper(name))
	if got := displayNameOf(t, svc, idB); got != "" {
		t.Errorf("Node that arrived later got %q; should join without display name", got)
	}
	if got := displayNameOf(t, svc, idA); got != name {
		t.Errorf("name owner lost display name: %q", got)
	}

	// Re-registration by the SAME owner with the same name is not a conflict with itself.
	nodes, _ := svc.List(ctx)
	var hostA string
	for _, n := range nodes {
		if n.ID == idA {
			hostA = n.Hostname
		}
	}
	if _, err := svc.Register(ctx, node.RegisterInput{Hostname: hostA, DisplayName: name, Engine: "qbittorrent"}); err != nil {
		t.Fatalf("owner re-registration: %v", err)
	}
	if got := displayNameOf(t, svc, idA); got != name {
		t.Errorf("re-registration cleared/changed the alias: %q", got)
	}
}

// A registration WITHOUT a name (legacy Agent, legacy path) cannot erase the display name the
// operator gave — it's the COALESCE of the upsert, and it's what prevents a restart/re-enroll from
// "un-naming" the Node.
func TestRegister_WithoutNameKeepsExistingDisplayName(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()
	host, name := uniq("dn-keep"), uniq("Kept")
	id := registerNode(t, svc, host, name)

	if _, err := svc.Register(ctx, node.RegisterInput{Hostname: host, Engine: "qbittorrent"}); err != nil {
		t.Fatalf("re-registration without name: %v", err)
	}
	if got := displayNameOf(t, svc, id); got != name {
		t.Errorf("display name after re-registration without name = %q, want %q", got, name)
	}
}

// The list follows the label the Portal displays: a Node with display name "aaa…" appears before
// a Node with no display name and hostname "mmm…", even if its hostname is "zzz…".
func TestList_OrdersByDisplayedLabel(t *testing.T) {
	svc := setup(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	idZ := registerNode(t, svc, "zzz-order-"+suffix, "aaa-order-"+suffix)
	idM := registerNode(t, svc, "mmm-order-"+suffix, "")

	nodes, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	pos := map[string]int{}
	for i, n := range nodes {
		pos[n.ID] = i
	}
	if pos[idZ] >= pos[idM] {
		t.Errorf("Node with display name aaa (hostname zzz) at %d should come before Node mmm at %d", pos[idZ], pos[idM])
	}
}

// The partial unique index from 00042 is the safeguard against races: the service checks
// (SetDisplayName, registrationName) are "read then write" and don't lock by themselves. This
// test bypasses the service and talks directly to the database, to prove that (1) the index exists and is
// case-insensitive, (2) a violation brings the name the service recognizes to fall into the
// registration fallback — if the index name changes in a future migration without changing the
// constant, the race becomes a 500 on onboarding again and this test is what warns us — and
// (3) the index is partial: a removed Node's name returns to the pool.
func TestDisplayNameUniqueIndex(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()
	pool, err := db.Connect(ctx, os.Getenv("SEEDGRID_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	idA := registerNode(t, svc, uniq("dn-idx-a"), "")
	idB := registerNode(t, svc, uniq("dn-idx-b"), "")
	name := uniq("Index")

	if _, err := pool.Exec(ctx, `UPDATE nodes SET display_name = $2 WHERE id = $1`, idA, name); err != nil {
		t.Fatalf("first UPDATE: %v", err)
	}
	_, err = pool.Exec(ctx, `UPDATE nodes SET display_name = $2 WHERE id = $1`, idB, strings.ToUpper(name))
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate display name (different case) directly in database: err = %v, want violation 23505", err)
	}
	if pgErr.ConstraintName != "uk_nodes_display_name_per_cluster" {
		t.Errorf("constraint = %q; the service only recognizes the race by the name uk_nodes_display_name_per_cluster", pgErr.ConstraintName)
	}

	// Partial: removing the Node frees the name.
	if err := svc.Delete(ctx, idA, false); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE nodes SET display_name = $2 WHERE id = $1`, idB, name); err != nil {
		t.Errorf("removed Node name should be free in the index: %v", err)
	}
}
