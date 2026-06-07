# External Integrations

**Analysis Date:** 2026-06-07

## APIs & External Services

**Model Context Protocol (MCP) host:**
- The AI coding agent (Claude Code) is the upstream consumer. tramp is an MCP server, not a client.
  - SDK/Client: `github.com/modelcontextprotocol/go-sdk` v1.4.1
  - Transport: stdio (`mcp.StdioTransport` in `cmd/tramp/main.go`)
  - Server identity: name `tramp`, version `0.1.0` (`internal/mcp/server.go`)
  - Exposes tools (target management, remote ops, port forwarding) and one resource (`tramp://context`).

**SSH (remote target transport):**
- Arbitrary remote hosts, configured per target. Used for command exec, file I/O, and port forwarding.
  - SDK/Client: `golang.org/x/crypto/ssh` + `github.com/pkg/sftp` in `internal/transport/ssh.go`
  - Auth: SSH agent (Unix `SSH_AUTH_SOCK`, Windows named pipe `\\.\pipe\openssh-ssh-agent`) → explicit `identityFile` → default keys (`~/.ssh/id_ed25519`, `id_rsa`, `id_ecdsa`). See `buildAuthMethods` in `internal/transport/ssh.go`.
  - Host: `host` field of the target config (`user@hostname`, port default 22).
  - Health check: `keepalive@openssh.com` global request.

**Docker Engine API (remote target transport):**
- The local Docker daemon; targets a named/ID'd container. Used for exec and file copy.
  - SDK/Client: `github.com/docker/docker/client` in `internal/transport/docker.go`
  - Connection: `client.FromEnv` + API version negotiation (honors `DOCKER_HOST` and standard Docker env).
  - Operations: `ContainerExecCreate`/`ExecAttach`/`ExecInspect` for commands; `CopyFromContainer`/`CopyToContainer` (tar streams) for file I/O.
  - Container identity: `container` field of the target config.

## Data Storage

**Databases:**
- None. tramp holds no database and persists no application data of its own.

**File Storage:**
- Remote filesystem access only (the point of the tool):
  - SSH targets: SFTP read/write, parent dirs created with `MkdirAll` (`internal/transport/ssh.go`).
  - Docker targets: tar-based copy to/from the container (`internal/transport/docker.go`).
- Local config files: `~/.claude/tramp.json` and `<project>/.claude/tramp.json` written via `PersistTarget` in `internal/target/config.go` (only when `target_add` is called with `persist: true`).

**Caching:**
- In-memory connection pool only. `internal/pool/pool.go` keeps live transports keyed by target name; reused across tool calls. Cleared on `target_remove` and on shutdown (`svc.Pool.CloseAll()` in `cmd/tramp/main.go`).

## Authentication & Identity

**Auth Provider:**
- None for the MCP surface itself — trust is delegated to the local agent/stdio channel.
- Remote auth is SSH key-based (agent / identity file / default keys). No passwords are handled by the host code path (password access exists only in the integration test container, `test/integration/docker-compose.yml`).
- Host-key verification via `~/.ssh/known_hosts` (`knownhosts.New`). Falls back to insecure if no `known_hosts` exists or when `insecureIgnoreHostKey: true` is set per target (`buildHostKeyCallback` in `internal/transport/ssh.go`).

## Monitoring & Observability

**Error Tracking:**
- None. No Sentry/error-reporting integration.

**Logs:**
- Standard library `log`/`fmt` to stderr only. Fatal startup errors in `cmd/tramp/main.go`. Tool errors are returned to the agent as MCP `IsError` results (`toolError` in `internal/mcp/server.go`), not logged externally.
- OpenTelemetry packages are present transitively (via the Docker client) but are not configured or exported.

## CI/CD & Deployment

**Hosting:**
- Not a hosted service. Distributed as a binary and as a Claude Code plugin.
- Plugin manifest `.claude-plugin/plugin.json`; MCP wiring `.claude-plugin/.mcp.json` (command = `${CLAUDE_PLUGIN_ROOT}/.claude-plugin/bin/bootstrap.sh serve`).
- `bootstrap.sh` downloads the matching release archive from GitHub Releases on first run.

**CI Pipeline (GitHub Actions):**
- `.github/workflows/ci.yml` — on push/PR to `main`/`develop`:
  - `test`: build + unit tests on ubuntu and windows; race detector on Linux.
  - `plugin`: builds binary, validates MCP handshake (`initialize` → `serverInfo`), validates `plugin.json` and `.mcp.json` (jq) on ubuntu and windows.
  - `integration-docker`: spins up `docker compose` services and runs `-tags integration` Docker tests (bash + pwsh matrix).
  - `integration-ssh`: generates an ed25519 keypair, starts the SSH container, runs SSH integration tests.
  - `lint`: `go vet` + `gofmt` check.
- `.github/workflows/release.yml` — on tag `v*`: runs GoReleaser (`goreleaser-action@v6`) with `GITHUB_TOKEN` to publish multi-platform archives + checksums.

**Release artifact registry:**
- GitHub Releases (`marcfargas/tramp`). Referenced by `bootstrap.sh` (`https://github.com/marcfargas/tramp/releases/...`) and README.

## Environment Configuration

**Required env vars:**
- None for normal host operation.
- Optional/contextual: `DOCKER_HOST` (Docker targets), `SSH_AUTH_SOCK` (Unix SSH agent), `CLAUDE_PLUGIN_ROOT` (plugin bootstrap).
- Integration tests: `TRAMP_TEST_DOCKER_CONTAINER`, `TRAMP_TEST_SSH_HOST`, `TRAMP_TEST_SSH_KEY`.
- Release CI: `GITHUB_TOKEN` (provided by the runner).

**Secrets location:**
- SSH private keys on disk under `~/.ssh/` or an explicit `identityFile` (never read into logs; only parsed via `ssh.ParsePrivateKey`).
- No secrets are stored in repo config. `.claude/tramp.json` holds connection metadata (host, container, key path) but not key material.

## Webhooks & Callbacks

**Incoming:**
- None.

**Outgoing:**
- None (no event callbacks). The only outbound HTTP is the one-time release-archive download performed by `bootstrap.sh`.

## Port Forwarding (SSH tunnels)

- `forward_port` / `forward_list` / `forward_stop` tools create local TCP listeners that tunnel to a remote address over the SSH connection. Implemented in `internal/transport/ssh.go` (`ForwardPort`) and orchestrated in `internal/mcp/tools_forward.go`. Listeners bind to `localhost:<port>`; SSH-only (rejected for Docker targets).

---

*Integration audit: 2026-06-07*
