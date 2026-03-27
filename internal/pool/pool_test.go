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
	state        transport.TransportState
	connectErr   error
	healthErr    error
	connectCount int
	closeCount   int
}

func (m *mockTransport) Type() transport.TransportType  { return transport.TransportSSH }
func (m *mockTransport) State() transport.TransportState { return m.state }
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
func (m *mockTransport) ReadFile(ctx context.Context, path string) ([]byte, error)       { return nil, nil }
func (m *mockTransport) WriteFile(ctx context.Context, path string, content []byte) error { return nil }
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
