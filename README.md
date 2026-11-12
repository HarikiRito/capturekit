# capturekit

One binary, `capture`, bundling independent packages for knowledge-base screenshot work.

## Build

    go build -o bin/capture ./cmd/capture

## Development

Requires golangci-lint v2.

- `make check` - gofmt check + lint + tests (run before pushing)
- `make lint` / `make test` / `make fmt`

## Packages

- `capture sim [--platform android|ios] ...` - drive a simulator/emulator and take screenshots (iOS: simulator via simctl + AXe, real iPhone via phone-harness; `CAPTURE_SIM_PLATFORM` sets the default platform). Default `--driver auto`: phone-harness if installed, else adb; reads (ui/screenshot/screen-size) retry once on adb, actions never fall back. `--driver adb|phoneharness` (or `CAPTURE_SIM_DRIVER`) forces one. iOS `auto` = AXe for a simulator, phone-harness after `connect <name> --device` (iPhone Mirroring, macOS 15+, iOS 18+ iPhone; no UI tree: coordinates only; no install/launch lifecycle/shell/back); `--driver axe|phoneharness`
- `capture browser ...` - drive a browser and take screenshots
- `capture annotate ...` - draw a red box + label on screenshots (ffmpeg)
- `capture setup` / `capture agent-guide` - `pkg/guide`: dependency setup and overview texts for coding agents

Each package lives in `pkg/<name>` and exposes only `Run(args, stdout, stderr) int` and `Instructions() string`.
Packages must not import each other; only `cmd/capture/main.go` knows all of them.
New platform support should only touch `pkg/sim`.

## For coding agents

Run `capture agent-guide` (overview, standard flow), then `capture setup` (install missing dependencies, verify with each `check`).
Before using a package run `capture <pkg> instructions` for the full command reference, flags, schemas and exit codes.

Snippet for a consuming repo's AGENTS.md / CLAUDE.md:

    Use the capture CLI for screenshots.
    Run `capture agent-guide` first.
    Run `capture setup` to install deps.
    Run `capture <pkg> instructions` before using a package.
