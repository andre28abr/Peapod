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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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
	var fwNet string
	if len(spec.Allow) > 0 {
		n, err := d.setupFirewall(ctx, id, spec.Allow)
		if err != nil {
			return sandbox.Sandbox{}, err
		}
		fwNet = n
	}
	created := time.Now()
	_, errOut, code, err := d.run(ctx, nil, createArgs(id, spec, created, fwNet)...)
	if err != nil {
		d.teardownFirewall(ctx, id)
		return sandbox.Sandbox{}, err
	}
	if code != 0 {
		d.teardownFirewall(ctx, id)
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
func createArgs(id string, spec sandbox.Spec, created time.Time, fwNet string) []string {
	name := containerName(id)
	netLabel := string(spec.Network)
	if fwNet != "" {
		netLabel = "allow" // firewalled egress via the proxy sidecar
	}
	args := []string{
		"run", "-d", "--name", name,
		"--label", "peapod.managed=true",
		"--label", "peapod.id=" + id,
		"--label", "peapod.image=" + spec.Image,
		"--label", "peapod.network=" + netLabel,
		"--label", "peapod.workdir=" + spec.Workdir,
		"--label", "peapod.created=" + strconv.FormatInt(created.UnixNano(), 10),
		"-w", spec.Workdir,
	}
	if spec.Name != "" {
		args = append(args, "--label", "peapod.name="+spec.Name)
	}
	if fwNet != "" {
		// Only route off the sandbox is the proxy sidecar; point tools at it.
		args = append(args, "--network", fwNet)
		proxyURL := "http://" + fwSidecarName(id) + ":8899"
		for _, k := range proxyEnvKeys {
			args = append(args, "-e", k+"="+proxyURL)
		}
	} else {
		// No firewall ⇒ no proxy env. Set them empty explicitly: docker commit
		// persists a container's Env, so a snapshot of a firewalled sandbox would
		// otherwise carry a dead sidecar URL into every fork.
		for _, k := range proxyEnvKeys {
			args = append(args, "-e", k+"=")
		}
		if spec.Network == sandbox.NetworkNone {
			args = append(args, "--network", "none")
		}
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
	for _, m := range spec.Mounts {
		v := m.Host + ":" + m.Target
		if m.ReadOnly {
			v += ":ro"
		}
		args = append(args, "-v", v)
	}
	for _, p := range spec.Ports {
		args = append(args, "-p", fmt.Sprintf("%d:%d", p.Host, p.Container))
	}
	for k, v := range spec.Env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, spec.Image, "sleep", "infinity")
	return args
}

func fwNetName(id string) string     { return "peapod-net-" + id }
func fwSidecarName(id string) string { return "peapod-fw-" + id }

// proxyEnvKeys are the env vars HTTP tooling honours for an egress proxy.
var proxyEnvKeys = []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"}

// fwSidecarImage is pinned so the firewall sidecar is reproducible.
const fwSidecarImage = "alpine:3.20"

// sweepGrace keeps Sweep from tearing down the firewall of a sandbox that is
// still being created (its container isn't listed yet).
const sweepGrace = 2 * time.Minute

var _ sandbox.Sweeper = (*Driver)(nil)

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// peapodLinuxBin locates the static linux `peapod` binary used to run the egress
// proxy inside the firewall sidecar (a Linux container). It is required for the
// bypass-proof --allow firewall; we fail closed if it's missing.
func peapodLinuxBin() (string, error) {
	// Resolve symlinks: `docker cp` of a symlink lands a dangling link in the
	// container (e.g. Homebrew's bin/peapod-linux-* points into the Cellar).
	resolve := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return p
	}
	if p := os.Getenv("PEAPOD_LINUX_BIN"); p != "" && fileExists(p) {
		return resolve(p), nil
	}
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(resolve(exe)), "peapod-linux-"+runtime.GOARCH)
		if fileExists(cand) {
			return resolve(cand), nil
		}
	}
	return "", fmt.Errorf("--allow firewall needs the linux proxy binary: set PEAPOD_LINUX_BIN "+
		"or place peapod-linux-%s next to the peapod binary", runtime.GOARCH)
}

