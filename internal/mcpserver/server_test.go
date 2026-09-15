package mcpserver_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"peapod/internal/driver/mock"
	"peapod/internal/mcpserver"
	"peapod/internal/sandbox"
)

// newSession wires the Peapod MCP server to an in-memory client, backed by the
// mock driver — the same seam the CLI and web tests use, no container runtime.
func newSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	cT, sT := mcp.NewInMemoryTransports()
	ss, err := mcpserver.New(sandbox.NewManager(mock.New())).Connect(ctx, sT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, cT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// call invokes a tool and decodes its structured result into out.
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any, out any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s returned tool error: %+v", name, res.Content)
	}
	if out != nil {
		raw, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s: decode %s: %v", name, raw, err)
		}
	}
	return res
}

// The 12 tools the README and MANUAL promise must all be registered, by name —
// a renamed or dropped tool silently breaks every agent config out there.
func TestExposesTheDocumentedTools(t *testing.T) {
	cs := newSession(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"peapod_sandbox_create", "peapod_exec", "peapod_write_file", "peapod_read_file",
		"peapod_list", "peapod_destroy", "peapod_snapshot", "peapod_fork",
		"peapod_snapshot_list", "peapod_snapshot_remove", "peapod_history", "peapod_snapshot_diff",
	}
	got := map[string]bool{}
	for _, tool := range res.Tools {
		got[tool.Name] = true
		if strings.TrimSpace(tool.Description) == "" {
			t.Errorf("tool %s has no description", tool.Name)
		}
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing tool %s", name)
		}
	}
	if len(res.Tools) != len(want) {
		t.Errorf("got %d tools, want %d (docs say 12)", len(res.Tools), len(want))
	}
}

// End-to-end through the protocol: create → write → read → exec → history →
// list → destroy, exactly the sequence an agent runs.
func TestAgentLifecycle(t *testing.T) {
	cs := newSession(t)

	var created struct {
		ID      string `json:"id"`
		Backend string `json:"backend"`
		Network string `json:"network"`
	}
	call(t, cs, "peapod_sandbox_create", map[string]any{"image": "alpine"}, &created)
	if created.ID == "" || created.Backend != "mock" {
		t.Fatalf("unexpected create result: %+v", created)
	}
	if created.Network != "none" {
		t.Errorf("network must default to none (privacy by default), got %q", created.Network)
	}

	call(t, cs, "peapod_write_file", map[string]any{"id": created.ID, "path": "/work/a.txt", "content": "olá"}, nil)
	var read struct {
		Content string `json:"content"`
	}
	call(t, cs, "peapod_read_file", map[string]any{"id": created.ID, "path": "/work/a.txt"}, &read)
	if read.Content != "olá" {
		t.Errorf("read back %q, want olá", read.Content)
	}

	var exec struct {
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exit_code"`
	}
	call(t, cs, "peapod_exec", map[string]any{"id": created.ID, "command": "echo hi"}, &exec)
	if exec.ExitCode != 0 || !strings.Contains(exec.Stdout, "echo hi") {
		t.Errorf("exec via mock should echo argv, got %+v", exec)
	}

	var hist struct {
		Entries []map[string]any `json:"entries"`
	}
	call(t, cs, "peapod_history", map[string]any{"id": created.ID}, &hist)
	if len(hist.Entries) == 0 {
		t.Error("exec must leave an audit entry in the history")
	}

	var list struct {
		Sandboxes []struct {
			ID string `json:"id"`
		} `json:"sandboxes"`
	}
	call(t, cs, "peapod_list", map[string]any{}, &list)
	if len(list.Sandboxes) != 1 || list.Sandboxes[0].ID != created.ID {
		t.Errorf("list should show exactly the created sandbox, got %+v", list.Sandboxes)
	}

	call(t, cs, "peapod_destroy", map[string]any{"id": created.ID}, nil)
	call(t, cs, "peapod_list", map[string]any{}, &list)
	if len(list.Sandboxes) != 0 {
		t.Errorf("sandbox still listed after destroy: %+v", list.Sandboxes)
	}
}

