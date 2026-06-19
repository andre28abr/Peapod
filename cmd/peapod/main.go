// Command peapod runs the MCP server and a small CLI for driving sandboxes.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"peapod/internal/backend"
	"peapod/internal/mcpserver"
	"peapod/internal/proxy"
	"peapod/internal/sandbox"
	"peapod/internal/web"
)

const usage = `peapod — disposable, isolated sandboxes for AI agents

usage:
  peapod [--backend oci|apple|mock] <command>

commands:
  mcp                                 start the MCP server (stdio)
  ui [--addr 127.0.0.1:7070]          start the local web dashboard
  sandbox create <image> [--name N] [--net none|egress] [--ports h:c,...] [--allow d1,d2]
  sandbox ls
  sandbox exec <id> <cmd>...
  sandbox rm <id>
  sandbox pause <id>
  sandbox resume <id>
  sandbox checkpoint <id> <name>      (experimental; CRIU-capable engines)
  sandbox restore <id> <name>
  sandbox logs <id> [--tail N]
  sandbox stats <id>
  sandbox history <id>                 audit trail of commands run in the sandbox
  sandbox snapshot <id> <name>
  sandbox fork <snapshot> [--name N] [--net none|egress]
  snapshot ls
  snapshot rm <ref>
  snapshot prune [--max-age 24h]
  snapshot diff <refA> <refB>
  preview up [--image IMG] [--net none|egress]    sandbox for the current git branch
  preview status
  preview down
  reap [--max-age 30m]                destroy sandboxes older than max-age
  pause-idle [--max-idle 15m]         pause sandboxes idle for too long
  up [-f peapod.json]                 start a multi-service group
  down [-f peapod.json]               stop the group
  ps [-f peapod.json]                 list the group
  templates [--json]                  list quick-start images
  proxy --allow d1,d2 [--addr :8899]  egress allowlist proxy for sandboxes
  version

backend also via env PEAPOD_BACKEND (default: oci)
`

var backendName = envOr("PEAPOD_BACKEND", "oci")

func main() {
	args := stripBackend(os.Args[1:])
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx := context.Background()
	switch args[0] {
	case "mcp":
		runMCP(ctx)
	case "ui":
		runUI(ctx, args[1:])
	case "sandbox":
		runSandbox(ctx, args[1:])
	case "snapshot":
		runSnapshot(ctx, args[1:])
	case "preview":
		runPreview(ctx, args[1:])
	case "reap":
		runReap(ctx, args[1:])
	case "pause-idle":
		runPauseIdle(ctx, args[1:])
	case "up":
		runUp(ctx, args[1:])
	case "down":
		runDown(ctx, args[1:])
	case "ps":
		runPs(ctx, args[1:])
	case "templates":
		runTemplates(args[1:])
	case "proxy":
		runProxy(args[1:])
	case "version":
		fmt.Println("peapod 0.1.0")
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", args[0], usage)
		os.Exit(2)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// stripBackend pulls a global "--backend X" / "--backend=X" out of args.
func stripBackend(args []string) []string {
	var rest []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--backend":
			if i+1 < len(args) {
				backendName = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--backend="):
			backendName = strings.TrimPrefix(a, "--backend=")
		default:
			rest = append(rest, a)
		}
	}
	return rest
}

func newManager() *sandbox.Manager {
	drv, err := backend.New(backendName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "peapod:", err)
		os.Exit(1)
	}
	return sandbox.NewManager(drv)
}

func runMCP(ctx context.Context) {
	mgr := newManager()
	fmt.Fprintf(os.Stderr, "peapod mcp: serving on stdio (backend %s)\n", mgr.Backend())
	if err := mcpserver.Serve(ctx, mgr); err != nil {
		fmt.Fprintln(os.Stderr, "peapod mcp:", err)
		os.Exit(1)
	}
}

func runUI(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:7070", "address to listen on")
	_ = fs.Parse(args)
	mgr := newManager()
	fmt.Printf("peapod ui: http://%s  (backend %s)\n", *addr, mgr.Backend())
	if err := web.Serve(ctx, mgr, *addr); err != nil {
		fmt.Fprintln(os.Stderr, "peapod ui:", err)
		os.Exit(1)
	}
}

