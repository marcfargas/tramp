<!-- refreshed: 2026-06-07 -->
# Architecture

**Analysis Date:** 2026-06-07

## System Overview

```text
┌─────────────────────────────────────────────────────────────┐
│            MCP CLIENT (Claude Code / agent, local)           │
│                  speaks MCP over stdio                        │
└───────────────────────────┬─────────────────────────────────┘
                            │ JSON-RPC (stdin/stdout)
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                      MCP SERVER LAYER                         │
│  `internal/mcp/server.go` — tool & resource registration     │
├──────────────────┬──────────────────┬───────────────────────┤
│  target tools    │   remote tools   │    forward tools      │
│ `tools_target.go`│ `tools_remote.go`│  `tools_forward.go`   │
│ add/switch/list  │ bash/read/write/ │  forward_port/list/   │
│ remove/status    │ edit/glob/grep/ls│  stop  +  resource.go │
└────────┬─────────┴────────┬─────────┴──────────┬────────────┘
         │ all handlers call into Service (shared state)        │
         ▼                  ▼                     ▼
┌──────────────────┐ ┌──────────────────┐ ┌────────────────────┐
│  Target Manager  │ │  Connection Pool │ │  Forward registry  │
│`target/manager.go│ │  `pool/pool.go`  │ │ (in Service struct)│
│ named targets +  │ │ cached conns +   │ │  active tunnels    │
│ active selection │ │ health/reconnect │ │                    │
└────────┬─────────┘ └────────┬─────────┘ └────────────────────┘
         │                    │ creates via factory
         │                    ▼
         │          ┌──────────────────────────────────────────┐
         │          │            TRANSPORT LAYER                │
         │          │  `transport/transport.go` (interface)     │
         │          ├──────────────────┬───────────────────────┤
         │          │  SSHTransport    │   DockerTransport      │
         │          │  `ssh.go`        │   `docker.go`          │
         │          │  x/crypto/ssh +  │   docker/docker        │
         │          │  sftp            │   client (exec API)    │
         │          └────────┬─────────┴──────────┬────────────┘
         │                   │ uses ShellDriver    │
         ▼                   ▼                     ▼
┌─────────────────────────────────────────────────────────────┐
│                       SHELL LAYER                            │
│  `shell/driver.go` (interface) — escaping, stat, mkdir, cwd  │
│  BashDriver `bash.go`   │   PwshDriver `pwsh.go`             │
└───────────────────────────┬─────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│        REMOTE TARGET (SSH host or Docker container)          │
└─────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| `main` | CLI entry, builds Service + Server, runs stdio loop, signal handling | `cmd/tramp/main.go` |
| MCP server wiring | Register all tools & the `tramp://context` resource | `internal/mcp/server.go` |
| `Service` | Shared state (Manager, Pool, ProjectDir, forwards) + business logic for all tool handlers | `internal/mcp/tools_target.go` (struct), `internal/mcp/tools_remote.go`, `internal/mcp/tools_forward.go` |
| Target tools | `target_add/switch/remove/list/status` handlers | `internal/mcp/tools_target.go` |
| Remote tools | `bash/read/write/edit/glob/grep/ls` handlers | `internal/mcp/tools_remote.go` |
| Forward tools | `forward_port/list/stop` handlers | `internal/mcp/tools_forward.go` |
| Context resource | Dynamic `tramp://context` describing active target + network boundary | `internal/mcp/resource.go` |
| `Manager` | Registry of named targets + current/active selection (thread-safe) | `internal/target/manager.go` |
| Config | Parse/merge/persist `tramp.json`, `TargetConfig` schema | `internal/target/config.go` |
| `Pool` | Cache transports per target, health-check, evict & reconnect | `internal/pool/pool.go` |
| `Transport` interface | Abstract Exec/ReadFile/WriteFile/Connect/Close/Info/HealthCheck | `internal/transport/transport.go` |
| `SSHTransport` | SSH + SFTP impl, port forwarding, remote env detection | `internal/transport/ssh.go` |
| `DockerTransport` | Docker exec impl, remote env detection | `internal/transport/docker.go` |
| Env detection | Parse shell/platform/arch from probe output | `internal/transport/detect.go` |
| `ShellDriver` interface | Per-shell escaping, stat, mkdir, cwd-wrapping | `internal/shell/driver.go` |
| Bash/Pwsh drivers | Concrete shell quoting & command builders | `internal/shell/bash.go`, `internal/shell/pwsh.go` |

## Pattern Overview

**Overall:** Layered MCP server with interface-based polymorphism (Hexagonal/ports-and-adapters flavor). The MCP protocol is the inbound port; `Transport` and `ShellDriver` are outbound ports with SSH/Docker and Bash/Pwsh adapters.

