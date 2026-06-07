# Codebase Structure

**Analysis Date:** 2026-06-07

## Directory Layout

```
tramp/
├── cmd/
│   └── tramp/
│       └── main.go              # CLI entry point: `tramp serve`
├── internal/                    # Private packages (not importable externally)
│   ├── mcp/                     # MCP server: tools, resource, Service state
│   │   ├── server.go            # Server/Service construction, config load, registration
│   │   ├── tools_target.go      # Service struct + target_* handlers
│   │   ├── tools_remote.go      # bash/read/write/edit/glob/grep/ls handlers
│   │   ├── tools_forward.go     # forward_port/list/stop handlers
│   │   └── resource.go          # tramp://context dynamic resource
│   ├── pool/                    # Connection caching & lifecycle
│   │   └── pool.go
│   ├── transport/               # Remote execution adapters (port: Transport)
│   │   ├── transport.go         # Transport interface + NewTransport factory
│   │   ├── ssh.go               # SSH + SFTP adapter, port forwarding
│   │   ├── docker.go            # Docker exec adapter
│   │   ├── detect.go            # Shell/platform/arch output parsers
│   │   ├── agent_unix.go        # SSH agent dialing (build tag: !windows)
│   │   └── agent_windows.go     # SSH agent dialing (build tag: windows)
│   ├── shell/                   # Shell dialect adapters (port: ShellDriver)
│   │   ├── driver.go            # ShellDriver interface
│   │   ├── bash.go              # Bash/sh quoting & command builders
│   │   └── pwsh.go              # PowerShell quoting & command builders
│   └── target/                  # Config schema + target registry (shared leaf)
│       ├── config.go            # TargetConfig/Config, parse/merge/persist
│       └── manager.go           # Manager: named targets + active selection
├── test/
│   └── integration/             # End-to-end tests against real SSH/Docker
│       ├── ssh_bash_test.go
│       └── docker_bash_test.go
├── docs/
│   ├── specs/                   # Design specs (dated)
│   └── plans/                   # Implementation plans (dated)
├── .claude-plugin/              # Claude Code plugin packaging
│   ├── plugin.json
│   ├── .mcp.json
│   └── bin/bootstrap.sh
├── .github/workflows/           # CI + release pipelines
├── .planning/                   # GSD planning artifacts (this map lives here)
├── go.mod / go.sum              # Go module: github.com/marcfargas/tramp
├── .goreleaser.yml              # Multi-platform release build config
└── README.md
```

## Directory Purposes

**`cmd/tramp/`:**
- Purpose: The sole executable's `main`.
- Contains: Argument check, Service/Server bootstrap, signal handling, stdio run loop.
- Key files: `main.go`.

**`internal/mcp/`:**
- Purpose: Inbound MCP adapter — turns tool/resource calls into `Service` method calls and formats results.
- Contains: tool registration closures, input structs with `jsonschema` tags, the `Service` struct, result helpers.
- Key files: `server.go` (wiring), `tools_remote.go` (most tools), `tools_target.go` (`Service` definition).

**`internal/pool/`:**
- Purpose: Cache, health-check, and reconnect transports keyed by target name.
- Key files: `pool.go`.

**`internal/transport/`:**
- Purpose: Outbound remote-execution adapters behind the `Transport` interface.
- Contains: interface, SSH and Docker concretes, env-detection parsers, OS-specific agent helpers.
- Key files: `transport.go` (contract), `ssh.go`, `docker.go`.

**`internal/shell/`:**
- Purpose: Outbound shell-dialect adapters behind `ShellDriver`.
- Key files: `driver.go` (contract), `bash.go`, `pwsh.go`.

**`internal/target/`:**
- Purpose: Shared leaf — config types and the target registry, imported by every other internal package.
- Key files: `config.go`, `manager.go`.

**`test/integration/`:**
- Purpose: Tests exercising real SSH/Docker backends (separate from package-local `_test.go` unit tests).

## Key File Locations

**Entry Points:**
- `cmd/tramp/main.go`: Process entry; only `serve` subcommand.
- `internal/mcp/resource.go`: `tramp://context` resource.

**Configuration:**
- `internal/target/config.go`: `TargetConfig`/`Config` schema, parse/merge/persist of `tramp.json`.
- Runtime config files (not in repo): `~/.claude/tramp.json` (global), `<project>/.claude/tramp.json` (project, overrides global).
- `go.mod`: module path and dependency pins.
- `.goreleaser.yml`: release/build matrix.

