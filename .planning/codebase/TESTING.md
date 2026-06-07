# Testing Patterns

**Analysis Date:** 2026-06-07

Standard library `testing` only — no third-party test framework, assertion library, or mock generator. Tests are split into fast unit tests (run everywhere, every push) and build-tagged integration tests (run against real Docker/SSH containers in CI).

## Test Framework

**Runner:**
- Go standard `testing` package (`testing.T`), driven by `go test`
- No config file — behaviour is controlled entirely by CLI flags and build tags
- `gotest.tools/v3` is present as an indirect dependency (pulled in by Docker SDK), not used directly in this codebase

**Assertion Library:**
- None. Tests use plain `if got != want { t.Errorf(...) }` comparisons.

**Run Commands:**
```bash
go test ./... -short -count=1            # All unit tests (CI: "Unit tests")
go test ./... -race -short -count=1      # With race detector (CI: Linux only)
go vet ./...                             # Static analysis (CI lint job)
gofmt -l .                              # Formatting check (must print nothing)

# Integration (require running containers + env vars):
go test -tags integration -count=1 -v ./test/integration/ -run "Docker"
go test -tags integration -count=1 -v ./test/integration/ -run "SSH"
```
`-count=1` is always passed to disable the test result cache. `-short` is passed in CI even though no test currently calls `testing.Short()` — new long-running unit tests should gate themselves with `if testing.Short() { t.Skip(...) }`.

## Test File Organization

**Location:**
- Unit tests are **co-located** with source in the same package: `internal/target/manager_test.go` declares `package target` (white-box — tests can call unexported funcs like `parseSSHHost`, see `internal/transport/ssh_test.go:44`)
- Integration tests are **separated** under `test/integration/` in `package integration`, importing the public API only (`internal/target`, `internal/transport`)

**Naming:**
- File: `<source>_test.go` for units; `<transport>_<shell>_test.go` for integration
- Functions: `Test<Type><Behaviour>` — e.g. `TestManagerAddRejectsLocal`, `TestSSHBashExitCode`, `TestRemoteEditNotFound`. The subject and the scenario are both encoded in the name.

**Distribution (function counts):**
| Package | File | Tests |
|---------|------|-------|
| `internal/target` | `manager_test.go` | 9 |
| `internal/target` | `config_test.go` | 5 |
| `internal/mcp` | `tools_remote_test.go` | 8 |
| `internal/mcp` | `tools_target_test.go` | 5 |
| `internal/pool` | `pool_test.go` | 5 |
| `internal/shell` | `bash_test.go` | 6 |
| `internal/shell` | `pwsh_test.go` | 7 |
| `internal/transport` | `detect_test.go` | 5 |
| `internal/transport` | `ssh_test.go` | 2 |
| `internal/transport` | `docker_test.go` / `factory_test.go` | 1 / 1 |
| `test/integration` | `ssh_bash_test.go` | 5 |
| `test/integration` | `docker_bash_test.go` | 4 |

## Test Structure

Flat functions, one behaviour per test, no `TestMain`, no shared suite type. Construct the subject, exercise it, assert. Example (`internal/target/manager_test.go:8-23`):
```go
func TestManagerAddAndList(t *testing.T) {
	m := NewManager()
	cfg := TargetConfig{Type: "ssh", Host: "user@host", Shell: "bash"}

	if err := m.Add("dev", cfg); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	targets := m.List()
	if len(targets) != 1 {
		t.Fatalf("List() len = %d, want 1", len(targets))
	}
	if targets[0].Name != "dev" {
		t.Errorf("Name = %q, want %q", targets[0].Name, "dev")
	}
}
```

**Conventions:**
- `t.Fatalf` when a failure makes the rest of the test meaningless (setup step, nil result); `t.Errorf` for independent assertions so multiple failures surface in one run.
- Assertion message format is consistent: `"<thing> = %q, want %q"` (or `%d`). Use `%q` for strings, `%d` for ints.
- Error-path tests assert presence, not message text: `if err == nil { t.Error("expected error for ...") }` (`internal/target/manager_test.go:28`). Do not assert on error string contents.
- No setup/teardown helpers in unit tests beyond inline construction. Integration tests use `defer tr.Close()` immediately after a successful `Connect`.

## Table-Driven Tests

Used for pure functions with many input/output cases. Anonymous struct slice + `t.Run` subtest per case (`internal/shell/bash_test.go:5-34`):
```go
tests := []struct {
	name     string
	input    string
	expected string
}{
	{"empty string", "", "''"},
	{"single quote", "it's", "'it'\"'\"'s'"},
	{"dollar sign", "$HOME", "'$HOME'"},
}
for _, tt := range tests {
	t.Run(tt.name, func(t *testing.T) {
		got := d.Escape(tt.input)
		if got != tt.expected {
			t.Errorf("Escape(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	})
}
```
The first struct field is the subtest `name`. `parseSSHHost` uses the same shape with the input string itself as the subtest name (`internal/transport/ssh_test.go:31-53`). Prefer this pattern over many near-identical top-level test functions.

