// Command peapod runs the MCP server and a small CLI for driving sandboxes.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"peapod/internal/backend"
	"peapod/internal/mcpserver"
	"peapod/internal/sandbox"
	"peapod/internal/web"
)

const usage = `peapod — disposable, isolated sandboxes for AI agents

usage:
  peapod [--backend oci|apple|mock] <command>

commands:
  mcp                                 start the MCP server (stdio)
  ui [--addr 127.0.0.1:7070]          start the local web dashboard
  sandbox create <image> [--name N] [--net none|egress]
  sandbox ls
  sandbox exec <id> <cmd>...
  sandbox rm <id>
  sandbox pause <id>
  sandbox resume <id>
  sandbox snapshot <id> <name>
  sandbox fork <snapshot> [--name N] [--net none|egress]
  snapshot ls
  snapshot rm <ref>
  preview up [--image IMG] [--net none|egress]    sandbox for the current git branch
  preview status
  preview down
  reap [--max-age 30m]                destroy sandboxes older than max-age
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
		_ = fs.Parse(args[1:])
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: peapod sandbox create <image> [--name N] [--net none|egress]")
			os.Exit(2)
		}
		sb, err := mgr.Create(ctx, sandbox.Spec{Image: fs.Arg(0), Name: *name, Network: sandbox.NetworkPolicy(*net)})
		check(err)
		fmt.Println(sb.ID)
	case "ls":
		boxes, err := mgr.List(ctx)
		check(err)
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
		_ = fs.Parse(args[1:])
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

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "peapod:", err)
		os.Exit(1)
	}
}
