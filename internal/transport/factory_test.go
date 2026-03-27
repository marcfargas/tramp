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
