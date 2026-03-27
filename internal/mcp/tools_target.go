package mcp

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/marcfargas/tramp/internal/pool"
	"github.com/marcfargas/tramp/internal/target"
)

// ForwardEntry tracks an active port forward.
type ForwardEntry struct {
	LocalAddr  string
	RemoteAddr string
	Target     string
	Listener   net.Listener
}

// Service holds shared state for all MCP tool handlers.
type Service struct {
	Manager    *target.Manager
	Pool       *pool.Pool
	ProjectDir string
	forwards   map[string]*ForwardEntry // keyed by localAddr
	fwdMu      sync.Mutex
}

// TargetAdd adds a new target. If persist is true, writes to .claude/tramp.json.
func (s *Service) TargetAdd(ctx context.Context, name string, config target.TargetConfig, persist bool) error {
	// Validate config
	switch config.Type {
	case "ssh":
		if config.Host == "" {
			return fmt.Errorf("ssh target requires host")
		}
	case "docker":
		if config.Container == "" {
			return fmt.Errorf("docker target requires container")
		}
	default:
		return fmt.Errorf("unknown type %q (expected ssh or docker)", config.Type)
	}

	if err := s.Manager.Add(name, config); err != nil {
		return err
	}

	if persist && s.ProjectDir != "" {
		return target.PersistTarget(s.ProjectDir, name, config)
	}
	return nil
}

// TargetSwitch switches to a target, eagerly connecting to validate.
func (s *Service) TargetSwitch(ctx context.Context, name string) error {
	if name == "local" {
		return s.Manager.Switch("local")
	}

	tgt := s.Manager.Get(name)
	if tgt == nil {
		return fmt.Errorf("target %q not found", name)
	}

	// Eager connect — validate the target works before switching
	_, err := s.Pool.Get(ctx, name, tgt.Config)
	if err != nil {
		return fmt.Errorf("connecting to %q: %w", name, err)
	}

	return s.Manager.Switch(name)
}

// TargetRemove removes a dynamic target and closes its connection.
func (s *Service) TargetRemove(name string) error {
	if s.Pool != nil {
		s.Pool.Close(name)
	}
	return s.Manager.Remove(name)
}

// TargetList returns all targets with their current state.
func (s *Service) TargetList() []target.Target {
	return s.Manager.List()
}

// TargetStatus returns info about the active target.
func (s *Service) TargetStatus() (*target.Target, string) {
	tgt := s.Manager.Current()
	if tgt == nil {
		return nil, "No active target (local mode)"
	}
	status := s.Pool.Status()
	state := "unknown"
	if st, ok := status[tgt.Name]; ok {
		state = string(st)
	}
	return tgt, state
}
