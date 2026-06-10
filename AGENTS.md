# Unicast MPV

Go daemon that exposes MPV over a JSON-RPC WebSocket API. Originally a Node.js/TypeScript project (see README), now rewritten in Go.

## Build & Run

- `make build` — builds binary to `bin/unicast-mpv`
- `go build -o bin/unicast-mpv ./cmd/unicast-mpv` — equivalent
- `go test ./...` — run all tests
- `go test ./pkg/...` — run unit tests only (no MPV binary required)
- `go test ./integration/` — integration tests; some require `mpv` in PATH, others skip automatically

## Architecture

```
cmd/unicast-mpv/main.go   — entrypoint; wires config→server→mpv→player→commands→events
pkg/
  config/   — YAML config loading, merging (default → platform → local)
  server/   — JSON-RPC WebSocket server (gorilla/websocket)
  mpv/      — MPV IPC client + process manager
    ipc/    — low-level Unix socket IPC to MPV
    process/— MPV binary process lifecycle
  player/   — high-level player state (status tracking, arg building)
  commands/ — RPC method registry (native, status, quit, play commands)
  events/   — bridges MPV events to server notifications
  schema/   — JSON-RPC param validation (tuple/array/number/string/any)
  logger/   — leveled logger
  util/     — case conversion helpers
config/     — embedded default YAML configs (default.yaml, default-win32.yaml)
```

Key wiring in `main.go`: `NewServer → NewMPV → NewPlayer → NewCommandRegistry → NewNativeCommands → Bridge(mpv, srv)`

## Testing

- Unit tests use Unix domain sockets and mock MPV connections — no external dependencies
- Integration tests in `integration/` use `skipIfNoMPV()` to skip when `mpv` binary is unavailable; set `MPV_BINARY` env var to override the path
- Tests in `pkg/mpv/mpv_test.go` use a `testHook` that creates a mock IPC socket pair
- No lint/typecheck config files exist yet; use `go vet ./...` for static analysis

## Config

Config loads in layers: embedded defaults → platform overrides (`default-{GOOS}.yaml`) → user file (optional CLI arg). Default port is 2019.

## Error Codes

`pkg/mpv/errors.go` defines MPV error codes matching the original NodeJS implementation (0=load failed, 1=invalid arg, 2=binary not found, etc.)

## Issue Tracker

When implementing issues, consult `issue-tracker.md` at the project root to know which are done (checked) vs pending (unchecked). Update `[x]` when verified. Each issue spec lives in `./issues/`.