package oci

import (
	"context"
	"strings"
	"testing"
	"time"

	"peapod/internal/sandbox"
)

func TestCreateArgs(t *testing.T) {
	spec := sandbox.Spec{
		Image:     "alpine",
		Name:      "demo",
		Network:   sandbox.NetworkNone,
		Workdir:   "/work",
		Resources: sandbox.Resources{CPUs: 2, MemoryMB: 512, PidsLimit: 256},
		Labels:    map[string]string{"peapod.id": "pp_x"},
	}
	got := strings.Join(createArgs("pp_x", spec, time.Unix(0, 0), ""), " ")
	for _, want := range []string{
		"run -d --name peapod-pp_x",
		"--label peapod.managed=true",
		"--label peapod.id=pp_x",
		"--network none",
		"--cpus 2",
		"--memory 512m",
		"--pids-limit 256",
		"-w /work",
		"-e HTTP_PROXY=", // no firewall ⇒ proxy env explicitly emptied
		"alpine sleep infinity",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("createArgs missing %q\n got: %s", want, got)
		}
	}
	if strings.Contains(got, "HTTP_PROXY=http") {
		t.Errorf("a no-firewall sandbox must not point at a proxy: %s", got)
	}
}

func TestCreateArgsEgressKeepsNetwork(t *testing.T) {
	spec := sandbox.Spec{Image: "alpine", Network: sandbox.NetworkEgress, Workdir: "/work",
		Labels: map[string]string{"peapod.id": "pp_y"}}
	got := strings.Join(createArgs("pp_y", spec, time.Unix(0, 0), ""), " ")
	if strings.Contains(got, "--network none") {
		t.Errorf("egress must not disable the network: %s", got)
	}
}

// TestCreateArgsFirewall checks that an --allow sandbox is wired to the internal
// firewall network and pointed at the sidecar proxy (and never left on "none").
func TestCreateArgsFirewall(t *testing.T) {
	spec := sandbox.Spec{Image: "alpine", Network: sandbox.NetworkNone, Workdir: "/work",
		Allow: []string{"pypi.org"}, Labels: map[string]string{"peapod.id": "pp_z"}}
	got := strings.Join(createArgs("pp_z", spec, time.Unix(0, 0), "peapod-net-pp_z"), " ")
	for _, want := range []string{
		"--network peapod-net-pp_z",
		"-e HTTP_PROXY=http://peapod-fw-pp_z:8899",
		"-e https_proxy=http://peapod-fw-pp_z:8899",
		"--label peapod.network=allow",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("firewall createArgs missing %q\n got: %s", want, got)
		}
	}
	if strings.Contains(got, "--network none") {
		t.Errorf("firewall sandbox must not be on network none: %s", got)
	}
}

// TestIntegration runs a real lifecycle. It skips unless a container runtime
// with a live daemon is present, so it's safe in CI / when the daemon is down.
func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	d, err := New()
	if err != nil {
		t.Skipf("no container runtime: %v", err)
	}
	ctx := context.Background()
	if _, _, code, err := d.run(ctx, nil, "info"); err != nil || code != 0 {
		t.Skip("container daemon not available")
	}

	mgr := sandbox.NewManager(d)
	sb, err := mgr.Create(ctx, sandbox.Spec{Image: "alpine"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = mgr.Destroy(ctx, sb.ID) }()

	res, err := mgr.Exec(ctx, sb.ID, []string{"echo", "hi"}, sandbox.ExecOpts{})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(res.Stdout, "hi") {
		t.Errorf("exec stdout = %q, want it to contain 'hi'", res.Stdout)
	}

	// The exec timeout must be enforced *inside* the sandbox: a long sleep gets
	// killed by the watchdog (non-zero exit) instead of outliving the client.
	start := time.Now()
	res, err = mgr.Exec(ctx, sb.ID, []string{"sleep", "60"}, sandbox.ExecOpts{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("exec with timeout: %v", err)
	}
	if el := time.Since(start); el > 15*time.Second {
		t.Errorf("timeout not enforced: took %v", el)
	}
	if res.ExitCode == 0 {
		t.Errorf("timed-out command should not exit 0 (got %d)", res.ExitCode)
	}
	// Match the exact timed-out command (the sandbox's own PID-1 `sleep infinity`
	// must be ignored); the [s] trick keeps grep from matching itself.
	res, _ = mgr.Exec(ctx, sb.ID, []string{"sh", "-c", "ps -o args | grep -q '[s]leep 60' && echo alive || echo none"}, sandbox.ExecOpts{})
	if !strings.Contains(res.Stdout, "none") {
		t.Errorf("sleep 60 survived the timeout inside the sandbox: %q", res.Stdout)
	}

	// WriteFile must treat shell metacharacters in the path literally.
	tricky := "/work/a$(echo pwned)/b`id`.txt"
	if err := mgr.WriteFile(ctx, sb.ID, tricky, []byte("safe"), 0o644); err != nil {
		t.Fatalf("write tricky path: %v", err)
	}
	data, err := mgr.ReadFile(ctx, sb.ID, tricky)
	if err != nil || string(data) != "safe" {
		t.Errorf("tricky path round-trip = %q, %v", data, err)
	}
	res, _ = mgr.Exec(ctx, sb.ID, []string{"sh", "-c", "ls -d /work/apwned /work/a 2>/dev/null || echo clean"}, sandbox.ExecOpts{})
	if !strings.Contains(res.Stdout, "clean") {
		t.Errorf("path was expanded by the shell: %q", res.Stdout)
	}
}
