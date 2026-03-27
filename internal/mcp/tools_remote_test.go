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

func (m *mockTransportRemote) Type() transport.TransportType     { return transport.TransportSSH }
func (m *mockTransportRemote) State() transport.TransportState   { return transport.StateConnected }
func (m *mockTransportRemote) Connect(ctx context.Context) error { return nil }
func (m *mockTransportRemote) Close() error                      { return nil }
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

	result, name, err := svc.RemoteBash(context.Background(), "", "echo hello", 0)
	if err != nil {
		t.Fatalf("RemoteBash failed: %v", err)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "hello\n")
	}
	if name != "dev" {
		t.Errorf("target name = %q, want %q", name, "dev")
	}
}

func TestRemoteBashNoTarget(t *testing.T) {
	mgr := target.NewManager()
	svc := &Service{Manager: mgr}

	_, _, err := svc.RemoteBash(context.Background(), "", "echo hello", 0)
	if err == nil {
		t.Error("expected error with no active target")
	}
}

func TestRemoteBashExplicitTarget(t *testing.T) {
	mock := &mockTransportRemote{
		execResult: &transport.ExecResult{Stdout: "hello\n", ExitCode: 0},
	}
	svc := setupTestService(mock)
	// Don't switch — use explicit target
	svc.Manager.Switch("local")

	result, name, err := svc.RemoteBash(context.Background(), "dev", "echo hello", 0)
	if err != nil {
		t.Fatalf("RemoteBash with explicit target failed: %v", err)
	}
	if result.Stdout != "hello\n" {
		t.Errorf("Stdout = %q, want %q", result.Stdout, "hello\n")
	}
	if name != "dev" {
		t.Errorf("target name = %q, want %q", name, "dev")
	}
}

func TestRemoteRead(t *testing.T) {
	mock := &mockTransportRemote{
		readResult: []byte("file content"),
	}
	svc := setupTestService(mock)

	content, err := svc.RemoteRead(context.Background(), "", "/home/user/file.txt", 0, 0)
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

	err := svc.RemoteWrite(context.Background(), "", "/home/user/file.txt", "new content")
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

	err := svc.RemoteEdit(context.Background(), "", "/home/user/file.txt", "hello", "goodbye", false)
	if err != nil {
		t.Fatalf("RemoteEdit failed: %v", err)
	}
	if string(mock.writeResult) != "goodbye world, hello universe" {
		t.Errorf("edited = %q, want %q", string(mock.writeResult), "goodbye world, hello universe")
	}
}

func TestRemoteEditAll(t *testing.T) {
	mock := &mockTransportRemote{
		readResult: []byte("hello world, hello universe"),
	}
	svc := setupTestService(mock)

	err := svc.RemoteEdit(context.Background(), "", "/home/user/file.txt", "hello", "goodbye", true)
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

	err := svc.RemoteEdit(context.Background(), "", "/home/user/file.txt", "nonexistent", "replacement", false)
	if err == nil {
		t.Error("expected error when old_string not found")
	}
}
