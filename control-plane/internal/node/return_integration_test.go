package node_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
)

// setupWithPool is setup, plus the pool, for the tests that look at the rows themselves.
func setupWithPool(t *testing.T) (*node.Service, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SEEDGRID_TEST_DATABASE_URL not set; skipping integration test")
	}
	if err := db.Migrate(dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	svc := node.NewService(pool, events.NewEmitter(pool))
	if err := svc.EnsureDefaultCluster(context.Background()); err != nil {
		t.Fatalf("ensure default cluster: %v", err)
	}
	return svc, pool
}

// removedNode registers a Node of machine mid with one torrent, and removes it (force).
func removedNode(t *testing.T, svc *node.Service, pool *pgxpool.Pool, hostname, mid string) string {
	t.Helper()
	ctx := context.Background()
	reg, err := svc.Register(ctx, node.RegisterInput{Hostname: hostname, Engine: "qbittorrent", MachineID: mid})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO torrents (node_id, hash, name, status) VALUES ($1, $2, 'kept', 'SEEDING')`,
		reg.NodeID, fmt.Sprintf("%040d", time.Now().UnixNano())); err != nil {
		t.Fatalf("insert torrent: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE nodes SET operation_mode = 'manual' WHERE id = $1`, reg.NodeID); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	if err := svc.Delete(ctx, reg.NodeID, true); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	return reg.NodeID
}

func liveCount(t *testing.T, pool *pgxpool.Pool, hostname string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM nodes WHERE hostname = $1 AND deleted_at IS NULL`, hostname).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

// TestRegister_SameMachineComesBack (ADR-044): the machine of a removed Node joining again through
// a join token comes back as itself — same id, mode, and the torrents removed with it.
func TestRegister_SameMachineComesBack(t *testing.T) {
	svc, pool := setupWithPool(t)
	ctx := context.Background()
	host := fmt.Sprintf("return-same-%d", time.Now().UnixNano())
	id := removedNode(t, svc, pool, host, "mid-"+host)

	reg, err := svc.Register(ctx, node.RegisterInput{Hostname: host, Engine: "qbittorrent", MachineID: "mid-" + host, ViaJoinToken: true})
	if err != nil {
		t.Fatalf("Register again: %v", err)
	}
	if reg.NodeID != id {
		t.Errorf("came back as %s, want the removed Node %s", reg.NodeID, id)
	}
	var mode string
	var torrents int
	if err := pool.QueryRow(ctx,
		`SELECT n.operation_mode, (SELECT count(*) FROM torrents t WHERE t.node_id = n.id AND t.deleted_at IS NULL)
		   FROM nodes n WHERE n.id = $1 AND n.deleted_at IS NULL`, id).Scan(&mode, &torrents); err != nil {
		t.Fatalf("the Node is not live again: %v", err)
	}
	if mode != "manual" || torrents != 1 {
		t.Errorf("came back with mode %q and %d torrent(s), want manual and 1", mode, torrents)
	}
	var events int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM events ev JOIN event_types et ON et.id = ev.event_type_id
		  WHERE et.code = 'NODE_RETURNED' AND ev.node_id = $1::uuid`, id).Scan(&events); err != nil || events != 1 {
		t.Errorf("NODE_RETURNED events = %d (err=%v), want 1", events, err)
	}
	listed := false
	nodes, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, n := range nodes {
		listed = listed || n.ID == id
	}
	if !listed {
		t.Error("the Node came back but List does not show it (#62)")
	}
}

// TestRegister_SameMachineByNodeID (ADR-044): a removed Node from before machine_id was stored is
// recognized by the Node id of the Agent's identity file.
func TestRegister_SameMachineByNodeID(t *testing.T) {
	svc, pool := setupWithPool(t)
	ctx := context.Background()
	host := fmt.Sprintf("return-legacy-%d", time.Now().UnixNano())
	id := removedNode(t, svc, pool, host, "")

	reg, err := svc.Register(ctx, node.RegisterInput{Hostname: host, Engine: "qbittorrent", MachineID: "new-mid", PreviousNodeID: id, ViaJoinToken: true})
	if err != nil {
		t.Fatalf("Register again: %v", err)
	}
	if reg.NodeID != id {
		t.Errorf("came back as %s, want the removed Node %s", reg.NodeID, id)
	}
}

