# Codebase Concerns

**Analysis Date:** 2026-06-07

> Scope note: This is a small (~3,500 LOC) Go MCP server with no `TODO`/`FIXME`/`HACK` markers and a passing test suite (`go test ./...` green, `go vet` clean). The concerns below are derived from reading the source, not from author annotations. The dominant theme is **shell command injection in the MCP tool layer** — escaping helpers exist in `internal/shell` but the higher-level tools bypass them.

## Tech Debt

**Escaping helpers exist but are bypassed by the MCP tool layer:**
- Issue: `internal/shell` defines a full `ShellDriver.Escape` / `escapeForPath` contract (`internal/shell/driver.go:11-17`, `internal/shell/bash.go:21-44`, `internal/shell/pwsh.go:39-54`) that correctly single-quotes arguments and rejects null bytes. The MCP tools in `internal/mcp/tools_remote.go` do **not** use them — they build commands with raw `fmt.Sprintf` and interpolate user-controlled `pattern`, `path`, and `glob` strings directly into shell command lines.
- Files: `internal/mcp/tools_remote.go:130-136` (RemoteGlob), `:157-178` (RemoteGrep), `:198-204` (RemoteLS)
- Impact: Two layers of escaping logic with no single source of truth. The driver is wired into `Exec` only via `CwdWrap` (`internal/transport/ssh.go:158`, `internal/transport/docker.go:120`); everything else hand-rolls quoting. See Security section for the injection consequences.
- Fix approach: Route every tool that constructs a remote command through the active transport's `ShellDriver.Escape` for each interpolated argument. Expose the driver (or an escaping method) on the `Transport` interface so the MCP layer is not forced to special-case bash vs pwsh inline (it currently sniffs `info.Shell == "pwsh"` in three places).

**`connecting` dedup map is declared but never used:**
- Issue: `Pool` declares `connecting map[string]chan struct{}` with the comment "prevents duplicate connect attempts" (`internal/pool/pool.go:20`) and initializes it (`:28`), but no code path ever reads or writes it. The intended single-flight behavior does not exist.
- Files: `internal/pool/pool.go:20,28,62-77`
- Impact: Dead field plus a missing feature masquerading as present. See Performance/Concurrency for the resulting duplicate-connection race.
- Fix approach: Either implement single-flight (hold the channel in the map while connecting, have concurrent callers wait on it) or delete the field. Implementing it is preferable since the race is real.

**Inconsistent path resolution between SSH and Docker:**
- Issue: `SSHTransport.resolvePath` treats both `filepath.IsAbs` and a leading `/` as absolute (`internal/transport/ssh.go:313-321`), while `DockerTransport.resolvePath` only checks a leading `/` (`internal/transport/docker.go:356-364`). On a Windows host, `filepath.IsAbs` has Windows semantics, so the two transports can classify the same path differently.
- Files: `internal/transport/ssh.go:313-321`, `internal/transport/docker.go:356-364`
- Impact: Subtle cross-platform inconsistency for relative-path file operations. Remote targets are always POSIX-pathed, so using host `filepath` semantics for remote-path decisions is conceptually wrong.
- Fix approach: Use `path` (not `filepath`) and a consistent POSIX rule (`strings.HasPrefix(path, "/")`) in both transports for remote path joins.

**Stray untracked file in repo root:**
- Issue: `git status` shows an untracked file `C:devtrampstderr_test.txt` (a literal-backslash filename, ~4KB) in the working tree, plus a committed `tramp.exe` binary at repo root.
- Files: repo root (`stderr_test.txt`, `tramp.exe`)
- Impact: Accidental test artifact; `tramp.exe` is a build output that should not be version-controlled.
- Fix approach: Delete the stray file and add `tramp.exe` / `*.exe` to `.gitignore`; remove the committed binary from tracking.

## Known Bugs

**Docker `WriteFile` crashes on filenames without a slash:**
- Symptoms: `resolved[:strings.LastIndex(resolved, "/")]` returns `resolved[:-1]` when there is no `/`, which panics with a slice bounds error.
- Files: `internal/transport/docker.go:170,180`
- Trigger: Call `write` against a Docker target with a bare filename and no configured `cwd` (so `resolvePath` returns the name unchanged with no leading slash). `LastIndex` returns `-1`, and `resolved[:-1]` panics.
- Workaround: Always pass absolute paths to the `write` tool on Docker targets. Proper fix: guard `LastIndex == -1` and default the directory to `.` or the container working dir.