// setupFirewall builds the bypass-proof egress firewall for a sandbox:
//   - an --internal Docker network the sandbox cannot route off of, and
//   - a proxy sidecar (the only thing bridging internal↔egress) that enforces
//     the domain allowlist.
//
// Because the sandbox's sole network is internal, even a process that ignores
// HTTP(S)_PROXY has no route out — so the allowlist can't be bypassed. Returns
// the internal network name to attach the sandbox to.
func (d *Driver) setupFirewall(ctx context.Context, id string, allow []string) (string, error) {
	lbin, err := peapodLinuxBin()
	if err != nil {
		return "", err
	}
	net, sc := fwNetName(id), fwSidecarName(id)
	fail := func(format string, a ...any) (string, error) {
		d.teardownFirewall(ctx, id)
		return "", fmt.Errorf(format, a...)
	}
	step := func(what string, args ...string) error {
		_, e, code, err := d.run(ctx, nil, args...)
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("%s: %s", what, strings.TrimSpace(e))
		}
		return nil
	}
	// 1) internal network — no route off it.
	if err := step("create firewall network", "network", "create", "--internal", "--label", "peapod.fw="+id, net); err != nil {
		return fail("%v", err)
	}
	// 2) sidecar on the egress (bridge) network, so its default route reaches out.
	if err := step("start firewall sidecar", "run", "-d", "--name", sc, "--label", "peapod.fw="+id,
		"--network", "bridge", "--cpus", "1", "--memory", "256m", "--pids-limit", "64",
		fwSidecarImage, "sleep", "infinity"); err != nil {
		return fail("%v", err)
	}
	// 3) attach the sidecar to the internal network so the sandbox can reach it.
	if err := step("attach sidecar", "network", "connect", net, sc); err != nil {
		return fail("%v", err)
	}
	// 4) inject the proxy binary and start it on the internal interface.
	if err := step("copy proxy", "cp", lbin, sc+":/peapod"); err != nil {
		return fail("%v", err)
	}
	// docker cp doesn't reliably preserve the executable bit across runtimes.
	if err := step("mark proxy executable", "exec", sc, "chmod", "+x", "/peapod"); err != nil {
		return fail("%v", err)
	}
	if err := step("start proxy", "exec", "-d", sc, "/peapod", "proxy", "--allow", strings.Join(allow, ","), "--addr", ":8899"); err != nil {
		return fail("%v", err)
	}
	return net, nil
}

// teardownFirewall removes a sandbox's firewall sidecar and network (best effort;
// no-op when the sandbox had no firewall).
func (d *Driver) teardownFirewall(ctx context.Context, id string) {
	_, _, _, _ = d.run(ctx, nil, "rm", "-f", fwSidecarName(id))
	_, _, _, _ = d.run(ctx, nil, "network", "rm", fwNetName(id))
}

// Sweep removes firewall sidecars and networks whose sandbox no longer exists
// (peapod killed mid-setup, a failed create…). Resources younger than
// sweepGrace are left alone so a sandbox still being created isn't torn down
// under it. Returns the names removed.
func (d *Driver) Sweep(ctx context.Context, liveIDs []string) ([]string, error) {
	live := map[string]bool{}
	for _, id := range liveIDs {
		live[id] = true
	}
	orphans := func(kind string, listArgs ...string) ([]string, error) {
		out, errOut, code, err := d.run(ctx, nil, listArgs...)
		if err != nil {
			return nil, err
		}
		if code != 0 {
			return nil, fmt.Errorf("sweep %ss: %s", kind, strings.TrimSpace(errOut))
		}
		var names []string
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			name, id, ok := strings.Cut(strings.TrimSpace(line), "|")
			if !ok || name == "" || live[id] || d.youngerThan(ctx, kind, name, sweepGrace) {
				continue
			}
			names = append(names, name)
		}
		return names, nil
	}
	var removed []string
	sidecars, err := orphans("container", "ps", "-a", "--filter", "label=peapod.fw", "--format", `{{.Names}}|{{.Label "peapod.fw"}}`)
	if err != nil {
		return nil, err
	}
	for _, n := range sidecars {
		if _, _, code, err := d.run(ctx, nil, "rm", "-f", n); err == nil && code == 0 {
			removed = append(removed, n)
		}
	}
	nets, err := orphans("network", "network", "ls", "--filter", "label=peapod.fw", "--format", `{{.Name}}|{{.Label "peapod.fw"}}`)
	if err != nil {
		return nil, err
	}
	for _, n := range nets {
		if _, _, code, err := d.run(ctx, nil, "network", "rm", n); err == nil && code == 0 {
			removed = append(removed, n)
		}
	}
	return removed, nil
}

// youngerThan reports whether the container/network was created less than dur
// ago. Unknown ages count as young — better to leave a stray behind than to
// delete something mid-creation.
func (d *Driver) youngerThan(ctx context.Context, kind, name string, dur time.Duration) bool {
	args := []string{"inspect", "--format", "{{.Created}}", name}
	if kind == "network" {
		args = append([]string{"network"}, args...)
	}
	out, _, code, err := d.run(ctx, nil, args...)
	if err != nil || code != 0 {
		return true
	}
	t, perr := time.Parse(time.RFC3339Nano, strings.TrimSpace(out))
	if perr != nil {
		return true
	}
	return time.Since(t) < dur
}