// TestRegister_RemovalSticksAgainstTheSharedToken (ADR-044): an Agent nobody stopped retries with
// the shared registration token after the removal revoked its credential; the removal sticks.
func TestRegister_RemovalSticksAgainstTheSharedToken(t *testing.T) {
	svc, pool := setupWithPool(t)
	host := fmt.Sprintf("return-shared-%d", time.Now().UnixNano())
	removedNode(t, svc, pool, host, "mid-"+host)

	_, err := svc.Register(context.Background(), node.RegisterInput{Hostname: host, Engine: "qbittorrent", MachineID: "mid-" + host})
	if !errors.Is(err, node.ErrNodeRemoved) {
		t.Fatalf("Register through the shared token = %v, want ErrNodeRemoved", err)
	}
	if n := liveCount(t, pool, host); n != 0 {
		t.Errorf("%d live Node(s) with the hostname, want none", n)
	}
}

// TestRegister_DifferentMachineIsANewNode (ADR-044): another machine with the hostname of a removed
// Node joins as a new Node, inheriting nothing; the removed row stays, for history.
func TestRegister_DifferentMachineIsANewNode(t *testing.T) {
	svc, pool := setupWithPool(t)
	ctx := context.Background()
	host := fmt.Sprintf("return-other-%d", time.Now().UnixNano())
	id := removedNode(t, svc, pool, host, "mid-old-"+host)

	for _, via := range []bool{true, false} {
		if via {
			// Removed again below, for the shared-token round.
			reg, err := svc.Register(ctx, node.RegisterInput{Hostname: host, Engine: "qbittorrent", MachineID: "mid-new-" + host, ViaJoinToken: true})
			if err != nil {
				t.Fatalf("Register of another machine: %v", err)
			}
			if reg.NodeID == id {
				t.Fatal("another machine took the removed Node's row")
			}
			var torrents int
			var mode string
			if err := pool.QueryRow(ctx,
				`SELECT operation_mode, (SELECT count(*) FROM torrents WHERE node_id = $1 AND deleted_at IS NULL) FROM nodes WHERE id = $1`,
				reg.NodeID).Scan(&mode, &torrents); err != nil {
				t.Fatalf("read new Node: %v", err)
			}
			if torrents != 0 || mode != "sg" {
				t.Errorf("the new Node inherited %d torrent(s) and mode %q", torrents, mode)
			}
			if err := svc.Delete(ctx, reg.NodeID, true); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			continue
		}
		// Through the shared token too: a different machine is not the removed Node's Agent.
		if _, err := svc.Register(ctx, node.RegisterInput{Hostname: host, Engine: "qbittorrent", MachineID: "mid-third-" + host}); err != nil {
			t.Fatalf("Register of a third machine through the shared token: %v", err)
		}
	}
	var removed int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM nodes WHERE hostname = $1 AND deleted_at IS NOT NULL`, host).Scan(&removed); err != nil || removed != 2 {
		t.Errorf("removed rows kept = %d (err=%v), want 2", removed, err)
	}
	if n := liveCount(t, pool, host); n != 1 {
		t.Errorf("%d live Node(s) with the hostname, want 1", n)
	}
}

// TestAssignPortFor_GivesTheOldPortBack (ADR-044): a removed Node coming back gets its old port when
// no live Node took it, so the port forward still points at it; another machine gets the next free one.
func TestAssignPortFor_GivesTheOldPortBack(t *testing.T) {
	svc, pool := setupWithPool(t)
	ctx := context.Background()
	host := fmt.Sprintf("return-port-%d", time.Now().UnixNano())
	const old = 7777 // outside the sequence: the next free one is never this
	reg, err := svc.Register(ctx, node.RegisterInput{Hostname: host, Engine: "qbittorrent", MachineID: "mid-" + host})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE nodes SET bt_port = $2 WHERE id = $1`, reg.NodeID, old); err != nil {
		t.Fatalf("set port: %v", err)
	}
	if err := svc.Delete(ctx, reg.NodeID, true); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	tok, _, err := svc.MintEnrollToken(ctx, time.Hour, "")
	if err != nil {
		t.Fatalf("MintEnrollToken: %v", err)
	}
	other, err := svc.AssignPortFor(ctx, tok, node.Machine{Hostname: host, MachineID: "someone-else"})
	if err != nil || other == old {
		t.Errorf("another machine got (%d, %v), want a port from the sequence", other, err)
	}
	tok2, _, err := svc.MintEnrollToken(ctx, time.Hour, "")
	if err != nil {
		t.Fatalf("MintEnrollToken 2: %v", err)
	}
	back, err := svc.AssignPortFor(ctx, tok2, node.Machine{Hostname: host, MachineID: "mid-" + host})
	if err != nil || back != old {
		t.Errorf("the same machine got (%d, %v), want its old port %d", back, err, old)
	}
}