func runReap(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("reap", flag.ExitOnError)
	maxAge := fs.Duration("max-age", 30*time.Minute, "destroy sandboxes older than this")
	_ = fs.Parse(args)
	mgr := newManager()
	ids, err := mgr.Reap(ctx, *maxAge)
	check(err)
	fmt.Printf("reaped %d sandbox(es)\n", len(ids))
	for _, id := range ids {
		fmt.Println("  -", id)
	}
}

func runPauseIdle(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("pause-idle", flag.ExitOnError)
	maxIdle := fs.Duration("max-idle", 15*time.Minute, "pause sandboxes idle longer than this")
	_ = fs.Parse(args)
	mgr := newManager()
	ids, err := mgr.PauseIdle(ctx, *maxIdle)
	check(err)
	fmt.Printf("paused %d idle sandbox(es)\n", len(ids))
	for _, id := range ids {
		fmt.Println("  -", id)
	}
}

var templates = []struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	Desc  string `json:"desc"`
}{
	{"python", "python:3.12-slim", "Python 3.12"},
	{"node", "node:22-slim", "Node.js 22"},
	{"go", "golang:1.23", "Go toolchain"},
	{"postgres", "postgres:16", "PostgreSQL 16"},
	{"ubuntu", "ubuntu:24.04", "Ubuntu 24.04"},
	{"alpine", "alpine", "Alpine (tiny)"},
}

func runProxy(args []string) {
	fs := flag.NewFlagSet("proxy", flag.ExitOnError)
	allow := fs.String("allow", "", "comma-separated allowed domains")
	addr := fs.String("addr", ":8899", "listen address")
	_ = fs.Parse(args)
	doms := splitComma(*allow)
	fmt.Fprintf(os.Stderr, "peapod proxy on %s — allow: %v\n", *addr, doms)
	if err := proxy.New(doms).ListenAndServe(*addr); err != nil {
		fmt.Fprintln(os.Stderr, "peapod proxy:", err)
		os.Exit(1)
	}
}

func runTemplates(args []string) {
	fs := flag.NewFlagSet("templates", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "output JSON")
	_ = fs.Parse(args)
	if *asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(templates)
		return
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tIMAGE\tDESCRIPTION")
	for _, t := range templates {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", t.Name, t.Image, t.Desc)
	}
	_ = tw.Flush()
}

func runSnapshot(ctx context.Context, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: peapod snapshot ls | rm <ref>")
		os.Exit(2)
	}
	mgr := newManager()
	switch args[0] {
	case "ls":
		snaps, err := mgr.ListSnapshots(ctx)
		check(err)
		tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "REF\tNAME\tCREATED\tSIZE")
		for _, s := range snaps {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Ref, s.Name, s.Created, s.Size)
		}
		_ = tw.Flush()
	case "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: peapod snapshot rm <ref>")
			os.Exit(2)
		}
		check(mgr.RemoveSnapshot(ctx, args[1]))
		fmt.Println("removed", args[1])
	case "prune":
		fs := flag.NewFlagSet("prune", flag.ExitOnError)
		maxAge := fs.Duration("max-age", 24*time.Hour, "remove snapshots older than this")
		_ = fs.Parse(args[1:])
		removed, err := mgr.PruneSnapshots(ctx, *maxAge)
		check(err)
		fmt.Printf("pruned %d snapshot(s)\n", len(removed))
		for _, r := range removed {
			fmt.Println("  -", r)
		}
	case "diff":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: peapod snapshot diff <refA> <refB>")
			os.Exit(2)
		}
		diff, err := mgr.DiffSnapshots(ctx, args[1], args[2])
		check(err)
		for _, f := range diff.Added {
			fmt.Println("+ " + f)
		}
		for _, f := range diff.Removed {
			fmt.Println("- " + f)
		}
		fmt.Printf("(%d added, %d removed)\n", len(diff.Added), len(diff.Removed))
	default:
		fmt.Fprintf(os.Stderr, "unknown snapshot subcommand %q\n", args[0])
		os.Exit(2)
	}
}