// inspectMeta is the slice of `docker inspect` output the driver needs.
type inspectMeta struct {
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Paused bool `json:"Paused"`
	} `json:"State"`
}

// inspect fetches metadata for the named containers in ONE runtime call.
// Names that vanished in between are simply absent from the result.
func (d *Driver) inspect(ctx context.Context, names ...string) ([]inspectMeta, error) {
	if len(names) == 0 {
		return nil, nil
	}
	out, errOut, code, err := d.run(ctx, nil, append([]string{"inspect"}, names...)...)
	if err != nil {
		return nil, err
	}
	var metas []inspectMeta
	if jerr := json.Unmarshal([]byte(strings.TrimSpace(out)), &metas); jerr != nil {
		if code != 0 {
			return nil, fmt.Errorf("inspect failed: %s", strings.TrimSpace(errOut))
		}
		return nil, fmt.Errorf("parse inspect: %w", jerr)
	}
	return metas, nil
}

// fromMeta rebuilds a Sandbox from container labels (the backend is the source
// of truth, so this works across CLI invocations).
func (d *Driver) fromMeta(m inspectMeta) sandbox.Sandbox {
	labels := m.Config.Labels
	id := labels["peapod.id"]
	var created time.Time
	if ns, perr := strconv.ParseInt(labels["peapod.created"], 10, 64); perr == nil {
		created = time.Unix(0, ns)
	}
	return sandbox.Sandbox{
		ID: id, Backend: d.Name(), Ref: containerName(id),
		Image:   labels["peapod.image"],
		Name:    labels["peapod.name"],
		Network: sandbox.NetworkPolicy(labels["peapod.network"]),
		Workdir: labels["peapod.workdir"],
		Created: created,
		Paused:  m.State.Paused,
	}
}

// Resolve looks up one sandbox by id.
func (d *Driver) Resolve(ctx context.Context, id string) (sandbox.Sandbox, error) {
	metas, err := d.inspect(ctx, containerName(id))
	if err != nil {
		return sandbox.Sandbox{}, err
	}
	if len(metas) == 0 || metas[0].Config.Labels["peapod.id"] == "" {
		return sandbox.Sandbox{}, sandbox.ErrNotFound
	}
	return d.fromMeta(metas[0]), nil
}

// List finds every peapod-managed container in two runtime calls (ps, then one
// inspect for all the names) instead of one inspect per sandbox.
func (d *Driver) List(ctx context.Context) ([]sandbox.Sandbox, error) {
	out, _, code, err := d.run(ctx, nil, "ps", "-a", "--filter", "label=peapod.managed=true", "--format", "{{.Names}}")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New("list failed")
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	metas, err := d.inspect(ctx, names...)
	if err != nil {
		return nil, err
	}
	res := make([]sandbox.Sandbox, 0, len(metas))
	for _, m := range metas {
		if m.Config.Labels["peapod.id"] == "" {
			continue
		}
		res = append(res, d.fromMeta(m))
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

// WriteFile pipes data into the container via `exec -i ... cat > path`. The
// path travels as a positional argument — never interpolated into the script —
// so shell metacharacters in it ($, backticks, quotes) stay inert.
func (d *Driver) WriteFile(ctx context.Context, ref, p string, data []byte, mode uint32) error {
	const script = `set -e; mkdir -p "$(dirname "$1")"; cat > "$1"`
	_, errOut, code, err := d.run(ctx, data, "exec", "-i", ref, "sh", "-c", script, "sh", p)
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
	d.teardownFirewall(ctx, strings.TrimPrefix(ref, "peapod-"))
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

// ListSnapshots lists the peapod-snapshot images.
func (d *Driver) ListSnapshots(ctx context.Context) ([]sandbox.Snapshot, error) {
	out, _, code, err := d.run(ctx, nil, "images",
		"--filter", "reference=peapod-snapshot",
		"--format", "{{.Repository}}:{{.Tag}}|{{.Tag}}|{{.CreatedAt}}|{{.Size}}")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, errors.New("list snapshots failed")
	}
	var res []sandbox.Snapshot
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.SplitN(line, "|", 4)
		s := sandbox.Snapshot{Ref: f[0]}
		if len(f) > 1 {
			s.Name = f[1]
		}
		if len(f) > 2 {
			s.Created = f[2]
			// docker CreatedAt looks like "2026-06-19 09:26:18 -0300 -03"; the
			// trailing zone abbreviation isn't Go-parseable, so drop it.
			if p := strings.Fields(f[2]); len(p) >= 3 {
				if t, perr := time.Parse("2006-01-02 15:04:05 -0700", strings.Join(p[:3], " ")); perr == nil {
					s.CreatedUnix = t.Unix()
				}
			}
		}
		if len(f) > 3 {
			s.Size = f[3]
		}
		res = append(res, s)
	}
	return res, nil
}

// RemoveSnapshot deletes a snapshot image.
func (d *Driver) RemoveSnapshot(ctx context.Context, ref string) error {
	_, errOut, code, err := d.run(ctx, nil, "rmi", ref)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("remove snapshot failed: %s", strings.TrimSpace(errOut))
	}
	return nil
}

