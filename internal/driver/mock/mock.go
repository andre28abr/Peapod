// Package mock is an in-memory sandbox.Driver for tests and daemon-less dev.
package mock

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"peapod/internal/sandbox"
)

// Driver keeps sandboxes and their files in memory.
type Driver struct {
	mu       sync.Mutex
	boxes    map[string]sandbox.Sandbox
	files    map[string]map[string][]byte // ref -> path -> data
	snaps    map[string]map[string][]byte // snapshotRef -> path -> data
	snapMeta map[string]sandbox.Snapshot  // snapshotRef -> metadata
	paused   map[string]bool              // ref -> paused
	seq      int
}

// New returns an empty in-memory driver.
func New() *Driver {
	return &Driver{
		boxes:    map[string]sandbox.Sandbox{},
		files:    map[string]map[string][]byte{},
		snaps:    map[string]map[string][]byte{},
		snapMeta: map[string]sandbox.Snapshot{},
		paused:   map[string]bool{},
	}
}

// Name reports the backend.
func (d *Driver) Name() string { return "mock" }

func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

// Create records a new sandbox.
func (d *Driver) Create(_ context.Context, spec sandbox.Spec) (sandbox.Sandbox, error) {
	id := spec.Labels["peapod.id"]
	if id == "" {
		return sandbox.Sandbox{}, fmt.Errorf("missing peapod.id label")
	}
	sb := sandbox.Sandbox{
		ID: id, Backend: d.Name(), Ref: id,
		Image: spec.Image, Name: spec.Name,
		Network: spec.Network, Workdir: spec.Workdir,
		Created: time.Now(),
	}
	d.mu.Lock()
	d.boxes[id] = sb
	d.files[id] = map[string][]byte{}
	d.mu.Unlock()
	return sb, nil
}

// Resolve looks up a sandbox by id.
func (d *Driver) Resolve(_ context.Context, id string) (sandbox.Sandbox, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	sb, ok := d.boxes[id]
	if !ok {
		return sandbox.Sandbox{}, sandbox.ErrNotFound
	}
	sb.Paused = d.paused[id]
	return sb, nil
}

// List returns all sandboxes.
func (d *Driver) List(_ context.Context) ([]sandbox.Sandbox, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	res := make([]sandbox.Sandbox, 0, len(d.boxes))
	for _, sb := range d.boxes {
		sb.Paused = d.paused[sb.ID]
		res = append(res, sb)
	}
	return res, nil
}

// Exec echoes the command (no real execution in the mock).
func (d *Driver) Exec(_ context.Context, _ string, argv []string, _ sandbox.ExecOpts) (sandbox.ExecResult, error) {
	return sandbox.ExecResult{Stdout: "mock$ " + strings.Join(argv, " ") + "\n", ExitCode: 0}, nil
}

// WriteFile stores a file in memory.
func (d *Driver) WriteFile(_ context.Context, ref, p string, data []byte, _ uint32) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.files[ref] == nil {
		return sandbox.ErrNotFound
	}
	d.files[ref][p] = cloneBytes(data)
	return nil
}

// ReadFile reads a stored file.
func (d *Driver) ReadFile(_ context.Context, ref, p string) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f, ok := d.files[ref]
	if !ok {
		return nil, sandbox.ErrNotFound
	}
	data, ok := f[p]
	if !ok {
		return nil, fmt.Errorf("no such file: %s", p)
	}
	return cloneBytes(data), nil
}

// Destroy removes a sandbox and its files.
func (d *Driver) Destroy(_ context.Context, ref string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.boxes, ref)
	delete(d.files, ref)
	return nil
}

// Snapshot copies a sandbox's files under a new ref.
func (d *Driver) Snapshot(_ context.Context, ref, name string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	f, ok := d.files[ref]
	if !ok {
		return "", sandbox.ErrNotFound
	}
	d.seq++
	snapRef := fmt.Sprintf("snap:%s:%s:%d", ref, name, d.seq)
	cp := map[string][]byte{}
	for k, v := range f {
		cp[k] = cloneBytes(v)
	}
	d.snaps[snapRef] = cp
	now := time.Now()
	d.snapMeta[snapRef] = sandbox.Snapshot{Ref: snapRef, Name: name, Created: now.UTC().Format(time.RFC3339), CreatedUnix: now.Unix()}
	return snapRef, nil
}

// Fork creates a sandbox seeded from a snapshot's files.
func (d *Driver) Fork(_ context.Context, snapshotRef string, spec sandbox.Spec) (sandbox.Sandbox, error) {
	id := spec.Labels["peapod.id"]
	if id == "" {
		return sandbox.Sandbox{}, fmt.Errorf("missing peapod.id label")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	snap, ok := d.snaps[snapshotRef]
	if !ok {
		return sandbox.Sandbox{}, fmt.Errorf("no such snapshot: %s", snapshotRef)
	}
	sb := sandbox.Sandbox{ID: id, Backend: d.Name(), Ref: id, Image: snapshotRef, Name: spec.Name, Network: spec.Network, Workdir: spec.Workdir, Created: time.Now()}
	d.boxes[id] = sb
	nf := map[string][]byte{}
	for k, v := range snap {
		nf[k] = cloneBytes(v)
	}
	d.files[id] = nf
	return sb, nil
}

// ListSnapshots returns the recorded snapshots.
func (d *Driver) ListSnapshots(_ context.Context) ([]sandbox.Snapshot, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	res := make([]sandbox.Snapshot, 0, len(d.snapMeta))
	for _, s := range d.snapMeta {
		res = append(res, s)
	}
	return res, nil
}

// RemoveSnapshot deletes a recorded snapshot.
func (d *Driver) RemoveSnapshot(_ context.Context, ref string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.snaps, ref)
	delete(d.snapMeta, ref)
	return nil
}

// Pause marks a sandbox as paused.
func (d *Driver) Pause(_ context.Context, ref string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.boxes[ref]; !ok {
		return sandbox.ErrNotFound
	}
	d.paused[ref] = true
	return nil
}

// Resume clears the paused mark.
func (d *Driver) Resume(_ context.Context, ref string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.boxes[ref]; !ok {
		return sandbox.ErrNotFound
	}
	delete(d.paused, ref)
	return nil
}