func runPreview(ctx context.Context, args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: peapod preview up|status|down")
		os.Exit(2)
	}
	mgr := newManager()
	repoRoot, branch, err := gitInfo()
	if err != nil {
		fmt.Fprintln(os.Stderr, "peapod preview:", err)
		os.Exit(1)
	}
	name := previewName(repoRoot, branch)

	find := func() (sandbox.Sandbox, bool) {
		boxes, _ := mgr.List(ctx)
		for _, b := range boxes {
			if b.Name == name {
				return b, true
			}
		}
		return sandbox.Sandbox{}, false
	}

	switch args[0] {
	case "up":
		fs := flag.NewFlagSet("up", flag.ExitOnError)
		image := fs.String("image", "alpine", "image for the preview env")
		net := fs.String("net", "egress", "network policy: none|egress")
		_ = fs.Parse(args[1:])
		if sb, ok := find(); ok {
			fmt.Printf("preview already up for %q: %s\n", branch, sb.ID)
			return
		}
		sb, err := mgr.Create(ctx, sandbox.Spec{
			Image:   *image,
			Name:    name,
			Network: sandbox.NetworkPolicy(*net),
			Workdir: "/repo",
			Mounts:  []sandbox.Mount{{Host: repoRoot, Target: "/repo"}},
		})
		check(err)
		fmt.Printf("preview up for branch %q: %s (repo mounted at /repo)\n", branch, sb.ID)
	case "status":
		if sb, ok := find(); ok {
			fmt.Printf("branch %q -> %s (%s, %s)\n", branch, sb.ID, sb.Image, sb.Network)
		} else {
			fmt.Printf("no preview for branch %q\n", branch)
		}
	case "down":
		sb, ok := find()
		if !ok {
			fmt.Printf("no preview for branch %q\n", branch)
			return
		}
		check(mgr.Destroy(ctx, sb.ID))
		fmt.Printf("preview down for branch %q (%s)\n", branch, sb.ID)
	default:
		fmt.Fprintf(os.Stderr, "unknown preview subcommand %q\n", args[0])
		os.Exit(2)
	}
}

func gitInfo() (root, branch string, err error) {
	r, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", "", fmt.Errorf("not in a git repo")
	}
	b, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", "", fmt.Errorf("cannot read git branch")
	}
	return strings.TrimSpace(string(r)), strings.TrimSpace(string(b)), nil
}

func previewName(root, branch string) string {
	san := func(s string) string {
		return strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
				return r
			}
			return '-'
		}, s)
	}
	return "preview-" + san(filepath.Base(root)) + "-" + san(branch)
}

