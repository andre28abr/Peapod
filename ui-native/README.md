# Peapod native UI (menu bar)

A minimal native macOS menu-bar app (SwiftUI) that lists, creates, and destroys
Peapod sandboxes by shelling out to the `peapod` CLI (`sandbox ls --json`).

This is the Phase 3 native-UI starting point — intentionally small. The richer
app (live stats, logs, pause/resume, snapshots) builds on the same approach.

## Build

```sh
./build.sh        # produces Peapod.app (needs the Swift toolchain / Xcode CLT)
```

## Run

The app calls the `peapod` binary. Point it at your build (or copy `peapod` to
`/usr/local/bin`):

```sh
PEAPOD_BIN="$(cd ../bin && pwd)/peapod" open Peapod.app
```

A 🫛-ish box icon appears in the menu bar; click it to manage sandboxes.
(Requires OrbStack/Docker running for the `oci` backend.)
