# Coding Conventions

**Analysis Date:** 2026-06-07

Go project (`go 1.26.1`, module `github.com/marcfargas/tramp`). An MCP server for transparent remote command execution. Conventions are idiomatic Go enforced by `gofmt` and `go vet` in CI — there is no `.golangci.yml`, `.editorconfig`, or `Makefile`. Formatting is non-negotiable: CI fails if `gofmt -l .` reports any file.

## Naming Patterns

**Files:**
- `lowercase.go` single word where possible (`manager.go`, `config.go`, `pool.go`, `driver.go`)
- Feature-prefixed for grouped concerns within a package: `tools_remote.go`, `tools_target.go`, `tools_forward.go`
- Platform-split files use Go build-constraint suffixes: `agent_unix.go`, `agent_windows.go`
- Tests sit beside source as `<name>_test.go` (`manager_test.go`, `bash_test.go`)
- Integration tests live under `test/integration/` as `<transport>_<shell>_test.go` (`ssh_bash_test.go`, `docker_bash_test.go`)

**Packages:**
- Short, lowercase, single-word, no underscores: `target`, `transport`, `shell`, `pool`, `mcp`
- One package per `internal/` subdirectory; package name matches directory name
- Import alias only to disambiguate the two MCP packages: `gomcp "github.com/modelcontextprotocol/go-sdk/mcp"` and `trampmcp "github.com/marcfargas/tramp/internal/mcp"`

**Types:**
- Exported types are PascalCase nouns: `Target`, `Manager`, `TargetConfig`, `Transport`, `ExecResult`, `ShellDriver`
- String enums via a named string type plus typed constants: `TransportType`/`TransportSSH`, `TransportState`/`StateConnected`, `ShellType`/`ShellBash` (see `internal/transport/transport.go:11-25`, `internal/shell/driver.go:3-9`)
- Per-tool input structs are declared locally inside the register function, not at package scope (`internal/mcp/tools_remote.go:215`, `internal/mcp/server.go:72`)

**Functions / Methods:**
- Exported: PascalCase verbs (`Add`, `Remove`, `Switch`, `Connect`, `Exec`, `ReadFile`)
- Unexported helpers: camelCase (`getConnection`, `parseSSHHost`, `escapeForPath`, `loadConfigs`, `toolText`, `toolError`)
- Constructors named `New<Type>` returning a pointer: `NewManager()`, `NewBashDriver()`, `NewSSHTransport()`, `NewServer()`, `NewService()`. The pool's constructor is just `New(...)` (package-qualified as `pool.New`)
- Receiver names are short single letters tied to the type: `m *Manager`, `p *Pool`, `d *BashDriver`, `s *Service`, `tr` for transports in tests

**Variables:**
- camelCase locals (`targetName`, `searchPath`, `contextLines`, `replaceAll`)
- Package-level vars are rare and explicit: `var bashSafeChars = regexp.MustCompile(...)` (`internal/shell/bash.go:9`)

## Code Style

**Formatting:**
- `gofmt` is the sole formatter. CI step "Check formatting" runs `gofmt -l .` and fails on any output (`.github/workflows/ci.yml:206-213`)
- Tabs for indentation (gofmt default), grouped `const`/`var` blocks aligned by gofmt

**Vetting:**
- `go vet ./...` runs in the `lint` CI job (`.github/workflows/ci.yml:203-204`). No third-party linter is configured — keep code vet-clean.

## Import Organization

Two groups separated by a blank line, ordered as gofmt/goimports would:

1. Standard library (`context`, `fmt`, `strings`, `sync`, `errors`, `os`)
2. Third-party + internal together in one block, with third-party first then `github.com/marcfargas/tramp/internal/...`

Example (`internal/mcp/tools_remote.go:3-12`):
```go
import (
	"context"
	"fmt"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/marcfargas/tramp/internal/target"
	"github.com/marcfargas/tramp/internal/transport"
)
```
Some files place the external SDK in the same block as internal imports (`internal/mcp/server.go:3-16`). No path aliases beyond the `gomcp`/`trampmcp` disambiguation. No barrel/re-export files.

## Error Handling

The dominant Go pattern: functions return `error` as the last value; callers check `if err != nil` immediately and return early.

- **Wrap with context using `%w`** when propagating, including the operative value:
  ```go
  return nil, fmt.Errorf("connecting to %q: %w", name, err)   // internal/pool/pool.go:69
  return fmt.Errorf("reading %s: %w", path, err)              // internal/mcp/tools_remote.go:100
  ```
- **`%q` for user-supplied identifiers** (target names, types, paths) so quoting is unambiguous: `fmt.Errorf("target %q not found", name)` (`internal/target/manager.go:59`)
- **Sentinel-style messages with `errors.New`** for static, no-interpolation errors: `errors.New(`target name "local" is reserved`)` (`internal/target/manager.go:35`)
- **`errors.Is` for stdlib sentinels**, not string matching: `if errors.Is(err, os.ErrNotExist)` returns an empty config rather than failing (`internal/target/config.go:93`)
- **Actionable error messages** include the remedy in MCP-facing errors: `"no remote target active — use target_switch or specify a target parameter"` (`internal/mcp/tools_remote.go:21`)
- **`panic` only for programmer errors / impossible-to-escape input**: `panic("shell argument contains null byte — cannot be safely escaped")` (`internal/shell/bash.go:23`). Never panic for runtime/remote failures.
- **`log.Fatalf` only in `main`** for unrecoverable startup failures (`cmd/tramp/main.go:24,47`). Library packages never call `log.Fatal`/`os.Exit`.
- **Best-effort, deliberately ignored errors** are marked with `_ =` and a comment explaining why: `_ = svc.TargetSwitch(...)` with `// Best-effort auto-connect — don't fail startup` (`internal/mcp/server.go:51-53`); config loads use `cfg, _ := LoadConfigFromFile(...)` where a missing file is acceptable.

