# Technology Stack

**Analysis Date:** 2026-06-07

## Languages

**Primary:**
- Go 1.26.1 - Entire codebase. Declared in `go.mod` (`go 1.26.1`). CI pins the toolchain via `go-version-file: go.mod`.

**Secondary:**
- Bash - Plugin bootstrap/installer script `.claude-plugin/bin/bootstrap.sh` and CI shell steps in `.github/workflows/ci.yml`.
- POSIX shell + PowerShell (pwsh) - Generated remotely, not part of the host codebase. Shell drivers in `internal/shell/bash.go` and `internal/shell/pwsh.go` produce remote command strings.

## Runtime

**Environment:**
- Native compiled binary - No managed runtime. Single static binary `tramp` (built with `CGO_ENABLED=0`, see `.goreleaser.yml`).
- Runs as an MCP server over stdio. Entry point `cmd/tramp/main.go` requires `tramp serve`.

**Package Manager:**
- Go modules. Manifest: `go.mod`. Lockfile: `go.sum` (present, committed).
- Dependencies fetched via `go mod download` in CI.

## Frameworks

**Core:**
- `github.com/modelcontextprotocol/go-sdk` v1.4.1 - MCP server SDK. Provides `mcp.NewServer`, `mcp.AddTool`, `mcp.StdioTransport`, resources. Used in `internal/mcp/server.go`, `cmd/tramp/main.go`. (Listed as indirect in `go.mod` but is the central framework — imported directly across `internal/mcp/`.)

**Testing:**
- Go standard `testing` package - All `*_test.go` files. No third-party test framework.
- `gotest.tools/v3` v3.5.2 - Transitive (pulled in by Docker client), not used directly in project tests.
- Build-tag `integration` gates real SSH/Docker tests under `test/integration/` (see `go test -tags integration`).

**Build/Dev:**
- GoReleaser v2 - Multi-platform release builds and archives. Config: `.goreleaser.yml`. Run via `goreleaser/goreleaser-action@v6` in `.github/workflows/release.yml`.
- `go vet` and `gofmt` - Lint/format gate in CI `lint` job.

## Key Dependencies

**Critical:**
- `golang.org/x/crypto` v0.49.0 - SSH client, SSH agent client, knownhosts host-key verification. Core of `internal/transport/ssh.go` (`golang.org/x/crypto/ssh`, `.../ssh/agent`, `.../ssh/knownhosts`).
- `github.com/pkg/sftp` v1.13.10 - SFTP file I/O over SSH (binary-safe read/write) in `internal/transport/ssh.go`.
- `github.com/docker/docker` v28.1.1+incompatible - Docker Engine API client for exec and tar-based file copy in `internal/transport/docker.go` (`docker/docker/client`, `.../api/types/container`).
- `github.com/Microsoft/go-winio` v0.4.21 - Windows named-pipe dial to the OpenSSH agent (`\\.\pipe\openssh-ssh-agent`) in `internal/transport/agent_windows.go`.

**Infrastructure (transitive, via Docker client):**
- `github.com/docker/go-connections` v0.6.0 - Docker connection helpers.
- `github.com/distribution/reference`, `github.com/opencontainers/image-spec`, `github.com/moby/*` - Docker image/spec plumbing.
- `go.opentelemetry.io/otel` v1.42.0 and related - Pulled in by Docker client; not configured or exported by tramp.
- `github.com/google/jsonschema-go` v0.4.2, `github.com/yosida95/uritemplate/v3` - Pulled in by the MCP SDK for tool input schemas and resource URIs.

## Configuration

**Environment:**
- No `.env` files. No dotenv loader.
- Docker transport honors standard Docker env (`DOCKER_HOST`, etc.) via `client.FromEnv` in `internal/transport/docker.go`.
- SSH agent discovered via `SSH_AUTH_SOCK` on Unix (`internal/transport/agent_unix.go`) or a fixed named pipe on Windows.
- `CLAUDE_PLUGIN_ROOT` consumed by `.claude-plugin/bin/bootstrap.sh` and `.claude-plugin/.mcp.json` to locate the binary.
- Test env vars: `TRAMP_TEST_DOCKER_CONTAINER`, `TRAMP_TEST_SSH_HOST`, `TRAMP_TEST_SSH_KEY` (integration tests only).

**Application config (JSON, not env):**
- `~/.claude/tramp.json` (global) and `<project>/.claude/tramp.json` (project) - Target definitions. Loaded and merged in `internal/mcp/server.go` (`loadConfigs`), parsed in `internal/target/config.go`. Project overrides global by target name; `default` target auto-connects on startup.

**Build:**
- `go.mod` / `go.sum` - Module and dependency pinning.
- `.goreleaser.yml` - Build matrix (linux/darwin/windows × amd64/arm64), ldflags `-s -w -X main.version={{.Version}}`.
- `.gitattributes`, `.gitignore` - Repo hygiene.

## Platform Requirements

**Development:**
- Go 1.26.1 toolchain.
- For integration tests: Docker (with `docker compose`) and the ability to run `ssh-keygen` / `ssh`.
- Cross-platform: CI runs on `ubuntu-latest` and `windows-latest` (`.github/workflows/ci.yml`).

**Production (host side):**
- Distributed as a standalone binary for linux, darwin, windows on amd64 and arm64 (`.goreleaser.yml`).
- Installed via `go install`, GitHub Releases download, or the Claude Code plugin bootstrap.
- Runs locally alongside the AI agent; communicates over stdio (no network listener on the host except SSH port-forward listeners bound to `localhost`).

**Remote target side:**
- SSH targets: any host reachable over SSH with an SFTP subsystem; bash (POSIX) or pwsh shell.
- Docker targets: a running container reachable via the local Docker Engine API; sh/bash or pwsh.

---

*Stack analysis: 2026-06-07*
