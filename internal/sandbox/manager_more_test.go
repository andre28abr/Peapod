package sandbox_test

import (
	"context"
	"testing"
	"time"

	"peapod/internal/driver/mock"
	"peapod/internal/sandbox"
)

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
