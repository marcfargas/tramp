# tramp — TRAMP-like Transparent Remote Execution for AI Agents

**Date:** 2026-03-27
**Status:** Draft
**Author:** Marc Fargas

## Overview

tramp is a Go MCP server that gives AI coding agents (Claude Code, and any MCP-compatible client) transparent access to remote targets over SSH and Docker. The agent stays local; its file and shell operations execute on the remote machine.

Inspired by Emacs TRAMP and pi-tramp (the TypeScript implementation for the pi coding agent), tramp reimplements the concept as a standalone MCP server with a better foundation: native SSH multiplexing (no sentinel protocol), SFTP for file I/O (no base64 encoding), and the Docker API directly (no CLI wrapping).

## Architecture

```
Claude Code ──stdio/JSON-RPC──▶ tramp (Go binary)
                                  │
                                  ├── MCP Layer (modelcontextprotocol/go-sdk)
                                  │   ├── Tools: target_*, bash, read, write, edit, glob, grep, ls
                                  │   └── Resource: tramp://context (dynamic prompt injection)
                                  │
                                  ├── Target Manager
                                  │   ├── Dynamic targets (in-memory, primary workflow)
                                  │   ├── File config (.claude/tramp.json, ~/.claude/tramp.json)
                                  │   └── Current target state
                                  │
                                  ├── Connection Pool
                                  │   ├── Lazy connect on first use
                                  │   ├── Health checks
                                  │   └── Evict on disconnect, reconnect on next use
                                  │
                                  ├── Transports
                                  │   ├── SSH (golang.org/x/crypto/ssh)
                                  │   │   ├── Native multiplexed channels
                                  │   │   ├── Separate stdout/stderr
                                  │   │   ├── SFTP for file I/O (github.com/pkg/sftp)
                                  │   │   └── Port forwarding capability
                                  │   └── Docker (docker client SDK)
                                  │       ├── Exec API for commands
                                  │       └── Copy API for file I/O
                                  │
                                  └── Shell Drivers
                                      ├── Bash (POSIX single-quote escaping)
                                      └── PowerShell (single-quote doubling, ANSI suppression)
```

### Key Simplifications Over pi-tramp

- **No sentinel protocol.** `golang.org/x/crypto/ssh` provides native multiplexed channels — each command runs in its own session channel with proper stdout/stderr separation.
- **SFTP for file I/O.** Binary-safe, no base64 encode/decode overhead.
- **Docker SDK.** Uses the Docker API directly (Exec API for commands, Copy API for files) instead of wrapping the `docker` CLI.
- **Separate stderr.** SSH channels give real stderr. No merging.

## MCP Tools

### Target Management

| Tool | Parameters | Description |
|------|-----------|-------------|
| `target_add` | `name`, `config` (JSON) | Add a dynamic target |
| `target_switch` | `name` | Connect and switch active target (eager connect) |
| `target_remove` | `name` | Remove a dynamic target |
| `target_list` | — | List all targets with connection status |
| `target_status` | — | Detailed info for active target |

### Remote Operations

These tools mirror Claude Code's built-in tool signatures. They only work when a target is active.

| Tool | Parameters | Description |
|------|-----------|-------------|
| `bash` | `command`, `description?`, `timeout?` | Execute command on remote target |
| `read` | `file_path`, `offset?`, `limit?` | Read file contents |
| `write` | `file_path`, `content` | Write/overwrite file |
| `edit` | `file_path`, `old_string`, `new_string`, `replace_all?` | Find-and-replace edit (SFTP read → in-process patch → SFTP write) |
| `glob` | `pattern`, `path?` | File pattern matching |
| `grep` | `pattern`, `path?`, `type?`, `glob?`, `output_mode?`, `context?` | Content search |
| `ls` | `path` | List directory contents |

### Port Forwarding (Stretch Goal)

| Tool | Parameters | Description |
|------|-----------|-------------|
| `forward_port` | `local_port`, `remote_port` | Create SSH port forward |
| `forward_list` | — | List active forwards |
| `forward_stop` | `local_port` | Stop a forward |

## MCP Resource: `tramp://context`

Dynamic resource that provides prompt injection for network boundary awareness.

**When no target is active:** Returns empty or a one-liner that tramp is available.

**When a target is active:**

