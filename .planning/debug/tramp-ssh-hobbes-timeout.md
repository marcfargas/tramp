---
slug: tramp-ssh-hobbes-timeout
status: investigating
trigger: "using tramp to access hobbes, ssh marc@192.168.122.29 does not work. ssh.exe works."
created: 2026-06-07
updated: 2026-06-07
---

# Debug Session: tramp-ssh-hobbes-timeout

## Symptoms

- **Expected:** tramp connects to target `hobbes` (`marc@192.168.122.29`) over SSH, same as the native `ssh.exe marc@192.168.122.29` does.
- **Actual:** tramp fails to connect. Native `ssh.exe marc@192.168.122.29` works fine from the same machine.
- **Error (reproduced by orchestrator):**
  ```
  connecting to "hobbes": SSH dial 192.168.122.29:22: dial tcp 192.168.122.29:22: i/o timeout
  ```
  The failure is at the **TCP dial layer, before the SSH handshake/auth** — the socket never connects.
- **Timeline:** New target, never worked via tramp.
- **Auth:** hobbes is reached via SSH agent when using `ssh.exe`.
- **Reproduction:**
  1. `target_add` name=hobbes config=`{"type":"ssh","host":"marc@192.168.122.29"}` persist=false
  2. `target_switch hobbes` → `dial tcp 192.168.122.29:22: i/o timeout`

## Key environmental facts

- `192.168.122.0/24` is the **default libvirt/KVM NAT network** (typically `virbr0` = `192.168.122.1/24`). hobbes is almost certainly a VM behind libvirt NAT.
- Native `ssh.exe` succeeds dialing the same IP:port from the same host.
- Go's `crypto/ssh` does **not** parse `~/.ssh/config` — no `ProxyJump`, `ProxyCommand`, `HostName`, or per-host routing that `ssh.exe` honors.

## Current Focus

- **hypothesis:** CONFIRMED — `ssh.exe` reaches 192.168.122.29 via a `ProxyJump` directive in `~/.ssh/config` (`Host 192.168.122.*` → `ProxyJump root@vm1.tailed30d.ts.net`) that tramp does not replicate. tramp does a direct `ssh.Dial("tcp", "192.168.122.29:22", ...)`, which has no route to the libvirt-NAT VM, so the TCP connect times out.
- **decision:** Fix direction chosen = **honor `~/.ssh/config`** (make tramp behave like `ssh.exe`), not an explicit per-target field.
- **next_action:** Implement ProxyJump-via-ssh_config. Library research done (see Resolution → Research). Library locked = **`github.com/kevinburke/ssh_config` v1.3** — confirmed safe: Marc's `~/.ssh/config` has no `Match` blocks and no `Include` directives (the only kevinburke caveats). Then: split the fused `ssh.Dial` at `internal/transport/ssh.go:83` into a jump→tunnel sequence, resolving HostName/Port/User/IdentityFile/ProxyJump per hop.

## Research — ssh_config parsing in Go (2026-06-08)

- `golang.org/x/crypto/ssh` (v0.49.0) has **no** ssh_config parser. Upstream req Go #18781 parked since 2017. We must add a dependency or hand-roll.
- **Recommended:** `github.com/kevinburke/ssh_config` v1.3 (Feb 2025), MIT (LGPL-3.0 compatible), zero transitive deps, ~10k dependents. Supports first-match-wins, `Host` wildcards (`192.168.122.*`), negated patterns, `Include`, and `Default(key)`.
  - **Caveat (important):** hard-errors on ANY `Match` block in the parsed file — not just the matched host. If Marc's `~/.ssh/config` contains any `Match` stanza, the whole parse fails.
  - Alternative if Match needed: `github.com/ncode/ssh_config` (kevinburke fork, MIT, adds `Resolve()` covering Match; no stable semver tag yet).
- All such libs are **parse/lookup only** — none build the tunnel. We dial twice ourselves: `ssh.Dial(jump)` → `jumpClient.Dial("tcp", target)` → `ssh.NewClientConn` → `ssh.NewClient`.
- ProxyJump value semantics: `user@host:port`, comma-separated multi-hop chain (resolve each hop recursively against the config), or literal `none` (= direct). No `%h/%p/%r` token expansion for ProxyJump (that's ProxyCommand only).

## Evidence

- timestamp: 2026-06-07 — Reproduced timeout via tramp MCP `target_add`/`target_switch`. Error: `dial tcp 192.168.122.29:22: i/o timeout`. Native `ssh.exe` confirmed working by user.

## Eliminated

(none yet)

## Resolution

(pending)
