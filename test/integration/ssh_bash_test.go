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
	keyFile := os.Getenv("TRAMP_TEST_SSH_KEY")
	port := 2222
	return target.TargetConfig{
		Type:         "ssh",
		Host:         host,
		Port:         port,
		Shell:        "bash",
		Cwd:          "/tmp",
		IdentityFile: keyFile,
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
