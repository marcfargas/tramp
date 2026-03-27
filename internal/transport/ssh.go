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
	"golang.org/x/crypto/ssh/knownhosts"
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

func (t *SSHTransport) Type() TransportType   { return TransportSSH }
func (t *SSHTransport) State() TransportState { return t.state }
func (t *SSHTransport) Info() *RemoteInfo     { return t.info }

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

	hostKeyCallback, err := buildHostKeyCallback(t.cfg.InsecureIgnoreHostKey)
	if err != nil {
		t.state = StateError
		return fmt.Errorf("host key verification: %w", err)
	}

	sshConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
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
func (t *SSHTransport) execSimple(_ context.Context, command string) (string, error) {
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
	if u, h, ok := strings.Cut(hostStr, "@"); ok {
		return u, h
	}
	return "", hostStr
}

// buildHostKeyCallback returns a host key callback.
// If insecure is true, accepts any host key.
// Otherwise, checks ~/.ssh/known_hosts.
func buildHostKeyCallback(insecure bool) (ssh.HostKeyCallback, error) {
	if insecure {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	knownHostsPath := filepath.Join(expandHome("~/.ssh"), "known_hosts")
	if _, err := os.Stat(knownHostsPath); err != nil {
		// No known_hosts file — fall back to insecure with a note
		return ssh.InsecureIgnoreHostKey(), nil
	}

	callback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", knownHostsPath, err)
	}
	return callback, nil
}

// buildAuthMethods creates SSH auth methods from available sources.
func buildAuthMethods(identityFile string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	var tried []string

	// 1. Try SSH agent
	if agentConn := sshAgentConn(); agentConn != nil {
		methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(agentConn).Signers))
		tried = append(tried, "agent: connected")
	} else {
		tried = append(tried, "agent: not available")
	}

	// 2. Try identity file
	if identityFile != "" {
		expanded := expandHome(identityFile)
		key, err := os.ReadFile(expanded)
		if err != nil {
			tried = append(tried, fmt.Sprintf("key %s: %v", expanded, err))
		} else {
			signer, err := ssh.ParsePrivateKey(key)
			if err != nil {
				tried = append(tried, fmt.Sprintf("key %s: parse error: %v", expanded, err))
			} else {
				methods = append(methods, ssh.PublicKeys(signer))
				tried = append(tried, fmt.Sprintf("key %s: loaded", expanded))
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
			tried = append(tried, fmt.Sprintf("key %s: parse error: %v", path, err))
			continue
		}
		methods = append(methods, ssh.PublicKeys(signer))
		tried = append(tried, fmt.Sprintf("key %s: loaded", path))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH auth methods available (tried: %s)", strings.Join(tried, "; "))
	}
	return methods, nil
}

// ForwardPort creates a local TCP listener that forwards to a remote address via SSH.
// Returns the listener (caller must close it) and starts forwarding in the background.
func (t *SSHTransport) ForwardPort(localAddr, remoteAddr string) (net.Listener, error) {
	if t.client == nil {
		return nil, fmt.Errorf("not connected")
	}

	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", localAddr, err)
	}

	go func() {
		for {
			local, err := listener.Accept()
			if err != nil {
				return // listener closed
			}
			go func(local net.Conn) {
				remote, err := t.client.Dial("tcp", remoteAddr)
				if err != nil {
					local.Close()
					return
				}
				go func() {
					defer remote.Close()
					io.Copy(remote, local)
				}()
				go func() {
					defer local.Close()
					io.Copy(local, remote)
				}()
			}(local)
		}
	}()

	return listener, nil
}

// sshAgentConn is implemented per-platform in agent_windows.go and agent_unix.go.

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
