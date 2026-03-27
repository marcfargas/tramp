# tramp

TRAMP-like transparent remote execution for AI coding agents. MCP server that routes file and shell operations to remote targets via SSH or Docker.

The agent stays local. Tools execute remotely.

## Install

### One-liner (Go required)

```bash
go install github.com/marcfargas/tramp/cmd/tramp@latest
claude mcp add tramp -- tramp serve
```

### From GitHub Releases

Download the binary for your platform from [Releases](https://github.com/marcfargas/tramp/releases):

```bash
# Linux (amd64)
curl -L https://github.com/marcfargas/tramp/releases/latest/download/tramp_0.1.0_linux_amd64.tar.gz | tar xz
sudo mv tramp /usr/local/bin/

# macOS (Apple Silicon)
curl -L https://github.com/marcfargas/tramp/releases/latest/download/tramp_0.1.0_darwin_arm64.tar.gz | tar xz
sudo mv tramp /usr/local/bin/

# Windows — download the zip from Releases, extract, add to PATH
```

Then add to Claude Code:

```bash
claude mcp add tramp -- tramp serve
```

## Quick Start

In a Claude Code session:

```
> target_add("myserver", {"type":"ssh","host":"user@myserver.example.com","shell":"bash","cwd":"/home/user/project"})
> target_switch("myserver")

# All tramp tools now execute on myserver
> bash("ls -la")
> read("/etc/hostname")
```

## Tools

All remote tools accept an optional `target` parameter to specify which target to use per-call, enabling multi-target workflows without switching.

### Target Management

| Tool | Description |
|------|-------------|
| `target_add` | Add a remote target (SSH or Docker). Set `persist: true` to save to `.claude/tramp.json` |
| `target_switch` | Connect and switch active target. Use `"local"` to disconnect |
| `target_remove` | Remove a target |
| `target_list` | List all targets with status |
| `target_status` | Show active target details |

### Remote Operations

Mirror Claude Code's built-in tool signatures:

| Tool | Description |
|------|-------------|
| `bash` | Execute shell command |
| `read` | Read file contents (with offset/limit) |
| `write` | Write/overwrite file |
| `edit` | Find-and-replace edit |
| `glob` | File pattern matching |
| `grep` | Content search |
| `ls` | List directory |

### Port Forwarding (SSH only)

| Tool | Description |
|------|-------------|
| `forward_port` | Create SSH tunnel (local port → remote address) |
| `forward_list` | List active port forwards |
| `forward_stop` | Stop a port forward |

## Configuration

### Dynamic Targets (Primary)

Add targets on the fly via `target_add`. Stored in memory for the session. Use `persist: true` to save to the project config file.

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

Project config overrides global by target name. The `default` target auto-connects on startup.

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
  "timeout": 60000,
  "insecureIgnoreHostKey": false
}
```

- Native multiplexed channels (`golang.org/x/crypto/ssh`) — no sentinel protocol
- SFTP for file I/O — binary-safe, no base64 encoding
- Auth: SSH agent (Unix + Windows OpenSSH) → identity file → default keys
- Host key verification via `~/.ssh/known_hosts` (set `insecureIgnoreHostKey: true` to skip)
- Persistent connection, reused across tool calls
- Port forwarding support

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

## License

LGPL-3.0
