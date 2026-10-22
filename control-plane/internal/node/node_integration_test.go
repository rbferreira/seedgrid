package node_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/control-plane/internal/db"
	"github.com/rbferreira/seedgrid/control-plane/internal/events"
	"github.com/rbferreira/seedgrid/control-plane/internal/node"
)

func setup(t *testing.T) *node.Service {
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
	return svc
}

func TestList(t *testing.T) {
	svc := setup(t)
	nodes, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if nodes == nil {
		t.Error("List returned nil slice; expected empty slice")
	}
}

// TestOperationMode covers the global gate + per-Node mode (ADR-024): under full_auto the
// per-Node mode is locked; under flex it applies; returning to full_auto resets the Nodes to sg.
func TestOperationMode(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()

	res, err := svc.Register(ctx, node.RegisterInput{Hostname: "node-mode-01", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Restores the gate at the end to not affect other tests in the package (shared DB).
	t.Cleanup(func() { _ = svc.SetGate(ctx, node.GateFullAuto) })

	// Gate default = full_auto.
	if g, err := svc.Gate(ctx); err != nil || g != node.GateFullAuto {
		t.Fatalf("initial gate = %q (err=%v), want full_auto", g, err)
	}
	// Under full_auto, setting the per-Node mode is blocked (ErrGateLocked).
	if err := svc.SetNodeMode(ctx, res.NodeID, node.NodeModeManual); !errors.Is(err, node.ErrGateLocked) {
		t.Errorf("SetNodeMode under full_auto should return ErrGateLocked; got %v", err)
	}
	// Flex → the per-Node mode takes effect.
	if err := svc.SetGate(ctx, node.GateFlex); err != nil {
		t.Fatalf("SetGate flex: %v", err)
	}
	if err := svc.SetNodeMode(ctx, res.NodeID, node.NodeModeManual); err != nil {
		t.Fatalf("manual SetNodeMode under flex: %v", err)
	}
	if got := nodeMode(t, svc, res.NodeID); got != node.NodeModeManual {
		t.Errorf("mode = %q, want manual", got)
	}
	// Invalid mode is rejected.
	if err := svc.SetNodeMode(ctx, res.NodeID, "bogus"); !errors.Is(err, node.ErrInvalidMode) {
		t.Errorf("invalid mode should return ErrInvalidMode; got %v", err)
	}
	// Returning to full_auto resets the Node to sg (full-auto is full-auto).
	if err := svc.SetGate(ctx, node.GateFullAuto); err != nil {
		t.Fatalf("SetGate full_auto: %v", err)
	}
	if got := nodeMode(t, svc, res.NodeID); got != node.NodeModeSG {
		t.Errorf("when returning to full_auto the Node should become sg; got %q", got)
	}
}

// nodeMode returns the current operation_mode of a Node from the listing.
func nodeMode(t *testing.T, svc *node.Service, nodeID string) string {
	t.Helper()
	nodes, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, n := range nodes {
		if n.ID == nodeID {
			return n.OperationMode
		}
	}
	t.Fatalf("node %s not found in the listing", nodeID)
	return ""
}

func TestRegisterHeartbeatAuthenticate(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()

	res, err := svc.Register(ctx, node.RegisterInput{
		Hostname:     "node-test-01",
		OSName:       "Ubuntu",
		Architecture: "arm64",
		AgentVersion: "test",
		Engine:       "qbittorrent",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.NodeID == "" || res.AgentToken == "" {
		t.Fatalf("incomplete registration result: %+v", res)
	}

	// The issued credential must authenticate and resolve to the same Node.
	gotID, err := svc.AuthenticateAgent(ctx, res.AgentToken)
	if err != nil {
		t.Fatalf("AuthenticateAgent: %v", err)
	}
	if gotID != res.NodeID {
		t.Errorf("node id = %q, want %q", gotID, res.NodeID)
	}
	if _, err := svc.AuthenticateAgent(ctx, "invalid-token"); err == nil {
		t.Error("invalid token should fail authentication")
	}

	// Disk 10% free → storage 50; mem 60% available → 100; overall = 0.6*50 + 0.4*100 = 70.
	if err := svc.Heartbeat(ctx, res.NodeID, node.HeartbeatInput{
		Status: "ONLINE", AgentVersion: "test", UptimeSeconds: 42,
		DiskTotalBytes: 100, DiskFreeBytes: 10, MemTotalBytes: 100, MemAvailBytes: 60, CPUCores: 4,
	}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	nodes, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var found bool
	for _, n := range nodes {
		if n.ID == res.NodeID {
			found = true
			if n.Status != "ONLINE" {
				t.Errorf("status = %q, want ONLINE", n.Status)
			}
			if n.HealthScore == nil || *n.HealthScore != 70 {
				t.Errorf("calculated health_score = %v, want 70", n.HealthScore)
			}
			if n.DiskFreeBytes == nil || *n.DiskFreeBytes != 10 {
				t.Errorf("disk_free_bytes = %v, want 10", n.DiskFreeBytes)
			}
		}
	}
	if !found {
		t.Error("registered node did not appear in listing")
	}
}

// TestAssignPort_IndexIsUniquePerNode (ADR-038): the Node index (source of the IPv6 suffix
// `::5eed:<N>`) is derived from the port assigned on enroll, thus inheriting its uniqueness.
// The database is shared among tests (ports from other tests might be in use),
// so nothing here assumes WHICH port is assigned — only what must always hold true.
func TestAssignPort_IndexIsUniquePerNode(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()

	tokA, _, err := svc.MintEnrollToken(ctx, time.Hour, "")
	if err != nil {
		t.Fatalf("MintEnrollToken A: %v", err)
	}
	tokB, _, err := svc.MintEnrollToken(ctx, time.Hour, "")
	if err != nil {
		t.Fatalf("MintEnrollToken B: %v", err)
	}

	portA, err := svc.AssignPort(ctx, tokA)
	if err != nil {
		t.Fatalf("AssignPort A: %v", err)
	}
	portB, err := svc.AssignPort(ctx, tokB)
	if err != nil {
		t.Fatalf("AssignPort B: %v", err)
	}
	if portA == portB {
		t.Fatalf("two tokens received the SAME port %d — the index (and IPv6 address) would collide", portA)
	}

	idxA, okA := node.IndexForPort(portA)
	idxB, okB := node.IndexForPort(portB)
	if !okA || !okB {
		t.Fatalf("assigned port out of sequence: A=%d (ok=%v) B=%d (ok=%v)", portA, okA, portB, okB)
	}
	if idxA == idxB {
		t.Errorf("same index (%d) for different ports (%d, %d)", idxA, portA, portB)
	}
	if idxA != portA-6880 || idxB != portB-6880 {
		t.Errorf("index != port-6880: A=%d/%d B=%d/%d", portA, idxA, portB, idxB)
	}

	// Idempotent: repeating the enroll with the same token returns the same port (and thus,
	// the same index) — the bootstrap can be repeated without changing the Node's address.
	again, err := svc.AssignPort(ctx, tokA)
	if err != nil || again != portA {
		t.Errorf("AssignPort repeated = (%d, %v), want same port %d", again, err, portA)
	}
}

// TestAuthenticateAgent_ThrottleLastUsed (#114): each Agent authenticates 4+ times per
// cycle; last_used_at should only be rewritten when stale (5min window).
func TestAuthenticateAgent_ThrottleLastUsed(t *testing.T) {
	svc := setup(t)
	dsn := os.Getenv("SEEDGRID_TEST_DATABASE_URL")
	pool, err := db.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()

	reg, err := svc.Register(ctx, node.RegisterInput{Hostname: "throttle-cred-node", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	lastUsed := func() *time.Time {
		var ts *time.Time
		if err := pool.QueryRow(ctx,
			`SELECT last_used_at FROM node_credentials WHERE node_id = $1 ORDER BY created_at DESC LIMIT 1`,
			reg.NodeID).Scan(&ts); err != nil {
			t.Fatalf("read last_used_at: %v", err)
		}
		return ts
	}

	// 1st authentication writes it (column starts NULL).
	if _, err := svc.AuthenticateAgent(ctx, reg.AgentToken); err != nil {
		t.Fatalf("authenticate agent: %v", err)
	}
	first := lastUsed()
	if first == nil {
		t.Fatal("1st authentication should record last_used_at")
	}

	// Within the window: DOES NOT rewrite.
	if _, err := svc.AuthenticateAgent(ctx, reg.AgentToken); err != nil {
		t.Fatalf("authenticate agent 2: %v", err)
	}
	if got := lastUsed(); got == nil || !got.Equal(*first) {
		t.Errorf("last_used_at changed within the window: %v → %v", first, got)
	}

	// Stale beyond the window: rewrites.
	if _, err := pool.Exec(ctx,
		`UPDATE node_credentials SET last_used_at = now() - interval '10 minutes' WHERE node_id = $1`,
		reg.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuthenticateAgent(ctx, reg.AgentToken); err != nil {
		t.Fatalf("authenticate agent 3: %v", err)
	}
	if got := lastUsed(); got == nil || !got.After(*first) {
		t.Errorf("stale last_used_at was not rewritten: %v", got)
	}
}

// TestReserveEnginePort (ADR-043): a Node joining with the Engine it already runs keeps that
// Engine's port. The port may be outside the sequence and may be another Node's — accepted,
// naming that Node — but never the joining machine itself.
func TestReserveEnginePort(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()
	const port = 41234 // outside the sequence, unlikely in other tests (shared database)

	tok, _, err := svc.MintEnrollToken(ctx, time.Hour, "")
	if err != nil {
		t.Fatalf("MintEnrollToken: %v", err)
	}
	if shared, err := svc.ReserveEnginePort(ctx, tok, port, "adopt-port-a"); err != nil || shared != "" {
		t.Fatalf("ReserveEnginePort first = (%q, %v), want (\"\", nil)", shared, err)
	}
	info, ok := svc.EnrollToken(ctx, tok)
	if !ok || info.BTPort == nil || *info.BTPort != port {
		t.Fatalf("the token does not carry the Engine's port: %+v ok=%v", info, ok)
	}
	p := port
	if _, err := svc.Register(ctx, node.RegisterInput{Hostname: "adopt-port-a", Engine: "qbittorrent", BTPort: &p}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Another machine on the same port: accepted, naming the Node already on it.
	tok2, _, err := svc.MintEnrollToken(ctx, time.Hour, "")
	if err != nil {
		t.Fatalf("MintEnrollToken 2: %v", err)
	}
	if shared, err := svc.ReserveEnginePort(ctx, tok2, port, "adopt-port-b"); err != nil || shared != "adopt-port-a" {
		t.Errorf("ReserveEnginePort shared = (%q, %v), want (\"adopt-port-a\", nil)", shared, err)
	}
	// The bootstrap run again on the same machine does not warn about itself.
	if shared, err := svc.ReserveEnginePort(ctx, tok2, port, "adopt-port-a"); err != nil || shared != "" {
		t.Errorf("ReserveEnginePort same host = (%q, %v), want (\"\", nil)", shared, err)
	}

	if _, err := svc.ReserveEnginePort(ctx, tok2, 0, "x"); !errors.Is(err, node.ErrInvalidPort) {
		t.Errorf("port 0: err = %v, want ErrInvalidPort", err)
	}
	if _, err := svc.ReserveEnginePort(ctx, tok2, 70000, "x"); !errors.Is(err, node.ErrInvalidPort) {
		t.Errorf("port 70000: err = %v, want ErrInvalidPort", err)
	}
	if _, err := svc.ReserveEnginePort(ctx, "not-a-token", port, "x"); !errors.Is(err, node.ErrInvalidEnrollToken) {
		t.Errorf("unknown token: err = %v, want ErrInvalidEnrollToken", err)
	}
}

// TestRegister_AdoptedStartsInManual (ADR-043): a Node that joins with its own Engine starts
// in Manual; a Cluster in Full auto moves to Flex for it, and an event says so. Registering
// it again does not undo what the operator chose afterwards, and a Node SeedGrid installs
// still starts in SG.
func TestRegister_AdoptedStartsInManual(t *testing.T) {
	svc := setup(t)
	ctx := context.Background()
	pool, err := db.Connect(ctx, os.Getenv("SEEDGRID_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := svc.SetGate(ctx, node.GateFullAuto); err != nil {
		t.Fatalf("SetGate: %v", err)
	}
	t.Cleanup(func() { _ = svc.SetGate(ctx, node.GateFullAuto) })

	byHost := func(host string) node.Node {
		t.Helper()
		nodes, err := svc.List(ctx)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		for _, n := range nodes {
			if n.Hostname == host {
				return n
			}
		}
		t.Fatalf("node %s not listed", host)
		return node.Node{}
	}
	adoptedEvent := func(nodeID string) (found bool, gateMoved bool) {
		t.Helper()
		err := pool.QueryRow(ctx,
			`SELECT (e.payload->>'gate_moved')::boolean FROM events e
			   JOIN event_types et ON et.id = e.event_type_id
			  WHERE et.code = 'NODE_ADOPTED_MANUAL' AND e.node_id = $1`, nodeID).Scan(&gateMoved)
		if err != nil {
			return false, false
		}
		return true, gateMoved
	}

	a, err := svc.Register(ctx, node.RegisterInput{Hostname: "adopted-manual-a", Engine: "qbittorrent", EngineAdopted: true})
	if err != nil {
		t.Fatalf("Register adopted: %v", err)
	}
	if n := byHost("adopted-manual-a"); n.OperationMode != node.NodeModeManual || !n.EngineAdopted {
		t.Errorf("adopted Node: mode=%q adopted=%v, want manual/true", n.OperationMode, n.EngineAdopted)
	}
	if gate, _ := svc.Gate(ctx); gate != node.GateFlex {
		t.Errorf("gate = %q, want flex", gate)
	}
	if found, moved := adoptedEvent(a.NodeID); !found || !moved {
		t.Errorf("NODE_ADOPTED_MANUAL: found=%v gate_moved=%v, want true/true", found, moved)
	}

	// The operator moves it to SG; the Agent registering again (a lost credential) keeps that.
	if err := svc.SetNodeMode(ctx, a.NodeID, node.NodeModeSG); err != nil {
		t.Fatalf("SetNodeMode: %v", err)
	}
	if _, err := svc.Register(ctx, node.RegisterInput{Hostname: "adopted-manual-a", Engine: "qbittorrent", EngineAdopted: true}); err != nil {
		t.Fatalf("Register again: %v", err)
	}
	if n := byHost("adopted-manual-a"); n.OperationMode != node.NodeModeSG {
		t.Errorf("registering again changed the mode to %q, want sg", n.OperationMode)
	}

	// Already in Flex: the second adopted Node starts in Manual and the gate does not "move".
	b, err := svc.Register(ctx, node.RegisterInput{Hostname: "adopted-manual-b", Engine: "qbittorrent", EngineAdopted: true})
	if err != nil {
		t.Fatalf("Register adopted b: %v", err)
	}
	if n := byHost("adopted-manual-b"); n.OperationMode != node.NodeModeManual {
		t.Errorf("second adopted Node: mode=%q, want manual", n.OperationMode)
	}
	if found, moved := adoptedEvent(b.NodeID); !found || moved {
		t.Errorf("NODE_ADOPTED_MANUAL b: found=%v gate_moved=%v, want true/false", found, moved)
	}

	// A Node SeedGrid installs: SG, not adopted, no event.
	c, err := svc.Register(ctx, node.RegisterInput{Hostname: "adopted-manual-c", Engine: "qbittorrent"})
	if err != nil {
		t.Fatalf("Register installed: %v", err)
	}
	if n := byHost("adopted-manual-c"); n.OperationMode != node.NodeModeSG || n.EngineAdopted {
		t.Errorf("installed Node: mode=%q adopted=%v, want sg/false", n.OperationMode, n.EngineAdopted)
	}
	if found, _ := adoptedEvent(c.NodeID); found {
		t.Error("an installed Node emitted NODE_ADOPTED_MANUAL")
	}
}
