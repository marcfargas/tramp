# tramp Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go MCP server that gives AI coding agents transparent remote access to SSH and Docker targets.

**Architecture:** Single Go binary speaking JSON-RPC over stdio. Layered: shell drivers → transports → connection pool → target manager → MCP tools/resources. SSH uses native multiplexing (golang.org/x/crypto/ssh) + SFTP; Docker uses the Docker client SDK directly.

**Tech Stack:** Go, modelcontextprotocol/go-sdk, golang.org/x/crypto/ssh, github.com/pkg/sftp, github.com/docker/docker/client

**Spec:** `docs/specs/2026-03-27-tramp-design.md`

---

## File Structure

```
tramp/
├── cmd/tramp/main.go                    # Entry point, MCP server bootstrap
├── internal/
│   ├── shell/
│   │   ├── driver.go                    # ShellDriver interface
│   │   ├── bash.go                      # BashDriver: escaping + command generation
│   │   ├── bash_test.go                 # BashDriver tests
│   │   ├── pwsh.go                      # PwshDriver: escaping + command generation
│   │   └── pwsh_test.go                 # PwshDriver tests
│   ├── transport/
│   │   ├── transport.go                 # Transport interface + types
│   │   ├── detect.go                    # Shell/platform/arch detection probes
│   │   ├── detect_test.go              # Detection probe tests
│   │   ├── ssh.go                       # SSH transport (golang.org/x/crypto/ssh + SFTP)
│   │   ├── ssh_test.go                 # SSH transport unit tests
│   │   ├── docker.go                    # Docker transport (Docker client SDK)
│   │   └── docker_test.go              # Docker transport unit tests
│   ├── pool/
│   │   ├── pool.go                      # Connection pool: lazy connect, health, eviction
│   │   └── pool_test.go                # Pool tests with mock transports
│   ├── target/
│   │   ├── manager.go                   # Target state, current target, switching
│   │   ├── manager_test.go             # Manager tests
│   │   ├── config.go                    # tramp.json parsing + merge
│   │   └── config_test.go              # Config tests
│   └── mcp/
│       ├── server.go                    # MCP server setup, tool/resource registration
│       ├── tools_target.go              # target_add, target_switch, target_remove, target_list, target_status
│       ├── tools_target_test.go        # Target tool tests
│       ├── tools_remote.go             # bash, read, write, edit, glob, grep, ls
│       ├── tools_remote_test.go        # Remote tool tests
│       └── resource.go                  # tramp://context dynamic resource
├── test/
│   └── integration/
│       ├── ssh_bash_test.go            # SSH × bash integration
│       ├── ssh_pwsh_test.go            # SSH × pwsh integration
│       ├── docker_bash_test.go         # Docker × bash integration
│       ├── docker_pwsh_test.go         # Docker × pwsh integration
│       └── docker-compose.yml          # Test target containers
├── go.mod
├── go.sum
└── LICENSE
```

---

### Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`, `cmd/tramp/main.go`, `LICENSE`

- [ ] **Step 1: Initialize Go module**

```bash
cd C:/dev/tramp
go mod init github.com/marcfargas/tramp
```

- [ ] **Step 2: Create LICENSE file**

Create `LICENSE` with LGPL-3.0 text. Fetch from https://www.gnu.org/licenses/lgpl-3.0.txt.

- [ ] **Step 3: Create minimal main.go**

```go
// cmd/tramp/main.go
package main

import (
	"context"
	"log"
	"os"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		log.Fatal("Usage: tramp serve")
	}

	ctx := context.Background()
	_ = ctx
	log.Println("tramp MCP server starting...")
}
```

- [ ] **Step 4: Verify it compiles**

```bash
cd C:/dev/tramp
go build ./cmd/tramp
```

Expected: binary created, no errors.

- [ ] **Step 5: Commit**

```bash
git init
git add go.mod cmd/tramp/main.go LICENSE
git commit -m "chore: scaffold tramp Go project"
```

---

### Task 2: Shell Driver Interface + BashDriver

**Files:**
- Create: `internal/shell/driver.go`, `internal/shell/bash.go`, `internal/shell/bash_test.go`

- [ ] **Step 1: Write failing tests for BashDriver escaping**

```go
// internal/shell/bash_test.go
package shell

import "testing"

func TestBashEscape(t *testing.T) {
	d := NewBashDriver()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", "''"},
		{"simple alphanumeric", "hello", "hello"},
		{"path with slashes", "/usr/bin/ls", "/usr/bin/ls"},
		{"safe chars", "a.b-c_d/e=f:g@h", "a.b-c_d/e=f:g@h"},
		{"space needs quoting", "hello world", "'hello world'"},
		{"single quote", "it's", "'it'\"'\"'s'"},
		{"multiple single quotes", "a'b'c", "'a'\"'\"'b'\"'\"'c'"},
		{"special chars", "foo;bar", "'foo;bar'"},
		{"backtick", "foo`bar", "'foo`bar'"},
		{"dollar sign", "$HOME", "'$HOME'"},
		{"newline", "line1\nline2", "'line1\nline2'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.Escape(tt.input)
			if got != tt.expected {
				t.Errorf("Escape(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBashEscapeNullByte(t *testing.T) {
	d := NewBashDriver()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on null byte")
		}
	}()
	d.Escape("hello\x00world")
}

func TestBashStatCommand(t *testing.T) {
	d := NewBashDriver()
	got := d.StatCommand("/home/user/file.txt")
	expected := "if [ -f '/home/user/file.txt' ]; then echo file; elif [ -d '/home/user/file.txt' ]; then echo directory; elif [ -e '/home/user/file.txt' ]; then echo other; else echo missing; fi"
	if got != expected {
		t.Errorf("StatCommand = %q, want %q", got, expected)
	}
}

func TestBashMkdirCommand(t *testing.T) {
	d := NewBashDriver()
	got := d.MkdirCommand("/home/user/new dir")
	expected := "mkdir -p '/home/user/new dir'"
	if got != expected {
		t.Errorf("MkdirCommand = %q, want %q", got, expected)
	}
}

func TestBashCwdWrap(t *testing.T) {
	d := NewBashDriver()
	got := d.CwdWrap("ls -la", "/home/user/project")
	expected := "cd '/home/user/project' && ls -la"
	if got != expected {
		t.Errorf("CwdWrap = %q, want %q", got, expected)
	}
}

