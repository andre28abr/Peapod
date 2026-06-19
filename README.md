# Peapod 🫛

**Disposable, isolated sandboxes for AI agents — local-first, on your Mac.**

Peapod gives every AI agent (or you) a throwaway, isolated Linux environment to
run untrusted or AI‑generated code. It's driven over the **Model Context
Protocol (MCP)**, a **CLI**, a **web dashboard**, and a **native macOS app** —
with **no network by default** and a full **audit trail** of everything that ran.

![Peapod demo](docs/demo.svg)

> Status: working end‑to‑end. Backends: `oci` (Docker/Podman), `apple-container`
> (one microVM per sandbox, macOS 26+), `mock` (tests). Go + SwiftUI. MIT.

## Why

Letting an AI agent run code directly on your machine is risky — a bug or a
malicious dependency touches your files, your network, your keys. Peapod wraps
each run in a disposable sandbox:

- **Isolated & disposable** — make a mess, throw it away; your Mac is untouched.
- **No network by default** — or an **egress allowlist** (only the domains you choose).
- **Audited** — every command an agent runs is recorded and reviewable.
- **Agent‑native** — agents drive it over MCP; no babysitting.

## Quick start

```sh
# build the CLI
go build -o bin/peapod ./cmd/peapod

# create an isolated sandbox and run code in it (needs OrbStack or Docker)
ID=$(bin/peapod sandbox create python:3.12-slim --net none)
bin/peapod sandbox exec "$ID" python3 -c 'print(6*7)'
bin/peapod sandbox history "$ID"     # audit trail
bin/peapod sandbox rm "$ID"
```

### Native macOS app

```sh
cd ui-native && ./build.sh      # produces Peapod.app + Peapod.dmg
```

Double‑click `Peapod.dmg`, drag **Peapod** to Applications, and open it. The
`peapod` binary is bundled inside — nothing to configure.

### Use from an AI agent (MCP)

Peapod ships a `.mcp.json`, so opening this folder in Claude Code auto‑loads the
server. An agent then has tools like `peapod_sandbox_create`, `peapod_exec`,
`peapod_history`, `peapod_snapshot`, … (12 in total).

## Architecture — one seam, many backends

Everything sits on a thin núcleo (`Manager`) over a swappable `Driver` interface,
so new isolation backends and features plug in without touching the top:

```
        MCP server  ·  CLI  ·  web dashboard  ·  native macOS app
                              │
                       Manager (núcleo)
                              │   Driver interface  ← the seam
        ┌──────────────┬──────┴────────┬──────────────┐
       oci          apple-container   libkrun         mock
   (docker/podman)   (microVM, m26)   (later)        (tests)
```

Optional capabilities (checkpoint, logs, stats, snapshot‑diff) are matched by
interface assertion, so a backend implements only what it supports.

## Features

- **Sandboxes**: create / exec / files / ls / destroy, with CPU/mem/PID limits and exec timeouts.
- **MCP server** (12 tools) so agents drive everything.
- **Audit trail**: every command recorded (`sandbox history`).
- **Domain firewall**: egress allowlist proxy (`peapod proxy --allow …`).
- **Snapshots**: snapshot / fork / list / prune / **diff**.
- **Lifecycle**: pause / resume, **auto‑pause** idle sandboxes, age‑based reap.
- **Templates**: one‑click images (Python, Node, Go, Postgres, …).
- **Multi‑service**: `up` / `down` / `ps` from a `peapod.json` manifest, with port publishing.
- **Preview envs**: a per‑git‑branch sandbox with the repo mounted.
- **UIs**: web dashboard (`peapod ui`) and a native macOS app.
- **Experimental**: CRIU checkpoint/restore (works where the engine supports it).

## Command reference

```
peapod sandbox create <image> [--net none|egress] [--ports h:c,…] [--allow d1,d2]
peapod sandbox exec|logs|stats|history|snapshot|pause|resume|rm <id>
peapod snapshot ls | rm <ref> | prune [--max-age 24h] | diff <a> <b>
peapod up | down | ps [-f peapod.json]      # multi-service groups
peapod preview up | status | down           # per-branch env
peapod proxy --allow d1,d2                  # egress allowlist
peapod reap [--max-age 30m] | pause-idle [--max-idle 15m]
peapod templates | ui | mcp | version
peapod --backend oci|apple|mock <command>
```

## Develop

```sh
go test ./...     # uses the in-memory mock driver — no daemon needed
go vet ./...
```

CI (GitHub Actions) runs vet/test/build on every push. A Homebrew formula is in
`Formula/peapod.rb`.

## License

MIT © Andre Souza