```
REMOTE TARGET ACTIVE: "{name}" ({type}: {host/container})
Platform: {platform}/{arch} | Shell: {shell} | CWD: {cwd}

NETWORK BOUNDARY:
- Your built-in tools (Read, Write, Edit, Bash, Glob, Grep, LS) operate on
  the LOCAL machine where you are running.
- To operate on the remote target, use the tramp MCP tools: read, write,
  edit, bash, glob, grep, ls. They have the same signatures as your built-ins.
- Local paths and remote paths are different filesystems. Do not mix them.
- If you need to transfer files between local and remote, use local Read +
  tramp write (or vice versa).
```

**Tool response hints:** Minimal — just the target name in response metadata:

```json
{"target": "dev", "stdout": "...", "exitCode": 0}
```

## Transports

### SSH

Built on `golang.org/x/crypto/ssh` + `github.com/pkg/sftp`.

- **Authentication priority:** SSH agent (SSH_AUTH_SOCK / Pageant) → key file → password
- **Multiplexing:** Single TCP connection, multiple channels. Each `bash` call opens a new session channel and can run concurrently.
- **File I/O:** SFTP subsystem — binary-safe, no encoding overhead.
- **Shell detection on connect:** Probes for shell type, platform, arch, homedir via a session channel.
- **Port forwarding:** Native support via `ssh.Client`.
- **Reconnect:** On failure, pool evicts the connection. Next use triggers lazy reconnect.

**Config:**

```json
{
  "type": "ssh",
  "host": "user@hostname",
  "port": 22,
  "identityFile": "~/.ssh/id_ed25519",
  "cwd": "/remote/working/dir",
  "shell": "bash",
  "timeout": 60000
}
```

### Docker

Built on `github.com/docker/docker/client`.

- **Commands:** Docker Exec API — create exec instance, attach stdin/stdout/stderr, run. Proper stream separation.
- **File I/O:** `CopyFromContainer` / `CopyToContainer` API (tar-based, handled internally).
- **Shell detection:** Same probes as SSH, run via exec.
- **No persistent connection needed.** Docker API is request/response, but client is kept in the pool for config reuse and container validation.

**Config:**

```json
{
  "type": "docker",
  "container": "my-dev-container",
  "cwd": "/workspace",
  "shell": "bash"
}
```

## Shell Drivers

Transports execute commands; shell drivers shape them.

### BashDriver

- **Escaping:** POSIX single-quote with embedded quote handling (`it's` → `'it'"'"'s'`)
- **Commands:** `cd /cwd && cmd` wrappers, `mkdir -p`, `base64` (not needed for file I/O but available), stat via `[ -f ... ]`
- **Compatible with:** bash, sh, dash, zsh, ash

### PwshDriver

- **Escaping:** Single-quote doubling (`it's` → `'it''s'`)
- **ANSI suppression:** `$PSStyle.OutputRendering = 'PlainText'`
- **Commands:** `Set-Location` wrappers, `New-Item -ItemType Directory -Force`, `Test-Path`
- **Note:** Paths must always be quoted inside .NET calls (unquoted `/` parsed as division)

Shell driver is selected once at connect time based on config or detection, then reused for all commands on that target.

## Target Manager

### State

- `targets: map[string]Target` — all known targets (dynamic + file-based)
- `currentTarget: string | nil` — active target name
- Thread-safe via Go mutex (MCP calls can arrive concurrently)

### Configuration

**File locations:**

- Project: `.claude/tramp.json` (highest priority)
- Global: `~/.claude/tramp.json`

Project config overrides global by target name. Merged at startup.

```json
{
  "default": "dev",
  "targets": {
    "dev": {
      "type": "ssh",
      "host": "user@dev.example.com",
      "cwd": "/home/user/project",
      "shell": "bash"
    }
  }
}
```

### Dynamic Targets

Primary workflow. Added via `target_add` tool at runtime, stored in memory only. A `persist` flag on `target_add` can optionally write to the project config file.

### Switching

`target_switch(name)`:

1. Validate target exists
2. Eagerly connect (fail fast if unreachable)
3. Detect shell/platform/arch/homedir
4. Set as current
5. `tramp://context` resource updates automatically

**Reserved name:** `"local"` clears the active target. Resource returns empty.

## Connection Pool