func TestBashCwdWrapEmpty(t *testing.T) {
	d := NewBashDriver()
	got := d.CwdWrap("ls -la", "")
	expected := "ls -la"
	if got != expected {
		t.Errorf("CwdWrap with empty cwd = %q, want %q", got, expected)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd C:/dev/tramp
go test ./internal/shell/ -v
```

Expected: compilation errors (types don't exist yet).

- [ ] **Step 3: Implement ShellDriver interface**

```go
// internal/shell/driver.go
package shell

// ShellType represents a remote shell type.
type ShellType string

const (
	ShellBash ShellType = "bash"
	ShellSh   ShellType = "sh"
	ShellPwsh ShellType = "pwsh"
)

// ShellDriver generates shell-safe commands for a specific shell type.
type ShellDriver interface {
	// Type returns the shell type this driver handles.
	Type() ShellType

	// Escape escapes a single argument for safe use in this shell.
	// Panics if the argument contains a null byte.
	Escape(arg string) string

	// StatCommand returns a command that prints "file", "directory", "other", or "missing".
	StatCommand(absolutePath string) string

	// MkdirCommand returns a command that creates a directory (and parents).
	MkdirCommand(absolutePath string) string

	// CwdWrap wraps a command with a cd/Set-Location prefix if cwd is non-empty.
	CwdWrap(command string, cwd string) string
}
```

- [ ] **Step 4: Implement BashDriver**

```go
// internal/shell/bash.go
package shell

import (
	"fmt"
	"regexp"
	"strings"
)

var bashSafeChars = regexp.MustCompile(`^[a-zA-Z0-9._\-/=:@]+$`)

// BashDriver generates commands for bash/sh/dash/zsh/ash shells.
type BashDriver struct{}

// NewBashDriver creates a new BashDriver.
func NewBashDriver() *BashDriver {
	return &BashDriver{}
}

func (d *BashDriver) Type() ShellType {
	return ShellBash
}

// Escape uses POSIX single-quote strategy.
// Embedded single quotes become: end quote, escaped quote in double quotes, restart quote.
// Example: it's → 'it'"'"'s'
func (d *BashDriver) Escape(arg string) string {
	if strings.ContainsRune(arg, 0) {
		panic("shell argument contains null byte — cannot be safely escaped")
	}
	if arg == "" {
		return "''"
	}
	if bashSafeChars.MatchString(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}

func (d *BashDriver) StatCommand(absolutePath string) string {
	escaped := d.Escape(absolutePath)
	return fmt.Sprintf(
		"if [ -f %s ]; then echo file; elif [ -d %s ]; then echo directory; elif [ -e %s ]; then echo other; else echo missing; fi",
		escaped, escaped, escaped,
	)
}

func (d *BashDriver) MkdirCommand(absolutePath string) string {
	return fmt.Sprintf("mkdir -p %s", d.Escape(absolutePath))
}

func (d *BashDriver) CwdWrap(command string, cwd string) string {
	if cwd == "" {
		return command
	}
	return fmt.Sprintf("cd %s && %s", d.Escape(cwd), command)
}

// dirname returns the parent directory of a POSIX path.
func bashDirname(path string) string {
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash <= 0 {
		return "/"
	}
	return path[:lastSlash]
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd C:/dev/tramp
go test ./internal/shell/ -v -run TestBash
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/shell/
git commit -m "feat: add ShellDriver interface and BashDriver"
```

---

### Task 3: PwshDriver

**Files:**
- Create: `internal/shell/pwsh.go`, `internal/shell/pwsh_test.go`

- [ ] **Step 1: Write failing tests for PwshDriver**

```go
// internal/shell/pwsh_test.go
package shell

import "testing"

func TestPwshEscape(t *testing.T) {
	d := NewPwshDriver()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", "''"},
		{"simple alphanumeric", "hello", "hello"},
		{"path with forward slashes", "/usr/bin/ls", "/usr/bin/ls"},
		{"path with backslashes", `C:\Users\marc`, `C:\Users\marc`},
		{"safe chars", "a.b-c_d/e=f:g@h", "a.b-c_d/e=f:g@h"},
		{"space needs quoting", "hello world", "'hello world'"},
		{"single quote doubled", "it's", "'it''s'"},
		{"multiple single quotes", "a'b'c", "'a''b''c'"},
		{"special chars", "foo;bar", "'foo;bar'"},
		{"dollar sign", "$HOME", "'$HOME'"},
		{"backtick", "foo`bar", "'foo`bar'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.Escape(tt.input)
			if got != tt.expected {
				t.Errorf("Escape(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPwshEscapeNullByte(t *testing.T) {
	d := NewPwshDriver()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on null byte")
		}
	}()
	d.Escape("hello\x00world")
}

func TestPwshForceQuote(t *testing.T) {
	d := NewPwshDriver()
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "'simple'"},
		{"it's", "'it''s'"},
		{"/usr/bin/ls", "'/usr/bin/ls'"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := d.ForceQuote(tt.input)
			if got != tt.expected {
				t.Errorf("ForceQuote(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestPwshStatCommand(t *testing.T) {
	d := NewPwshDriver()
	got := d.StatCommand(`C:\Users\file.txt`)
	expected := `if (Test-Path -PathType Leaf C:\Users\file.txt) { 'file' } elseif (Test-Path -PathType Container C:\Users\file.txt) { 'directory' } elseif (Test-Path C:\Users\file.txt) { 'other' } else { 'missing' }`
	if got != expected {
		t.Errorf("StatCommand = %q, want %q", got, expected)
	}
}

func TestPwshMkdirCommand(t *testing.T) {
	d := NewPwshDriver()
	got := d.MkdirCommand("/home/user/new dir")
	expected := "New-Item -ItemType Directory -Force -Path '/home/user/new dir' | Out-Null"
	if got != expected {
		t.Errorf("MkdirCommand = %q, want %q", got, expected)
	}
}

func TestPwshCwdWrap(t *testing.T) {
	d := NewPwshDriver()
	got := d.CwdWrap("Get-ChildItem", `C:\Users\project`)
	expected := `Set-Location C:\Users\project; Get-ChildItem`
	if got != expected {
		t.Errorf("CwdWrap = %q, want %q", got, expected)
	}
}

func TestPwshCwdWrapEmpty(t *testing.T) {
	d := NewPwshDriver()
	got := d.CwdWrap("Get-ChildItem", "")
	expected := "Get-ChildItem"
	if got != expected {
		t.Errorf("CwdWrap with empty cwd = %q, want %q", got, expected)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd C:/dev/tramp
go test ./internal/shell/ -v -run TestPwsh
```

Expected: compilation errors.

- [ ] **Step 3: Implement PwshDriver**

```go
// internal/shell/pwsh.go
package shell

import (
	"fmt"
	"regexp"
	"strings"
)

var pwshSafeChars = regexp.MustCompile(`^[a-zA-Z0-9._\-/\\=:@]+$`)

// PwshDriver generates commands for PowerShell (pwsh) shells.
type PwshDriver struct{}

// NewPwshDriver creates a new PwshDriver.
func NewPwshDriver() *PwshDriver {
	return &PwshDriver{}
}

func (d *PwshDriver) Type() ShellType {
	return ShellPwsh
}

// Escape uses PowerShell single-quote strategy.
// Embedded single quotes are doubled: it's → 'it''s'
func (d *PwshDriver) Escape(arg string) string {
	if strings.ContainsRune(arg, 0) {
		panic("shell argument contains null byte — cannot be safely escaped")
	}
	if arg == "" {
		return "''"
	}
	if pwshSafeChars.MatchString(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "''") + "'"
}

// ForceQuote always wraps in single quotes, even for safe strings.
// Required for .NET method arguments where unquoted / is parsed as division.
func (d *PwshDriver) ForceQuote(arg string) string {
	return "'" + strings.ReplaceAll(arg, "'", "''") + "'"
}

func (d *PwshDriver) StatCommand(absolutePath string) string {
	escaped := d.Escape(absolutePath)
	return fmt.Sprintf(
		"if (Test-Path -PathType Leaf %s) { 'file' } elseif (Test-Path -PathType Container %s) { 'directory' } elseif (Test-Path %s) { 'other' } else { 'missing' }",
		escaped, escaped, escaped,
	)
}

func (d *PwshDriver) MkdirCommand(absolutePath string) string {
	return fmt.Sprintf("New-Item -ItemType Directory -Force -Path %s | Out-Null", d.Escape(absolutePath))
}

func (d *PwshDriver) CwdWrap(command string, cwd string) string {
	if cwd == "" {
		return command
	}
	return fmt.Sprintf("Set-Location %s; %s", d.Escape(cwd), command)
}

// SessionSetup returns PowerShell commands to run at connection init.
func (d *PwshDriver) SessionSetup() string {
	return "$PSStyle.OutputRendering = 'PlainText'; $ErrorActionPreference = 'Continue'; $ProgressPreference = 'SilentlyContinue'"
}

// pwshDirname returns the parent directory of a path (handles / and \).
func pwshDirname(path string) string {
	lastFwd := strings.LastIndex(path, "/")
	lastBack := strings.LastIndex(path, "\\")
	lastSep := lastFwd
	if lastBack > lastSep {
		lastSep = lastBack
	}
	if lastSep <= 0 {
		if strings.HasPrefix(path, "/") {
			return "/"
		}
		return "."
	}
	// Handle Windows drive root: C:\
	if lastSep == 2 && len(path) > 1 && path[1] == ':' {
		return path[:3]
	}
	return path[:lastSep]
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd C:/dev/tramp
go test ./internal/shell/ -v -run TestPwsh
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/shell/pwsh.go internal/shell/pwsh_test.go
git commit -m "feat: add PwshDriver with PowerShell escaping"
```

---

### Task 4: Transport Interface + Detection Probes

**Files:**
- Create: `internal/transport/transport.go`, `internal/transport/detect.go`, `internal/transport/detect_test.go`

- [ ] **Step 1: Write failing tests for detection probes**

```go
// internal/transport/detect_test.go
package transport

import "testing"

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no ansi", "hello", "hello"},
		{"color code", "\x1b[31mhello\x1b[0m", "hello"},
		{"cursor move", "\x1b[2Jhello", "hello"},
		{"osc sequence", "\x1b]0;title\x07hello", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripAnsi(tt.input)
			if got != tt.expected {
				t.Errorf("StripAnsi(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseShellName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"bash", "bash"},
		{"/bin/bash", "bash"},
		{"-bash", "bash"},
		{"sh", "sh"},
		{"/bin/sh", "sh"},
		{"dash", "sh"},
		{"ash", "sh"},
		{"/bin/ash", "sh"},
		{"zsh", "bash"},
		{"/bin/zsh", "bash"},
		{"-zsh", "bash"},
		{"pwsh", "pwsh"},
		{"powershell", "pwsh"},
		{"cmd", "cmd"},
		{"cmd.exe", "cmd"},
		{"unknown_shell", "unknown"},
		{"\x1b[31mbash\x1b[0m", "bash"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseShellName(tt.input)
			if got != tt.expected {
				t.Errorf("ParseShellName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Linux", "linux"},
		{"Darwin", "darwin"},
		{"MINGW64_NT-10.0", "windows"},
		{"MSYS_NT-10.0", "windows"},
		{"CYGWIN_NT-10.0", "windows"},
		{"Windows_NT", "windows"},
		{"FreeBSD", "unknown"},
		{"\x1b[0mLinux", "linux"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParsePlatform(tt.input)
			if got != tt.expected {
				t.Errorf("ParsePlatform(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseArch(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"x86_64", "x86_64"},
		{"aarch64", "aarch64"},
		{"arm64", "aarch64"},
		{"armv7l", "armv7l"},
		{"i686", "i686"},
		{"", "unknown"},
		{"\x1b[0mx86_64", "x86_64"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseArch(tt.input)
			if got != tt.expected {
				t.Errorf("ParseArch(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParsePwshVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		ok       bool
	}{
		{"7", 7, true},
		{"5", 5, true},
		{"not a number", 0, false},
		{"", 0, false},
		{"\x1b[0m7", 7, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParsePwshVersion(tt.input)
			if ok != tt.ok || (ok && got != tt.expected) {
				t.Errorf("ParsePwshVersion(%q) = (%d, %v), want (%d, %v)",
					tt.input, got, ok, tt.expected, tt.ok)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd C:/dev/tramp
go test ./internal/transport/ -v
```

Expected: compilation errors.

- [ ] **Step 3: Implement Transport interface**

```go
// internal/transport/transport.go
package transport

import (
	"context"
	"io"
)

// TransportType identifies the transport mechanism.
type TransportType string

const (
	TransportSSH    TransportType = "ssh"
	TransportDocker TransportType = "docker"
)

// TransportState represents the connection state.
type TransportState string

const (
	StateConnecting   TransportState = "connecting"
	StateConnected    TransportState = "connected"
	StateDisconnected TransportState = "disconnected"
	StateError        TransportState = "error"
)

// ExecResult holds the result of a remote command execution.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ExecOptions configures command execution.
type ExecOptions struct {
	// Timeout in milliseconds. 0 means no timeout.
	Timeout int
	// Stdin to feed to the command.
	Stdin io.Reader
}

// RemoteInfo holds detected information about the remote environment.
type RemoteInfo struct {
	Shell    string // "bash", "sh", "pwsh"
	Platform string // "linux", "darwin", "windows"
	Arch     string // "x86_64", "aarch64", etc.
	Homedir  string // Remote home directory
}

// Transport defines the interface for remote execution backends.
type Transport interface {
	// Type returns the transport type (ssh, docker).
	Type() TransportType

	// State returns the current connection state.
	State() TransportState

	// Connect establishes the connection to the remote target.
	Connect(ctx context.Context) error

	// Close terminates the connection.
	Close() error

	// Exec executes a command on the remote target.
	Exec(ctx context.Context, command string, opts *ExecOptions) (*ExecResult, error)

	// ReadFile reads a file from the remote target.
	ReadFile(ctx context.Context, path string) ([]byte, error)

	// WriteFile writes a file on the remote target.
	WriteFile(ctx context.Context, path string, content []byte) error

	// Info returns detected remote environment info. Only valid after Connect.
	Info() *RemoteInfo

	// HealthCheck verifies the connection is still alive.
	HealthCheck(ctx context.Context) error
}
```

- [ ] **Step 4: Implement detection probes**

```go
// internal/transport/detect.go
package transport

import (
	"regexp"
	"strconv"
	"strings"
)

var ansiPattern = regexp.MustCompile(`\x1b[[(][^\x1b]*?[a-zA-Z]|\x1b\][^\x07]*\x07`)

// StripAnsi removes ANSI escape sequences from a string.
func StripAnsi(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// ParseShellName detects shell type from `echo "$0"` output.
func ParseShellName(output string) string {
	cleaned := strings.ToLower(strings.TrimSpace(StripAnsi(output)))
	// Strip leading - (login shell indicator)
	cleaned = strings.TrimPrefix(cleaned, "-")
	// Extract basename
	parts := strings.FieldsFunc(cleaned, func(r rune) bool { return r == '/' || r == '\\' })
	basename := cleaned
	if len(parts) > 0 {
		basename = parts[len(parts)-1]
	}
	// Strip .exe suffix
	basename = strings.TrimSuffix(basename, ".exe")

	switch basename {
	case "bash":
		return "bash"
	case "sh", "dash", "ash":
		return "sh"
	case "zsh":
		return "bash" // treat as bash-compatible
	case "pwsh", "powershell":
		return "pwsh"
	case "cmd":
		return "cmd"
	default:
		return "unknown"
	}
}

// ParsePlatform detects platform from `uname -s` output.
func ParsePlatform(output string) string {
	cleaned := strings.TrimSpace(StripAnsi(output))

	switch {
	case cleaned == "Linux":
		return "linux"
	case cleaned == "Darwin":
		return "darwin"
	case strings.HasPrefix(cleaned, "MINGW"),
		strings.HasPrefix(cleaned, "MSYS"),
		strings.HasPrefix(cleaned, "CYGWIN"),
		strings.HasPrefix(cleaned, "Windows_NT"),
		strings.HasPrefix(cleaned, "Windows"),
		strings.EqualFold(cleaned, "windows"):
		return "windows"
	default:
		return "unknown"
	}
}

// ParseArch detects architecture from `uname -m` output.
func ParseArch(output string) string {
	cleaned := strings.ToLower(strings.TrimSpace(StripAnsi(output)))
	if len(cleaned) > 64 || cleaned == "" {
		return "unknown"
	}
	// Normalize macOS arm64 → aarch64
	if cleaned == "arm64" {
		return "aarch64"
	}
	return cleaned
}

// ParsePwshVersion detects PowerShell from `$PSVersionTable.PSVersion.Major` output.
func ParsePwshVersion(output string) (int, bool) {
	cleaned := strings.TrimSpace(StripAnsi(output))
	v, err := strconv.Atoi(cleaned)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// PwshPlatformCommand returns a PowerShell command to detect platform.
func PwshPlatformCommand() string {
	return "if ($IsLinux) { 'linux' } elseif ($IsMacOS) { 'darwin' } else { 'windows' }"
}

// PwshArchCommand returns a PowerShell command to detect architecture.
func PwshArchCommand() string {
	return "[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture"
}

// ParsePwshArch normalizes PowerShell architecture output.
func ParsePwshArch(output string) string {
	cleaned := strings.TrimSpace(StripAnsi(output))
	switch cleaned {
	case "X64":
		return "x86_64"
	case "Arm64":
		return "aarch64"
	case "X86":
		return "x86"
	case "Arm":
		return "arm"
	default:
		return "unknown"
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd C:/dev/tramp
go test ./internal/transport/ -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/transport/
git commit -m "feat: add Transport interface and detection probes"
```

---

### Task 5: Target Manager + Config

**Files:**
- Create: `internal/target/manager.go`, `internal/target/manager_test.go`, `internal/target/config.go`, `internal/target/config_test.go`

- [ ] **Step 1: Write failing tests for config parsing**

```go
// internal/target/config_test.go
package target

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfig(t *testing.T) {
	content := `{
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
				"container": "my-container",
				"cwd": "/workspace"
			}
		}
	}`

	cfg, err := ParseConfig([]byte(content))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	if cfg.Default != "dev" {
		t.Errorf("Default = %q, want %q", cfg.Default, "dev")
	}
	if len(cfg.Targets) != 2 {
		t.Errorf("len(Targets) = %d, want 2", len(cfg.Targets))
	}

	dev := cfg.Targets["dev"]
	if dev.Type != "ssh" {
		t.Errorf("dev.Type = %q, want %q", dev.Type, "ssh")
	}
	if dev.Host != "user@dev.example.com" {
		t.Errorf("dev.Host = %q, want %q", dev.Host, "user@dev.example.com")
	}
}

func TestParseConfigRejectsLocalName(t *testing.T) {
	content := `{
		"targets": {
			"local": {
				"type": "ssh",
				"host": "user@host",
				"shell": "bash"
			}
		}
	}`
	_, err := ParseConfig([]byte(content))
	if err == nil {
		t.Error("expected error for reserved name 'local'")
	}
}

func TestMergeConfigs(t *testing.T) {
	global := &Config{
		Default: "global-default",
		Targets: map[string]TargetConfig{
			"shared": {Type: "ssh", Host: "global@host", Shell: "bash"},
			"only-global": {Type: "ssh", Host: "global@host2", Shell: "bash"},
		},
	}
	project := &Config{
		Default: "project-default",
		Targets: map[string]TargetConfig{
			"shared": {Type: "ssh", Host: "project@host", Shell: "bash"},
		},
	}

	merged := MergeConfigs(global, project)
	if merged.Default != "project-default" {
		t.Errorf("Default = %q, want %q", merged.Default, "project-default")
	}
	if merged.Targets["shared"].Host != "project@host" {
		t.Error("project should override global for same target name")
	}
	if _, ok := merged.Targets["only-global"]; !ok {
		t.Error("global-only target should be preserved")
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tramp.json")
	content := `{"targets": {"test": {"type": "docker", "container": "c1"}}}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFromFile failed: %v", err)
	}
	if cfg.Targets["test"].Container != "c1" {
		t.Error("expected container c1")
	}
}

func TestLoadConfigFromFileMissing(t *testing.T) {
	cfg, err := LoadConfigFromFile("/nonexistent/tramp.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected empty config, got nil")
	}
	if len(cfg.Targets) != 0 {
		t.Error("expected empty targets for missing file")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd C:/dev/tramp
go test ./internal/target/ -v -run TestParse -run TestMerge -run TestLoad
```

Expected: compilation errors.

- [ ] **Step 3: Implement config types and parsing**

```go
// internal/target/config.go
package target

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// TargetConfig holds the configuration for a single target.
type TargetConfig struct {
	Type         string `json:"type"`                    // "ssh" or "docker"
	Host         string `json:"host,omitempty"`          // SSH: user@hostname
	Port         int    `json:"port,omitempty"`          // SSH: port (default 22)
	IdentityFile string `json:"identityFile,omitempty"`  // SSH: path to key
	Container    string `json:"container,omitempty"`     // Docker: container name/ID
	Cwd          string `json:"cwd,omitempty"`           // Remote working directory
	Shell        string `json:"shell,omitempty"`         // "bash" or "pwsh" (optional for docker)
	Timeout      int    `json:"timeout,omitempty"`       // Connection timeout in ms
}

// Config holds the full tramp configuration from a tramp.json file.
type Config struct {
	Default string                  `json:"default,omitempty"`
	Targets map[string]TargetConfig `json:"targets"`
}

// ParseConfig parses a tramp.json file's contents.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid tramp.json: %w", err)
	}
	if cfg.Targets == nil {
		cfg.Targets = make(map[string]TargetConfig)
	}

	// Validate reserved name
	if _, ok := cfg.Targets["local"]; ok {
		return nil, errors.New(`target name "local" is reserved`)
	}

	// Validate target types
	for name, tc := range cfg.Targets {
		switch tc.Type {
		case "ssh":
			if tc.Host == "" {
				return nil, fmt.Errorf("target %q: ssh target requires host", name)
			}
		case "docker":
			if tc.Container == "" {
				return nil, fmt.Errorf("target %q: docker target requires container", name)
			}
		default:
			return nil, fmt.Errorf("target %q: unknown type %q (expected ssh or docker)", name, tc.Type)
		}
	}

	return &cfg, nil
}

// MergeConfigs merges global and project configs. Project overrides global.
func MergeConfigs(global, project *Config) *Config {
	merged := &Config{
		Targets: make(map[string]TargetConfig),
	}

	// Start with global
	if global != nil {
		merged.Default = global.Default
		for k, v := range global.Targets {
			merged.Targets[k] = v
		}
	}

	// Project overrides
	if project != nil {
		if project.Default != "" {
			merged.Default = project.Default
		}
		for k, v := range project.Targets {
			merged.Targets[k] = v
		}
	}

	return merged
}

// LoadConfigFromFile loads config from a file path. Returns empty config if file doesn't exist.
func LoadConfigFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Targets: make(map[string]TargetConfig)}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return ParseConfig(data)
}
```

- [ ] **Step 4: Run config tests to verify they pass**

```bash
cd C:/dev/tramp
go test ./internal/target/ -v -run "TestParse|TestMerge|TestLoad"
```

Expected: all PASS.

- [ ] **Step 5: Write failing tests for target manager**

```go
// internal/target/manager_test.go
package target

import (
	"testing"
)

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

func TestManagerAddRejectsLocal(t *testing.T) {
	m := NewManager()
	err := m.Add("local", TargetConfig{Type: "ssh", Host: "h", Shell: "bash"})
	if err == nil {
		t.Error("expected error for reserved name 'local'")
	}
}

func TestManagerAddRejectsDuplicate(t *testing.T) {
	m := NewManager()
	cfg := TargetConfig{Type: "ssh", Host: "h", Shell: "bash"}
	m.Add("dev", cfg)
	err := m.Add("dev", cfg)
	if err == nil {
		t.Error("expected error for duplicate name")
	}
}

func TestManagerSwitch(t *testing.T) {
	m := NewManager()
	m.Add("dev", TargetConfig{Type: "ssh", Host: "h", Shell: "bash"})

	if err := m.Switch("dev"); err != nil {
		t.Fatalf("Switch failed: %v", err)
	}
	if m.CurrentName() != "dev" {
		t.Errorf("CurrentName = %q, want %q", m.CurrentName(), "dev")
	}
}

func TestManagerSwitchLocal(t *testing.T) {
	m := NewManager()
	m.Add("dev", TargetConfig{Type: "ssh", Host: "h", Shell: "bash"})
	m.Switch("dev")

	if err := m.Switch("local"); err != nil {
		t.Fatalf("Switch to local failed: %v", err)
	}
	if m.CurrentName() != "" {
		t.Errorf("CurrentName after local = %q, want empty", m.CurrentName())
	}
	if m.Current() != nil {
		t.Error("Current after local should be nil")
	}
}

func TestManagerSwitchNonexistent(t *testing.T) {
	m := NewManager()
	err := m.Switch("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent target")
	}
}

func TestManagerRemove(t *testing.T) {
	m := NewManager()
	m.Add("dev", TargetConfig{Type: "ssh", Host: "h", Shell: "bash"})
	m.Switch("dev")

	if err := m.Remove("dev"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if m.CurrentName() != "" {
		t.Error("current should be cleared after removing active target")
	}
	if len(m.List()) != 0 {
		t.Error("target should be removed from list")
	}
}

func TestManagerGet(t *testing.T) {
	m := NewManager()
	m.Add("dev", TargetConfig{Type: "docker", Container: "c1"})

	tgt := m.Get("dev")
	if tgt == nil {
		t.Fatal("Get returned nil")
	}
	if tgt.Config.Container != "c1" {
		t.Errorf("Container = %q, want %q", tgt.Config.Container, "c1")
	}
}

func TestManagerLoadConfig(t *testing.T) {
	m := NewManager()
	cfg := &Config{
		Default: "staging",
		Targets: map[string]TargetConfig{
			"staging": {Type: "ssh", Host: "user@staging", Shell: "bash"},
		},
	}
	m.LoadFromConfig(cfg)

	targets := m.List()
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Name != "staging" {
		t.Errorf("Name = %q, want %q", targets[0].Name, "staging")
	}
}
```

- [ ] **Step 6: Run manager tests to verify they fail**

```bash
cd C:/dev/tramp
go test ./internal/target/ -v -run TestManager
```

Expected: compilation errors.

- [ ] **Step 7: Implement target manager**

```go
// internal/target/manager.go
package target

import (
	"errors"
	"fmt"
	"sync"
)

// Target represents a named target with its configuration.
type Target struct {
	Name      string
	Config    TargetConfig
	IsDynamic bool // true = added at runtime, not from config file
}

// Manager manages targets and tracks the current active target.
// Thread-safe via mutex.
type Manager struct {
	mu      sync.RWMutex
	targets map[string]*Target
	current string // empty means no active target ("local")
}

// NewManager creates a new target manager.
func NewManager() *Manager {
	return &Manager{
		targets: make(map[string]*Target),
	}
}

// Add adds a dynamic target. Returns error if name is reserved or duplicate.
func (m *Manager) Add(name string, config TargetConfig) error {
	if name == "local" {
		return errors.New(`target name "local" is reserved`)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.targets[name]; exists {
		return fmt.Errorf("target %q already exists", name)
	}

	m.targets[name] = &Target{
		Name:      name,
		Config:    config,
		IsDynamic: true,
	}
	return nil
}

// Remove removes a target. If it's the current target, clears current.
func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.targets[name]; !exists {
		return fmt.Errorf("target %q not found", name)
	}

	delete(m.targets, name)
	if m.current == name {
		m.current = ""
	}
	return nil
}

// Switch sets the current target. "local" clears the current target.
func (m *Manager) Switch(name string) error {
	if name == "local" {
		m.mu.Lock()
		m.current = ""
		m.mu.Unlock()
		return nil
	}

	m.mu.RLock()
	_, exists := m.targets[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("target %q not found", name)
	}

	m.mu.Lock()
	m.current = name
	m.mu.Unlock()
	return nil
}

// CurrentName returns the current target name, or empty if local.
func (m *Manager) CurrentName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Current returns the current target, or nil if local.
func (m *Manager) Current() *Target {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current == "" {
		return nil
	}
	return m.targets[m.current]
}

// Get returns a target by name, or nil.
func (m *Manager) Get(name string) *Target {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.targets[name]
}

// List returns all targets.
func (m *Manager) List() []Target {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Target, 0, len(m.targets))
	for _, t := range m.targets {
		result = append(result, *t)
	}
	return result
}

// LoadFromConfig populates targets from a parsed config. Existing dynamic targets are preserved.
func (m *Manager) LoadFromConfig(cfg *Config) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, tc := range cfg.Targets {
		m.targets[name] = &Target{
			Name:      name,
			Config:    tc,
			IsDynamic: false,
		}
	}
}
```

- [ ] **Step 8: Run all target tests to verify they pass**

```bash
cd C:/dev/tramp
go test ./internal/target/ -v
```

Expected: all PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/target/
git commit -m "feat: add target manager and config parsing"
```

---

### Task 6: Connection Pool

**Files:**
- Create: `internal/pool/pool.go`, `internal/pool/pool_test.go`

- [ ] **Step 1: Write failing tests for connection pool**

```go
// internal/pool/pool_test.go
package pool

import (
	"context"
	"errors"
	"testing"

	"github.com/marcfargas/tramp/internal/target"
	"github.com/marcfargas/tramp/internal/transport"
)

// mockTransport implements Transport for testing.
type mockTransport struct {
	state       transport.TransportState
	connectErr  error
	healthErr   error
	connectCount int
	closeCount   int
}

func (m *mockTransport) Type() transport.TransportType      { return transport.TransportSSH }
func (m *mockTransport) State() transport.TransportState     { return m.state }
func (m *mockTransport) Connect(ctx context.Context) error {
	m.connectCount++
	if m.connectErr != nil {
		return m.connectErr
	}
	m.state = transport.StateConnected
	return nil
}
func (m *mockTransport) Close() error {
	m.closeCount++
	m.state = transport.StateDisconnected
	return nil
}
func (m *mockTransport) Exec(ctx context.Context, cmd string, opts *transport.ExecOptions) (*transport.ExecResult, error) {
	return &transport.ExecResult{Stdout: "ok", ExitCode: 0}, nil
}
func (m *mockTransport) ReadFile(ctx context.Context, path string) ([]byte, error)           { return nil, nil }
func (m *mockTransport) WriteFile(ctx context.Context, path string, content []byte) error     { return nil }
func (m *mockTransport) Info() *transport.RemoteInfo { return &transport.RemoteInfo{Shell: "bash"} }
func (m *mockTransport) HealthCheck(ctx context.Context) error { return m.healthErr }

func TestPoolGetConnection(t *testing.T) {
	mock := &mockTransport{}
	factory := func(cfg target.TargetConfig) (transport.Transport, error) {
		return mock, nil
	}

	p := New(factory)
	cfg := target.TargetConfig{Type: "ssh", Host: "h", Shell: "bash"}

	conn, err := p.Get(context.Background(), "dev", cfg)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if conn != mock {
		t.Error("expected mock transport")
	}
	if mock.connectCount != 1 {
		t.Errorf("connectCount = %d, want 1", mock.connectCount)
	}
}

func TestPoolCachesConnection(t *testing.T) {
	mock := &mockTransport{}
	factory := func(cfg target.TargetConfig) (transport.Transport, error) {
		return mock, nil
	}

	p := New(factory)
	cfg := target.TargetConfig{Type: "ssh", Host: "h", Shell: "bash"}

	p.Get(context.Background(), "dev", cfg)
	p.Get(context.Background(), "dev", cfg)

	if mock.connectCount != 1 {
		t.Errorf("connectCount = %d, want 1 (should cache)", mock.connectCount)
	}
}

func TestPoolEvictsUnhealthy(t *testing.T) {
	callCount := 0
	factory := func(cfg target.TargetConfig) (transport.Transport, error) {
		callCount++
		m := &mockTransport{}
		if callCount == 1 {
			m.healthErr = errors.New("dead")
		}
		return m, nil
	}

	p := New(factory)
	cfg := target.TargetConfig{Type: "ssh", Host: "h", Shell: "bash"}

	// First connection
	p.Get(context.Background(), "dev", cfg)
	// Second call — health check fails, should create new
	p.Get(context.Background(), "dev", cfg)

	if callCount != 2 {
		t.Errorf("factory called %d times, want 2 (evict + reconnect)", callCount)
	}
}

func TestPoolCloseAll(t *testing.T) {
	mock := &mockTransport{}
	factory := func(cfg target.TargetConfig) (transport.Transport, error) {
		return mock, nil
	}

	p := New(factory)
	cfg := target.TargetConfig{Type: "ssh", Host: "h", Shell: "bash"}
	p.Get(context.Background(), "dev", cfg)

	p.CloseAll()

	if mock.closeCount != 1 {
		t.Errorf("closeCount = %d, want 1", mock.closeCount)
	}
}

func TestPoolConnectError(t *testing.T) {
	factory := func(cfg target.TargetConfig) (transport.Transport, error) {
		return &mockTransport{connectErr: errors.New("refused")}, nil
	}

	p := New(factory)
	cfg := target.TargetConfig{Type: "ssh", Host: "h", Shell: "bash"}

	_, err := p.Get(context.Background(), "dev", cfg)
	if err == nil {
		t.Error("expected error on connection failure")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd C:/dev/tramp
go test ./internal/pool/ -v
```

Expected: compilation errors.

- [ ] **Step 3: Implement connection pool**

```go
// internal/pool/pool.go
package pool

import (
	"context"
	"fmt"
	"sync"

	"github.com/marcfargas/tramp/internal/target"
	"github.com/marcfargas/tramp/internal/transport"
)

// TransportFactory creates a new transport for a given target config.
type TransportFactory func(cfg target.TargetConfig) (transport.Transport, error)

// Pool manages cached connections to remote targets.
type Pool struct {
	mu          sync.Mutex
	connections map[string]transport.Transport
	connecting  map[string]chan struct{} // prevents duplicate connect attempts
	factory     TransportFactory
}

// New creates a new connection pool with the given transport factory.
func New(factory TransportFactory) *Pool {
	return &Pool{
		connections: make(map[string]transport.Transport),
		connecting:  make(map[string]chan struct{}),
		factory:     factory,
	}
}

// Get returns a healthy connection for the given target, creating one if needed.
func (p *Pool) Get(ctx context.Context, name string, cfg target.TargetConfig) (transport.Transport, error) {
	p.mu.Lock()

	// Check cache
	if conn, ok := p.connections[name]; ok {
		p.mu.Unlock()

		// Health check
		if err := conn.HealthCheck(ctx); err == nil {
			return conn, nil
		}

		// Unhealthy — evict and reconnect
		p.mu.Lock()
		if existing, ok := p.connections[name]; ok && existing == conn {
			delete(p.connections, name)
			conn.Close()
		}
		p.mu.Unlock()

		return p.connect(ctx, name, cfg)
	}

	p.mu.Unlock()
	return p.connect(ctx, name, cfg)
}

// connect creates and caches a new connection.
func (p *Pool) connect(ctx context.Context, name string, cfg target.TargetConfig) (transport.Transport, error) {
	conn, err := p.factory(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating transport for %q: %w", name, err)
	}

	if err := conn.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connecting to %q: %w", name, err)
	}

	p.mu.Lock()
	p.connections[name] = conn
	p.mu.Unlock()

	return conn, nil
}

// Close closes a specific connection.
func (p *Pool) Close(name string) {
	p.mu.Lock()
	conn, ok := p.connections[name]
	if ok {
		delete(p.connections, name)
	}
	p.mu.Unlock()

	if ok {
		conn.Close()
	}
}

// CloseAll closes all connections.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	conns := make(map[string]transport.Transport, len(p.connections))
	for k, v := range p.connections {
		conns[k] = v
	}
	p.connections = make(map[string]transport.Transport)
	p.mu.Unlock()

	for _, conn := range conns {
		conn.Close()
	}
}

// Status returns the connection state for all cached connections.
func (p *Pool) Status() map[string]transport.TransportState {
	p.mu.Lock()
	defer p.mu.Unlock()

	status := make(map[string]transport.TransportState, len(p.connections))
	for name, conn := range p.connections {
		status[name] = conn.State()
	}
	return status
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd C:/dev/tramp
go test ./internal/pool/ -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pool/
git commit -m "feat: add connection pool with lazy connect and health eviction"
```

---

### Task 7: SSH Transport

**Files:**
- Create: `internal/transport/ssh.go`, `internal/transport/ssh_test.go`

This task requires adding the SSH and SFTP dependencies.

- [ ] **Step 1: Add dependencies**

```bash
cd C:/dev/tramp
go get golang.org/x/crypto/ssh
go get golang.org/x/crypto/ssh/agent
go get github.com/pkg/sftp
```

- [ ] **Step 2: Write failing test for SSH transport construction**

```go
// internal/transport/ssh_test.go
package transport

import (
	"testing"

	"github.com/marcfargas/tramp/internal/target"
)

func TestNewSSHTransport(t *testing.T) {
	cfg := target.TargetConfig{
		Type:  "ssh",
		Host:  "user@localhost",
		Port:  22,
		Shell: "bash",
		Cwd:   "/home/user",
	}

	tr := NewSSHTransport(cfg)
	if tr == nil {
		t.Fatal("NewSSHTransport returned nil")
	}
	if tr.Type() != TransportSSH {
		t.Errorf("Type = %q, want %q", tr.Type(), TransportSSH)
	}
	if tr.State() != StateDisconnected {
		t.Errorf("State = %q, want %q", tr.State(), StateDisconnected)
	}
}

func TestSSHParseHost(t *testing.T) {
	tests := []struct {
		input    string
		wantUser string
		wantHost string
	}{
		{"user@host.example.com", "user", "host.example.com"},
		{"root@192.168.1.1", "root", "192.168.1.1"},
		{"host.example.com", "", "host.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			user, host := parseSSHHost(tt.input)
			if user != tt.wantUser {
				t.Errorf("user = %q, want %q", user, tt.wantUser)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
		})
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
cd C:/dev/tramp
go test ./internal/transport/ -v -run TestSSH -run TestNewSSH
```

Expected: compilation errors.

- [ ] **Step 4: Implement SSH transport**

```go
// internal/transport/ssh.go
package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/marcfargas/tramp/internal/shell"
	"github.com/marcfargas/tramp/internal/target"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// SSHTransport implements Transport over SSH using golang.org/x/crypto/ssh.
type SSHTransport struct {
	mu     sync.Mutex
	cfg    target.TargetConfig
	state  TransportState
	client *ssh.Client
	sftp   *sftp.Client
	info   *RemoteInfo
	driver shell.ShellDriver
}

// NewSSHTransport creates a new SSH transport for the given config.
func NewSSHTransport(cfg target.TargetConfig) *SSHTransport {
	return &SSHTransport{
		cfg:   cfg,
		state: StateDisconnected,
	}
}

func (t *SSHTransport) Type() TransportType    { return TransportSSH }
func (t *SSHTransport) State() TransportState   { return t.state }
func (t *SSHTransport) Info() *RemoteInfo        { return t.info }

// Connect establishes the SSH connection, SFTP subsystem, and detects remote environment.
func (t *SSHTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.state = StateConnecting

	user, host := parseSSHHost(t.cfg.Host)
	port := t.cfg.Port
	if port == 0 {
		port = 22
	}

	authMethods, err := buildAuthMethods(t.cfg.IdentityFile)
	if err != nil {
		t.state = StateError
		return fmt.Errorf("building auth methods: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: proper host key verification
		Timeout:         time.Duration(t.cfg.Timeout) * time.Millisecond,
	}
	if sshConfig.Timeout == 0 {
		sshConfig.Timeout = 15 * time.Second
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		t.state = StateError
		return fmt.Errorf("SSH dial %s: %w", addr, err)
	}
	t.client = client

	// Open SFTP subsystem
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		t.state = StateError
		return fmt.Errorf("SFTP subsystem: %w", err)
	}
	t.sftp = sftpClient

	// Select shell driver
	if t.cfg.Shell == "pwsh" {
		t.driver = shell.NewPwshDriver()
	} else {
		t.driver = shell.NewBashDriver()
	}

	// Detect remote environment
	info, err := t.detectRemoteInfo(ctx)
	if err != nil {
		sftpClient.Close()
		client.Close()
		t.state = StateError
		return fmt.Errorf("detecting remote info: %w", err)
	}
	t.info = info

	// Run session setup for PowerShell
	if t.cfg.Shell == "pwsh" {
		pwshDriver := t.driver.(*shell.PwshDriver)
		t.execSimple(ctx, pwshDriver.SessionSetup())
	}

	t.state = StateConnected
	return nil
}

// Close terminates the SSH connection.
func (t *SSHTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.state = StateDisconnected
	var errs []error
	if t.sftp != nil {
		if err := t.sftp.Close(); err != nil {
			errs = append(errs, err)
		}
		t.sftp = nil
	}
	if t.client != nil {
		if err := t.client.Close(); err != nil {
			errs = append(errs, err)
		}
		t.client = nil
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}

// Exec executes a command on the remote host via a new SSH session channel.
func (t *SSHTransport) Exec(ctx context.Context, command string, opts *ExecOptions) (*ExecResult, error) {
	if t.state != StateConnected {
		return nil, fmt.Errorf("not connected")
	}

	// Wrap with cwd if configured
	wrapped := t.driver.CwdWrap(command, t.cfg.Cwd)

	session, err := t.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if opts != nil && opts.Stdin != nil {
		session.Stdin = opts.Stdin
	}

	// Handle timeout
	if opts != nil && opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Millisecond)
		defer cancel()
	}

	// Run with context cancellation
	done := make(chan error, 1)
	go func() {
		done <- session.Run(wrapped)
	}()

	select {
	case <-ctx.Done():
		session.Signal(ssh.SIGKILL)
		return nil, fmt.Errorf("command timed out")
	case err := <-done:
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else {
				return nil, fmt.Errorf("exec error: %w", err)
			}
		}
		return &ExecResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
		}, nil
	}
}

// ReadFile reads a file from the remote via SFTP.
func (t *SSHTransport) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if t.sftp == nil {
		return nil, fmt.Errorf("not connected")
	}

	resolved := t.resolvePath(path)
	f, err := t.sftp.Open(resolved)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", resolved, err)
	}
	defer f.Close()

	return io.ReadAll(f)
}

// WriteFile writes a file on the remote via SFTP.
func (t *SSHTransport) WriteFile(ctx context.Context, path string, content []byte) error {
	if t.sftp == nil {
		return fmt.Errorf("not connected")
	}

	resolved := t.resolvePath(path)

	// Ensure parent directory exists
	dir := filepath.Dir(resolved)
	if err := t.sftp.MkdirAll(dir); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	f, err := t.sftp.Create(resolved)
	if err != nil {
		return fmt.Errorf("create %s: %w", resolved, err)
	}
	defer f.Close()

	_, err = f.Write(content)
	return err
}

// HealthCheck verifies the connection is still alive.
func (t *SSHTransport) HealthCheck(ctx context.Context) error {
	if t.client == nil {
		return fmt.Errorf("not connected")
	}
	// Send a global request that does nothing — if it doesn't error, the connection is alive
	_, _, err := t.client.SendRequest("keepalive@openssh.com", true, nil)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	return nil
}

// detectRemoteInfo probes the remote for shell, platform, arch, and homedir.
func (t *SSHTransport) detectRemoteInfo(ctx context.Context) (*RemoteInfo, error) {
	info := &RemoteInfo{}

	// Shell is configured, not detected for SSH
	info.Shell = t.cfg.Shell
	if info.Shell == "" {
		info.Shell = "bash"
	}

	if info.Shell == "pwsh" {
		// PowerShell detection
		if out, err := t.execSimple(ctx, PwshPlatformCommand()); err == nil {
			info.Platform = strings.TrimSpace(out)
		}
		if out, err := t.execSimple(ctx, PwshArchCommand()); err == nil {
			info.Arch = ParsePwshArch(out)
		}
		if out, err := t.execSimple(ctx, "(Get-Location).Path"); err == nil {
			info.Homedir = strings.TrimSpace(out)
		}
	} else {
		// Bash/POSIX detection
		if out, err := t.execSimple(ctx, "uname -s"); err == nil {
			info.Platform = ParsePlatform(out)
		}
		if out, err := t.execSimple(ctx, "uname -m"); err == nil {
			info.Arch = ParseArch(out)
		}
		if out, err := t.execSimple(ctx, "echo $HOME"); err == nil {
			info.Homedir = strings.TrimSpace(StripAnsi(out))
		}
	}

	return info, nil
}

// execSimple runs a command and returns stdout. Used during setup probes.
func (t *SSHTransport) execSimple(ctx context.Context, command string) (string, error) {
	session, err := t.client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout bytes.Buffer
	session.Stdout = &stdout
	err = session.Run(command)
	return stdout.String(), err
}

// resolvePath resolves a path relative to the configured cwd.
func (t *SSHTransport) resolvePath(path string) string {
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return path
	}
	if t.cfg.Cwd != "" {
		return t.cfg.Cwd + "/" + path
	}
	return path
}

// parseSSHHost splits "user@host" into user and host parts.
func parseSSHHost(hostStr string) (user, host string) {
	if i := strings.Index(hostStr, "@"); i >= 0 {
		return hostStr[:i], hostStr[i+1:]
	}
	return "", hostStr
}

// buildAuthMethods creates SSH auth methods from available sources.
func buildAuthMethods(identityFile string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// 1. Try SSH agent
	if agentConn := sshAgentConn(); agentConn != nil {
		methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(agentConn).Signers))
	}

	// 2. Try identity file
	if identityFile != "" {
		expanded := expandHome(identityFile)
		key, err := os.ReadFile(expanded)
		if err == nil {
			signer, err := ssh.ParsePrivateKey(key)
			if err == nil {
				methods = append(methods, ssh.PublicKeys(signer))
			}
		}
	}

	// 3. Try default key locations
	for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa"} {
		path := filepath.Join(expandHome("~/.ssh"), name)
		key, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			continue
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH auth methods available (no agent, no keys)")
	}
	return methods, nil
}

// sshAgentConn connects to the SSH agent. Returns nil if unavailable.
func sshAgentConn() net.Conn {
	socket := os.Getenv("SSH_AUTH_SOCK")
	if socket == "" {
		return nil
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil
	}
	return conn
}

// expandHome replaces ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd C:/dev/tramp
go test ./internal/transport/ -v -run "TestNewSSH|TestSSHParse"
```

Expected: PASS (unit tests only — integration tests come later).

- [ ] **Step 6: Commit**

```bash
git add internal/transport/ssh.go internal/transport/ssh_test.go go.mod go.sum
git commit -m "feat: add SSH transport with SFTP and native multiplexing"
```

---

### Task 8: Docker Transport

**Files:**
- Create: `internal/transport/docker.go`, `internal/transport/docker_test.go`

- [ ] **Step 1: Add Docker dependency**

```bash
cd C:/dev/tramp
go get github.com/docker/docker/client
go get github.com/docker/docker/api/types
```

- [ ] **Step 2: Write failing test for Docker transport construction**

```go
// internal/transport/docker_test.go
package transport

import (
	"testing"

	"github.com/marcfargas/tramp/internal/target"
)

func TestNewDockerTransport(t *testing.T) {
	cfg := target.TargetConfig{
		Type:      "docker",
		Container: "my-container",
		Cwd:       "/workspace",
		Shell:     "bash",
	}

	tr := NewDockerTransport(cfg)
	if tr == nil {
		t.Fatal("NewDockerTransport returned nil")
	}
	if tr.Type() != TransportDocker {
		t.Errorf("Type = %q, want %q", tr.Type(), TransportDocker)
	}
	if tr.State() != StateDisconnected {
		t.Errorf("State = %q, want %q", tr.State(), StateDisconnected)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
cd C:/dev/tramp
go test ./internal/transport/ -v -run TestNewDocker
```

Expected: compilation error.

- [ ] **Step 4: Implement Docker transport**

```go
// internal/transport/docker.go
package transport

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/marcfargas/tramp/internal/shell"
	"github.com/marcfargas/tramp/internal/target"
)

// DockerTransport implements Transport using the Docker Engine API.
type DockerTransport struct {
	mu     sync.Mutex
	cfg    target.TargetConfig
	state  TransportState
	docker *client.Client
	info   *RemoteInfo
	driver shell.ShellDriver
}

// NewDockerTransport creates a new Docker transport for the given config.
func NewDockerTransport(cfg target.TargetConfig) *DockerTransport {
	return &DockerTransport{
		cfg:   cfg,
		state: StateDisconnected,
	}
}

func (t *DockerTransport) Type() TransportType  { return TransportDocker }
func (t *DockerTransport) State() TransportState { return t.state }
func (t *DockerTransport) Info() *RemoteInfo     { return t.info }

// Connect verifies the container is running and detects the remote environment.
func (t *DockerTransport) Connect(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.state = StateConnecting

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.state = StateError
		return fmt.Errorf("Docker client: %w", err)
	}
	t.docker = cli

	// Verify container is running
	inspect, err := cli.ContainerInspect(ctx, t.cfg.Container)
	if err != nil {
		t.state = StateError
		return fmt.Errorf("container %q: %w", t.cfg.Container, err)
	}
	if !inspect.State.Running {
		t.state = StateError
		return fmt.Errorf("container %q is not running (state: %s)", t.cfg.Container, inspect.State.Status)
	}

	// Select or detect shell
	shellType := t.cfg.Shell
	if shellType == "" {
		shellType, err = t.detectShell(ctx)
		if err != nil {
			shellType = "sh" // fallback
		}
	}

	if shellType == "pwsh" {
		t.driver = shell.NewPwshDriver()
	} else {
		t.driver = shell.NewBashDriver()
	}

	// Detect remote environment
	info, err := t.detectRemoteInfo(ctx, shellType)
	if err != nil {
		t.state = StateError
		return fmt.Errorf("detecting remote info: %w", err)
	}
	t.info = info

	// Run session setup for PowerShell
	if shellType == "pwsh" {
		pwshDriver := t.driver.(*shell.PwshDriver)
		t.execRaw(ctx, "pwsh", []string{"-NoProfile", "-NonInteractive", "-Command", pwshDriver.SessionSetup()})
	}

	t.state = StateConnected
	return nil
}

// Close closes the Docker client.
func (t *DockerTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.state = StateDisconnected
	if t.docker != nil {
		err := t.docker.Close()
		t.docker = nil
		return err
	}
	return nil
}

// Exec executes a command in the container via the Docker Exec API.
func (t *DockerTransport) Exec(ctx context.Context, command string, opts *ExecOptions) (*ExecResult, error) {
	if t.state != StateConnected {
		return nil, fmt.Errorf("not connected")
	}

	wrapped := t.driver.CwdWrap(command, t.cfg.Cwd)

	var shellCmd []string
	if t.driver.Type() == shell.ShellPwsh {
		shellCmd = []string{"pwsh", "-NoProfile", "-NonInteractive", "-Command", wrapped}
	} else {
		shellCmd = []string{"sh", "-c", wrapped}
	}

	// Handle timeout
	if opts != nil && opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(opts.Timeout)*time.Millisecond)
		defer cancel()
	}

	return t.execRaw(ctx, shellCmd[0], shellCmd[1:])
}

// ReadFile reads a file from the container using CopyFromContainer.
func (t *DockerTransport) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if t.docker == nil {
		return nil, fmt.Errorf("not connected")
	}

	resolved := t.resolvePath(path)
	reader, _, err := t.docker.CopyFromContainer(ctx, t.cfg.Container, resolved)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", resolved, err)
	}
	defer reader.Close()

	// CopyFromContainer returns a tar archive
	tr := tar.NewReader(reader)
	if _, err := tr.Next(); err != nil {
		return nil, fmt.Errorf("read tar entry: %w", err)
	}

	return io.ReadAll(tr)
}

// WriteFile writes a file to the container using CopyToContainer.
func (t *DockerTransport) WriteFile(ctx context.Context, path string, content []byte) error {
	if t.docker == nil {
		return fmt.Errorf("not connected")
	}

	resolved := t.resolvePath(path)

	// Ensure parent directory exists
	dir := resolved[:strings.LastIndex(resolved, "/")]
	if dir != "" {
		t.execRaw(ctx, "sh", []string{"-c", fmt.Sprintf("mkdir -p '%s'", dir)})
	}

	// Build a tar archive with the file
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Extract just the filename for the tar header
	filename := resolved[strings.LastIndex(resolved, "/")+1:]
	hdr := &tar.Header{
		Name: filename,
		Mode: 0644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("tar header: %w", err)
	}
	if _, err := tw.Write(content); err != nil {
		return fmt.Errorf("tar write: %w", err)
	}
	tw.Close()

	// CopyToContainer expects the tar to be extracted into the directory
	err := t.docker.CopyToContainer(ctx, t.cfg.Container, dir+"/", &buf, container.CopyToContainerOptions{})
	if err != nil {
		return fmt.Errorf("write %s: %w", resolved, err)
	}

	return nil
}

// HealthCheck verifies the container is still running.
func (t *DockerTransport) HealthCheck(ctx context.Context) error {
	if t.docker == nil {
		return fmt.Errorf("not connected")
	}
	inspect, err := t.docker.ContainerInspect(ctx, t.cfg.Container)
	if err != nil {
		return fmt.Errorf("container inspect: %w", err)
	}
	if !inspect.State.Running {
		return fmt.Errorf("container not running: %s", inspect.State.Status)
	}
	return nil
}

// execRaw executes a command via Docker Exec API and returns the result.
func (t *DockerTransport) execRaw(ctx context.Context, cmd string, args []string) (*ExecResult, error) {
	fullCmd := append([]string{cmd}, args...)

	execCfg := container.ExecOptions{
		Cmd:          fullCmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := t.docker.ContainerExecCreate(ctx, t.cfg.Container, execCfg)
	if err != nil {
		return nil, fmt.Errorf("exec create: %w", err)
	}

	resp, err := t.docker.ContainerExecAttach(ctx, execID.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("exec attach: %w", err)
	}
	defer resp.Close()

	// Read all output (Docker multiplexes stdout/stderr with a header)
	var stdout, stderr bytes.Buffer
	// Use stdcopy to demultiplex
	// Docker uses a multiplexed stream format when not using TTY
	_, err = stdCopyFromDockerStream(resp.Reader, &stdout, &stderr)
	if err != nil {
		return nil, fmt.Errorf("reading output: %w", err)
	}

	// Get exit code
	inspectResp, err := t.docker.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return nil, fmt.Errorf("exec inspect: %w", err)
	}

	return &ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: inspectResp.ExitCode,
	}, nil
}

// stdCopyFromDockerStream demultiplexes Docker's stream format.
// Docker prepends an 8-byte header to each frame: [stream_type, 0, 0, 0, size(4 bytes big-endian)].
func stdCopyFromDockerStream(reader io.Reader, stdout, stderr io.Writer) (int64, error) {
	var total int64
	header := make([]byte, 8)
	for {
		_, err := io.ReadFull(reader, header)
		if err != nil {
			if err == io.EOF {
				return total, nil
			}
			return total, err
		}
		size := int64(header[4])<<24 | int64(header[5])<<16 | int64(header[6])<<8 | int64(header[7])

		var dst io.Writer
		switch header[0] {
		case 1:
			dst = stdout
		case 2:
			dst = stderr
		default:
			dst = stdout
		}

		n, err := io.CopyN(dst, reader, size)
		total += n
		if err != nil {
			return total, err
		}
	}
}

// detectShell probes the container for available shells.
func (t *DockerTransport) detectShell(ctx context.Context) (string, error) {
	// 1. Try PowerShell
	result, err := t.execRaw(ctx, "pwsh", []string{"-NoProfile", "-NonInteractive", "-Command", "$PSVersionTable.PSVersion.Major"})
	if err == nil && result.ExitCode == 0 {
		if _, ok := ParsePwshVersion(result.Stdout); ok {
			return "pwsh", nil
		}
	}

	// 2. Try login shell from /etc/passwd
	result, err = t.execRaw(ctx, "sh", []string{"-c", `getent passwd $(whoami) | cut -d: -f7`})
	if err == nil && result.ExitCode == 0 {
		shellName := ParseShellName(result.Stdout)
		if shellName != "unknown" {
			return shellName, nil
		}
	}

	// 3. Try echo $0
	result, err = t.execRaw(ctx, "sh", []string{"-c", `echo "$0"`})
	if err == nil && result.ExitCode == 0 {
		shellName := ParseShellName(result.Stdout)
		if shellName != "unknown" {
			return shellName, nil
		}
	}

	// 4. Fallback to sh
	return "sh", nil
}

// detectRemoteInfo probes the container for platform, arch, and homedir.
func (t *DockerTransport) detectRemoteInfo(ctx context.Context, shellType string) (*RemoteInfo, error) {
	info := &RemoteInfo{Shell: shellType}

	if shellType == "pwsh" {
		if result, err := t.execRaw(ctx, "pwsh", []string{"-NoProfile", "-NonInteractive", "-Command", PwshPlatformCommand()}); err == nil && result.ExitCode == 0 {
			info.Platform = strings.TrimSpace(result.Stdout)
		}
		if result, err := t.execRaw(ctx, "pwsh", []string{"-NoProfile", "-NonInteractive", "-Command", PwshArchCommand()}); err == nil && result.ExitCode == 0 {
			info.Arch = ParsePwshArch(result.Stdout)
		}
		if result, err := t.execRaw(ctx, "pwsh", []string{"-NoProfile", "-NonInteractive", "-Command", "(Get-Location).Path"}); err == nil && result.ExitCode == 0 {
			info.Homedir = strings.TrimSpace(result.Stdout)
		}
	} else {
		if result, err := t.execRaw(ctx, "sh", []string{"-c", "uname -s"}); err == nil && result.ExitCode == 0 {
			info.Platform = ParsePlatform(result.Stdout)
		}
		if result, err := t.execRaw(ctx, "sh", []string{"-c", "uname -m"}); err == nil && result.ExitCode == 0 {
			info.Arch = ParseArch(result.Stdout)
		}
		if result, err := t.execRaw(ctx, "sh", []string{"-c", "echo $HOME"}); err == nil && result.ExitCode == 0 {
			info.Homedir = strings.TrimSpace(StripAnsi(result.Stdout))
		}
	}

	return info, nil
}

// resolvePath resolves a relative path against the configured cwd.
func (t *DockerTransport) resolvePath(path string) string {
	if strings.HasPrefix(path, "/") {
		return path
	}
	if t.cfg.Cwd != "" {
		return t.cfg.Cwd + "/" + path
	}
	return path
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd C:/dev/tramp
go test ./internal/transport/ -v -run TestNewDocker
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/transport/docker.go internal/transport/docker_test.go go.mod go.sum
git commit -m "feat: add Docker transport with Exec API and Copy API"
```

---

### Task 9: Transport Factory

**Files:**
- Modify: `internal/transport/transport.go`

- [ ] **Step 1: Write failing test for factory**

Add to `internal/transport/ssh_test.go` (or a new `factory_test.go`):

```go
// internal/transport/factory_test.go
package transport

import (
	"testing"

	"github.com/marcfargas/tramp/internal/target"
)

func TestNewTransport(t *testing.T) {
	sshCfg := target.TargetConfig{Type: "ssh", Host: "user@host", Shell: "bash"}
	dockerCfg := target.TargetConfig{Type: "docker", Container: "c1"}
	unknownCfg := target.TargetConfig{Type: "wsl"}

	tr, err := NewTransport(sshCfg)
	if err != nil {
		t.Fatalf("SSH: %v", err)
	}
	if tr.Type() != TransportSSH {
		t.Errorf("SSH type = %q", tr.Type())
	}

	tr, err = NewTransport(dockerCfg)
	if err != nil {
		t.Fatalf("Docker: %v", err)
	}
	if tr.Type() != TransportDocker {
		t.Errorf("Docker type = %q", tr.Type())
	}

	_, err = NewTransport(unknownCfg)
	if err == nil {
		t.Error("expected error for unknown type")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd C:/dev/tramp
go test ./internal/transport/ -v -run TestNewTransport
```

Expected: compilation error.

- [ ] **Step 3: Add factory function to transport.go**

Append to `internal/transport/transport.go`:

```go
// NewTransport creates the appropriate transport for a target config.
func NewTransport(cfg target.TargetConfig) (Transport, error) {
	switch cfg.Type {
	case "ssh":
		return NewSSHTransport(cfg), nil
	case "docker":
		return NewDockerTransport(cfg), nil
	default:
		return nil, fmt.Errorf("unknown transport type: %q", cfg.Type)
	}
}
```

Add the necessary imports (`fmt`, `github.com/marcfargas/tramp/internal/target`) to `transport.go`.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd C:/dev/tramp
go test ./internal/transport/ -v -run TestNewTransport
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/transport/
git commit -m "feat: add transport factory function"
```

---

### Task 10: MCP Server Setup + Target Tools

**Files:**
- Create: `internal/mcp/server.go`, `internal/mcp/tools_target.go`, `internal/mcp/tools_target_test.go`

- [ ] **Step 1: Add MCP SDK dependency**

```bash
cd C:/dev/tramp
go get github.com/modelcontextprotocol/go-sdk/mcp
```

- [ ] **Step 2: Write failing tests for target tools**

```go
// internal/mcp/tools_target_test.go
package mcp

import (
	"context"
	"testing"

	"github.com/marcfargas/tramp/internal/pool"
	"github.com/marcfargas/tramp/internal/target"
	"github.com/marcfargas/tramp/internal/transport"
)

// mockFactory returns a mock transport factory for testing.
func mockFactory() pool.TransportFactory {
	return func(cfg target.TargetConfig) (transport.Transport, error) {
		return &mockTransportForMCP{}, nil
	}
}

type mockTransportForMCP struct{}

func (m *mockTransportForMCP) Type() transport.TransportType    { return transport.TransportSSH }
func (m *mockTransportForMCP) State() transport.TransportState   { return transport.StateConnected }
func (m *mockTransportForMCP) Connect(ctx context.Context) error { return nil }
func (m *mockTransportForMCP) Close() error                     { return nil }
func (m *mockTransportForMCP) Exec(ctx context.Context, cmd string, opts *transport.ExecOptions) (*transport.ExecResult, error) {
	return &transport.ExecResult{Stdout: "ok", ExitCode: 0}, nil
}
func (m *mockTransportForMCP) ReadFile(ctx context.Context, path string) ([]byte, error)       { return nil, nil }
func (m *mockTransportForMCP) WriteFile(ctx context.Context, path string, content []byte) error { return nil }
func (m *mockTransportForMCP) Info() *transport.RemoteInfo {
	return &transport.RemoteInfo{Shell: "bash", Platform: "linux", Arch: "x86_64", Homedir: "/home/user"}
}
func (m *mockTransportForMCP) HealthCheck(ctx context.Context) error { return nil }

func TestHandleTargetAdd(t *testing.T) {
	mgr := target.NewManager()
	s := &Service{Manager: mgr}

	err := s.TargetAdd(context.Background(), "dev", target.TargetConfig{
		Type: "ssh", Host: "user@host", Shell: "bash",
	})
	if err != nil {
		t.Fatalf("TargetAdd failed: %v", err)
	}

	targets := mgr.List()
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
}

func TestHandleTargetAddRejectsInvalid(t *testing.T) {
	mgr := target.NewManager()
	s := &Service{Manager: mgr}

	err := s.TargetAdd(context.Background(), "dev", target.TargetConfig{
		Type: "ssh", // missing host
	})
	if err == nil {
		t.Error("expected error for missing host")
	}
}

func TestHandleTargetSwitch(t *testing.T) {
	mgr := target.NewManager()
	p := pool.New(mockFactory())
	s := &Service{Manager: mgr, Pool: p}

	mgr.Add("dev", target.TargetConfig{Type: "ssh", Host: "user@host", Shell: "bash"})
	err := s.TargetSwitch(context.Background(), "dev")
	if err != nil {
		t.Fatalf("TargetSwitch failed: %v", err)
	}
	if mgr.CurrentName() != "dev" {
		t.Errorf("CurrentName = %q, want %q", mgr.CurrentName(), "dev")
	}
}

func TestHandleTargetList(t *testing.T) {
	mgr := target.NewManager()
	s := &Service{Manager: mgr}

	mgr.Add("dev", target.TargetConfig{Type: "ssh", Host: "user@host", Shell: "bash"})
	mgr.Add("staging", target.TargetConfig{Type: "docker", Container: "c1"})

	list := s.TargetList()
	if len(list) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(list))
	}
}

func TestHandleTargetRemove(t *testing.T) {
	mgr := target.NewManager()
	s := &Service{Manager: mgr}

	mgr.Add("dev", target.TargetConfig{Type: "ssh", Host: "user@host", Shell: "bash"})
	err := s.TargetRemove("dev")
	if err != nil {
		t.Fatalf("TargetRemove failed: %v", err)
	}
	if len(mgr.List()) != 0 {
		t.Error("target should be removed")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

```bash
cd C:/dev/tramp
go test ./internal/mcp/ -v
```

Expected: compilation errors.

- [ ] **Step 4: Implement MCP Service struct and target tool handlers**

```go
// internal/mcp/tools_target.go
package mcp

import (
	"context"
	"fmt"

	"github.com/marcfargas/tramp/internal/pool"
	"github.com/marcfargas/tramp/internal/target"
)

// Service holds shared state for all MCP tool handlers.
type Service struct {
	Manager *target.Manager
	Pool    *pool.Pool
}

// TargetAdd adds a new dynamic target.
func (s *Service) TargetAdd(ctx context.Context, name string, config target.TargetConfig) error {
	// Validate config
	switch config.Type {
	case "ssh":
		if config.Host == "" {
			return fmt.Errorf("ssh target requires host")
		}
	case "docker":
		if config.Container == "" {
			return fmt.Errorf("docker target requires container")
		}
	default:
		return fmt.Errorf("unknown type %q (expected ssh or docker)", config.Type)
	}

	return s.Manager.Add(name, config)
}

// TargetSwitch switches to a target, eagerly connecting to validate.
func (s *Service) TargetSwitch(ctx context.Context, name string) error {
	if name == "local" {
		return s.Manager.Switch("local")
	}

	tgt := s.Manager.Get(name)
	if tgt == nil {
		return fmt.Errorf("target %q not found", name)
	}

	// Eager connect — validate the target works before switching
	_, err := s.Pool.Get(ctx, name, tgt.Config)
	if err != nil {
		return fmt.Errorf("connecting to %q: %w", name, err)
	}

	return s.Manager.Switch(name)
}

// TargetRemove removes a dynamic target and closes its connection.
func (s *Service) TargetRemove(name string) error {
	s.Pool.Close(name)
	return s.Manager.Remove(name)
}

// TargetList returns all targets with their current state.
func (s *Service) TargetList() []target.Target {
	return s.Manager.List()
}

// TargetStatus returns info about the active target.
func (s *Service) TargetStatus() (*target.Target, string) {
	tgt := s.Manager.Current()
	if tgt == nil {
		return nil, "No active target (local mode)"
	}
	status := s.Pool.Status()
	state := "unknown"
	if s, ok := status[tgt.Name]; ok {
		state = string(s)
	}
	return tgt, state
}
```

- [ ] **Step 5: Run tests to verify they pass**

```bash
cd C:/dev/tramp
go test ./internal/mcp/ -v
```

Expected: all PASS.

- [ ] **Step 6: Implement MCP server registration**

```go
// internal/mcp/server.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/marcfargas/tramp/internal/pool"
	"github.com/marcfargas/tramp/internal/target"
	"github.com/marcfargas/tramp/internal/transport"
)

// NewServer creates and configures the MCP server with all tools and resources.
func NewServer(svc *Service) *gomcp.Server {
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "tramp",
		Version: "0.1.0",
	}, nil)

	registerTargetTools(server, svc)
	registerRemoteTools(server, svc)
	registerContextResource(server, svc)

	return server
}

// NewService creates a Service with real transport factory.
func NewService(projectDir string) *Service {
	mgr := target.NewManager()

	factory := func(cfg target.TargetConfig) (transport.Transport, error) {
		return transport.NewTransport(cfg)
	}
	p := pool.New(factory)

	svc := &Service{
		Manager: mgr,
		Pool:    p,
	}

	// Load config files
	loadConfigs(mgr, projectDir)

	return svc
}

// loadConfigs loads global and project tramp.json files.
func loadConfigs(mgr *target.Manager, projectDir string) {
	home, _ := os.UserHomeDir()
	globalPath := filepath.Join(home, ".claude", "tramp.json")
	globalCfg, _ := target.LoadConfigFromFile(globalPath)
	projectCfg, _ := target.LoadConfigFromFile(filepath.Join(projectDir, ".claude", "tramp.json"))
	merged := target.MergeConfigs(globalCfg, projectCfg)
	mgr.LoadFromConfig(merged)
}

// registerTargetTools registers target_add, target_switch, target_remove, target_list, target_status.
func registerTargetTools(server *gomcp.Server, svc *Service) {
	type TargetAddInput struct {
		Name   string `json:"name" jsonschema:"target name"`
		Config string `json:"config" jsonschema:"target config as JSON string"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "target_add",
		Description: "Add a new remote target. Config is a JSON object with type (ssh/docker), host/container, cwd, shell, etc.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args TargetAddInput) (*gomcp.CallToolResult, any, error) {
		var cfg target.TargetConfig
		if err := json.Unmarshal([]byte(args.Config), &cfg); err != nil {
			return toolError("Invalid config JSON: " + err.Error()), nil, nil
		}
		if err := svc.TargetAdd(ctx, args.Name, cfg); err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(fmt.Sprintf("Target %q added.", args.Name)), nil, nil
	})

	type TargetSwitchInput struct {
		Name string `json:"name" jsonschema:"target name to switch to, or 'local' to clear"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "target_switch",
		Description: "Switch active target. Use 'local' to clear and return to local mode.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args TargetSwitchInput) (*gomcp.CallToolResult, any, error) {
		if err := svc.TargetSwitch(ctx, args.Name); err != nil {
			return toolError(err.Error()), nil, nil
		}
		if args.Name == "local" {
			return toolText("Switched to local mode."), nil, nil
		}
		return toolText(fmt.Sprintf("Switched to target %q.", args.Name)), nil, nil
	})

	type TargetRemoveInput struct {
		Name string `json:"name" jsonschema:"target name to remove"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "target_remove",
		Description: "Remove a dynamic target and close its connection.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args TargetRemoveInput) (*gomcp.CallToolResult, any, error) {
		if err := svc.TargetRemove(args.Name); err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(fmt.Sprintf("Target %q removed.", args.Name)), nil, nil
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "target_list",
		Description: "List all configured targets with connection status.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args struct{}) (*gomcp.CallToolResult, any, error) {
		targets := svc.TargetList()
		current := svc.Manager.CurrentName()
		status := svc.Pool.Status()

		var lines []string
		for _, tgt := range targets {
			marker := "  "
			if tgt.Name == current {
				marker = "* "
			}
			state := "disconnected"
			if s, ok := status[tgt.Name]; ok {
				state = string(s)
			}
			line := fmt.Sprintf("%s%s (%s) [%s]", marker, tgt.Name, tgt.Config.Type, state)
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			return toolText("No targets configured."), nil, nil
		}
		return toolText(joinLines(lines)), nil, nil
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "target_status",
		Description: "Show detailed status of the active target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args struct{}) (*gomcp.CallToolResult, any, error) {
		tgt, state := svc.TargetStatus()
		if tgt == nil {
			return toolText("No active target (local mode)."), nil, nil
		}

		info := ""
		switch tgt.Config.Type {
		case "ssh":
			info = fmt.Sprintf("Host: %s", tgt.Config.Host)
		case "docker":
			info = fmt.Sprintf("Container: %s", tgt.Config.Container)
		}

		text := fmt.Sprintf("Target: %s\nType: %s\n%s\nCwd: %s\nShell: %s\nState: %s",
			tgt.Name, tgt.Config.Type, info, tgt.Config.Cwd, tgt.Config.Shell, state)
		return toolText(text), nil, nil
	})
}

// toolText creates a successful text tool result.
func toolText(text string) *gomcp.CallToolResult {
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: text}},
	}
}

// toolError creates an error tool result.
func toolError(msg string) *gomcp.CallToolResult {
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}
```

- [ ] **Step 7: Commit**

```bash
git add internal/mcp/ go.mod go.sum
git commit -m "feat: add MCP server setup and target management tools"
```

---

### Task 11: Remote Operation Tools (bash, read, write, edit)

**Files:**
- Create: `internal/mcp/tools_remote.go`, `internal/mcp/tools_remote_test.go`

- [ ] **Step 1: Write failing tests for remote tools**

```go
// internal/mcp/tools_remote_test.go
package mcp

import (
	"context"
	"testing"

	"github.com/marcfargas/tramp/internal/pool"
	"github.com/marcfargas/tramp/internal/target"
	"github.com/marcfargas/tramp/internal/transport"
)

type mockTransportRemote struct {
	lastCmd     string
	readResult  []byte
	writeResult []byte
	execResult  *transport.ExecResult
}

func (m *mockTransportRemote) Type() transport.TransportType    { return transport.TransportSSH }
func (m *mockTransportRemote) State() transport.TransportState   { return transport.StateConnected }
func (m *mockTransportRemote) Connect(ctx context.Context) error { return nil }
func (m *mockTransportRemote) Close() error                     { return nil }
func (m *mockTransportRemote) Exec(ctx context.Context, cmd string, opts *transport.ExecOptions) (*transport.ExecResult, error) {
	m.lastCmd = cmd
	if m.execResult != nil {
		return m.execResult, nil
	}
	return &transport.ExecResult{Stdout: "ok", ExitCode: 0}, nil
}
func (m *mockTransportRemote) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return m.readResult, nil
}
func (m *mockTransportRemote) WriteFile(ctx context.Context, path string, content []byte) error {
	m.writeResult = content
	return nil
}
func (m *mockTransportRemote) Info() *transport.RemoteInfo {
	return &transport.RemoteInfo{Shell: "bash", Platform: "linux", Arch: "x86_64", Homedir: "/home/user"}
}
func (m *mockTransportRemote) HealthCheck(ctx context.Context) error { return nil }

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

func TestRemoteBash(t *testing.T) {
	mock := &mockTransportRemote{
		execResult: &transport.ExecResult{Stdout: "hello\n", ExitCode: 0},
	}
	svc := setupTestService(mock)

	result, err := svc.RemoteBash(context.Background(), "echo hello", 0)
	if err != nil {
		t.Fatalf("RemoteBash failed: %v", err)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "hello\n")
	}
}

func TestRemoteBashNoTarget(t *testing.T) {
	mgr := target.NewManager()
	svc := &Service{Manager: mgr}

	_, err := svc.RemoteBash(context.Background(), "echo hello", 0)
	if err == nil {
		t.Error("expected error with no active target")
	}
}

func TestRemoteRead(t *testing.T) {
	mock := &mockTransportRemote{
		readResult: []byte("file content"),
	}
	svc := setupTestService(mock)

	content, err := svc.RemoteRead(context.Background(), "/home/user/file.txt", 0, 0)
	if err != nil {
		t.Fatalf("RemoteRead failed: %v", err)
	}
	if content != "file content" {
		t.Errorf("content = %q, want %q", content, "file content")
	}
}

func TestRemoteWrite(t *testing.T) {
	mock := &mockTransportRemote{}
	svc := setupTestService(mock)

	err := svc.RemoteWrite(context.Background(), "/home/user/file.txt", "new content")
	if err != nil {
		t.Fatalf("RemoteWrite failed: %v", err)
	}
	if string(mock.writeResult) != "new content" {
		t.Errorf("written = %q, want %q", string(mock.writeResult), "new content")
	}
}

func TestRemoteEdit(t *testing.T) {
	mock := &mockTransportRemote{
		readResult: []byte("hello world, hello universe"),
	}
	svc := setupTestService(mock)

	err := svc.RemoteEdit(context.Background(), "/home/user/file.txt", "hello", "goodbye", false)
	if err != nil {
		t.Fatalf("RemoteEdit failed: %v", err)
	}
	// Should replace first occurrence only
	if string(mock.writeResult) != "goodbye world, hello universe" {
		t.Errorf("edited = %q, want %q", string(mock.writeResult), "goodbye world, hello universe")
	}
}

func TestRemoteEditAll(t *testing.T) {
	mock := &mockTransportRemote{
		readResult: []byte("hello world, hello universe"),
	}
	svc := setupTestService(mock)

	err := svc.RemoteEdit(context.Background(), "/home/user/file.txt", "hello", "goodbye", true)
	if err != nil {
		t.Fatalf("RemoteEdit failed: %v", err)
	}
	if string(mock.writeResult) != "goodbye world, goodbye universe" {
		t.Errorf("edited = %q, want %q", string(mock.writeResult), "goodbye world, goodbye universe")
	}
}

func TestRemoteEditNotFound(t *testing.T) {
	mock := &mockTransportRemote{
		readResult: []byte("hello world"),
	}
	svc := setupTestService(mock)

	err := svc.RemoteEdit(context.Background(), "/home/user/file.txt", "nonexistent", "replacement", false)
	if err == nil {
		t.Error("expected error when old_string not found")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd C:/dev/tramp
go test ./internal/mcp/ -v -run TestRemote
```

Expected: compilation errors.

- [ ] **Step 3: Implement remote operation methods on Service**

```go
// internal/mcp/tools_remote.go
package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/marcfargas/tramp/internal/transport"
)

// getActiveConnection returns the transport for the current target.
func (s *Service) getActiveConnection(ctx context.Context) (transport.Transport, error) {
	tgt := s.Manager.Current()
	if tgt == nil {
		return nil, fmt.Errorf("no remote target active — use target_switch first")
	}
	return s.Pool.Get(ctx, tgt.Name, tgt.Config)
}

// RemoteBash executes a command on the active target.
func (s *Service) RemoteBash(ctx context.Context, command string, timeout int) (*transport.ExecResult, error) {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return nil, err
	}

	opts := &transport.ExecOptions{}
	if timeout > 0 {
		opts.Timeout = timeout
	}

	return conn.Exec(ctx, command, opts)
}

// RemoteRead reads a file from the active target.
// offset and limit are line-based (0 means ignore).
func (s *Service) RemoteRead(ctx context.Context, path string, offset, limit int) (string, error) {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return "", err
	}

	data, err := conn.ReadFile(ctx, path)
	if err != nil {
		return "", err
	}

	content := string(data)

	// Apply offset and limit (line-based, matching Claude Code's Read tool)
	if offset > 0 || limit > 0 {
		lines := strings.Split(content, "\n")
		start := 0
		if offset > 0 {
			start = offset - 1 // offset is 1-based
			if start >= len(lines) {
				return "", nil
			}
		}
		end := len(lines)
		if limit > 0 && start+limit < end {
			end = start + limit
		}
		content = strings.Join(lines[start:end], "\n")
	}

	return content, nil
}

// RemoteWrite writes a file on the active target.
func (s *Service) RemoteWrite(ctx context.Context, path string, content string) error {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return err
	}

	return conn.WriteFile(ctx, path, []byte(content))
}

// RemoteEdit performs a find-and-replace on a remote file.
func (s *Service) RemoteEdit(ctx context.Context, path, oldString, newString string, replaceAll bool) error {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return err
	}

	// Read current content
	data, err := conn.ReadFile(ctx, path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(data)

	// Check that old_string exists
	if !strings.Contains(content, oldString) {
		return fmt.Errorf("old_string not found in %s", path)
	}

	// Replace
	var updated string
	if replaceAll {
		updated = strings.ReplaceAll(content, oldString, newString)
	} else {
		updated = strings.Replace(content, oldString, newString, 1)
	}

	// Write back
	return conn.WriteFile(ctx, path, []byte(updated))
}

// RemoteGlob performs file pattern matching on the active target.
func (s *Service) RemoteGlob(ctx context.Context, pattern, path string) (string, error) {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return "", err
	}

	tgt := s.Manager.Current()
	searchPath := path
	if searchPath == "" && tgt.Config.Cwd != "" {
		searchPath = tgt.Config.Cwd
	}

	// Use find with glob pattern for bash, Get-ChildItem for pwsh
	var cmd string
	info := conn.Info()
	if info != nil && info.Shell == "pwsh" {
		cmd = fmt.Sprintf("Get-ChildItem -Path '%s' -Recurse -Name -Filter '%s'", searchPath, pattern)
	} else {
		cmd = fmt.Sprintf("find %s -name '%s' -type f 2>/dev/null | head -250 | sort", searchPath, pattern)
	}

	result, err := conn.Exec(ctx, cmd, nil)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// RemoteGrep performs content search on the active target.
func (s *Service) RemoteGrep(ctx context.Context, pattern, path, fileType, glob, outputMode string, contextLines int) (string, error) {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return "", err
	}

	tgt := s.Manager.Current()
	searchPath := path
	if searchPath == "" && tgt.Config.Cwd != "" {
		searchPath = tgt.Config.Cwd
	}

	// Build grep/rg command
	var args []string
	args = append(args, "grep", "-r", "-n")

	if contextLines > 0 {
		args = append(args, fmt.Sprintf("-C%d", contextLines))
	}

	if glob != "" {
		args = append(args, fmt.Sprintf("--include='%s'", glob))
	}

	if outputMode == "files_with_matches" {
		args = append(args, "-l")
	} else if outputMode == "count" {
		args = append(args, "-c")
	}

	args = append(args, fmt.Sprintf("'%s'", pattern))
	args = append(args, searchPath)
	args = append(args, "2>/dev/null", "| head -250")

	cmd := strings.Join(args, " ")
	result, err := conn.Exec(ctx, cmd, nil)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// RemoteLS lists a directory on the active target.
func (s *Service) RemoteLS(ctx context.Context, path string) (string, error) {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return "", err
	}

	tgt := s.Manager.Current()
	lsPath := path
	if lsPath == "" && tgt.Config.Cwd != "" {
		lsPath = tgt.Config.Cwd
	}

	info := conn.Info()
	var cmd string
	if info != nil && info.Shell == "pwsh" {
		cmd = fmt.Sprintf("Get-ChildItem -Path '%s' | Format-Table -AutoSize Name, Length, LastWriteTime", lsPath)
	} else {
		cmd = fmt.Sprintf("ls -la %s", lsPath)
	}

	result, err := conn.Exec(ctx, cmd, nil)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd C:/dev/tramp
go test ./internal/mcp/ -v -run TestRemote
```

Expected: all PASS.

- [ ] **Step 5: Register remote tools in server.go**

Add to `internal/mcp/server.go`, implementing the `registerRemoteTools` function:

```go
// registerRemoteTools registers bash, read, write, edit, glob, grep, ls.
func registerRemoteTools(server *gomcp.Server, svc *Service) {
	type BashInput struct {
		Command     string `json:"command" jsonschema:"the command to execute on the remote target"`
		Description string `json:"description" jsonschema:"description of what the command does"`
		Timeout     int    `json:"timeout" jsonschema:"timeout in milliseconds (0 for no timeout)"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "bash",
		Description: "Execute a shell command on the active remote target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args BashInput) (*gomcp.CallToolResult, any, error) {
		result, err := svc.RemoteBash(ctx, args.Command, args.Timeout)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		output := result.Stdout
		if result.Stderr != "" {
			output += "\nSTDERR:\n" + result.Stderr
		}
		text := fmt.Sprintf("[target: %s] exit code: %d\n%s", svc.Manager.CurrentName(), result.ExitCode, output)
		return toolText(text), nil, nil
	})

	type ReadInput struct {
		FilePath string `json:"file_path" jsonschema:"absolute path to the file to read"`
		Offset   int    `json:"offset" jsonschema:"line number to start reading from (1-based)"`
		Limit    int    `json:"limit" jsonschema:"number of lines to read"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "read",
		Description: "Read a file from the active remote target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args ReadInput) (*gomcp.CallToolResult, any, error) {
		content, err := svc.RemoteRead(ctx, args.FilePath, args.Offset, args.Limit)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(content), nil, nil
	})

	type WriteInput struct {
		FilePath string `json:"file_path" jsonschema:"absolute path to the file to write"`
		Content  string `json:"content" jsonschema:"content to write to the file"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "write",
		Description: "Write a file on the active remote target. Creates parent directories if needed.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args WriteInput) (*gomcp.CallToolResult, any, error) {
		if err := svc.RemoteWrite(ctx, args.FilePath, args.Content); err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(fmt.Sprintf("Written to %s on %s.", args.FilePath, svc.Manager.CurrentName())), nil, nil
	})

	type EditInput struct {
		FilePath   string `json:"file_path" jsonschema:"absolute path to the file to edit"`
		OldString  string `json:"old_string" jsonschema:"text to find and replace"`
		NewString  string `json:"new_string" jsonschema:"replacement text"`
		ReplaceAll bool   `json:"replace_all" jsonschema:"replace all occurrences (default false)"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "edit",
		Description: "Find-and-replace edit on a file on the active remote target. Reads via SFTP, patches in-process, writes back.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args EditInput) (*gomcp.CallToolResult, any, error) {
		if err := svc.RemoteEdit(ctx, args.FilePath, args.OldString, args.NewString, args.ReplaceAll); err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(fmt.Sprintf("Edited %s on %s.", args.FilePath, svc.Manager.CurrentName())), nil, nil
	})

	type GlobInput struct {
		Pattern string `json:"pattern" jsonschema:"glob pattern to match files"`
		Path    string `json:"path" jsonschema:"directory to search in (defaults to target cwd)"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "glob",
		Description: "Find files matching a glob pattern on the active remote target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args GlobInput) (*gomcp.CallToolResult, any, error) {
		result, err := svc.RemoteGlob(ctx, args.Pattern, args.Path)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(result), nil, nil
	})

	type GrepInput struct {
		Pattern    string `json:"pattern" jsonschema:"regex pattern to search for"`
		Path       string `json:"path" jsonschema:"file or directory to search in (defaults to target cwd)"`
		Type       string `json:"type" jsonschema:"file type filter (e.g. js, py, go)"`
		Glob       string `json:"glob" jsonschema:"glob pattern to filter files"`
		OutputMode string `json:"output_mode" jsonschema:"content, files_with_matches, or count"`
		Context    int    `json:"context" jsonschema:"lines of context around matches"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "grep",
		Description: "Search file contents on the active remote target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args GrepInput) (*gomcp.CallToolResult, any, error) {
		result, err := svc.RemoteGrep(ctx, args.Pattern, args.Path, args.Type, args.Glob, args.OutputMode, args.Context)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(result), nil, nil
	})

	type LSInput struct {
		Path string `json:"path" jsonschema:"directory path to list (defaults to target cwd)"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "ls",
		Description: "List directory contents on the active remote target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args LSInput) (*gomcp.CallToolResult, any, error) {
		result, err := svc.RemoteLS(ctx, args.Path)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(result), nil, nil
	})
}
```

- [ ] **Step 6: Run all MCP tests**

```bash
cd C:/dev/tramp
go test ./internal/mcp/ -v
```

Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/mcp/tools_remote.go internal/mcp/tools_remote_test.go internal/mcp/server.go
git commit -m "feat: add remote operation tools (bash, read, write, edit, glob, grep, ls)"
```

---

### Task 12: Context Resource

**Files:**
- Create: `internal/mcp/resource.go`

- [ ] **Step 1: Implement the tramp://context resource**

```go
// internal/mcp/resource.go
package mcp

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerContextResource registers the tramp://context dynamic resource.
func registerContextResource(server *gomcp.Server, svc *Service) {
	server.AddResource(&gomcp.Resource{
		URI:         "tramp://context",
		Name:        "tramp-context",
		Description: "Dynamic context about the active remote target and network boundary.",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
		tgt := svc.Manager.Current()

		if tgt == nil {
			return &gomcp.ReadResourceResult{
				Contents: []*gomcp.ResourceContents{{
					URI:  "tramp://context",
					Text: "No remote target active. Tramp tools (bash, read, write, edit, glob, grep, ls) are available but require an active target. Use target_add and target_switch to connect.",
				}},
			}, nil
		}

		// Build target info line
		var targetInfo string
		switch tgt.Config.Type {
		case "ssh":
			targetInfo = fmt.Sprintf("SSH: %s", tgt.Config.Host)
		case "docker":
			targetInfo = fmt.Sprintf("Docker: %s", tgt.Config.Container)
		}

		// Get remote info if available
		var envInfo string
		status := svc.Pool.Status()
		if _, connected := status[tgt.Name]; connected {
			// Try to get cached info from pool
			conn, err := svc.Pool.Get(ctx, tgt.Name, tgt.Config)
			if err == nil && conn.Info() != nil {
				info := conn.Info()
				envInfo = fmt.Sprintf("Platform: %s/%s | Shell: %s", info.Platform, info.Arch, info.Shell)
			}
		}
		if envInfo == "" {
			envInfo = fmt.Sprintf("Shell: %s", tgt.Config.Shell)
		}

		cwdInfo := tgt.Config.Cwd
		if cwdInfo == "" {
			cwdInfo = "(not set)"
		}

		text := fmt.Sprintf(`REMOTE TARGET ACTIVE: "%s" (%s)
%s | CWD: %s

NETWORK BOUNDARY:
- Your built-in tools (Read, Write, Edit, Bash, Glob, Grep, LS) operate on the LOCAL machine where you are running.
- To operate on the remote target, use the tramp MCP tools: bash, read, write, edit, glob, grep, ls. They have the same signatures as your built-ins.
- Local paths and remote paths are different filesystems. Do not mix them.
- If you need to transfer files between local and remote, use local Read + tramp write (or vice versa).`,
			tgt.Name, targetInfo, envInfo, cwdInfo)

		return &gomcp.ReadResourceResult{
			Contents: []*gomcp.ResourceContents{{
				URI:  "tramp://context",
				Text: text,
			}},
		}, nil
	})
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd C:/dev/tramp
go build ./internal/mcp/
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/mcp/resource.go
git commit -m "feat: add tramp://context dynamic resource for prompt injection"
```

---

### Task 13: Wire Up main.go

**Files:**
- Modify: `cmd/tramp/main.go`

- [ ] **Step 1: Update main.go to start the MCP server**

```go
// cmd/tramp/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	trampmcp "github.com/marcfargas/tramp/internal/mcp"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "serve" {
		fmt.Fprintln(os.Stderr, "Usage: tramp serve")
		os.Exit(1)
	}

	// Determine project directory (cwd)
	projectDir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Cannot determine working directory: %v", err)
	}

	// Create service with config loading
	svc := trampmcp.NewService(projectDir)

	// Create MCP server
	server := trampmcp.NewServer(svc)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		svc.Pool.CloseAll()
		cancel()
	}()

	// Run MCP server over stdio
	if err := server.Run(ctx, &gomcp.StdioTransport{}); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
```

- [ ] **Step 2: Build the binary**

```bash
cd C:/dev/tramp
go build ./cmd/tramp
```

Expected: `tramp.exe` (or `tramp` on Linux) created successfully.

- [ ] **Step 3: Commit**

```bash
git add cmd/tramp/main.go
git commit -m "feat: wire up main.go to start MCP server on stdio"
```

---

### Task 14: Integration Test Infrastructure

**Files:**
- Create: `test/integration/docker-compose.yml`, `test/integration/ssh_bash_test.go`, `test/integration/docker_bash_test.go`

- [ ] **Step 1: Create Docker Compose for test targets**

```yaml
# test/integration/docker-compose.yml
version: "3.8"

services:
  ssh-bash:
    image: linuxserver/openssh-server:latest
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=UTC
      - SUDO_ACCESS=true
      - PASSWORD_ACCESS=true
      - USER_PASSWORD=testpass
      - USER_NAME=testuser
    ports:
      - "2222:2222"
    tmpfs:
      - /tmp

  ssh-pwsh:
    image: mcr.microsoft.com/powershell:latest
    command: >
      pwsh -NoProfile -Command "
        apt-get update && apt-get install -y openssh-server;
        mkdir -p /run/sshd;
        echo 'testuser:testpass' | chpasswd;
        useradd -m -s /usr/bin/pwsh testuser || true;
        echo 'testuser:testpass' | chpasswd;
        /usr/sbin/sshd -D -p 2223
      "
    ports:
      - "2223:2223"

  docker-bash:
    image: ubuntu:22.04
    command: ["sleep", "infinity"]
    tmpfs:
      - /tmp
      - /workspace

  docker-pwsh:
    image: mcr.microsoft.com/powershell:latest
    command: ["sleep", "infinity"]
    tmpfs:
      - /tmp
      - /workspace
```

- [ ] **Step 2: Write SSH × bash integration test**

```go
// test/integration/ssh_bash_test.go
//go:build integration

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/marcfargas/tramp/internal/target"
	"github.com/marcfargas/tramp/internal/transport"
)

func sshBashConfig() target.TargetConfig {
	host := os.Getenv("TRAMP_TEST_SSH_HOST")
	if host == "" {
		host = "testuser@localhost"
	}
	port := 2222
	return target.TargetConfig{
		Type:  "ssh",
		Host:  host,
		Port:  port,
		Shell: "bash",
		Cwd:   "/tmp",
	}
}

func TestSSHBashExec(t *testing.T) {
	cfg := sshBashConfig()
	tr := transport.NewSSHTransport(cfg)
	ctx := context.Background()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	result, err := tr.Exec(ctx, "echo hello", nil)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "hello\n")
	}
}

func TestSSHBashFileRoundTrip(t *testing.T) {
	cfg := sshBashConfig()
	tr := transport.NewSSHTransport(cfg)
	ctx := context.Background()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	content := []byte("hello world\nline 2\n")
	path := "/tmp/tramp-test-roundtrip.txt"

	// Write
	if err := tr.WriteFile(ctx, path, content); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Read
	got, err := tr.ReadFile(ctx, path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(got), string(content))
	}

	// Cleanup
	tr.Exec(ctx, "rm "+path, nil)
}

func TestSSHBashStderr(t *testing.T) {
	cfg := sshBashConfig()
	tr := transport.NewSSHTransport(cfg)
	ctx := context.Background()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	result, err := tr.Exec(ctx, "echo out; echo err >&2", nil)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.Stdout != "out\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "out\n")
	}
	if result.Stderr != "err\n" {
		t.Errorf("Stderr = %q, want %q", result.Stderr, "err\n")
	}
}

func TestSSHBashExitCode(t *testing.T) {
	cfg := sshBashConfig()
	tr := transport.NewSSHTransport(cfg)
	ctx := context.Background()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	result, err := tr.Exec(ctx, "exit 42", nil)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("ExitCode = %d, want 42", result.ExitCode)
	}
}

func TestSSHBashRemoteInfo(t *testing.T) {
	cfg := sshBashConfig()
	tr := transport.NewSSHTransport(cfg)
	ctx := context.Background()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	info := tr.Info()
	if info == nil {
		t.Fatal("Info is nil")
	}
	if info.Shell != "bash" {
		t.Errorf("Shell = %q, want %q", info.Shell, "bash")
	}
	if info.Platform != "linux" {
		t.Errorf("Platform = %q, want %q", info.Platform, "linux")
	}
}
```

- [ ] **Step 3: Write Docker × bash integration test**

```go
// test/integration/docker_bash_test.go
//go:build integration

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/marcfargas/tramp/internal/target"
	"github.com/marcfargas/tramp/internal/transport"
)

func dockerBashConfig() target.TargetConfig {
	container := os.Getenv("TRAMP_TEST_DOCKER_CONTAINER")
	if container == "" {
		container = "integration-docker-bash-1"
	}
	return target.TargetConfig{
		Type:      "docker",
		Container: container,
		Shell:     "bash",
		Cwd:       "/workspace",
	}
}

func TestDockerBashExec(t *testing.T) {
	cfg := dockerBashConfig()
	tr := transport.NewDockerTransport(cfg)
	ctx := context.Background()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	result, err := tr.Exec(ctx, "echo hello", nil)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "hello\n")
	}
}

func TestDockerBashFileRoundTrip(t *testing.T) {
	cfg := dockerBashConfig()
	tr := transport.NewDockerTransport(cfg)
	ctx := context.Background()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	content := []byte("docker test content\nline 2\n")
	path := "/workspace/tramp-test-roundtrip.txt"

	if err := tr.WriteFile(ctx, path, content); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := tr.ReadFile(ctx, path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(got), string(content))
	}
}

func TestDockerBashExitCode(t *testing.T) {
	cfg := dockerBashConfig()
	tr := transport.NewDockerTransport(cfg)
	ctx := context.Background()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	result, err := tr.Exec(ctx, "exit 7", nil)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if result.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", result.ExitCode)
	}
}

func TestDockerBashRemoteInfo(t *testing.T) {
	cfg := dockerBashConfig()
	tr := transport.NewDockerTransport(cfg)
	ctx := context.Background()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	info := tr.Info()
	if info == nil {
		t.Fatal("Info is nil")
	}
	if info.Platform != "linux" {
		t.Errorf("Platform = %q, want %q", info.Platform, "linux")
	}
}
```

- [ ] **Step 4: Verify unit tests still pass**

```bash
cd C:/dev/tramp
go test ./... -v -short
```

Expected: all unit tests PASS (integration tests skipped due to build tag).

- [ ] **Step 5: Commit**

```bash
git add test/
git commit -m "test: add integration test infrastructure with Docker Compose"
```

---

### Task 15: End-to-End Verification

- [ ] **Step 1: Build final binary**

```bash
cd C:/dev/tramp
go build -o tramp.exe ./cmd/tramp
```

- [ ] **Step 2: Run all unit tests with race detector**

```bash
cd C:/dev/tramp
go test ./... -race -v -short
```

Expected: all PASS, no race conditions.

- [ ] **Step 3: Verify MCP server starts**

```bash
cd C:/dev/tramp
echo '{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./tramp.exe serve
```

Expected: JSON-RPC response with server info and capabilities.

- [ ] **Step 4: Commit final state**

```bash
git add -A
git commit -m "chore: end-to-end build verification"
```

---

## Task Dependency Map

```
Task 1 (scaffolding)
  └── Task 2 (bash driver) ──┐
  └── Task 3 (pwsh driver) ──┤
  └── Task 4 (transport) ────┤
                              ├── Task 5 (target manager)
                              ├── Task 6 (pool) ──────────┐
                              ├── Task 7 (SSH transport) ──┤
                              ├── Task 8 (Docker transport)┤
                              └── Task 9 (factory) ────────┤
                                                           ├── Task 10 (MCP + target tools)
                                                           ├── Task 11 (remote tools)
                                                           ├── Task 12 (context resource)
                                                           └── Task 13 (main.go)
                                                                └── Task 14 (integration tests)
                                                                    └── Task 15 (e2e verification)
```

Tasks 2, 3, 4 can run in parallel.
Tasks 7, 8 can run in parallel.
Tasks 10, 11, 12 can run in parallel after their dependencies.
