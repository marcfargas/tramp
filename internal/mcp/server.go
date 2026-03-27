package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/marcfargas/tramp/internal/pool"
	"github.com/marcfargas/tramp/internal/target"
	"github.com/marcfargas/tramp/internal/transport"
)

// NewServer creates and configures the MCP server with all tools and resources.
func NewServer(svc *Service) *gomcp.Server {
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "tramp",
		Version: "0.1.0",
	}, nil)

	registerTargetTools(server, svc)
	registerRemoteTools(server, svc)
	registerForwardTools(server, svc)
	registerContextResource(server, svc)

	return server
}

// NewService creates a Service with real transport factory.
func NewService(projectDir string) *Service {
	mgr := target.NewManager()

	factory := func(cfg target.TargetConfig) (transport.Transport, error) {
		return transport.NewTransport(cfg)
	}
	p := pool.New(factory)

	svc := &Service{
		Manager:    mgr,
		Pool:       p,
		ProjectDir: projectDir,
		forwards:   make(map[string]*ForwardEntry),
	}

	// Load config files and auto-connect to default target
	cfg := loadConfigs(mgr, projectDir)
	if cfg.Default != "" {
		// Best-effort auto-connect — don't fail startup if it doesn't work
		_ = svc.TargetSwitch(context.Background(), cfg.Default)
	}

	return svc
}

// loadConfigs loads global and project tramp.json files. Returns the merged config.
func loadConfigs(mgr *target.Manager, projectDir string) *target.Config {
	home, _ := os.UserHomeDir()
	globalPath := filepath.Join(home, ".claude", "tramp.json")
	globalCfg, _ := target.LoadConfigFromFile(globalPath)
	projectCfg, _ := target.LoadConfigFromFile(filepath.Join(projectDir, ".claude", "tramp.json"))
	merged := target.MergeConfigs(globalCfg, projectCfg)
	mgr.LoadFromConfig(merged)
	return merged
}

// registerTargetTools registers target_add, target_switch, target_remove, target_list, target_status.
func registerTargetTools(server *gomcp.Server, svc *Service) {
	type TargetAddInput struct {
		Name    string `json:"name" jsonschema:"target name"`
		Config  string `json:"config" jsonschema:"target config as JSON string"`
		Persist bool   `json:"persist" jsonschema:"if true, save to .claude/tramp.json (default false)"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "target_add",
		Description: "Add a new remote target. Config is a JSON object with type (ssh/docker), host/container, cwd, shell, etc. Set persist=true to save to project config.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args TargetAddInput) (*gomcp.CallToolResult, any, error) {
		var cfg target.TargetConfig
		if err := json.Unmarshal([]byte(args.Config), &cfg); err != nil {
			return toolError("Invalid config JSON: " + err.Error()), nil, nil
		}
		if err := svc.TargetAdd(ctx, args.Name, cfg, args.Persist); err != nil {
			return toolError(err.Error()), nil, nil
		}
		msg := fmt.Sprintf("Target %q added.", args.Name)
		if args.Persist {
			msg += " Saved to .claude/tramp.json."
		}
		return toolText(msg), nil, nil
	})

	type TargetSwitchInput struct {
		Name string `json:"name" jsonschema:"target name to switch to, or 'local' to clear"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "target_switch",
		Description: "Switch active target. Use 'local' to clear and return to local mode.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args TargetSwitchInput) (*gomcp.CallToolResult, any, error) {
		if err := svc.TargetSwitch(ctx, args.Name); err != nil {
			return toolError(err.Error()), nil, nil
		}
		if args.Name == "local" {
			return toolText("Switched to local mode."), nil, nil
		}
		return toolText(fmt.Sprintf("Switched to target %q.", args.Name)), nil, nil
	})

	type TargetRemoveInput struct {
		Name string `json:"name" jsonschema:"target name to remove"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "target_remove",
		Description: "Remove a dynamic target and close its connection.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args TargetRemoveInput) (*gomcp.CallToolResult, any, error) {
		if err := svc.TargetRemove(args.Name); err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(fmt.Sprintf("Target %q removed.", args.Name)), nil, nil
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "target_list",
		Description: "List all configured targets with connection status.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args struct{}) (*gomcp.CallToolResult, any, error) {
		targets := svc.TargetList()
		current := svc.Manager.CurrentName()
		status := svc.Pool.Status()

		var lines []string
		for _, tgt := range targets {
			marker := "  "
			if tgt.Name == current {
				marker = "* "
			}
			state := "disconnected"
			if s, ok := status[tgt.Name]; ok {
				state = string(s)
			}
			line := fmt.Sprintf("%s%s (%s) [%s]", marker, tgt.Name, tgt.Config.Type, state)
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			return toolText("No targets configured."), nil, nil
		}
		return toolText(joinLines(lines)), nil, nil
	})

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "target_status",
		Description: "Show detailed status of the active target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args struct{}) (*gomcp.CallToolResult, any, error) {
		tgt, state := svc.TargetStatus()
		if tgt == nil {
			return toolText("No active target (local mode)."), nil, nil
		}

		info := ""
		switch tgt.Config.Type {
		case "ssh":
			info = fmt.Sprintf("Host: %s", tgt.Config.Host)
		case "docker":
			info = fmt.Sprintf("Container: %s", tgt.Config.Container)
		}

		text := fmt.Sprintf("Target: %s\nType: %s\n%s\nCwd: %s\nShell: %s\nState: %s",
			tgt.Name, tgt.Config.Type, info, tgt.Config.Cwd, tgt.Config.Shell, state)
		return toolText(text), nil, nil
	})
}

// toolText creates a successful text tool result.
func toolText(text string) *gomcp.CallToolResult {
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: text}},
	}
}

// toolError creates an error tool result.
func toolError(msg string) *gomcp.CallToolResult {
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n")
}
