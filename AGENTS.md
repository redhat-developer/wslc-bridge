# AGENTS.md

This file provides guidance to AI agents when working with code in this repository.

## Project Overview

wslc-bridge is a Windows executable that exposes a Docker-compatible HTTP API on a Windows named pipe and bridges it to Microsoft's WSL Containers (WSLC) runtime. API reads go to the guest Docker daemon; operations that need Windows port relay or Windows host binds call `wslc.exe` directly.

## Build and Test Commands

All build and test commands are available through the Makefile. Run `make help` to see all available commands.

```bash
# Build the Windows amd64/arm64 executables, checksums.txt and bridge-manifest.json into dist/
make build

# Run all tests
make test

# Run tests with coverage report
make test-coverage

# Format code / check formatting
make fmt
make check-fmt

# Run go vet
make vet

# Remove build artifacts
make clean
```

For more granular testing, use Go directly:

```bash
go test ./pkg/cmd
go test -run TestName ./pkg/cmd
```

Some tests only build on Windows (`*_windows_test.go`). Cross-check them from other platforms with `GOOS=windows go vet ./...`; CI runs them on Windows.

## Architecture

### Command Structure (Cobra-based)

- Entry point: `cmd/wslc-bridge/main.go` calls `cmd.NewRootCmd().Execute()` and exits with `cmd.ExitCode(err)`
- Root command: `pkg/cmd/root.go` exports `NewRootCmd()`; a bare invocation runs `serve`
- Subcommands: each command is in `pkg/cmd/<command>.go` with a `New<Command>Cmd()` factory function
- Command registration: `NewRootCmd()` calls `rootCmd.AddCommand(New<Command>Cmd())` for each subcommand
- No global variables or `init()` functions in `pkg/cmd` - all configuration is explicit through factory functions

### Exit Codes

Errors that need a specific process exit code are returned as `*bridge.ExitError`; `cmd.ExitCode` maps them, and any other error exits with 1. Usage errors (unknown command, unknown flag, unexpected positional arguments) exit with 2.

### Packages

- `pkg/bridge` - runtime detection, `Serve` (named-pipe server) and `Doctor`
- `pkg/dockerapi` - Docker API router and handlers
- `pkg/native` - `wslc.exe` calls for operations that need Windows port relay or host binds
- `pkg/guest` - transport to the guest Docker daemon
- `pkg/process` - running `wslc.exe` / `wsl.exe`
- `pkg/execstore` - exec instance bookkeeping
- `pkg/overlay` - merges native port data into guest responses
- `pkg/platform` - Windows named-pipe listener
- `pkg/buildinfo` - version metadata injected by the Makefile
- `pkg/wslutil` - decoding Windows command output
- `tools/buildmanifest` - writes `checksums.txt` and `bridge-manifest.json`

### Module Design Pattern

Use concrete types by default. Introduce an interface only where a consumer
needs substitutable behaviour (for example to inject a fake in tests), keep
interfaces small, and define them next to their consumer. Returning
established abstractions such as `http.Handler` is fine.

Examples:

- `native.Client` and `process.Runner` are interfaces because tests substitute
  them. `native.Client` exposes typed operations (`CreateContainer` takes a
  `ContainerSpec`); WSLC argument construction, output interpretation and
  error classification (`native.Error` with an `ErrorClass`) live in
  `pkg/native`, while Docker semantics and validation live in `pkg/dockerapi`.
- `dockerapi` declares `guestAPI` (just `Do`) next to the handler, so its test
  fakes implement a single method.
- `bridge.RuntimeInfo`, `execstore.Store` and `guest.Client` are concrete
  structs: they have one implementation and no consumer needs to swap them.
  `bridge.Serve` owns the guest transport and closes it on shutdown.
- `bridge.DetectRuntime`, `Doctor` and `Serve` call methods on an unexported
  `dependencies` struct that tests fill with fakes.

### Adding a New Command

All commands MUST:

- Define the `Args` field for argument validation (use `noArgs` for commands without positional arguments)
- Include an `Example` field with usage examples
- Have a corresponding `Test<Command>Cmd_Examples` test that validates the examples with `pkg/cmd/testutil`

## Documentation Standards

All fenced code blocks in Markdown files must have a language tag (`bash`, `powershell`, `go`, `json`, `text`, ...).

## Copyright Headers

All source files must include the Apache License 2.0 copyright header with Red Hat copyright. The current year is 2026.

## Dependencies

- Cobra (github.com/spf13/cobra): CLI framework
- Go 1.26+

## Testing

Tests follow Go conventions with `*_test.go` files alongside source files and use the standard `testing` package.

**All tests MUST call `t.Parallel()` as the first line of the test function.**

**Exceptions:** tests using `t.Setenv()` or replacing process-wide state (`os.Args`, `os.Stdout`) cannot use `t.Parallel()`.

## GitHub Actions

Workflows in `.github/workflows/` run on Windows only and must use commit SHA1 hashes instead of version tags, with the version as a comment:

```yaml
- uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
```
