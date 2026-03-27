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

func (t *DockerTransport) Type() TransportType   { return TransportDocker }
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
