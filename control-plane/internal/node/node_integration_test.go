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