// Pause freezes the container's processes in memory (docker pause).
func (d *Driver) Pause(ctx context.Context, ref string) error {
	_, errOut, code, err := d.run(ctx, nil, "pause", ref)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("pause failed: %s", strings.TrimSpace(errOut))
	}
	return nil
}

// Resume unfreezes the container (docker unpause).
func (d *Driver) Resume(ctx context.Context, ref string) error {
	_, errOut, code, err := d.run(ctx, nil, "unpause", ref)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("resume failed: %s", strings.TrimSpace(errOut))
	}
	return nil
}

// Checkpoint persists the container's running state to disk (docker checkpoint).
// Experimental: needs a CRIU-capable engine. Note: restore is broken on OrbStack.
func (d *Driver) Checkpoint(ctx context.Context, ref, name string) error {
	_, errOut, code, err := d.run(ctx, nil, "checkpoint", "create", ref, name)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("checkpoint failed: %s", strings.TrimSpace(errOut))
	}
	return nil
}

// Restore restarts the container from a checkpoint (docker start --checkpoint).
func (d *Driver) Restore(ctx context.Context, ref, name string) error {
	_, errOut, code, err := d.run(ctx, nil, "start", "--checkpoint", name, ref)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("restore failed: %s", strings.TrimSpace(errOut))
	}
	return nil
}

// Logs returns the last `tail` lines of the container's output (stdout+stderr).
func (d *Driver) Logs(ctx context.Context, ref string, tail int) (string, error) {
	if tail <= 0 {
		tail = 200
	}
	out, errOut, code, err := d.run(ctx, nil, "logs", "--tail", strconv.Itoa(tail), ref)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("logs failed: %s", strings.TrimSpace(errOut))
	}
	return out + errOut, nil // docker splits container stdout/stderr across both
}

// Stats samples CPU and memory usage once (non-streaming).
func (d *Driver) Stats(ctx context.Context, ref string) (sandbox.Stat, error) {
	out, errOut, code, err := d.run(ctx, nil, "stats", "--no-stream",
		"--format", "{{.CPUPerc}}|{{.MemUsage}}|{{.MemPerc}}", ref)
	if err != nil {
		return sandbox.Stat{}, err
	}
	if code != 0 {
		return sandbox.Stat{}, fmt.Errorf("stats failed: %s", strings.TrimSpace(errOut))
	}
	f := strings.SplitN(strings.TrimSpace(out), "|", 3)
	var st sandbox.Stat
	if len(f) > 0 {
		st.CPUPerc = f[0]
	}
	if len(f) > 1 {
		st.MemUsage = f[1]
	}
	if len(f) > 2 {
		st.MemPerc = f[2]
	}
	return st, nil
}

// DiffSnapshots compares the file lists of two snapshot images.
func (d *Driver) DiffSnapshots(ctx context.Context, a, b string) (sandbox.SnapshotDiff, error) {
	listFiles := func(img string) (map[string]bool, error) {
		out, errOut, code, err := d.run(ctx, nil, "run", "--rm", "--entrypoint", "sh",
			"--network", "none", img, "-c", "find / -xdev -type f 2>/dev/null | sort")
		if err != nil {
			return nil, err
		}
		if code != 0 {
			return nil, fmt.Errorf("list %s: %s", img, strings.TrimSpace(errOut))
		}
		set := map[string]bool{}
		for _, l := range strings.Split(out, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				set[l] = true
			}
		}
		return set, nil
	}
	fa, err := listFiles(a)
	if err != nil {
		return sandbox.SnapshotDiff{}, err
	}
	fb, err := listFiles(b)
	if err != nil {
		return sandbox.SnapshotDiff{}, err
	}
	var diff sandbox.SnapshotDiff
	for f := range fb {
		if !fa[f] {
			diff.Added = append(diff.Added, f)
		}
	}
	for f := range fa {
		if !fb[f] {
			diff.Removed = append(diff.Removed, f)
		}
	}
	sort.Strings(diff.Added)
	sort.Strings(diff.Removed)
	return diff, nil
}