**`RemoteRead` offset/limit slicing can produce an off-by-one / negative window:**
- Symptoms: With `offset > 0` and `limit > 0`, the code computes `start = offset - 1` then `end = start + limit`. If `start+limit` overflows or `start` lands exactly at `len(lines)` it is guarded, but `limit` of 0 vs negative is not validated and `offset` is not bounded below 1.
- Files: `internal/mcp/tools_remote.go:63-77`
- Trigger: Edge values of `offset`/`limit` from the tool input (both are `*int` with no range validation).
- Workaround: Pass sensible positive values. Fix: clamp/validate `offset >= 1` and `limit >= 0` before slicing.

**PowerShell session setup result is silently discarded:**
- Symptoms: `t.execSimple(ctx, pwshDriver.SessionSetup())` (`internal/transport/ssh.go:119`) and the Docker equivalent (`internal/transport/docker.go:93`) ignore both the error and the output. If session setup fails, the connection still reports `StateConnected` and later pwsh commands may misbehave.
- Files: `internal/transport/ssh.go:117-120`, `internal/transport/docker.go:91-94`
- Trigger: A pwsh target where `SessionSetup` fails (missing module, profile error).
- Workaround: None at runtime. Fix: check the error and either fail the connect or surface a warning via the context resource.

## Security Considerations

**Command injection via glob/grep/ls tool arguments (highest priority):**
- Risk: `RemoteGlob`, `RemoteGrep`, and `RemoteLS` interpolate caller-supplied strings into shell command lines without escaping. Examples:
  - `find %s -name '%s' -type f ...` with `searchPath` and `pattern` (`internal/mcp/tools_remote.go:135`)
  - `grep ... '%s' %s 2>/dev/null | head -250` with `pattern` and `searchPath` (`:174-176`)
  - `ls -la %s` with `lsPath` (`:203`)
  - pwsh variants embed args in single-quoted strings that PowerShell treats differently (`:133`, `:201`)
- Files: `internal/mcp/tools_remote.go:130-211`
- Current mitigation: None. The arguments flow straight from MCP tool inputs to the remote shell. A `pattern` like `'; rm -rf ~; '` or a `path` containing `$(...)` / backticks executes arbitrary remote commands. (Note: the `bash` tool intentionally runs arbitrary commands, so it is not the concern — but `glob`/`grep`/`ls` are presented as structured, safe operations and are not.)
- Recommendations: Escape every interpolated argument with the active `ShellDriver.Escape`. For grep, the `--include='%s'` and `'%s'` pattern construction must be quoted via the driver, not by manually wrapping in single quotes (manual `'...'` wrapping is itself defeated by an embedded `'`).

**Host key verification silently downgrades to insecure:**
- Risk: When `~/.ssh/known_hosts` does not exist, `buildHostKeyCallback` returns `ssh.InsecureIgnoreHostKey()` (`internal/transport/ssh.go:340-342`) — accepting any host key with no warning. Combined with `insecureIgnoreHostKey:true` being shown as the default in the user-facing config schema examples (`internal/mcp/resource.go:27,73`), this normalizes MITM-vulnerable connections.
- Files: `internal/transport/ssh.go:334-350`, `internal/mcp/resource.go:27,73`
- Current mitigation: `knownhosts.New` is used when the file exists.
- Recommendations: Fail closed (return an error) when host key verification cannot be performed and the user did not explicitly opt into insecure mode. Change the documented schema examples to `insecureIgnoreHostKey:false`.

**Persisted config written world-readable:**
- Risk: `PersistTarget` writes `.claude/tramp.json` with mode `0644` (`internal/target/config.go:123`). That file can contain `host`, `identityFile` paths, and other connection details (no secret key material, but the SSH identity path and infrastructure topology are sensitive).
- Files: `internal/target/config.go:114,123`
- Current mitigation: Directory created `0755`.
- Recommendations: Write the config file `0600` and the directory `0700` to match the sensitivity of SSH-related metadata.

