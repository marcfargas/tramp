package mcp

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/marcfargas/tramp/internal/transport"
)

// ForwardPort creates a local port forward to a remote address via SSH.
// localPort is just the port number (e.g. "8080"); the listener binds to localhost:{localPort}.
func (s *Service) ForwardPort(ctx context.Context, targetName, localPort, remoteAddr string) (string, error) {
	conn, tgt, err := s.getConnection(ctx, targetName)
	if err != nil {
		return "", err
	}

	sshTransport, ok := conn.(*transport.SSHTransport)
	if !ok {
		return "", fmt.Errorf("target %q is not an SSH target — port forwarding requires SSH", tgt.Name)
	}

	localAddr := "localhost:" + localPort

	s.fwdMu.Lock()
	if _, exists := s.forwards[localAddr]; exists {
		s.fwdMu.Unlock()
		return "", fmt.Errorf("a forward for %s already exists — call forward_stop first", localAddr)
	}
	s.fwdMu.Unlock()

	listener, err := sshTransport.ForwardPort(localAddr, remoteAddr)
	if err != nil {
		return "", fmt.Errorf("forwarding %s -> %s: %w", localAddr, remoteAddr, err)
	}

	s.fwdMu.Lock()
	s.forwards[localAddr] = &ForwardEntry{
		LocalAddr:  localAddr,
		RemoteAddr: remoteAddr,
		Target:     tgt.Name,
		Listener:   listener,
	}
	s.fwdMu.Unlock()

	return fmt.Sprintf("Forwarding %s -> %s via %s", localAddr, remoteAddr, tgt.Name), nil
}

// ForwardList returns all active port forwards.
func (s *Service) ForwardList() []ForwardEntry {
	s.fwdMu.Lock()
	defer s.fwdMu.Unlock()

	result := make([]ForwardEntry, 0, len(s.forwards))
	for _, entry := range s.forwards {
		result = append(result, ForwardEntry{
			LocalAddr:  entry.LocalAddr,
			RemoteAddr: entry.RemoteAddr,
			Target:     entry.Target,
		})
	}
	return result
}

// ForwardStop closes and removes a port forward by its local address.
func (s *Service) ForwardStop(localAddr string) error {
	s.fwdMu.Lock()
	defer s.fwdMu.Unlock()

	entry, ok := s.forwards[localAddr]
	if !ok {
		return fmt.Errorf("no active forward for %s", localAddr)
	}

	if err := entry.Listener.Close(); err != nil {
		return fmt.Errorf("closing listener for %s: %w", localAddr, err)
	}

	delete(s.forwards, localAddr)
	return nil
}

// registerForwardTools registers forward_port, forward_list, forward_stop.
func registerForwardTools(server *gomcp.Server, svc *Service) {
	type ForwardPortInput struct {
		Target     string `json:"target" jsonschema:"target name (optional, uses active target if empty)"`
		LocalPort  string `json:"local_port" jsonschema:"local port to listen on (e.g. 5901)"`
		RemoteHost string `json:"remote_host" jsonschema:"remote host to forward to (default: 127.0.0.1)"`
		RemotePort string `json:"remote_port" jsonschema:"remote port to forward to (e.g. 5432)"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "forward_port",
		Description: "Forward a local port to a remote host:port via SSH tunnel. SSH targets only. Example: local_port=5901, remote_host=127.0.0.1, remote_port=5901. Use forward_stop to close.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args ForwardPortInput) (*gomcp.CallToolResult, any, error) {
		remoteHost := args.RemoteHost
		if remoteHost == "" {
			remoteHost = "127.0.0.1"
		}
		remoteAddr := remoteHost + ":" + args.RemotePort
		msg, err := svc.ForwardPort(ctx, args.Target, args.LocalPort, remoteAddr)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(msg), nil, nil
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "forward_list",
		Description: "List all active port forwards.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args struct{}) (*gomcp.CallToolResult, any, error) {
		entries := svc.ForwardList()
		if len(entries) == 0 {
			return toolText("No active port forwards."), nil, nil
		}
		lines := make([]string, 0, len(entries))
		for _, e := range entries {
			lines = append(lines, fmt.Sprintf("%s -> %s (via %s)", e.LocalAddr, e.RemoteAddr, e.Target))
		}
		return toolText(joinLines(lines)), nil, nil
	})

	type ForwardStopInput struct {
		LocalAddr string `json:"local_addr" jsonschema:"local address of the forward to stop (e.g. localhost:8080)"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "forward_stop",
		Description: "Stop and remove an active port forward by its local address.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args ForwardStopInput) (*gomcp.CallToolResult, any, error) {
		if err := svc.ForwardStop(args.LocalAddr); err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(fmt.Sprintf("Stopped forward for %s.", args.LocalAddr)), nil, nil
	})
}