**Core Logic:**
- `internal/mcp/tools_remote.go`: remote tool business logic (`Service.Remote*`).
- `internal/pool/pool.go`: connection lifecycle.
- `internal/transport/ssh.go`, `docker.go`: the actual remote execution.

**Testing:**
- Co-located unit tests: `internal/**/<name>_test.go`.
- Integration tests: `test/integration/`.

## Naming Conventions

**Files:**
- Go source: lowercase, no separators where possible (`server.go`, `manager.go`).
- Multi-word grouped tool files use snake-ish prefixing by concern: `tools_target.go`, `tools_remote.go`, `tools_forward.go`.
- Tests mirror their subject: `pool_test.go`, `ssh_test.go`.
- OS-specific files use Go build-constraint suffixes: `agent_unix.go`, `agent_windows.go`.

**Packages:**
- Short, single-word, lowercase, matching the directory (`mcp`, `pool`, `transport`, `shell`, `target`).
- External MCP SDK imported with aliases to avoid clash: `trampmcp` (internal) vs `gomcp` (sdk) — see `cmd/tramp/main.go`, `internal/mcp/*`.

**Types & functions:**
- Exported types are PascalCase (`Transport`, `Service`, `TargetConfig`).
- Interface ports named after the role (`Transport`, `ShellDriver`); concretes name the technology (`SSHTransport`, `DockerTransport`, `BashDriver`, `PwshDriver`).
- Constructors: `New<Type>` (`NewServer`, `NewService`, `NewManager`, `NewSSHTransport`, `NewBashDriver`).
- Service business methods grouped by concern prefix: `Target*`, `Remote*`, `Forward*`.
- MCP tool names are lowercase, mirroring Claude built-ins where applicable (`bash`, `read`, `write`, `edit`, `glob`, `grep`, `ls`).

**Docs:**
- Dated, kebab-case: `docs/specs/YYYY-MM-DD-<topic>.md`, `docs/plans/YYYY-MM-DD-<topic>.md`.

## Where to Add New Code

**New MCP tool:**
- Handler/business logic: add a `Remote*`/`Target*`/`Forward*` method on `Service`, in the matching `internal/mcp/tools_*.go` file (create a new `tools_<concern>.go` for a new concern).
- Registration: add the input struct + `gomcp.AddTool` closure in the same file's `register*Tools` function, and call that register function from `NewServer` (`internal/mcp/server.go:24`).
- Keep closures thin: unpack input, call the `Service` method, format with `toolText`/`toolError`.

**New transport backend (e.g. WSL, kubectl exec):**
- Implement the `Transport` interface in a new `internal/transport/<name>.go`.
- Add a case to `NewTransport` (`internal/transport/transport.go:58`) and a `Type` constant.
- Add the type string to config validation in `internal/target/config.go` and `Service.TargetAdd`.

**New shell dialect:**
- Implement `ShellDriver` in `internal/shell/<name>.go` and add a `ShellType` constant in `driver.go`.
- Wire selection where transports choose a driver (`internal/transport/ssh.go:101`, `docker.go:77`).

**New config field:**
- Add to `TargetConfig` in `internal/target/config.go` with a `json` tag; update validation and the schema strings in tool descriptions (`tools_target.go`) and `resource.go`.

**Tests:**
- Unit: co-locate `<file>_test.go` in the same package.
- Integration (real backend): `test/integration/`.

**Utilities / shared helpers:**
- Cross-layer shared data → `internal/target` (the leaf). Avoid creating a generic `utils` package; place helpers next to their primary consumer.

## Special Directories

**`internal/`:**
- Purpose: All implementation packages.
- Generated: No. Committed: Yes.
- Note: Go enforces these are not importable outside this module.

**`.claude-plugin/`:**
- Purpose: Packaging tramp as a Claude Code plugin (manifest, `.mcp.json`, bootstrap script).
- Generated: No. Committed: Yes.

**`.planning/`:**
- Purpose: GSD planning artifacts including this codebase map (`.planning/codebase/`).
- Generated: Partly (codebase maps are agent-generated). Committed: Yes.

**`tramp.exe`:**
- Purpose: A locally-built Windows binary present at repo root.
- Generated: Yes (build artifact). Committed: Check `.gitignore` before relying on its absence.

---

*Structure analysis: 2026-06-07*
