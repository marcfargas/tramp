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

func (m *mockTransportForMCP) Type() transport.TransportType     { return transport.TransportSSH }
func (m *mockTransportForMCP) State() transport.TransportState   { return transport.StateConnected }
func (m *mockTransportForMCP) Connect(ctx context.Context) error { return nil }
func (m *mockTransportForMCP) Close() error                      { return nil }
func (m *mockTransportForMCP) Exec(ctx context.Context, cmd string, opts *transport.ExecOptions) (*transport.ExecResult, error) {
	return &transport.ExecResult{Stdout: "ok", ExitCode: 0}, nil
}
func (m *mockTransportForMCP) ReadFile(ctx context.Context, path string) ([]byte, error) {
	return nil, nil
}
func (m *mockTransportForMCP) WriteFile(ctx context.Context, path string, content []byte) error {
	return nil
}
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