func runSandbox(ctx context.Context, args []string) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	mgr := newManager()
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("create", flag.ExitOnError)
		name := fs.String("name", "", "human label")
		net := fs.String("net", "none", "network policy: none|egress")
		portsArg := fs.String("ports", "", "comma-separated host:container ports")
		allow := fs.String("allow", "", "egress allowlist domains (via proxy)")
		parseFlagsAnywhere(fs, args[1:])
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox create <image> [--name N] [--net none|egress] [--ports h:c,...] [--allow d1,d2]")
			os.Exit(2)
		}
		ports, err := parsePorts(splitComma(*portsArg))
		check(err)
		spec := sandbox.Spec{Image: fs.Arg(0), Name: *name, Network: sandbox.NetworkPolicy(*net), Ports: ports}
		if doms := splitComma(*allow); len(doms) > 0 {
			spec.Network = sandbox.NetworkEgress
			spec.Env = map[string]string{}
			for _, k := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
				spec.Env[k] = "http://host.docker.internal:8899"
			}
			fmt.Fprintf(os.Stderr, "note: run `peapod proxy --allow %s` so this sandbox can reach those domains.\n", *allow)
		}
		sb, err := mgr.Create(ctx, spec)
		check(err)
		fmt.Println(sb.ID)
	case "ls":
		fs := flag.NewFlagSet("ls", flag.ExitOnError)
		asJSON := fs.Bool("json", false, "output JSON")
		_ = fs.Parse(args[1:])
		boxes, err := mgr.List(ctx)
		check(err)
		if boxes == nil {
			boxes = []sandbox.Sandbox{}
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(boxes)
			return
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tIMAGE\tNETWORK\tSTATUS\tNAME")
		for _, b := range boxes {
			status := "running"
			if b.Paused {
				status = "paused"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", b.ID, b.Image, b.Network, status, b.Name)
		}
		_ = tw.Flush()
	case "exec":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox exec <id> <cmd>...")
			os.Exit(2)
		}
		res, err := mgr.Exec(ctx, args[1], args[2:], sandbox.ExecOpts{})
		check(err)
		fmt.Print(res.Stdout)
		fmt.Fprint(os.Stderr, res.Stderr)
		os.Exit(res.ExitCode)
	case "rm":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox rm <id>")
			os.Exit(2)
		}
		check(mgr.Destroy(ctx, args[1]))
		fmt.Println("destroyed", args[1])
	case "pause":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox pause <id>")
			os.Exit(2)
		}
		check(mgr.Pause(ctx, args[1]))
		fmt.Println("paused", args[1])
	case "resume":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox resume <id>")
			os.Exit(2)
		}
		check(mgr.Resume(ctx, args[1]))
		fmt.Println("resumed", args[1])
	case "checkpoint":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox checkpoint <id> <name>")
			os.Exit(2)
		}
		check(mgr.Checkpoint(ctx, args[1], args[2]))
		fmt.Println("checkpointed", args[1], "->", args[2])
	case "restore":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox restore <id> <name>")
			os.Exit(2)
		}
		check(mgr.Restore(ctx, args[1], args[2]))
		fmt.Println("restored", args[1], "from", args[2])
	case "logs":
		fs := flag.NewFlagSet("logs", flag.ExitOnError)
		tail := fs.Int("tail", 200, "lines from the end")
		parseFlagsAnywhere(fs, args[1:])
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox logs <id> [--tail N]")
			os.Exit(2)
		}
		out, err := mgr.Logs(ctx, fs.Arg(0), *tail)
		check(err)
		fmt.Print(out)
	case "stats":
		fs := flag.NewFlagSet("stats", flag.ExitOnError)
		asJSON := fs.Bool("json", false, "output JSON")
		parseFlagsAnywhere(fs, args[1:])
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox stats <id>")
			os.Exit(2)
		}
		st, err := mgr.Stats(ctx, fs.Arg(0))
		check(err)
		if *asJSON {
			_ = json.NewEncoder(os.Stdout).Encode(st)
		} else {
			fmt.Printf("CPU %s   MEM %s (%s)\n", st.CPUPerc, st.MemUsage, st.MemPerc)
		}
	case "history":
		fs := flag.NewFlagSet("history", flag.ExitOnError)
		asJSON := fs.Bool("json", false, "output JSON")
		parseFlagsAnywhere(fs, args[1:])
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox history <id>")
			os.Exit(2)
		}
		entries, err := mgr.History(fs.Arg(0))
		check(err)
		if *asJSON {
			_ = json.NewEncoder(os.Stdout).Encode(entries)
		} else if len(entries) == 0 {
			fmt.Println("(no history)")
		} else {
			for _, e := range entries {
				fmt.Printf("%s  exit=%d  %s\n", e.Time, e.ExitCode, e.Command)
			}
		}
	case "snapshot":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox snapshot <id> <name>")
			os.Exit(2)
		}
		ref, err := mgr.Snapshot(ctx, args[1], args[2])
		check(err)
		fmt.Println(ref)
	case "fork":
		fs := flag.NewFlagSet("fork", flag.ExitOnError)
		name := fs.String("name", "", "human label")
		net := fs.String("net", "none", "network policy: none|egress")
		parseFlagsAnywhere(fs, args[1:])
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox fork <snapshot> [--name N] [--net none|egress]")
			os.Exit(2)
		}
		sb, err := mgr.Fork(ctx, fs.Arg(0), sandbox.Spec{Name: *name, Network: sandbox.NetworkPolicy(*net)})
		check(err)
		fmt.Println(sb.ID)
	default:
		fmt.Fprintf(os.Stderr, "unknown sandbox subcommand %q\n", args[0])
		os.Exit(2)
	}
}

type manifest struct {
	Name     string                     `json:"name"`
	Services map[string]manifestService `json:"services"`
}

type manifestService struct {
	Image   string            `json:"image"`
	Network string            `json:"network"`
	Ports   []string          `json:"ports"`
	Mounts  []string          `json:"mounts"`
	Env     map[string]string `json:"env"`
	Workdir string            `json:"workdir"`
}

func splitComma(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// parseFlagsAnywhere lets flags appear after positional args (Go's flag package
// stops at the first non-flag token). It reorders value-taking flags to the front.
func parseFlagsAnywhere(fs *flag.FlagSet, args []string) {
	valueFlag := map[string]bool{
		"-name": true, "--name": true, "-net": true, "--net": true,
		"-ports": true, "--ports": true, "-image": true, "--image": true,
		"-tail": true, "--tail": true, "-allow": true, "--allow": true,
	}
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && valueFlag[a] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			pos = append(pos, a)
		}
	}
	_ = fs.Parse(append(flags, pos...))
}

