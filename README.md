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

- `oci` (Phase 1) — docker/podman, container-per-sandbox.
- `apple-container` (next) — one microVM per sandbox (Apple Virtualization.framework, macOS 26+).
- `libkrun` (later) — embedded microVM.

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

## Roadmap

- **Phase 1 (now):** núcleo + MCP, `oci` driver.
- **Phase 2:** snapshot & fork of running stacks (interface already present).
- **Phase 3:** per-branch local preview envs + UI.
