# Peapod 🫛

Disposable, isolated sandboxes for AI agents — local-first, on your Mac.

Peapod gives each agent run a throwaway, isolated environment to execute
untrusted or AI-generated code, driven over the Model Context Protocol (MCP).

> Status: **Phase 1 (MVP)** — núcleo + MCP server, `oci` driver (docker/podman).

## Why

Letting an agent run code directly on your machine is risky. Peapod wraps it in
a disposable sandbox with **no network by default**, exposed to agents as simple
MCP tools (`peapod_sandbox_create`, `peapod_exec`, `peapod_write_file`, …).

## Architecture

A thin núcleo (`Manager`) over a swappable `Driver` seam:

- `oci` (default) — docker/podman, container-per-sandbox.
- `apple-container` — one microVM per sandbox (Apple `container` / Virtualization.framework,
  macOS 26+). Validated on container 1.0.0. Select with `--backend apple` or `PEAPOD_BACKEND=apple`.
- `libkrun` (later) — embedded microVM.
- `mock` — in-memory, for tests / daemon-less dev.

The MCP server, the CLI, and the future UI all sit on the same `Manager`, so new
isolation backends and Phase 2/3 features plug in without touching the top.

## Build

```sh
go build -o bin/peapod ./cmd/peapod
```

## Test (no daemon needed — uses the in-memory mock driver)

```sh
go test ./...
```

## Try it (needs Docker or Podman running)

```sh
bin/peapod sandbox create python:3.12-slim --net none
bin/peapod sandbox exec <id> python3 -c 'print("hi from the sandbox")'
bin/peapod sandbox ls
bin/peapod sandbox snapshot <id> v1
bin/peapod sandbox rm <id>
```

## Use from Claude Code

```sh
claude mcp add peapod -- /absolute/path/to/bin/peapod mcp
```

Then an agent can call `peapod_sandbox_create` → `peapod_write_file` →
`peapod_exec` → `peapod_destroy`.

## More commands

```sh
peapod ui                     # local web dashboard at http://127.0.0.1:7070
peapod sandbox pause <id>     # freeze a sandbox (memory preserved, zero CPU)
peapod sandbox resume <id>    # unfreeze it
peapod snapshot ls            # list snapshots
peapod snapshot rm <ref>      # delete a snapshot
peapod preview up             # sandbox for the current git branch (repo at /repo)
peapod preview status
peapod preview down
peapod reap --max-age 30m     # destroy idle sandboxes
peapod --backend apple ...    # microVM isolation (macOS 26 + apple/container)
```

## Roadmap

- **Phase 1 (done):** núcleo + MCP, `oci` + `apple-container` drivers, resource limits,
  exec timeouts, age-based auto-reap.
- **Phase 2 (done):** filesystem snapshot & fork + snapshot lifecycle (`snapshot ls/rm`),
  and `pause` / `resume` to freeze a running sandbox (memory preserved, zero CPU).
  Disk-persisted CRIU checkpoint is not used — `docker checkpoint` *restore* is broken
  on OrbStack.
- **Phase 3 (started):** per-branch preview envs (`preview up/status/down`) and a
  minimal web dashboard (`peapod ui`) both work; a native UI comes later.