### MCP tool error convention
Tool handlers do **not** return Go errors to the SDK for business failures. They convert errors into an MCP error result and return `nil` as the error:
```go
result, name, err := svc.RemoteBash(ctx, args.Target, args.Command, timeout)
if err != nil {
    return toolError(err.Error()), nil, nil
}
```
`toolError` / `toolText` are the only two ways to build a `*gomcp.CallToolResult` (`internal/mcp/server.go:177-190`):
- `toolText(s)` — success, text content
- `toolError(msg)` — sets `IsError: true` and prefixes the text with `"Error: "`

## Concurrency

- Shared mutable state is guarded by a `sync.Mutex` / `sync.RWMutex` embedded as the first field, named `mu` (`internal/target/manager.go:19-23`, `internal/pool/pool.go:17-22`)
- `defer mu.Unlock()` immediately after `Lock()` for read paths; explicit lock/unlock spans where the critical section must be narrow (the pool releases the lock around the network `HealthCheck`/`connect`, see `internal/pool/pool.go:34-59`)
- Use `RLock`/`RUnlock` for read-only accessors (`CurrentName`, `Current`, `Get`, `List`)
- A `connecting map[string]chan struct{}` is reserved to dedupe concurrent connect attempts (`internal/pool/pool.go:21`)
- The race detector runs on Linux in CI (`go test ./... -race`) — keep all shared access mutex-protected.

## Logging

- No logging framework. The library packages do not log at all.
- `cmd/tramp/main.go` uses stdlib `log` for fatal startup errors and `fmt.Fprintln(os.Stderr, ...)` for usage. The MCP server speaks JSON-RPC over stdio, so stdout is reserved for protocol traffic — diagnostics go to stderr only.

## Comments & Documentation

- **Every exported type, function, and method has a doc comment** starting with its name: `// Manager manages targets ...`, `// Add adds a dynamic target. Returns error if name is reserved or duplicate.`
- Struct fields are documented inline with trailing `//` comments, especially config fields that map to JSON (`internal/target/config.go:14-24`)
- A few files carry a leading path comment as the first line (`// internal/target/manager.go`), present in `manager.go`, `config.go`, `manager_test.go`, `pool.go` — not universal, but follow it when editing those files.
- Non-obvious logic gets a short rationale comment (e.g. the null-byte panic, the best-effort connect, the pool's evict-and-reconnect block).
- No JSDoc-equivalent tooling; standard godoc conventions apply.

## Struct Tags

- JSON tags use lowerCamelCase keys to match the `tramp.json` schema and `omitempty` on every optional field: `json:"identityFile,omitempty"`, `json:"insecureIgnoreHostKey,omitempty"` (`internal/target/config.go:14-24`). Required fields (`type`, and the targets map) omit `omitempty`.
- MCP input structs combine a `json` tag with a `jsonschema` tag carrying the human-readable parameter description: `Command string `json:"command" jsonschema:"the command to execute on the remote target"`` (`internal/mcp/tools_remote.go:217`).
- **Optional tool parameters use pointer types** (`*int`, `*bool`) so "absent" is distinguishable from "zero", then are dereferenced into a plain local with a default (`internal/mcp/tools_remote.go:225-228`, `285-291`). This is a deliberate fix recorded in commit `0d76c7c` — apply it for any new optional numeric/bool tool param.

## Function & Module Design

- Functions are small and single-purpose. Shared logic is extracted to unexported helpers within the package (`getConnection` centralises target resolution for all remote tools; `escapeForPath` centralises path quoting for all shell-command builders).
- Interfaces are small and behaviour-focused: `Transport` (10 methods, `internal/transport/transport.go:45-55`) and `ShellDriver` (5 methods, `internal/shell/driver.go:11-17`). New transports/shells implement these interfaces; a factory switch maps a config `Type` string to a concrete constructor (`NewTransport`, `internal/transport/transport.go:57-67`).
- Dependency injection via function-typed fields enables testing: the pool takes a `TransportFactory func(cfg) (Transport, error)` rather than constructing transports directly (`internal/pool/pool.go:13-31`).
- Constructors return pointers; collection accessors return value copies to avoid leaking internal pointers (`Manager.List()` dereferences `*Target` into `[]Target`, `internal/target/manager.go:117-126`).
- No global singletons. The `Service` struct (`internal/mcp`) is the composition root, wired in `NewService` (`internal/mcp/server.go:34-57`).

## Platform / Cross-Compilation

- OS-specific code is isolated behind `//go:build` constraints (`agent_unix.go` / `agent_windows.go`), never `runtime.GOOS` branching in hot paths.
- CI builds and tests on both `ubuntu-latest` and `windows-latest` (`.github/workflows/ci.yml:16`). Keep paths and shell assumptions platform-aware; the codebase supports both bash and pwsh remote shells via `ShellDriver`.

---

*Convention analysis: 2026-06-07*
