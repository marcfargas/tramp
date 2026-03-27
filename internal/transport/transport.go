package transport

import (
	"context"
	"fmt"
	"io"

	"github.com/marcfargas/tramp/internal/target"
)

type TransportType string

const (
	TransportSSH    TransportType = "ssh"
	TransportDocker TransportType = "docker"
)

type TransportState string

const (
	StateConnecting   TransportState = "connecting"
	StateConnected    TransportState = "connected"
	StateDisconnected TransportState = "disconnected"
	StateError        TransportState = "error"
)

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

type ExecOptions struct {
	Timeout int
	Stdin   io.Reader
}

type RemoteInfo struct {
	Shell    string
	Platform string
	Arch     string
	Homedir  string
}

type Transport interface {
	Type() TransportType
	State() TransportState
	Connect(ctx context.Context) error
	Close() error
	Exec(ctx context.Context, command string, opts *ExecOptions) (*ExecResult, error)
	ReadFile(ctx context.Context, path string) ([]byte, error)
	WriteFile(ctx context.Context, path string, content []byte) error
	Info() *RemoteInfo
	HealthCheck(ctx context.Context) error
}

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