func parsePorts(ss []string) ([]sandbox.Port, error) {
	var ports []sandbox.Port
	for _, s := range ss {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		var h, c int
		if _, err := fmt.Sscanf(s, "%d:%d", &h, &c); err != nil {
			return nil, fmt.Errorf("bad port %q (want host:container)", s)
		}
		ports = append(ports, sandbox.Port{Host: h, Container: c})
	}
	return ports, nil
}

func parseMounts(ss []string, base string) []sandbox.Mount {
	var ms []sandbox.Mount
	for _, s := range ss {
		parts := strings.SplitN(s, ":", 2)
		if len(parts) != 2 {
			continue
		}
		host := parts[0]
		if !filepath.IsAbs(host) {
			host = filepath.Join(base, host)
		}
		ms = append(ms, sandbox.Mount{Host: host, Target: parts[1]})
	}
	return ms
}

func loadManifest(path string) (manifest, string, error) {
	base, _ := filepath.Abs(filepath.Dir(path))
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, base, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, base, fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Name == "" {
		m.Name = filepath.Base(base)
	}
	return m, base, nil
}

func groupName(path string) string {
	if m, _, err := loadManifest(path); err == nil {
		return m.Name
	}
	wd, _ := os.Getwd()
	return filepath.Base(wd)
}

func runUp(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("up", flag.ExitOnError)
	file := fs.String("f", "peapod.json", "manifest file")
	_ = fs.Parse(args)
	m, base, err := loadManifest(*file)
	check(err)
	if len(m.Services) == 0 {
		fmt.Fprintln(os.Stderr, "no services in", *file)
		os.Exit(1)
	}
	mgr := newManager()
	existing := map[string]bool{}
	if boxes, e := mgr.List(ctx); e == nil {
		for _, b := range boxes {
			existing[b.Name] = true
		}
	}
	names := make([]string, 0, len(m.Services))
	for k := range m.Services {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, svc := range names {
		s := m.Services[svc]
		full := m.Name + "-" + svc
		if existing[full] {
			fmt.Printf("%s already up\n", full)
			continue
		}
		ports, err := parsePorts(s.Ports)
		check(err)
		net := s.Network
		if net == "" {
			net = "egress"
		}
		sb, err := mgr.Create(ctx, sandbox.Spec{
			Image: s.Image, Name: full, Network: sandbox.NetworkPolicy(net),
			Workdir: s.Workdir, Env: s.Env, Ports: ports, Mounts: parseMounts(s.Mounts, base),
		})
		check(err)
		fmt.Printf("up %s: %s\n", full, sb.ID)
	}
}

func runDown(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("down", flag.ExitOnError)
	file := fs.String("f", "peapod.json", "manifest file")
	_ = fs.Parse(args)
	mgr := newManager()
	prefix := groupName(*file) + "-"
	boxes, err := mgr.List(ctx)
	check(err)
	n := 0
	for _, b := range boxes {
		if strings.HasPrefix(b.Name, prefix) {
			if mgr.Destroy(ctx, b.ID) == nil {
				fmt.Printf("down %s (%s)\n", b.Name, b.ID)
				n++
			}
		}
	}
	if n == 0 {
		fmt.Printf("nothing to bring down for group %q\n", strings.TrimSuffix(prefix, "-"))
	}
}

func runPs(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("ps", flag.ExitOnError)
	file := fs.String("f", "peapod.json", "manifest file")
	_ = fs.Parse(args)
	mgr := newManager()
	prefix := groupName(*file) + "-"
	boxes, err := mgr.List(ctx)
	check(err)
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "SERVICE\tID\tIMAGE\tNETWORK\tSTATUS")
	for _, b := range boxes {
		if !strings.HasPrefix(b.Name, prefix) {
			continue
		}
		status := "running"
		if b.Paused {
			status = "paused"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", strings.TrimPrefix(b.Name, prefix), b.ID, b.Image, b.Network, status)
	}
	_ = tw.Flush()
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "peapod:", err)
		os.Exit(1)
	}
}
