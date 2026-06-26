package sandbox

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a sandbox id can't be resolved by the driver.
var ErrNotFound = errors.New("sandbox not found")

// NetworkPolicy controls a sandbox's network access.
type NetworkPolicy string

const (
	// NetworkNone disables all networking — the safe default for untrusted code.
	NetworkNone NetworkPolicy = "none"
	// NetworkEgress allows outbound network access (default bridge).
	NetworkEgress NetworkPolicy = "egress"
)

// Resources caps a sandbox's resource usage. Zero means "apply the safe default".
type Resources struct {
	CPUs      float64 `json:"cpus,omitempty"`
	MemoryMB  int     `json:"memory_mb,omitempty"`
	PidsLimit int     `json:"pids_limit,omitempty"`
}

// Mount is a host directory bind-mounted into the sandbox.
type Mount struct {
	Host     string `json:"host"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// Port publishes a container port on the host.
type Port struct {
	Host      int `json:"host"`
	Container int `json:"container"`
}

// Spec describes a sandbox to create.
type Spec struct {
	Image     string            `json:"image"`
	Name      string            `json:"name,omitempty"`
	Network   NetworkPolicy     `json:"network,omitempty"`
	Workdir   string            `json:"workdir,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Resources Resources         `json:"resources,omitempty"`
	Mounts    []Mount           `json:"mounts,omitempty"`
	Ports     []Port            `json:"ports,omitempty"`
	// Allow is an egress domain allowlist. When set, the driver builds a
	// bypass-proof firewall (internal network + proxy sidecar) instead of just
	// trusting HTTP(S)_PROXY env vars.
	Allow []string `json:"allow,omitempty"`
	// Labels carries internal metadata (e.g. peapod.id). The Manager fills it in.
	Labels map[string]string `json:"-"`
}

// Sandbox is a live (or stored) sandbox.
type Sandbox struct {
	ID      string        `json:"id"`
	Backend string        `json:"backend"`
	Ref     string        `json:"ref"`
	Image   string        `json:"image"`
	Name    string        `json:"name,omitempty"`
	Network NetworkPolicy `json:"network"`
	Workdir string        `json:"workdir"`
	Created time.Time     `json:"created,omitempty"`
	Paused  bool          `json:"paused,omitempty"`
}

// ExecResult is the outcome of running a command in a sandbox.
type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

// Snapshot is a saved sandbox image that can be forked.
type Snapshot struct {
	Ref         string `json:"ref"`
	Name        string `json:"name"`
	Created     string `json:"created,omitempty"`
	CreatedUnix int64  `json:"created_unix,omitempty"`
	Size        string `json:"size,omitempty"`
}

// Stat is a point-in-time resource sample for a sandbox.
type Stat struct {
	CPUPerc  string `json:"cpu_perc"`
	MemUsage string `json:"mem_usage"`
	MemPerc  string `json:"mem_perc"`
}

// SnapshotDiff lists files added/removed between two snapshots.
type SnapshotDiff struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

// HistoryEntry is one recorded command run inside a sandbox (the audit trail).
type HistoryEntry struct {
	Time     string `json:"time"`
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Preview  string `json:"preview,omitempty"`
}
