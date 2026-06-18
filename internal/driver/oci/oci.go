// Package oci implements sandbox.Driver on top of the docker (or podman) CLI.
//
// Isolation is container-level inside the runtime's shared Linux VM — fast and
// good enough for semi-trusted agent code. For one-microVM-per-sandbox
// isolation, swap in the apple-container or libkrun driver behind sandbox.Driver.
package oci

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

// Driver runs each sandbox as a container.
type Driver struct {
	bin string
}

// New picks the first available runtime: docker, then podman.
func New() (*Driver, error) {
	for _, b := range []string{"docker", "podman"} {
		if _, err := exec.LookPath(b); err == nil {
			return &Driver{bin: b}, nil
		}
	}
	return nil, errors.New("no container runtime found in PATH (install docker or podman)")
}

// Name reports the backend, e.g. "oci:docker".
func (d *Driver) Name() string { return "oci:" + d.bin }

func containerName(id string) string { return "peapod-" + id }

// run executes the runtime CLI. A non-zero exit is returned as exitCode (not an
// error); only a failure to launch the process is an error.
func (d *Driver) run(ctx context.Context, stdin []byte, args ...string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, d.bin, args...)
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
		return out.String(), errb.String(), -1, fmt.Errorf("%s %s: %w", d.bin, strings.Join(args, " "), runErr)
	}
	return out.String(), errb.String(), 0, nil
}

// Create starts a long-lived container we can exec into.
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

// createArgs builds the argv for `<runtime> run ...`. It performs no I/O so it
// can be unit-tested without a container daemon.
func createArgs(id string, spec sandbox.Spec, created time.Time) []string {
	name := containerName(id)
	args := []string{
		"run", "-d", "--name", name,
		"--label", "peapod.managed=true",
		"--label", "peapod.id=" + id,
		"--label", "peapod.image=" + spec.Image,
		"--label", "peapod.network=" + string(spec.Network),
		"--label", "peapod.workdir=" + spec.Workdir,
		"--label", "peapod.created=" + strconv.FormatInt(created.UnixNano(), 10),
		"-w", spec.Workdir,
	}
	if spec.Name != "" {
		args = append(args, "--label", "peapod.name="+spec.Name)
	}
	if spec.Network == sandbox.NetworkNone {
		args = append(args, "--network", "none")
	}
	if spec.Resources.CPUs > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(spec.Resources.CPUs, 'g', -1, 64))
	}
	if spec.Resources.MemoryMB > 0 {
		args = append(args, "--memory", strconv.Itoa(spec.Resources.MemoryMB)+"m")
	}
	if spec.Resources.PidsLimit > 0 {
		args = append(args, "--pids-limit", strconv.Itoa(spec.Resources.PidsLimit))
	}
	for k, v := range spec.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, spec.Image, "sleep", "infinity")
	return args
}

// Resolve reconstructs a sandbox from the container's labels (the backend is
// the source of truth, so it works across CLI invocations).
func (d *Driver) Resolve(ctx context.Context, id string) (sandbox.Sandbox, error) {
	name := containerName(id)
	out, _, code, err := d.run(ctx, nil, "inspect", "--format", "{{json .Config.Labels}}", name)
	if err != nil {
		return sandbox.Sandbox{}, err
	}
	if code != 0 {
		return sandbox.Sandbox{}, sandbox.ErrNotFound
	}
	var labels map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &labels); err != nil {
		return sandbox.Sandbox{}, fmt.Errorf("parse labels: %w", err)
	}
	var created time.Time
	if ns, perr := strconv.ParseInt(labels["peapod.created"], 10, 64); perr == nil {
		created = time.Unix(0, ns)
	}
	return sandbox.Sandbox{
		ID: id, Backend: d.Name(), Ref: name,
		Image:   labels["peapod.image"],
		Name:    labels["peapod.name"],
		Network: sandbox.NetworkPolicy(labels["peapod.network"]),
		Workdir: labels["peapod.workdir"],
		Created: created,
	}, nil
}

// List finds every peapod-managed container.
func (d *Driver) List(ctx context.Context) ([]sandbox.Sandbox, error) {
	out, _, code, err := d.run(ctx, nil, "ps", "-a", "--filter", "label=peapod.managed=true", "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New("list failed")
	}
	var res []sandbox.Sandbox
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sb, err := d.Resolve(ctx, strings.TrimPrefix(line, "peapod-"))
		if err != nil {
			continue
		}
		res = append(res, sb)
	}
	return res, nil
}

// Exec runs argv inside the container.
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
	if opts.Workdir != "" {
		args = append(args, "-w", opts.Workdir)
	}
	for k, v := range opts.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, ref)
	args = append(args, argv...)
	out, errOut, code, err := d.run(ctx, nil, args...)
	if err != nil {
		return sandbox.ExecResult{}, err
	}
	return sandbox.ExecResult{Stdout: out, Stderr: errOut, ExitCode: code}, nil
}

// WriteFile pipes data into the container via `exec -i ... cat > path`.
func (d *Driver) WriteFile(ctx context.Context, ref, p string, data []byte, mode uint32) error {
	script := fmt.Sprintf("set -e; mkdir -p %q; cat > %q", path.Dir(p), p)
	_, errOut, code, err := d.run(ctx, data, "exec", "-i", ref, "sh", "-c", script)
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

// ReadFile cats a file out of the container.
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

// Destroy force-removes the container.
func (d *Driver) Destroy(ctx context.Context, ref string) error {
	_, errOut, code, err := d.run(ctx, nil, "rm", "-f", ref)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("destroy failed: %s", strings.TrimSpace(errOut))
	}
	return nil
}

// Snapshot commits the container's filesystem to an image (Phase 2 preview).
// Running-state (memory) snapshots arrive with the microVM drivers.
func (d *Driver) Snapshot(ctx context.Context, ref, name string) (string, error) {
	img := "peapod-snapshot:" + name
	_, errOut, code, err := d.run(ctx, nil, "commit", ref, img)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("snapshot failed: %s", strings.TrimSpace(errOut))
	}
	return img, nil
}

// Fork creates a fresh sandbox from a snapshot image.
func (d *Driver) Fork(ctx context.Context, snapshotRef string, spec sandbox.Spec) (sandbox.Sandbox, error) {
	spec.Image = snapshotRef
	return d.Create(ctx, spec)
}