**`forward_port` binds to localhost only — verify it stays that way:**
- Risk: `ForwardPort` hardcodes `localhost:` + port (`internal/mcp/tools_forward.go:25`), which is correct. No concern today, but the `localAddr` is keyed verbatim into the forwards map, so any future change that lets callers supply a bind address would expose the tunnel on all interfaces.
- Files: `internal/mcp/tools_forward.go:14,25`
- Current mitigation: Localhost binding is enforced.
- Recommendations: Keep the bind address non-configurable, or validate it against loopback if it ever becomes configurable.

## Performance Bottlenecks

**Duplicate connections under concurrent first-use (race):**
- Problem: `Pool.Get` releases the lock before calling `connect` for an uncached target (`internal/pool/pool.go:57-58`), and `connect` only takes the lock to store the result (`:72-74`). Two concurrent `Get` calls for the same new target both miss the cache, both dial, and the second overwrites the first in the map — leaking the first connection (never `Close`d).
- Files: `internal/pool/pool.go:34-77`
- Cause: The `connecting` single-flight map (`:20`) was designed to prevent exactly this but is never used.
- Improvement path: Implement single-flight using the existing `connecting` map (or `golang.org/x/sync/singleflight`), so concurrent callers share one dial.

**Whole-file reads with no size cap over SFTP / Docker tar:**
- Problem: `ReadFile` does `io.ReadAll` of the entire remote file into memory before any offset/limit is applied at the MCP layer (`internal/transport/ssh.go:221`, `internal/transport/docker.go:158`). The `read` tool's `offset`/`limit` only slice *after* the full file is loaded (`internal/mcp/tools_remote.go:56-77`).
- Files: `internal/transport/ssh.go:209-222`, `internal/transport/docker.go:140-159`, `internal/mcp/tools_remote.go:50-80`
- Cause: No streaming or server-side `head`/`tail` for the read path.
- Improvement path: For large files, push offset/limit down to a remote `sed -n`/`Get-Content -TotalCount` command, or stream and stop after `offset+limit` lines.

## Fragile Areas

**Docker stream demultiplexer (hand-rolled stdcopy):**
- Files: `internal/transport/docker.go:261-292`
- Why fragile: `stdCopyFromDockerStream` reimplements Docker's 8-byte framed multiplex protocol by hand instead of using `github.com/docker/docker/pkg/stdcopy`. The 4-byte big-endian size parse and the `header[0]` stream-type switch are correct for the documented format but will break silently if a frame is partially read or if TTY mode is ever enabled (TTY streams are *not* framed, so this loop would corrupt output).
- Safe modification: Replace with `stdcopy.StdCopy`. If kept, never enable `Tty: true` on the exec config without branching the reader.
- Test coverage: Covered indirectly by `test/integration/docker_bash_test.go` (requires Docker; not run in unit CI). No unit test for malformed frames.

**`TransportState` mutated without consistent locking on reads:**
- Files: `internal/transport/ssh.go:44` (`State()` reads `t.state` without holding `t.mu`), `internal/transport/docker.go:39`, all the `t.state = ...` writes inside `Connect`/`Close` (which *do* hold the lock).
- Why fragile: `State()` and `Exec`'s `if t.state != StateConnected` check (`ssh.go:153`, `docker.go:116`) read `state` without the mutex while `Connect`/`Close` write it under the mutex. This is a data race on the `state` field — `go test -race` would likely flag it; the pool calls `HealthCheck`/`State` concurrently with potential reconnects.
- Safe modification: Read `state` under the lock (or make it an atomic). Run the suite with `-race`.
- Test coverage: Unit tests pass but are not run with `-race` in the documented flow.

**Three-way shell branching duplicated across layers:**
- Files: `internal/mcp/tools_remote.go:132,200` (sniff `info.Shell == "pwsh"`), `internal/transport/docker.go:122-127`, `internal/transport/ssh.go:100-104`
- Why fragile: The bash-vs-pwsh-vs-sh decision is made in at least four places with slightly different logic (the MCP layer only knows "pwsh or not"; docker has a separate `sh` fallback). Adding a shell or changing detection requires touching all of them.
- Safe modification: Centralize command construction behind the `ShellDriver` so the MCP layer never branches on shell type.

## Scaling Limits

