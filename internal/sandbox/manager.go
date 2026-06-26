package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if sb.Paused { // auto-resume an idle-paused sandbox before running
		if rerr := m.drv.Resume(ctx, sb.Ref); rerr == nil {
			sb.Paused = false
		}
	}
	if opts.Workdir == "" {
		opts.Workdir = sb.Workdir
	}
	res, err := m.drv.Exec(ctx, sb.Ref, argv, opts)
	if err == nil {
		m.record(id, argv, res)
	}
	return res, err
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
	if err := m.drv.Destroy(ctx, sb.Ref); err != nil {
		return err
	}
	_ = os.Remove(m.histPath(id))
	return nil
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

// DiffSnapshots returns the files added/removed between two snapshots.
func (m *Manager) DiffSnapshots(ctx context.Context, a, b string) (SnapshotDiff, error) {
	d, ok := m.drv.(SnapshotDiffer)
	if !ok {
		return SnapshotDiff{}, fmt.Errorf("backend %s does not support snapshot diff", m.drv.Name())
	}
	return d.DiffSnapshots(ctx, a, b)
}

// PruneSnapshots removes snapshots older than maxAge; returns the refs removed.
func (m *Manager) PruneSnapshots(ctx context.Context, maxAge time.Duration) ([]string, error) {
	snaps, err := m.drv.ListSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().Add(-maxAge).Unix()
	var removed []string
	for _, s := range snaps {
		if s.CreatedUnix == 0 || s.CreatedUnix >= cutoff {
			continue
		}
		if err := m.drv.RemoveSnapshot(ctx, s.Ref); err == nil {
			removed = append(removed, s.Ref)
		}
	}
	return removed, nil
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

// Checkpoint persists a running sandbox's state to disk (experimental; needs a
// CRIU-capable backend).
func (m *Manager) Checkpoint(ctx context.Context, id, name string) error {
	c, ok := m.drv.(Checkpointer)
	if !ok {
		return fmt.Errorf("backend %s does not support checkpoint", m.drv.Name())
	}
	sb, err := m.drv.Resolve(ctx, id)
	if err != nil {
		return err
	}
	return c.Checkpoint(ctx, sb.Ref, name)
}

// Restore restarts a sandbox from a checkpoint (experimental).
func (m *Manager) Restore(ctx context.Context, id, name string) error {
	c, ok := m.drv.(Checkpointer)
	if !ok {
		return fmt.Errorf("backend %s does not support restore", m.drv.Name())
	}
	sb, err := m.drv.Resolve(ctx, id)
	if err != nil {
		return err
	}
	return c.Restore(ctx, sb.Ref, name)
}

// Logs returns recent output from a sandbox.
func (m *Manager) Logs(ctx context.Context, id string, tail int) (string, error) {
	l, ok := m.drv.(Logger)
	if !ok {
		return "", fmt.Errorf("backend %s does not support logs", m.drv.Name())
	}
	sb, err := m.drv.Resolve(ctx, id)
	if err != nil {
		return "", err
	}
	return l.Logs(ctx, sb.Ref, tail)
}

// Stats samples a sandbox's resource usage.
func (m *Manager) Stats(ctx context.Context, id string) (Stat, error) {
	s, ok := m.drv.(Statser)
	if !ok {
		return Stat{}, fmt.Errorf("backend %s does not support stats", m.drv.Name())
	}
	sb, err := m.drv.Resolve(ctx, id)
	if err != nil {
		return Stat{}, err
	}
	return s.Stats(ctx, sb.Ref)
}

func (m *Manager) histPath(id string) string {
	base := os.Getenv("PEAPOD_HISTORY_DIR")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".peapod", "history")
	}
	return filepath.Join(base, id+".jsonl")
}

// record appends an audit entry for a command run in the sandbox.
func (m *Manager) record(id string, argv []string, res ExecResult) {
	p := m.histPath(id)
	if os.MkdirAll(filepath.Dir(p), 0o755) != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	preview := strings.TrimSpace(res.Stdout + res.Stderr)
	if len(preview) > 200 {
		preview = preview[:200] + "…"
	}
	b, _ := json.Marshal(HistoryEntry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Command:  strings.Join(argv, " "),
		ExitCode: res.ExitCode,
		Preview:  preview,
	})
	_, _ = f.Write(append(b, '\n'))
}

// lastActivity is the time of the sandbox's most recent exec (history file
// mtime), falling back to its creation time.
func (m *Manager) lastActivity(b Sandbox) time.Time {
	if fi, err := os.Stat(m.histPath(b.ID)); err == nil {
		return fi.ModTime()
	}
	return b.Created
}

// PauseIdle pauses running sandboxes with no activity for longer than maxIdle.
// They auto-resume on the next exec. Returns the ids paused.
func (m *Manager) PauseIdle(ctx context.Context, maxIdle time.Duration) ([]string, error) {
	boxes, err := m.List(ctx)
	if err != nil {
		return nil, err
	}
	cutoff := time.Now().Add(-maxIdle)
	var paused []string
	for _, b := range boxes {
		if b.Paused || m.lastActivity(b).After(cutoff) {
			continue
		}
		if err := m.drv.Pause(ctx, b.Ref); err == nil {
			paused = append(paused, b.ID)
		}
	}
	return paused, nil
}

// History returns the recorded audit trail for a sandbox (oldest first).
func (m *Manager) History(id string) ([]HistoryEntry, error) {
	data, err := os.ReadFile(m.histPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []HistoryEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e HistoryEntry
		if json.Unmarshal([]byte(line), &e) == nil {
			out = append(out, e)
		}
	}
	return out, nil
}
