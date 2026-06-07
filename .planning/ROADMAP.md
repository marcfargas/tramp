# Roadmap

> Project not yet initialized. Run `/gsd-new-project` to define milestones and phases.
> Items below are parked in the backlog until then.

## Backlog

### Phase 999.1: honor ssh config ProxyJump in SSH transport (BACKLOG)

**Goal:** tramp's SSH transport must honor `~/.ssh/config` (specifically `ProxyJump`) so it can reach hosts only routable via a jump host — like native `ssh.exe`. Today tramp does a direct `ssh.Dial` and times out at the TCP layer for such hosts (e.g. a libvirt-NAT VM reachable only via a configured jump host).
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /gsd-review-backlog when ready)

**Notes:** Root cause + library research already done. Recommended approach: add `github.com/kevinburke/ssh_config` v1.3 (MIT, LGPL-compatible; confirmed no Match/Include in the user's config), resolve the target through ssh_config, and split the fused `ssh.Dial` at `internal/transport/ssh.go:83` into dial-jump → `jumpClient.Dial(target)` → `ssh.NewClientConn` → `ssh.NewClient`. Single-hop first; multi-hop chains and `none` as follow-ups. Full investigation: `.planning/debug/tramp-ssh-hobbes-timeout.md`.
