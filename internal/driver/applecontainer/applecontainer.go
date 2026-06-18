// Package applecontainer implements sandbox.Driver on Apple's `container` CLI,
// which runs each container in its own lightweight VM via Virtualization.framework
// (macOS 26+, Apple silicon) — one-microVM-per-sandbox isolation, the real
// isolation upgrade over the shared-VM oci driver.
//
// Validated live against apple `container` 1.0.0 on macOS 26: `--network none`
// disables networking, and labels appear under configuration.labels in the
// `inspect` JSON (located via walkLabels). apple `container` has no image
// commit, so Snapshot/Fork return unsupported here.
package applecontainer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"peapod/internal/sandbox"
)

const bin = "container"

// errUnsupported: apple `container` has no image-commit, so filesystem snapshots
// (the Phase 2 preview the oci driver offers) aren't available here yet.
var errUnsupported = errors.New("apple-container backend does not support snapshot/fork yet (no image commit)")

// Driver runs each sandbox as a microVM-backed container.
type Driver struct{}

// New returns the driver if the `container` CLI is installed.
func New() (*Driver, error) {
	if _, err := exec.LookPath(bin); err != nil {
		return nil, errors.New("apple `container` CLI not found in PATH (needs macOS 26 + github.com/apple/container)")
	}
	return &Driver{}, nil
}

// Name reports the backend.
func (d *Driver) Name() string { return "apple-container" }

func containerName(id string) string { return "peapod-" + id }

func (d *Driver) run(ctx context.Context, stdin []byte, args ...string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	runErr := cmd.Run()
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			return out.String(), errb.String(), ee.ExitCode(), nil
		}
		return out.String(), errb.String(), -1, fmt.Errorf("%s %s: %w", bin, strings.Join(args, " "), runErr)
	}
	return out.String(), errb.String(), 0, nil
}

// createArgs builds the argv for `container run ...` (pure, unit-testable).
func createArgs(id string, spec sandbox.Spec, created time.Time) []string {
	name := containerName(id)
	args := []string{
		"run", "-d", "--name", name,
		"-l", "peapod.managed=true",
		"-l", "peapod.id=" + id,
		"-l", "peapod.image=" + spec.Image,
		"-l", "peapod.network=" + string(spec.Network),
		"-l", "peapod.workdir=" + spec.Workdir,
		"-l", "peapod.created=" + strconv.FormatInt(created.UnixNano(), 10),
	}
	if spec.Name != "" {
		args = append(args, "-l", "peapod.name="+spec.Name)
	}
	if spec.Network == sandbox.NetworkNone {
		// Validated on container 1.0.0: leaves the microVM with no network.
		args = append(args, "--network", "none")
	}
	if spec.Resources.CPUs > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(spec.Resources.CPUs, 'g', -1, 64))
	}
	if spec.Resources.MemoryMB > 0 {
		args = append(args, "--memory", strconv.Itoa(spec.Resources.MemoryMB)+"M")
	}
	for _, m := range spec.Mounts {
		v := m.Host + ":" + m.Target
		if m.ReadOnly {
			v += ":ro"
		}
		args = append(args, "-v", v)
	}
	for k, v := range spec.Env {
		args = append(args, "--env", k+"="+v)
	}
	args = append(args, spec.Image, "sleep", "infinity")
	return args
}

// Create starts a long-lived microVM container.
func (d *Driver) Create(ctx context.Context, spec sandbox.Spec) (sandbox.Sandbox, error) {
	id := spec.Labels["peapod.id"]
	if id == "" {
		return sandbox.Sandbox{}, errors.New("missing peapod.id label (call via Manager)")
	}
	created := time.Now()
	_, errOut, code, err := d.run(ctx, nil, createArgs(id, spec, created)...)
	if err != nil {
		return sandbox.Sandbox{}, err
	}
	if code != 0 {
		return sandbox.Sandbox{}, fmt.Errorf("create failed: %s", strings.TrimSpace(errOut))
	}
	return sandbox.Sandbox{
		ID: id, Backend: d.Name(), Ref: containerName(id),
		Image: spec.Image, Name: spec.Name,
		Network: spec.Network, Workdir: spec.Workdir, Created: created,
	}, nil
}

func sandboxFromLabels(backend string, labels map[string]string) sandbox.Sandbox {
	id := labels["peapod.id"]
	var created time.Time
	if ns, err := strconv.ParseInt(labels["peapod.created"], 10, 64); err == nil {
		created = time.Unix(0, ns)
	}
	return sandbox.Sandbox{
		ID: id, Backend: backend, Ref: containerName(id),
		Image:   labels["peapod.image"],
		Name:    labels["peapod.name"],
		Network: sandbox.NetworkPolicy(labels["peapod.network"]),
		Workdir: labels["peapod.workdir"],
		Created: created,
	}
}

