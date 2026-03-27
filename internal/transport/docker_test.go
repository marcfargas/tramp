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
