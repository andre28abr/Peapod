package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// Manager is the núcleo: it applies policy/defaults and orchestrates a Driver.
// It is deliberately driver-agnostic — that's what lets phases 2 and 3 plug in.
type Manager struct {
	drv Driver
}

// NewManager wraps a driver in the núcleo.
func NewManager(drv Driver) *Manager { return &Manager{drv: drv} }

// Backend reports the active driver name.
func (m *Manager) Backend() string { return m.drv.Name() }

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "pp_" + hex.EncodeToString(b)
}

func applyDefaults(spec *Spec) {
	if spec.Network == "" {
		spec.Network = NetworkNone
	}
	if spec.Workdir == "" {
		spec.Workdir = "/work"
	}
	if spec.Resources.CPUs == 0 {
		spec.Resources.CPUs = 2
	}
	if spec.Resources.MemoryMB == 0 {
		spec.Resources.MemoryMB = 1024
	}
	if spec.Resources.PidsLimit == 0 {
		spec.Resources.PidsLimit = 512
	}
}

func (m *Manager) tag(spec *Spec) string {
	id := newID()
	if spec.Labels == nil {
		spec.Labels = map[string]string{}
	}
	spec.Labels["peapod.id"] = id
	return id
}

// Create validates, applies defaults, tags the sandbox with a fresh id, and
// asks the driver to start it.
func (m *Manager) Create(ctx context.Context, spec Spec) (Sandbox, error) {
	if spec.Image == "" {
		return Sandbox{}, fmt.Errorf("image is required")
	}
	applyDefaults(&spec)
	m.tag(&spec)
	return m.drv.Create(ctx, spec)
}

// List returns all managed sandboxes.
func (m *Manager) List(ctx context.Context) ([]Sandbox, error) { return m.drv.List(ctx) }

// Exec runs argv in the sandbox with the given id.
func (m *Manager) Exec(ctx context.Context, id string, argv []string, opts ExecOpts) (ExecResult, error) {
	sb, err := m.drv.Resolve(ctx, id)
	if err != nil {
		return ExecResult{}, err
	}
	if opts.Workdir == "" {
		opts.Workdir = sb.Workdir
	}
	return m.drv.Exec(ctx, sb.Ref, argv, opts)
}

// WriteFile writes a file into the sandbox.
func (m *Manager) WriteFile(ctx context.Context, id, path string, data []byte, mode uint32) error {
	sb, err := m.drv.Resolve(ctx, id)
	if err != nil {
		return err
	}
	return m.drv.WriteFile(ctx, sb.Ref, path, data, mode)
}

// ReadFile reads a file from the sandbox.
func (m *Manager) ReadFile(ctx context.Context, id, path string) ([]byte, error) {
	sb, err := m.drv.Resolve(ctx, id)
	if err != nil {
		return nil, err
	}
	return m.drv.ReadFile(ctx, sb.Ref, path)
}

// Destroy tears down the sandbox.
func (m *Manager) Destroy(ctx context.Context, id string) error {
	sb, err := m.drv.Resolve(ctx, id)
	if err != nil {
		return err
	}
	return m.drv.Destroy(ctx, sb.Ref)
}

// Snapshot captures a sandbox (Phase 2 entry point).
func (m *Manager) Snapshot(ctx context.Context, id, name string) (string, error) {
	sb, err := m.drv.Resolve(ctx, id)
	if err != nil {
		return "", err
	}
	return m.drv.Snapshot(ctx, sb.Ref, name)
}

// Fork creates a new sandbox from a snapshot (Phase 2 entry point).
func (m *Manager) Fork(ctx context.Context, snapshotRef string, spec Spec) (Sandbox, error) {
	applyDefaults(&spec)
	m.tag(&spec)
	return m.drv.Fork(ctx, snapshotRef, spec)
}

// Reap destroys sandboxes whose age exceeds maxAge and returns the ids reaped.
// Age is measured from creation (Phase 1); true idle tracking comes later.
func (m *Manager) Reap(ctx context.Context, maxAge time.Duration) ([]string, error) {
	boxes, err := m.List(ctx)
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().Add(-maxAge)
	var reaped []string
	for _, b := range boxes {
		if b.Created.IsZero() || !b.Created.Before(cutoff) {
			continue
		}
		if err := m.Destroy(ctx, b.ID); err == nil {
			reaped = append(reaped, b.ID)
		}
	}
	return reaped, nil
}

// ListSnapshots returns saved snapshots.
func (m *Manager) ListSnapshots(ctx context.Context) ([]Snapshot, error) {
	return m.drv.ListSnapshots(ctx)
}

// RemoveSnapshot deletes a snapshot by ref.
func (m *Manager) RemoveSnapshot(ctx context.Context, ref string) error {
	return m.drv.RemoveSnapshot(ctx, ref)
}

// Pause freezes a sandbox's processes (memory preserved, zero CPU).
func (m *Manager) Pause(ctx context.Context, id string) error {
	sb, err := m.drv.Resolve(ctx, id)
	if err != nil {
		return err
	}
	return m.drv.Pause(ctx, sb.Ref)
}

// Resume unfreezes a paused sandbox.
func (m *Manager) Resume(ctx context.Context, id string) error {
	sb, err := m.drv.Resolve(ctx, id)
	if err != nil {
		return err
	}
	return m.drv.Resume(ctx, sb.Ref)
}
