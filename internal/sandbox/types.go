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

// Spec describes a sandbox to create.
type Spec struct {
	Image     string            `json:"image"`
	Name      string            `json:"name,omitempty"`
	Network   NetworkPolicy     `json:"network,omitempty"`
	Workdir   string            `json:"workdir,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Resources Resources         `json:"resources,omitempty"`
	Mounts    []Mount           `json:"mounts,omitempty"`
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
	Ref     string `json:"ref"`
	Name    string `json:"name"`
	Created string `json:"created,omitempty"`
	Size    string `json:"size,omitempty"`
}