**Key Characteristics:**
- Single binary, single MCP server over stdio (`StdioTransport`).
- Strict layering: `mcp → pool → transport → shell`. Lower layers never import higher ones.
- `internal/target` is a shared leaf imported by `mcp`, `pool`, and `transport` (holds `TargetConfig` and `Manager` — no upward dependencies).
- Dependency injection via factory: `Pool` receives a `TransportFactory`, decoupling pool from concrete transports and enabling test fakes.
- Per-call target override: every remote tool accepts an optional `target` param; falls back to the active target.
- "Local mode" = no active target; remote tools error, instructing the user to switch.

## Layers

**MCP layer (`internal/mcp`):**
- Purpose: Translate MCP tool/resource calls to/from `Service` methods; format text results.
- Location: `internal/mcp/`
- Contains: tool registration closures, input structs with `jsonschema` tags, the `Service` struct, result helpers (`toolText`, `toolError`).
- Depends on: `pool`, `target`, `transport`, the MCP go-sdk.
- Used by: `cmd/tramp/main.go`.

**Pool layer (`internal/pool`):**
- Purpose: Lifecycle and caching of transport connections.
- Location: `internal/pool/`
- Contains: `Pool`, `TransportFactory`, health-check/evict/reconnect logic, `Status()`.
- Depends on: `target`, `transport`.
- Used by: `mcp`.

**Transport layer (`internal/transport`):**
- Purpose: Execute commands and move files on a remote target.
- Location: `internal/transport/`
- Contains: `Transport` interface, `SSHTransport`, `DockerTransport`, `NewTransport` factory, env-detection parsers, build-tagged SSH agent helpers (`agent_unix.go`, `agent_windows.go`).
- Depends on: `target`, `shell`, external SSH/Docker SDKs.
- Used by: `pool`, `mcp` (only for the SSH type assertion in port forwarding).

**Shell layer (`internal/shell`):**
- Purpose: Shell-specific command construction and argument escaping.
- Location: `internal/shell/`
- Contains: `ShellDriver` interface, `BashDriver`, `PwshDriver`.
- Depends on: nothing internal (leaf).
- Used by: `transport`.

**Target leaf (`internal/target`):**
- Purpose: Config schema, parsing/merging/persistence, target registry.
- Location: `internal/target/`
- Depends on: stdlib only.
- Used by: `mcp`, `pool`, `transport`.

## Data Flow

### Primary Request Path (a remote tool, e.g. `bash`)

1. MCP client invokes `bash` tool over stdio (`cmd/tramp/main.go:46`, `internal/mcp/server.go:26`).
2. Tool closure unpacks `BashInput`, calls `svc.RemoteBash` (`internal/mcp/tools_remote.go:221`).
3. `RemoteBash` → `getConnection` resolves target via `Manager` then `Pool.Get` (`internal/mcp/tools_remote.go:16`).
4. `Pool.Get` returns cached healthy conn or reconnects via factory (`internal/pool/pool.go:34`).
5. `conn.Exec` runs the command on the remote, using the `ShellDriver` for cwd-wrapping/escaping (`internal/transport/ssh.go`, `internal/transport/docker.go`).
6. Result (`ExecResult`) bubbles up; handler formats `[target: x] exit code: n` text (`internal/mcp/tools_remote.go:237`).

### Target Switch Path

1. `target_switch` → `svc.TargetSwitch` (`internal/mcp/tools_target.go:57`).
2. Eager `Pool.Get` validates the connection before committing (`internal/mcp/tools_target.go:68`).
3. On success, `Manager.Switch` sets `current` (`internal/target/manager.go:70`).

### Startup / Auto-connect Path

1. `NewService` builds Manager + Pool, then `loadConfigs` merges global `~/.claude/tramp.json` and project `./.claude/tramp.json` (`internal/mcp/server.go:34`, `:60`).
2. If a `default` target exists, best-effort `TargetSwitch` auto-connects (`internal/mcp/server.go:52`).

**State Management:**
- Active target name: `Manager.current` (RWMutex-guarded).
- Live connections: `Pool.connections` map (Mutex-guarded).
- Active forwards: `Service.forwards` map (separate `fwdMu` Mutex).
- All persistent config lives in `tramp.json` files, never in process memory beyond load.

## Key Abstractions

**`Transport` (port):**
- Purpose: Uniform remote execution & file IO regardless of SSH vs Docker.
- Examples: `internal/transport/ssh.go`, `internal/transport/docker.go`.
- Pattern: Interface + concrete adapters, selected by `NewTransport(cfg)` switch on `cfg.Type`.

