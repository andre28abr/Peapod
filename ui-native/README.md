# Peapod native app (macOS)

A native SwiftUI app (a normal window) to list, create, pause/resume, and destroy
Peapod sandboxes. The `peapod` CLI is bundled **inside** the app, so there's
nothing to configure — just open it. Requires OrbStack (or Docker) running.

## Build it

```sh
./build.sh
```

This produces two things in this folder:

- `Peapod.app` — **double-click to run.**
- `Peapod.dmg` — double-click, then **drag Peapod into Applications**, and launch
  it from Launchpad/Applications.

> First launch: because the app isn't signed with an Apple Developer ID, macOS may
> ask you to confirm. If it says "unidentified developer", right-click the app →
> **Open** → **Open**, or allow it in System Settings → Privacy & Security.

## What you'll see

A window listing your sandboxes (id, image, network, status) with buttons to
create a new one, pause/resume, and delete. It refreshes automatically.
