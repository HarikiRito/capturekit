# capturekit

One binary, `capture`, bundling independent packages for knowledge-base screenshot work.

## Build

    go build -o bin/capture ./cmd/capture

## Development

Requires golangci-lint v2.

- `make check` - gofmt check + lint + tests (run before pushing)
- `make lint` / `make test` / `make fmt`

## Packages

- `capture sim ...` - drive a simulator/emulator and take screenshots
- `capture browser ...` - drive a browser and take screenshots
- `capture annotate ...` - draw a red box + label on screenshots (ffmpeg)

Each package lives in `pkg/<name>` and exposes only `Run(args, stdout, stderr) int` and `Instructions() string`.
Packages must not import each other; only `cmd/capture/main.go` knows all of them.
New platform support (e.g. iOS) should only touch `pkg/sim`.

## For coding agents

Run `capture <pkg> instructions` first. It prints the full command reference, flags, schemas and exit codes for that package.