**`ShellDriver` (port):**
- Purpose: Encapsulate shell-dialect differences (quoting, stat, mkdir, cwd).
- Examples: `internal/shell/bash.go`, `internal/shell/pwsh.go`.
- Pattern: Interface chosen at connect time based on detected/declared shell; held on the transport.

**`TransportFactory` (DI seam):**
- Purpose: Decouple `Pool` from concrete transports; the real factory is `transport.NewTransport`, tests inject fakes.
- Examples: `internal/mcp/server.go:37`, `internal/pool/pool.go:14`.

**`Service`:**
- Purpose: Single shared-state object backing every tool handler.
- Examples: `internal/mcp/tools_target.go:22`.

## Entry Points

**`tramp serve`:**
- Location: `cmd/tramp/main.go`
- Triggers: Launched by the MCP client (`claude mcp add tramp -- tramp serve`).
- Responsibilities: Resolve cwd as project dir, build Service + Server, install SIGINT/SIGTERM handler (closes all pool connections), run the stdio MCP loop.

**`tramp://context` resource:**
- Location: `internal/mcp/resource.go`
- Triggers: Client reads the resource.
- Responsibilities: Report active target, platform/arch/shell, cwd, and the local-vs-remote network-boundary guidance.

## Architectural Constraints

- **Threading:** MCP go-sdk drives handlers; shared state is protected by mutexes — `Manager` (`sync.RWMutex`), `Pool` (`sync.Mutex`), `Service.forwards` (`sync.Mutex` `fwdMu`), each transport (`sync.Mutex`). Assume tool handlers may run concurrently; never touch shared maps without the relevant lock.
- **Layering rule:** Imports flow downward only (`mcp → pool → transport → shell`); `target` is a shared leaf. Do not import `mcp` from lower layers.
- **Single concrete leak:** `mcp` reaches into a concrete type exactly once — the `conn.(*transport.SSHTransport)` assertion in `ForwardPort` (`internal/mcp/tools_forward.go:20`), because forwarding is SSH-only.
- **No global mutable state:** Everything hangs off the `Service` instance created in `main`; there are no package-level singletons.
- **Reserved name:** `"local"` is reserved and means "no active target"; rejected by both `Manager.Add` and `ParseConfig`.

## Anti-Patterns

### Adding remote-command logic directly in tool closures

**What happens:** Putting `conn.Exec(...)` / command-string building inside the `gomcp.AddTool` closure.
**Why it's wrong:** Mixes MCP formatting with business logic, bypasses the testable `Service` methods, and duplicates `getConnection` resolution.
**Do this instead:** Add a `Remote*` method on `Service` (see `RemoteGrep`, `internal/mcp/tools_remote.go:146`) and call it from a thin closure.

### Hand-quoting shell arguments in transports

**What happens:** Concatenating user paths/args into a command string without escaping.
**Why it's wrong:** Shell injection and breakage on spaces/quotes; bash vs pwsh differ.
**Do this instead:** Use the connection's `ShellDriver` (`Escape`, `StatCommand`, `MkdirCommand`, `CwdWrap` — `internal/shell/bash.go`).

### Bypassing the Pool to create transports

**What happens:** Calling `transport.NewTransport` / `Connect` directly inside a handler.
**Why it's wrong:** Loses caching, health-check, eviction, and concurrent-connect protection.
**Do this instead:** Resolve through `getConnection` → `Pool.Get` (`internal/mcp/tools_remote.go:16`).

## Error Handling

**Strategy:** Errors are wrapped with `fmt.Errorf("...: %w", err)` and returned up the stack; at the MCP boundary they are converted to a non-throwing tool result via `toolError` (`IsError: true`) so the agent sees the message instead of a protocol fault.

**Patterns:**
- Validation errors (bad config, missing target) returned as `toolError`, not Go panics.
- Connect failures during `target_switch` are surfaced eagerly so a switch never silently "succeeds" against a dead target.
- One deliberate `panic` exists: `BashDriver.Escape` on a null byte in an argument (`internal/shell/bash.go:23`) — treated as a programming error, not user input.

## Cross-Cutting Concerns

**Logging:** Minimal; `log.Fatalf` only in `main` for fatal startup/run failures. Tool errors travel back as results, not logs (stdout is reserved for the MCP protocol).
**Validation:** `TargetConfig` validated in two places — `Service.TargetAdd` (`internal/mcp/tools_target.go:31`) and `target.ParseConfig` (`internal/target/config.go:48`).
**Authentication:** SSH key/agent auth in `internal/transport/ssh.go` (identity file + `agent_unix.go`/`agent_windows.go`); Docker via local daemon socket (`client.FromEnv`).
**Network boundary:** Communicated to the agent through the `tramp://context` resource so local built-in tools and remote tramp tools are not confused.

---

*Architecture analysis: 2026-06-07*