func TestSnapshotForkAndDiff(t *testing.T) {
	cs := newSession(t)

	var created struct {
		ID string `json:"id"`
	}
	call(t, cs, "peapod_sandbox_create", map[string]any{"image": "alpine"}, &created)
	call(t, cs, "peapod_write_file", map[string]any{"id": created.ID, "path": "/work/v1.txt", "content": "1"}, nil)

	var snap1, snap2 struct {
		Snapshot string `json:"snapshot"`
	}
	call(t, cs, "peapod_snapshot", map[string]any{"id": created.ID, "name": "v1"}, &snap1)
	call(t, cs, "peapod_write_file", map[string]any{"id": created.ID, "path": "/work/v2.txt", "content": "2"}, nil)
	call(t, cs, "peapod_snapshot", map[string]any{"id": created.ID, "name": "v2"}, &snap2)
	if snap1.Snapshot == "" || snap1.Snapshot == snap2.Snapshot {
		t.Fatalf("snapshots must have distinct refs: %q %q", snap1.Snapshot, snap2.Snapshot)
	}

	var snaps struct {
		Snapshots []map[string]any `json:"snapshots"`
	}
	call(t, cs, "peapod_snapshot_list", map[string]any{}, &snaps)
	if len(snaps.Snapshots) != 2 {
		t.Errorf("want 2 snapshots listed, got %d", len(snaps.Snapshots))
	}

	// Fork from v1: the file written after v1 must not be there.
	var forked struct {
		ID string `json:"id"`
	}
	call(t, cs, "peapod_fork", map[string]any{"snapshot": snap1.Snapshot}, &forked)
	if forked.ID == "" || forked.ID == created.ID {
		t.Fatalf("fork must yield a new sandbox, got %q", forked.ID)
	}
	var read struct {
		Content string `json:"content"`
	}
	call(t, cs, "peapod_read_file", map[string]any{"id": forked.ID, "path": "/work/v1.txt"}, &read)
	if read.Content != "1" {
		t.Errorf("fork of v1 should contain v1.txt, got %q", read.Content)
	}
	if res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "peapod_read_file", Arguments: map[string]any{"id": forked.ID, "path": "/work/v2.txt"},
	}); err == nil && !res.IsError {
		t.Error("fork of v1 must not contain v2.txt")
	}

	// Diff is an optional driver capability the mock does not implement: the
	// agent must get a clear tool error naming the backend, not a crash.
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "peapod_snapshot_diff", Arguments: map[string]any{"a": snap1.Snapshot, "b": snap2.Snapshot},
	})
	if err != nil {
		t.Fatalf("snapshot_diff: protocol error %v", err)
	}
	if !res.IsError {
		t.Fatal("snapshot_diff on the mock backend should be a tool error (unsupported capability)")
	}
	if raw, _ := json.Marshal(res.Content); !strings.Contains(string(raw), "does not support snapshot diff") {
		t.Errorf("unsupported-capability error should say so, got %s", raw)
	}

	call(t, cs, "peapod_snapshot_remove", map[string]any{"ref": snap2.Snapshot}, nil)
	call(t, cs, "peapod_snapshot_list", map[string]any{}, &snaps)
	if len(snaps.Snapshots) != 1 {
		t.Errorf("want 1 snapshot after remove, got %d", len(snaps.Snapshots))
	}
}

// Errors from the núcleo must surface as tool errors (IsError), never as
// protocol failures that would drop the agent's session.
func TestErrorsAreToolErrorsNotProtocolErrors(t *testing.T) {
	cs := newSession(t)
	cases := []struct {
		name string
		args map[string]any
	}{
		{"peapod_exec", map[string]any{"id": "nope", "command": "true"}},
		{"peapod_destroy", map[string]any{"id": "nope"}},
		{"peapod_read_file", map[string]any{"id": "nope", "path": "/x"}},
		{"peapod_fork", map[string]any{"snapshot": "nope"}},
		{"peapod_sandbox_create", map[string]any{"image": "alpine", "network": "bogus"}},
	}
	for _, c := range cases {
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: c.name, Arguments: c.args})
		if err != nil {
			t.Errorf("%s: protocol-level error %v (want tool error)", c.name, err)
			continue
		}
		if !res.IsError {
			t.Errorf("%s with bad input succeeded: %+v", c.name, res.StructuredContent)
		}
	}
}