- Keyed by target name
- Lazy connect on first operation (or eager on `target_switch`)
- Health check before reuse (SSH keepalive / Docker ping)
- Evict on disconnect, reconnect on next use
- `closeAll()` on process shutdown

## Error Handling

### Error Types

| Type | Cause | Behavior |
|------|-------|----------|
| `connection_lost` | SSH dropped, container stopped | Pool evicts, next op triggers reconnect |
| `command_failed` | Non-zero exit code | Not an error — returned as-is with stdout/stderr/exitCode |
| `timeout` | Command exceeded deadline | Killed remotely, error returned |
| `not_connected` | Tool called with no active target | Clear message: "No remote target active. Use target_switch first." |
| `auth_failed` | SSH auth rejected | Surfaced on connect with actionable details |

### File Operation Errors

- Not found → path + "does not exist on remote target {name}"
- Permission denied → path + permission error + suggest checking ownership
- Edit `old_string` not found → same behavior as Claude Code's Edit tool

### Reconnect Strategy

No automatic retry of failed operations. Pool reconnects lazily on next use. If a command was in-flight when connection dropped, it fails — the agent can retry.

## Project Structure

```
tramp/
├── cmd/
│   └── tramp/
│       └── main.go              # Entry point, MCP server setup
├── internal/
│   ├── mcp/
│   │   ├── server.go            # MCP server registration
│   │   ├── tools_target.go      # target_add, target_switch, etc.
│   │   ├── tools_remote.go      # bash, read, write, edit, glob, grep, ls
│   │   └── resource.go          # tramp://context dynamic resource
│   ├── target/
│   │   ├── manager.go           # Target state, switching
│   │   └── config.go            # tramp.json parsing, merge logic
│   ├── pool/
│   │   └── pool.go              # Connection pool, health checks, eviction
│   ├── transport/
│   │   ├── transport.go         # Transport interface
│   │   ├── ssh.go               # SSH transport
│   │   ├── sftp.go              # SFTP file operations
│   │   ├── docker.go            # Docker transport
│   │   └── detect.go            # Shell/platform/arch detection probes
│   └── shell/
│       ├── driver.go            # ShellDriver interface
│       ├── bash.go              # Bash escaping + command generation
│       └── pwsh.go              # PowerShell escaping + command generation
├── go.mod
├── go.sum
├── LICENSE                      # LGPL-3.0
└── README.md
```

### Key Dependencies

| Dependency | Purpose |
|-----------|---------|
| `github.com/modelcontextprotocol/go-sdk/mcp` | MCP protocol (official SDK, Google collaboration) |
| `golang.org/x/crypto/ssh` | SSH transport |
| `github.com/pkg/sftp` | SFTP file operations |
| `github.com/docker/docker/client` | Docker API |

### Build & Distribution

```bash
go build ./cmd/tramp
```

Single binary. Claude Code configuration (`~/.claude/settings.json`):

```json
{
  "mcpServers": {
    "tramp": {
      "command": "tramp",
      "args": ["serve"]
    }
  }
}
```

## Testing Strategy

### Unit Tests

- Shell drivers: escaping round-trips, command generation (pure functions)
- Target manager: config loading, merging, switching, reserved names
- Connection pool: lazy connect, eviction, health check (mocked transports)
- Path resolution: absolute/relative, cross-platform
- MCP tool input validation

### Integration Tests

Require real targets. Matrix: `{SSH, Docker} × {bash, pwsh}` (4 combinations).

- SSH transport against a real SSH server (dockerized openssh in CI)
- Docker transport against a real container
- File round-trip: write → read → compare
- Edit round-trip: write → edit → read → verify
- Bash: execution, exit codes, stderr separation, timeout
- Glob/grep/ls: correctness against known directory structures
- SFTP operations (SSH) and container copy (Docker)

### CI

- Linux: SSH + Docker, bash + pwsh
- Windows: SSH + Docker, bash + pwsh
- Go test with `-race` flag
- Docker Compose for test targets (openssh-server + pwsh container)

## Lineage

tramp is a reimplementation of [pi-tramp](https://github.com/marcfargas/pi-tramp) (TypeScript, pi coding agent extension) as a standalone Go MCP server. The core concepts — target management, connection pooling, shell drivers, context injection — carry over. The transport layer is rebuilt from scratch to take advantage of Go's SSH library (native multiplexing, SFTP) and the Docker SDK.