**Connection pool is unbounded and never idle-evicts:**
- Current capacity: One cached `Transport` per target name, held for the process lifetime.
- Limit: Connections are only evicted on health-check failure (`internal/pool/pool.go:42-54`) or explicit `Close`/`CloseAll`. A long-running server that switches among many targets accumulates open SSH/Docker clients with no idle timeout or max-connection cap.
- Scaling path: Add idle-expiry (close connections unused for N minutes) and/or an LRU cap. For a single-user MCP server this is low priority but worth noting.

## Dependencies at Risk

**`github.com/docker/docker v28.1.1+incompatible`:**
- Risk: The `+incompatible` suffix means the module is not using Go modules' semantic-import versioning; the Docker SDK is large and its API surface (`container.ExecOptions`, `CopyToContainerOptions`) shifts between releases.
- Impact: Upgrades can break `internal/transport/docker.go` silently at compile time; the SDK pulls in a heavy transitive tree (OpenTelemetry, containerd/log, moby/*).
- Migration plan: Pin deliberately and gate upgrades behind the integration tests in `test/integration/`. Consider whether the full Engine API client is needed or whether a thinner exec client would do.

**`github.com/modelcontextprotocol/go-sdk v1.4.1` marked indirect:**
- Risk: The MCP SDK is the core runtime dependency (imported as `gomcp` throughout `internal/mcp`) yet appears in the `// indirect` require block of `go.mod` (`go.mod:27`), suggesting `go mod tidy` classified it oddly or a direct require was dropped.
- Impact: Cosmetic/risk of accidental removal on a future tidy. The SDK is young (v1.x) and its tool-registration API may change.
- Migration plan: Run `go mod tidy` and confirm it moves to the direct-require block; track SDK releases for breaking changes to `AddTool`/`AddResource`.

## Missing Critical Features

**No `-race` in the standard test flow:**
- Problem: Given the concurrent pool access and unlocked `State()` reads, the absence of race-detector runs means real data races (see Fragile Areas) go undetected.
- Blocks: Confidence in concurrency correctness.

**No validation/limits on tool inputs:**
- Problem: Tool inputs (`pattern`, `path`, `offset`, `limit`, `local_port`, `remote_port`) are passed through with no validation. `local_port`/`remote_port` are strings concatenated into addresses (`internal/mcp/tools_forward.go:25,101`) with no numeric or range check.
- Blocks: Robustness; a non-numeric port silently produces a bad `net.Listen` address error rather than a clear message.

## Test Coverage Gaps

**MCP tool command construction is untested (and is where the injection bug lives):**
- What's not tested: `RemoteGlob`, `RemoteGrep`, `RemoteLS`, `RemoteRead` slicing, `RemoteEdit` — none have dedicated unit tests for the command strings they build. `internal/mcp/tools_remote_test.go` exists but does not cover the shell-command assembly that contains the injection vulnerability.
- Files: `internal/mcp/tools_remote.go` (whole file), tested only by `internal/mcp/tools_remote_test.go` and `tools_target_test.go`
- Risk: The command-injection issue and the offset/limit edge cases could be fixed and regress with no test signal.
- Priority: High

**Docker transport unit coverage relies on integration tests requiring a live daemon:**
- What's not tested (without Docker): `WriteFile` slash-edge-case panic, `stdCopyFromDockerStream` framing, `ReadFile` tar extraction. `internal/transport/docker_test.go` exists but the meaningful paths are in `test/integration/docker_bash_test.go` (gated on Docker availability).
- Files: `internal/transport/docker.go:140-292`
- Risk: The `WriteFile` panic (see Known Bugs) is reachable and untested.
- Priority: High

**Pool concurrency / eviction untested for races:**
- What's not tested: Concurrent `Get` for the same uncached target (the duplicate-connection race), and `Get`'s evict-and-reconnect branch under contention.
- Files: `internal/pool/pool.go:34-77`, tested by `internal/pool/pool_test.go`
- Risk: Single-flight regressions and connection leaks go unnoticed.
- Priority: Medium

**SSH auth / host-key fallback paths untested:**
- What's not tested: `buildAuthMethods` ordering and the "no methods available" error; `buildHostKeyCallback` insecure-fallback when `known_hosts` is absent.
- Files: `internal/transport/ssh.go:334-402`
- Risk: The silent insecure downgrade (see Security) has no test asserting the intended policy.
- Priority: Medium

---

*Concerns audit: 2026-06-07*