## Mocking

**Framework:** None — hand-written mocks implementing the relevant interface.

The `mockTransportRemote` in `internal/mcp/tools_remote_test.go:12-40` implements the full `transport.Transport` interface. Methods that are irrelevant to a test return trivial values; methods under test record inputs and return configurable fields:
```go
type mockTransportRemote struct {
	lastCmd     string
	readResult  []byte
	writeResult []byte
	execResult  *transport.ExecResult
}

func (m *mockTransportRemote) Exec(ctx context.Context, cmd string, opts *transport.ExecOptions) (*transport.ExecResult, error) {
	m.lastCmd = cmd                       // capture input for assertions
	if m.execResult != nil {              // configurable canned response
		return m.execResult, nil
	}
	return &transport.ExecResult{Stdout: "ok", ExitCode: 0}, nil
}
func (m *mockTransportRemote) WriteFile(ctx context.Context, path string, content []byte) error {
	m.writeResult = content               // capture so tests can assert what was written
	return nil
}
```

**Dependency injection point:** the mock is fed in through `pool.New`'s factory function — the production wiring (`internal/mcp/server.go:37-40`) passes the real `transport.NewTransport`, tests pass a closure returning the mock:
```go
func setupTestService(mock *mockTransportRemote) *Service {
	mgr := target.NewManager()
	p := pool.New(func(cfg target.TargetConfig) (transport.Transport, error) {
		return mock, nil
	})
	svc := &Service{Manager: mgr, Pool: p}
	mgr.Add("dev", target.TargetConfig{Type: "ssh", Host: "user@host", Shell: "bash"})
	mgr.Switch("dev")
	return svc
}
```

**What to mock:**
- The `Transport` interface — to test `Service` / MCP-tool logic (target resolution, edit find-replace, offset/limit slicing) without a network.
- Inject via the `TransportFactory` function field, never by patching globals.

**What NOT to mock:**
- Pure logic with no I/O (shell escaping, host parsing, config parsing/merging) — test directly with table-driven cases.
- Real transports — verify those in `test/integration` against actual containers, not mocks.

## Fixtures and Test Data

- No fixture files or factory packages. Test data is inline literals (`TargetConfig{...}`, `[]byte("hello world")`).
- A small per-package helper builds the system-under-test (`setupTestService` in mcp, `sshBashConfig`/`dockerBashConfig` in integration). Keep these private to the test file.
- Integration configs read from environment with hardcoded local fallbacks (`internal/../test/integration/ssh_bash_test.go:14-29`): `TRAMP_TEST_SSH_HOST` defaults to `testuser@localhost`, port `2222`.

## Integration Tests

Gated behind a build tag so they never run in the default `go test ./...`:
```go
//go:build integration

package integration
```
- Run only via `-tags integration` against containers started with `docker compose` (see `test/integration/` and CI jobs `integration-docker`, `integration-ssh`).
- Driven by environment variables set by CI: `TRAMP_TEST_DOCKER_CONTAINER`, `TRAMP_TEST_SSH_HOST`, `TRAMP_TEST_SSH_KEY`. SSH CI generates an ephemeral ed25519 keypair and waits for the container's sshd before running.
- Each test owns its connection lifecycle: `Connect` → `defer Close()` → exercise. File round-trip tests clean up after themselves (`tr.Exec(ctx, "rm "+path, nil)`).
- These jobs `needs: test`, so units must pass first; containers are always torn down via `if: always()`.

## Coverage

- No coverage threshold is enforced and no `-cover` step exists in CI.
- View locally with: `go test ./... -cover` (per-package) or `go test ./... -coverprofile=cover.out && go tool cover -html=cover.out`.

## Common Patterns

**No-active-target / error path** (`internal/mcp/tools_remote_test.go:71-79`):
```go
mgr := target.NewManager()
svc := &Service{Manager: mgr}
_, _, err := svc.RemoteBash(context.Background(), "", "echo hello", 0)
if err == nil {
	t.Error("expected error with no active target")
}
```

**Context:** unit tests pass `context.Background()`; integration tests do the same. No timeout/cancellation contexts are exercised in tests yet.

**Panic testing** uses `defer`/`recover` (`internal/shell/bash_test.go:36-44`):
```go
defer func() {
	if r := recover(); r == nil {
		t.Error("expected panic on null byte")
	}
}()
d.Escape("hello\x00world")
```

**Behaviour-capture assertions:** after exercising a mock, assert on the recorded field (`mock.writeResult`, `mock.lastCmd`) to verify the command/content that would have hit the wire (`internal/mcp/tools_remote_test.go:116-157`).

---

*Testing analysis: 2026-06-07*