// Resolve reconstructs a sandbox from its container labels.
func (d *Driver) Resolve(ctx context.Context, id string) (sandbox.Sandbox, error) {
	name := containerName(id)
	out, _, code, err := d.run(ctx, nil, "inspect", name)
	if err != nil {
		return sandbox.Sandbox{}, err
	}
	if code != 0 {
		return sandbox.Sandbox{}, sandbox.ErrNotFound
	}
	labels := findPeapodLabels([]byte(out))
	if labels == nil {
		// Exists, but labels weren't found in the inspect JSON — minimal view.
		return sandbox.Sandbox{ID: id, Backend: d.Name(), Ref: name, Workdir: "/work"}, nil
	}
	return sandboxFromLabels(d.Name(), labels), nil
}

// List finds every peapod-managed microVM container.
func (d *Driver) List(ctx context.Context) ([]sandbox.Sandbox, error) {
	out, _, code, err := d.run(ctx, nil, "ls", "--all", "--quiet")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New("list failed")
	}
	var res []sandbox.Sandbox
	for _, ref := range strings.Split(strings.TrimSpace(out), "\n") {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		io, _, c, e := d.run(ctx, nil, "inspect", ref)
		if e != nil || c != 0 {
			continue
		}
		labels := findPeapodLabels([]byte(io))
		if labels == nil || labels["peapod.managed"] != "true" {
			continue
		}
		res = append(res, sandboxFromLabels(d.Name(), labels))
	}
	return res, nil
}

// findPeapodLabels recursively searches inspect JSON for the labels object that
// carries our peapod.id key, tolerating an unknown surrounding schema.
func findPeapodLabels(data []byte) map[string]string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil
	}
	return walkLabels(v)
}

func walkLabels(v any) map[string]string {
	switch t := v.(type) {
	case map[string]any:
		if _, ok := t["peapod.id"]; ok {
			out := map[string]string{}
			for k, val := range t {
				if s, ok := val.(string); ok {
					out[k] = s
				}
			}
			return out
		}
		for _, val := range t {
			if r := walkLabels(val); r != nil {
				return r
			}
		}
	case []any:
		for _, val := range t {
			if r := walkLabels(val); r != nil {
				return r
			}
		}
	}
	return nil
}

// Exec runs argv inside the microVM. apple `container exec` has no --workdir flag,
// so a working directory is emulated with a tiny sh wrapper.
func (d *Driver) Exec(ctx context.Context, ref string, argv []string, opts sandbox.ExecOpts) (sandbox.ExecResult, error) {
	if len(argv) == 0 {
		return sandbox.ExecResult{}, errors.New("empty command")
	}
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	args := []string{"exec"}
	for k, v := range opts.Env {
		args = append(args, "--env", k+"="+v)
	}
	args = append(args, ref)
	if opts.Workdir != "" {
		// apple `container` (unlike docker) doesn't create the workdir, so make
		// it here before cd'ing into it.
		args = append(args, "sh", "-c", `mkdir -p "$1"; cd "$1"; shift; exec "$@"`, "sh", opts.Workdir)
	}
	args = append(args, argv...)
	out, errOut, code, err := d.run(ctx, nil, args...)
	if err != nil {
		return sandbox.ExecResult{}, err
	}
	return sandbox.ExecResult{Stdout: out, Stderr: errOut, ExitCode: code}, nil
}

// WriteFile pipes data into the microVM via `exec --interactive ... cat > path`.
func (d *Driver) WriteFile(ctx context.Context, ref, p string, data []byte, mode uint32) error {
	script := fmt.Sprintf("set -e; mkdir -p %q; cat > %q", path.Dir(p), p)
	_, errOut, code, err := d.run(ctx, data, "exec", "--interactive", ref, "sh", "-c", script)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("write failed: %s", strings.TrimSpace(errOut))
	}
	if mode != 0 {
		_, _, _, _ = d.run(ctx, nil, "exec", ref, "chmod", fmt.Sprintf("%o", mode), p)
	}
	return nil
}

// ReadFile cats a file out of the microVM.
func (d *Driver) ReadFile(ctx context.Context, ref, p string) ([]byte, error) {
	out, errOut, code, err := d.run(ctx, nil, "exec", ref, "cat", p)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("read failed: %s", strings.TrimSpace(errOut))
	}
	return []byte(out), nil
}

// Destroy force-removes the microVM container.
func (d *Driver) Destroy(ctx context.Context, ref string) error {
	_, errOut, code, err := d.run(ctx, nil, "rm", "--force", ref)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("destroy failed: %s", strings.TrimSpace(errOut))
	}
	return nil
}

// Snapshot is not supported by the apple-container backend yet.
func (d *Driver) Snapshot(ctx context.Context, ref, name string) (string, error) {
	return "", errUnsupported
}

// Fork is not supported by the apple-container backend yet.
func (d *Driver) Fork(ctx context.Context, snapshotRef string, spec sandbox.Spec) (sandbox.Sandbox, error) {
	return sandbox.Sandbox{}, errUnsupported
}

// ListSnapshots returns nothing — apple container has no image commit.
func (d *Driver) ListSnapshots(ctx context.Context) ([]sandbox.Snapshot, error) {
	return nil, nil
}

// RemoveSnapshot is not supported by the apple-container backend.
func (d *Driver) RemoveSnapshot(ctx context.Context, ref string) error {
	return errUnsupported
}
