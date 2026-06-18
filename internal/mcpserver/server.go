// Package mcpserver exposes the Peapod núcleo to AI agents over MCP — the Phase 1
// way agents create and drive disposable sandboxes.
package mcpserver

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"peapod/internal/sandbox"
)

// defaultExecTimeout caps agent-driven exec calls that don't specify one.
const defaultExecTimeout = 120 * time.Second

// New builds the Peapod MCP server backed by mgr.
func New(mgr *sandbox.Manager) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "peapod", Version: "0.1.0"}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "peapod_sandbox_create",
		Description: "Create a fresh, isolated, disposable sandbox (a container) to run untrusted or agent-generated code. Networking is OFF by default. Returns a sandbox id used by the other tools.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, createOut, error) {
		sb, err := mgr.Create(ctx, sandbox.Spec{Image: in.Image, Name: in.Name, Network: sandbox.NetworkPolicy(in.Network)})
		if err != nil {
			return nil, createOut{}, err
		}
		return nil, asCreateOut(sb), nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "peapod_exec",
		Description: "Run a shell command inside a sandbox and capture stdout, stderr, and exit code.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in execIn) (*mcp.CallToolResult, execOut, error) {
		timeout := time.Duration(in.TimeoutSeconds) * time.Second
		switch {
		case in.TimeoutSeconds == 0:
			timeout = defaultExecTimeout
		case in.TimeoutSeconds < 0:
			timeout = 0
		}
		res, err := mgr.Exec(ctx, in.ID, []string{"sh", "-lc", in.Command}, sandbox.ExecOpts{
			Workdir: in.Workdir,
			Timeout: timeout,
		})
		if err != nil {
			return nil, execOut{}, err
		}
		return nil, execOut{Stdout: res.Stdout, Stderr: res.Stderr, ExitCode: res.ExitCode}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "peapod_write_file",
		Description: "Write a text file inside a sandbox, creating parent directories as needed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in writeIn) (*mcp.CallToolResult, okOut, error) {
		if err := mgr.WriteFile(ctx, in.ID, in.Path, []byte(in.Content), 0o644); err != nil {
			return nil, okOut{}, err
		}
		return nil, okOut{OK: true}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "peapod_read_file",
		Description: "Read a text file from inside a sandbox.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in readIn) (*mcp.CallToolResult, readOut, error) {
		data, err := mgr.ReadFile(ctx, in.ID, in.Path)
		if err != nil {
			return nil, readOut{}, err
		}
		return nil, readOut{Content: string(data)}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "peapod_list",
		Description: "List all Peapod-managed sandboxes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, listOut, error) {
		boxes, err := mgr.List(ctx)
		if err != nil {
			return nil, listOut{}, err
		}
		if boxes == nil {
			boxes = []sandbox.Sandbox{} // serialize as [] not null
		}
		return nil, listOut{Sandboxes: boxes}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "peapod_destroy",
		Description: "Destroy a sandbox and free its resources.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in idIn) (*mcp.CallToolResult, okOut, error) {
		if err := mgr.Destroy(ctx, in.ID); err != nil {
			return nil, okOut{}, err
		}
		return nil, okOut{OK: true}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "peapod_snapshot",
		Description: "Snapshot a sandbox's filesystem so it can be forked later. (Phase 2 preview.)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in snapIn) (*mcp.CallToolResult, snapOut, error) {
		ref, err := mgr.Snapshot(ctx, in.ID, in.Name)
		if err != nil {
			return nil, snapOut{}, err
		}
		return nil, snapOut{Snapshot: ref}, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "peapod_fork",
		Description: "Create a new sandbox from a snapshot. (Phase 2 preview.)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in forkIn) (*mcp.CallToolResult, createOut, error) {
		sb, err := mgr.Fork(ctx, in.Snapshot, sandbox.Spec{Name: in.Name, Network: sandbox.NetworkPolicy(in.Network)})
		if err != nil {
			return nil, createOut{}, err
		}
		return nil, asCreateOut(sb), nil
	})

	return s
}

// Serve builds the server and runs it over stdio until the client disconnects.
// If PEAPOD_REAP_TTL is set (e.g. "30m"), sandboxes older than the TTL are
// reaped in the background.
func Serve(ctx context.Context, mgr *sandbox.Manager) error {
	if ttl := os.Getenv("PEAPOD_REAP_TTL"); ttl != "" {
		if d, err := time.ParseDuration(ttl); err == nil && d > 0 {
			go reapLoop(ctx, mgr, d)
		}
	}
	return New(mgr).Run(ctx, &mcp.StdioTransport{})
}

func reapLoop(ctx context.Context, mgr *sandbox.Manager, ttl time.Duration) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if ids, err := mgr.Reap(ctx, ttl); err == nil && len(ids) > 0 {
				fmt.Fprintf(os.Stderr, "peapod: reaped %d idle sandbox(es): %v\n", len(ids), ids)
			}
		}
	}
}

func asCreateOut(sb sandbox.Sandbox) createOut {
	return createOut{ID: sb.ID, Backend: sb.Backend, Image: sb.Image, Network: string(sb.Network), Workdir: sb.Workdir}
}

// --- tool input/output types (schemas are derived from these) ---

type createIn struct {
	Image   string `json:"image" jsonschema:"OCI image for the sandbox, e.g. 'python:3.12-slim' or 'alpine'"`
	Name    string `json:"name,omitempty" jsonschema:"optional human-friendly label"`
	Network string `json:"network,omitempty" jsonschema:"network policy: 'none' (default, no network) or 'egress' (outbound allowed)"`
}

type createOut struct {
	ID      string `json:"id"`
	Backend string `json:"backend"`
	Image   string `json:"image"`
	Network string `json:"network"`
	Workdir string `json:"workdir"`
}

type execIn struct {
	ID             string `json:"id" jsonschema:"sandbox id from peapod_sandbox_create"`
	Command        string `json:"command" jsonschema:"shell command to run inside the sandbox"`
	Workdir        string `json:"workdir,omitempty" jsonschema:"working directory (defaults to the sandbox workdir)"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" jsonschema:"max seconds to wait; 0 means no limit"`
}

type execOut struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

type writeIn struct {
	ID      string `json:"id" jsonschema:"sandbox id"`
	Path    string `json:"path" jsonschema:"absolute path inside the sandbox, e.g. /work/main.py"`
	Content string `json:"content" jsonschema:"file contents"`
}

type readIn struct {
	ID   string `json:"id" jsonschema:"sandbox id"`
	Path string `json:"path" jsonschema:"absolute path inside the sandbox"`
}

type readOut struct {
	Content string `json:"content"`
}

type idIn struct {
	ID string `json:"id" jsonschema:"sandbox id"`
}

type okOut struct {
	OK bool `json:"ok"`
}

type emptyIn struct{}

type listOut struct {
	Sandboxes []sandbox.Sandbox `json:"sandboxes"`
}

type snapIn struct {
	ID   string `json:"id" jsonschema:"sandbox id"`
	Name string `json:"name" jsonschema:"name/tag for the snapshot"`
}

type snapOut struct {
	Snapshot string `json:"snapshot"`
}

type forkIn struct {
	Snapshot string `json:"snapshot" jsonschema:"snapshot ref from peapod_snapshot"`
	Name     string `json:"name,omitempty" jsonschema:"optional label for the new sandbox"`
	Network  string `json:"network,omitempty" jsonschema:"network policy: 'none' or 'egress'"`
}
