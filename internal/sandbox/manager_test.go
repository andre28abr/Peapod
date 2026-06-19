package sandbox_test

import (
	"context"
	"testing"

	"peapod/internal/driver/mock"
	"peapod/internal/sandbox"
)

// TestManagerLifecycle exercises the full núcleo surface through the Driver seam
// using the in-memory mock — no container runtime required.
func TestManagerLifecycle(t *testing.T) {
	ctx := context.Background()
	mgr := sandbox.NewManager(mock.New())

	sb, err := mgr.Create(ctx, sandbox.Spec{Image: "alpine"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sb.ID == "" {
		t.Fatal("expected a generated id")
	}
	if sb.Network != sandbox.NetworkNone {
		t.Errorf("default network = %q, want %q", sb.Network, sandbox.NetworkNone)
	}
	if sb.Workdir != "/work" {
		t.Errorf("default workdir = %q, want /work", sb.Workdir)
	}

	if err := mgr.WriteFile(ctx, sb.ID, "/work/main.py", []byte("print('hi')"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := mgr.ReadFile(ctx, sb.ID, "/work/main.py")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "print('hi')" {
		t.Errorf("read back %q", data)
	}

	if _, err := mgr.Exec(ctx, sb.ID, []string{"echo", "ok"}, sandbox.ExecOpts{}); err != nil {
		t.Fatalf("exec: %v", err)
	}

	// pause / resume toggles the running state.
	if err := mgr.Pause(ctx, sb.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if boxes, _ := mgr.List(ctx); !isPaused(boxes, sb.ID) {
		t.Error("sandbox should be paused")
	}
	if err := mgr.Resume(ctx, sb.ID); err != nil {
		t.Fatalf("resume: %v", err)
	}
	if boxes, _ := mgr.List(ctx); isPaused(boxes, sb.ID) {
		t.Error("sandbox should be running after resume")
	}

	// Phase 2 surface: snapshot + fork through the same seam.
	snap, err := mgr.Snapshot(ctx, sb.ID, "v1")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	forked, err := mgr.Fork(ctx, snap, sandbox.Spec{})
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if forked.ID == sb.ID {
		t.Error("fork should get a fresh id")
	}
	got, err := mgr.ReadFile(ctx, forked.ID, "/work/main.py")
	if err != nil {
		t.Fatalf("read forked: %v", err)
	}
	if string(got) != "print('hi')" {
		t.Errorf("forked file = %q, want carried-over content", got)
	}

	if err := mgr.Destroy(ctx, sb.ID); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	boxes, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, b := range boxes {
		if b.ID == sb.ID {
			t.Errorf("sandbox %s still present after destroy", sb.ID)
		}
	}
}

func isPaused(boxes []sandbox.Sandbox, id string) bool {
	for _, b := range boxes {
		if b.ID == id {
			return b.Paused
		}
	}
	return false
}
