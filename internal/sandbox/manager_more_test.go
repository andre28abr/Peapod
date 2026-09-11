package sandbox_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peapod/internal/driver/mock"
	"peapod/internal/sandbox"
)

// TestAllowWithPortsRejected: a firewalled sandbox lives on an internal network,
// which can't publish ports — asking for both must fail loudly, not silently.
func TestAllowWithPortsRejected(t *testing.T) {
	mgr := sandbox.NewManager(mock.New())
	_, err := mgr.Create(context.Background(), sandbox.Spec{
		Image: "alpine", Allow: []string{"pypi.org"}, Ports: []sandbox.Port{{Host: 8080, Container: 80}},
	})
	if err == nil {
		t.Fatal("Create with --allow and --ports should fail")
	}
}

// TestReapUsesActivityNotCreation: a sandbox whose last exec is old gets reaped
// even though it was created just now — reap keys on activity, not on birth.
func TestReapUsesActivityNotCreation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PEAPOD_HISTORY_DIR", dir)
	ctx := context.Background()
	mgr := sandbox.NewManager(mock.New())
	sb, err := mgr.Create(ctx, sandbox.Spec{Image: "alpine"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := mgr.Exec(ctx, sb.ID, []string{"true"}, sandbox.ExecOpts{}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	// Backdate the audit trail: "last activity two hours ago".
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, sb.ID+".jsonl"), old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	reaped, err := mgr.Reap(ctx, time.Hour)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if len(reaped) != 1 || reaped[0] != sb.ID {
		t.Errorf("reaped = %v, want [%s] (old activity despite recent creation)", reaped, sb.ID)
	}
}

// TestRejectsUnknownNetwork guards the fail-closed rule: an unknown policy must
// error instead of falling through to the runtime's default (full) network.
func TestRejectsUnknownNetwork(t *testing.T) {
	ctx := context.Background()
	mgr := sandbox.NewManager(mock.New())
	for _, bad := range []string{"bridge", "egres", "host", "yes"} {
		if _, err := mgr.Create(ctx, sandbox.Spec{Image: "alpine", Network: sandbox.NetworkPolicy(bad)}); err == nil {
			t.Errorf("Create with network %q should fail", bad)
		}
	}
	sb, err := mgr.Create(ctx, sandbox.Spec{Image: "alpine", Network: sandbox.NetworkEgress})
	if err != nil {
		t.Fatalf("egress should be accepted: %v", err)
	}
	snap, err := mgr.Snapshot(ctx, sb.ID, "v1")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := mgr.Fork(ctx, snap, sandbox.Spec{Network: "bridge"}); err == nil {
		t.Error("Fork with an unknown network should fail")
	}
}

// TestExecTimeoutWrapsButAuditsOriginal checks that a timeout wraps the command
// in the in-sandbox watchdog while the audit trail records what the caller ran.
func TestExecTimeoutWrapsButAuditsOriginal(t *testing.T) {
	t.Setenv("PEAPOD_HISTORY_DIR", t.TempDir())
	ctx := context.Background()
	mgr := sandbox.NewManager(mock.New())
	sb, err := mgr.Create(ctx, sandbox.Spec{Image: "alpine"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	res, err := mgr.Exec(ctx, sb.ID, []string{"echo", "hi"}, sandbox.ExecOpts{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	// The mock echoes the argv it received: the watchdog must be in front...
	if !strings.Contains(res.Stdout, "sleep") || !strings.Contains(res.Stdout, "echo hi") {
		t.Errorf("driver did not receive the wrapped command: %q", res.Stdout)
	}
	// ...but the audit trail keeps the caller's command, not the wrapper.
	hist, _ := mgr.History(sb.ID)
	if len(hist) != 1 || hist[0].Command != "echo hi" {
		t.Errorf("history = %+v, want the original 'echo hi'", hist)
	}
	// No timeout ⇒ no wrapper.
	res, _ = mgr.Exec(ctx, sb.ID, []string{"echo", "plain"}, sandbox.ExecOpts{})
	if strings.Contains(res.Stdout, "sleep") {
		t.Errorf("no timeout must not wrap: %q", res.Stdout)
	}
}

// TestHistoryAndReap covers the audit trail (every exec is recorded) and reaping
// by age, using a temp history dir so the test never touches ~/.peapod.
func TestHistoryAndReap(t *testing.T) {
	t.Setenv("PEAPOD_HISTORY_DIR", t.TempDir())
	ctx := context.Background()
	mgr := sandbox.NewManager(mock.New())

	sb, err := mgr.Create(ctx, sandbox.Spec{Image: "alpine"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, argv := range [][]string{{"echo", "hi"}, {"ls", "-la"}} {
		if _, err := mgr.Exec(ctx, sb.ID, argv, sandbox.ExecOpts{}); err != nil {
			t.Fatalf("exec: %v", err)
		}
	}

	hist, err := mgr.History(sb.ID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history has %d entries, want 2", len(hist))
	}
	if hist[0].Command != "echo hi" || hist[1].Command != "ls -la" {
		t.Errorf("history commands = %q, %q", hist[0].Command, hist[1].Command)
	}

	// Reap with a negative max-age forces every sandbox past the cutoff.
	reaped, err := mgr.Reap(ctx, -time.Second)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if len(reaped) != 1 || reaped[0] != sb.ID {
		t.Errorf("reaped = %v, want [%s]", reaped, sb.ID)
	}
	if boxes, _ := mgr.List(ctx); len(boxes) != 0 {
		t.Errorf("expected no sandboxes after reap, got %d", len(boxes))
	}
}

// TestPruneSnapshots covers age-based snapshot pruning.
func TestPruneSnapshots(t *testing.T) {
	ctx := context.Background()
	mgr := sandbox.NewManager(mock.New())

	sb, err := mgr.Create(ctx, sandbox.Spec{Image: "alpine"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	ref, err := mgr.Snapshot(ctx, sb.ID, "v1")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snaps, _ := mgr.ListSnapshots(ctx); len(snaps) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps))
	}

	removed, err := mgr.PruneSnapshots(ctx, -time.Second)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(removed) != 1 || removed[0] != ref {
		t.Errorf("removed = %v, want [%s]", removed, ref)
	}
	if snaps, _ := mgr.ListSnapshots(ctx); len(snaps) != 0 {
		t.Errorf("expected 0 snapshots after prune, got %d", len(snaps))
	}
}

// TestUnsupportedCapabilities verifies the optional-capability seam: the mock
// driver implements none of them, so the Manager must report a clear error
// rather than panic.
func TestUnsupportedCapabilities(t *testing.T) {
	ctx := context.Background()
	mgr := sandbox.NewManager(mock.New())
	sb, err := mgr.Create(ctx, sandbox.Spec{Image: "alpine"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := mgr.Logs(ctx, sb.ID, 0); err == nil {
		t.Error("Logs should be unsupported on the mock backend")
	}
	if _, err := mgr.Stats(ctx, sb.ID); err == nil {
		t.Error("Stats should be unsupported on the mock backend")
	}
	if err := mgr.Checkpoint(ctx, sb.ID, "c1"); err == nil {
		t.Error("Checkpoint should be unsupported on the mock backend")
	}
	if _, err := mgr.DiffSnapshots(ctx, "a", "b"); err == nil {
		t.Error("DiffSnapshots should be unsupported on the mock backend")
	}
}
