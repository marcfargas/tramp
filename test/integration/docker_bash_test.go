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
