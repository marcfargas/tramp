# tramp

TRAMP-like transparent remote execution for AI coding agents. MCP server that routes file and shell operations to remote targets via SSH or Docker.

The agent stays local. Tools execute remotely.

## Quick Start

```bash
# Build
go build -o tramp ./cmd/tramp

# Add to Claude Code
claude mcp add tramp -- /path/to/tramp serve
```

Then in a Claude Code session:

```
> Use target_add to add a server, then target_switch to connect:
>
> target_add("myserver", {"type":"ssh","host":"user@myserver.example.com","shell":"bash","cwd":"/home/user/project"})
> target_switch("myserver")
>
> # All tramp tools now execute on myserver
> bash("ls -la")
> read("/etc/hostname")
```

## Tools

All remote tools accept an optional `target` parameter to specify which target to use per-call, enabling multi-target workflows without switching.

### Target Management

| Tool | Description |
|------|-------------|
| `target_add` | Add a remote target (SSH or Docker) |
| `target_switch` | Connect and switch active target |
| `target_remove` | Remove a target |
| `target_list` | List all targets with status |
| `target_status` | Show active target details |

### Remote Operations

Mirror Claude Code's built-in tool signatures:

| Tool | Description |
|------|-------------|
| `bash` | Execute shell command |
| `read` | Read file contents |
| `write` | Write file |
| `edit` | Find-and-replace edit |
| `glob` | File pattern matching |
| `grep` | Content search |
| `ls` | List directory |

## Configuration

### Dynamic Targets (Primary)

Add targets on the fly via the `target_add` tool. Stored in memory for the session.

### File-based

Create `.claude/tramp.json` in your project or `~/.claude/tramp.json` globally:

```json
{
  "default": "dev",
  "targets": {
    "dev": {
      "type": "ssh",
      "host": "user@dev.example.com",
      "cwd": "/home/user/project",
      "shell": "bash"
    },
    "docker-dev": {
      "type": "docker",
      "container": "my-dev-container",
      "cwd": "/workspace"
    }
  }
}
```

Project config overrides global by target name.

## Transport Types

### SSH

```json
{
  "type": "ssh",
  "host": "user@hostname",
  "port": 22,
  "identityFile": "~/.ssh/id_ed25519",
  "cwd": "/remote/working/directory",
  "shell": "bash",
  "timeout": 60000
}
```

- Native multiplexed channels (`golang.org/x/crypto/ssh`) — no sentinel protocol
- SFTP for file I/O — binary-safe, no base64 encoding
- Auth: SSH agent (Unix + Windows OpenSSH) → identity file → default keys
- Persistent connection, reused across tool calls

### Docker

```json
{
  "type": "docker",
  "container": "my-container",
  "cwd": "/workspace",
  "shell": "bash"
}
```

- Docker Exec API for commands
- Copy API (tar-based) for file I/O
- Shell auto-detection if not configured

## Network Boundary

When a target is active, tramp exposes a `tramp://context` MCP resource that injects network boundary awareness into the agent's context:

- Built-in tools (Read, Write, Bash, etc.) operate locally
- Tramp tools (read, write, bash, etc.) operate on the remote target
- Local and remote paths are different filesystems

## Shells

Both **bash** (POSIX) and **PowerShell** (pwsh) are supported on remote targets. Shell drivers handle escaping and command generation per-shell.

## Building

```bash
go build -o tramp ./cmd/tramp
```

Single binary, no runtime dependencies.

## License

LGPL-3.0
