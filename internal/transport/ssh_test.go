// internal/transport/ssh_test.go
package transport

import (
	"testing"

	"github.com/marcfargas/tramp/internal/target"
)

func TestNewSSHTransport(t *testing.T) {
	cfg := target.TargetConfig{
		Type:  "ssh",
		Host:  "user@localhost",
		Port:  22,
		Shell: "bash",
		Cwd:   "/home/user",
	}

	tr := NewSSHTransport(cfg)
	if tr == nil {
		t.Fatal("NewSSHTransport returned nil")
	}
	if tr.Type() != TransportSSH {
		t.Errorf("Type = %q, want %q", tr.Type(), TransportSSH)
	}
	if tr.State() != StateDisconnected {
		t.Errorf("State = %q, want %q", tr.State(), StateDisconnected)
	}
}

func TestSSHParseHost(t *testing.T) {
	tests := []struct {
		input    string
		wantUser string
		wantHost string
	}{
		{"user@host.example.com", "user", "host.example.com"},
		{"root@192.168.1.1", "root", "192.168.1.1"},
		{"host.example.com", "", "host.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			user, host := parseSSHHost(tt.input)
			if user != tt.wantUser {
				t.Errorf("user = %q, want %q", user, tt.wantUser)
			}
			if host != tt.wantHost {
				t.Errorf("host = %q, want %q", host, tt.wantHost)
			}
		})
	}
}
