package mcp

import (
	"context"
	"fmt"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerContextResource registers the tramp://context dynamic resource.
func registerContextResource(server *gomcp.Server, svc *Service) {
	server.AddResource(&gomcp.Resource{
		URI:         "tramp://context",
		Name:        "tramp-context",
		Description: "Dynamic context about the active remote target and network boundary.",
		MIMEType:    "text/plain",
	}, func(ctx context.Context, req *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
		tgt := svc.Manager.Current()

		if tgt == nil {
			return &gomcp.ReadResourceResult{
				Contents: []*gomcp.ResourceContents{{
					URI: "tramp://context",
					Text: `No remote target active. Use target_add and target_switch to connect.

TARGET CONFIG SCHEMA (for target_add):
SSH: {"type":"ssh","host":"user@hostname","port":22,"shell":"bash","cwd":"/path","identityFile":"~/.ssh/id_ed25519","insecureIgnoreHostKey":true,"timeout":60000}
Docker: {"type":"docker","container":"name","shell":"bash","cwd":"/path","timeout":60000}
Note: Do NOT pass SSH CLI flags. Use the config fields above. All fields except type and host/container are optional.`,
				}},
			}, nil
		}

		// Build target info line
		var targetInfo string
		switch tgt.Config.Type {
		case "ssh":
			targetInfo = fmt.Sprintf("SSH: %s", tgt.Config.Host)
		case "docker":
			targetInfo = fmt.Sprintf("Docker: %s", tgt.Config.Container)
		}

		// Get remote info if available
		var envInfo string
		status := svc.Pool.Status()
		if _, connected := status[tgt.Name]; connected {
			// Try to get cached info from pool
			conn, err := svc.Pool.Get(ctx, tgt.Name, tgt.Config)
			if err == nil && conn.Info() != nil {
				info := conn.Info()
				envInfo = fmt.Sprintf("Platform: %s/%s | Shell: %s", info.Platform, info.Arch, info.Shell)
			}
		}
		if envInfo == "" {
			envInfo = fmt.Sprintf("Shell: %s", tgt.Config.Shell)
		}

		cwdInfo := tgt.Config.Cwd
		if cwdInfo == "" {
			cwdInfo = "(not set)"
		}

		text := fmt.Sprintf(`REMOTE TARGET ACTIVE: "%s" (%s)
%s | CWD: %s

NETWORK BOUNDARY:
- Your built-in tools (Read, Write, Edit, Bash, Glob, Grep, LS) operate on the LOCAL machine where you are running.
- To operate on the remote target, use the tramp MCP tools: bash, read, write, edit, glob, grep, ls. They have the same signatures as your built-ins.
- Local paths and remote paths are different filesystems. Do not mix them.
- If you need to transfer files between local and remote, use local Read + tramp write (or vice versa).

TARGET CONFIG SCHEMA (for target_add):
SSH: {"type":"ssh","host":"user@hostname","port":22,"shell":"bash","cwd":"/path","identityFile":"~/.ssh/id_ed25519","insecureIgnoreHostKey":true,"timeout":60000}
Docker: {"type":"docker","container":"name","shell":"bash","cwd":"/path","timeout":60000}
Note: Do NOT pass SSH CLI flags (like -o StrictHostKeyChecking=no). Use the config fields above instead.`,
			tgt.Name, targetInfo, envInfo, cwdInfo)

		return &gomcp.ReadResourceResult{
			Contents: []*gomcp.ResourceContents{{
				URI:  "tramp://context",
				Text: text,
			}},
		}, nil
	})
}
