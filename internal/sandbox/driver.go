package sandbox

import (
	"context"
	"time"
)

// Driver is the seam every isolation backend implements. Swapping the driver
// swaps the isolation model without touching the Manager, MCP server, or CLI.
//
// Phase 1 ships the "oci" driver (docker/podman). The same interface is the
// drop-in point for stronger isolation later:
//   - apple-container: one microVM per sandbox (Apple Virtualization.framework)
//   - libkrun:         embedded microVM
//   - mock:            in-memory, for tests
type Driver interface {
	// Name identifies the backend, e.g. "oci:docker".
	Name() string

	// Create starts a new sandbox from spec and returns it (with Ref set).
	Create(ctx context.Context, spec Spec) (Sandbox, error)
	// Resolve looks up a sandbox by its peapod id.
	Resolve(ctx context.Context, id string) (Sandbox, error)
	// List returns all peapod-managed sandboxes.
	List(ctx context.Context) ([]Sandbox, error)

	// Exec runs argv inside the sandbox referenced by ref.
	Exec(ctx context.Context, ref string, argv []string, opts ExecOpts) (ExecResult, error)
	// WriteFile writes data to path inside the sandbox (creating parent dirs).
	WriteFile(ctx context.Context, ref, path string, data []byte, mode uint32) error
	// ReadFile reads path from inside the sandbox.
	ReadFile(ctx context.Context, ref, path string) ([]byte, error)
	// Destroy removes the sandbox.
	Destroy(ctx context.Context, ref string) error

	// --- Phase 2 lives here, already part of the contract ---

	// Snapshot captures the sandbox state and returns a snapshot ref.
	Snapshot(ctx context.Context, ref, name string) (string, error)
	// Fork creates a new sandbox from a snapshot ref.
	Fork(ctx context.Context, snapshotRef string, spec Spec) (Sandbox, error)

	// ListSnapshots returns saved snapshots.
	ListSnapshots(ctx context.Context) ([]Snapshot, error)
	// RemoveSnapshot deletes a snapshot by ref.
	RemoveSnapshot(ctx context.Context, ref string) error

	// Pause freezes a sandbox's processes in memory (zero CPU).
	Pause(ctx context.Context, ref string) error
	// Resume unfreezes a paused sandbox.
	Resume(ctx context.Context, ref string) error
}

// ExecOpts tunes a single Exec call.
type ExecOpts struct {
	Workdir string
	Env     map[string]string
	Timeout time.Duration // 0 = no timeout
}

// Checkpointer is an optional capability: persist a running sandbox's memory
// state to disk (CRIU) and restore it later. A driver implements it only if the
// backend supports it; the Manager checks via a type assertion.
type Checkpointer interface {
	Checkpoint(ctx context.Context, ref, name string) error
	Restore(ctx context.Context, ref, name string) error
}
