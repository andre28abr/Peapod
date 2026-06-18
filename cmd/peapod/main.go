// Command peapod runs the MCP server and a small CLI for driving sandboxes.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"peapod/internal/backend"
	"peapod/internal/mcpserver"
	"peapod/internal/sandbox"
)

const usage = `peapod — disposable, isolated sandboxes for AI agents

usage:
  peapod [--backend oci|apple|mock] <command>

commands:
  mcp                                 start the MCP server (stdio)
  sandbox create <image> [--name N] [--net none|egress]
  sandbox ls
  sandbox exec <id> <cmd>...
  sandbox rm <id>
  sandbox snapshot <id> <name>
  sandbox fork <snapshot> [--name N] [--net none|egress]
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
	case "sandbox":
		runSandbox(ctx, args[1:])
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
		fmt.Fprintln(tw, "ID\tIMAGE\tNETWORK\tNAME")
		for _, b := range boxes {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", b.ID, b.Image, b.Network, b.Name)
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
