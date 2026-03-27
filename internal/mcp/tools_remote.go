package mcp

import (
	"context"
	"fmt"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/marcfargas/tramp/internal/transport"
)

// getActiveConnection returns the transport for the current target.
func (s *Service) getActiveConnection(ctx context.Context) (transport.Transport, error) {
	tgt := s.Manager.Current()
	if tgt == nil {
		return nil, fmt.Errorf("no remote target active — use target_switch first")
	}
	return s.Pool.Get(ctx, tgt.Name, tgt.Config)
}

// RemoteBash executes a command on the active target.
func (s *Service) RemoteBash(ctx context.Context, command string, timeout int) (*transport.ExecResult, error) {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return nil, err
	}

	opts := &transport.ExecOptions{}
	if timeout > 0 {
		opts.Timeout = timeout
	}

	return conn.Exec(ctx, command, opts)
}

// RemoteRead reads a file from the active target.
// offset and limit are line-based (0 means ignore).
func (s *Service) RemoteRead(ctx context.Context, path string, offset, limit int) (string, error) {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return "", err
	}

	data, err := conn.ReadFile(ctx, path)
	if err != nil {
		return "", err
	}

	content := string(data)

	// Apply offset and limit (line-based, matching Claude Code's Read tool)
	if offset > 0 || limit > 0 {
		lines := strings.Split(content, "\n")
		start := 0
		if offset > 0 {
			start = offset - 1 // offset is 1-based
			if start >= len(lines) {
				return "", nil
			}
		}
		end := len(lines)
		if limit > 0 && start+limit < end {
			end = start + limit
		}
		content = strings.Join(lines[start:end], "\n")
	}

	return content, nil
}

// RemoteWrite writes a file on the active target.
func (s *Service) RemoteWrite(ctx context.Context, path string, content string) error {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return err
	}

	return conn.WriteFile(ctx, path, []byte(content))
}

// RemoteEdit performs a find-and-replace on a remote file.
func (s *Service) RemoteEdit(ctx context.Context, path, oldString, newString string, replaceAll bool) error {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return err
	}

	// Read current content
	data, err := conn.ReadFile(ctx, path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	content := string(data)

	// Check that old_string exists
	if !strings.Contains(content, oldString) {
		return fmt.Errorf("old_string not found in %s", path)
	}

	// Replace
	var updated string
	if replaceAll {
		updated = strings.ReplaceAll(content, oldString, newString)
	} else {
		updated = strings.Replace(content, oldString, newString, 1)
	}

	// Write back
	return conn.WriteFile(ctx, path, []byte(updated))
}

// RemoteGlob performs file pattern matching on the active target.
func (s *Service) RemoteGlob(ctx context.Context, pattern, path string) (string, error) {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return "", err
	}

	tgt := s.Manager.Current()
	searchPath := path
	if searchPath == "" && tgt.Config.Cwd != "" {
		searchPath = tgt.Config.Cwd
	}

	// Use find with glob pattern for bash, Get-ChildItem for pwsh
	var cmd string
	info := conn.Info()
	if info != nil && info.Shell == "pwsh" {
		cmd = fmt.Sprintf("Get-ChildItem -Path '%s' -Recurse -Name -Filter '%s'", searchPath, pattern)
	} else {
		cmd = fmt.Sprintf("find %s -name '%s' -type f 2>/dev/null | head -250 | sort", searchPath, pattern)
	}

	result, err := conn.Exec(ctx, cmd, nil)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// RemoteGrep performs content search on the active target.
func (s *Service) RemoteGrep(ctx context.Context, pattern, path, fileType, glob, outputMode string, contextLines int) (string, error) {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return "", err
	}

	tgt := s.Manager.Current()
	searchPath := path
	if searchPath == "" && tgt.Config.Cwd != "" {
		searchPath = tgt.Config.Cwd
	}

	// Build grep/rg command
	var args []string
	args = append(args, "grep", "-r", "-n")

	if contextLines > 0 {
		args = append(args, fmt.Sprintf("-C%d", contextLines))
	}

	if glob != "" {
		args = append(args, fmt.Sprintf("--include='%s'", glob))
	}

	if outputMode == "files_with_matches" {
		args = append(args, "-l")
	} else if outputMode == "count" {
		args = append(args, "-c")
	}

	args = append(args, fmt.Sprintf("'%s'", pattern))
	args = append(args, searchPath)
	args = append(args, "2>/dev/null", "| head -250")

	cmd := strings.Join(args, " ")
	result, err := conn.Exec(ctx, cmd, nil)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// RemoteLS lists a directory on the active target.
func (s *Service) RemoteLS(ctx context.Context, path string) (string, error) {
	conn, err := s.getActiveConnection(ctx)
	if err != nil {
		return "", err
	}

	tgt := s.Manager.Current()
	lsPath := path
	if lsPath == "" && tgt.Config.Cwd != "" {
		lsPath = tgt.Config.Cwd
	}

	info := conn.Info()
	var cmd string
	if info != nil && info.Shell == "pwsh" {
		cmd = fmt.Sprintf("Get-ChildItem -Path '%s' | Format-Table -AutoSize Name, Length, LastWriteTime", lsPath)
	} else {
		cmd = fmt.Sprintf("ls -la %s", lsPath)
	}

	result, err := conn.Exec(ctx, cmd, nil)
	if err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// registerRemoteTools registers bash, read, write, edit, glob, grep, ls.
func registerRemoteTools(server *gomcp.Server, svc *Service) {
	type BashInput struct {
		Command     string `json:"command" jsonschema:"the command to execute on the remote target"`
		Description string `json:"description" jsonschema:"description of what the command does"`
		Timeout     int    `json:"timeout" jsonschema:"timeout in milliseconds (0 for no timeout)"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "bash",
		Description: "Execute a shell command on the active remote target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args BashInput) (*gomcp.CallToolResult, any, error) {
		result, err := svc.RemoteBash(ctx, args.Command, args.Timeout)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		output := result.Stdout
		if result.Stderr != "" {
			output += "\nSTDERR:\n" + result.Stderr
		}
		text := fmt.Sprintf("[target: %s] exit code: %d\n%s", svc.Manager.CurrentName(), result.ExitCode, output)
		return toolText(text), nil, nil
	})

	type ReadInput struct {
		FilePath string `json:"file_path" jsonschema:"absolute path to the file to read"`
		Offset   int    `json:"offset" jsonschema:"line number to start reading from (1-based)"`
		Limit    int    `json:"limit" jsonschema:"number of lines to read"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "read",
		Description: "Read a file from the active remote target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args ReadInput) (*gomcp.CallToolResult, any, error) {
		content, err := svc.RemoteRead(ctx, args.FilePath, args.Offset, args.Limit)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(content), nil, nil
	})

	type WriteInput struct {
		FilePath string `json:"file_path" jsonschema:"absolute path to the file to write"`
		Content  string `json:"content" jsonschema:"content to write to the file"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "write",
		Description: "Write a file on the active remote target. Creates parent directories if needed.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args WriteInput) (*gomcp.CallToolResult, any, error) {
		if err := svc.RemoteWrite(ctx, args.FilePath, args.Content); err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(fmt.Sprintf("Written to %s on %s.", args.FilePath, svc.Manager.CurrentName())), nil, nil
	})

	type EditInput struct {
		FilePath   string `json:"file_path" jsonschema:"absolute path to the file to edit"`
		OldString  string `json:"old_string" jsonschema:"text to find and replace"`
		NewString  string `json:"new_string" jsonschema:"replacement text"`
		ReplaceAll bool   `json:"replace_all" jsonschema:"replace all occurrences (default false)"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "edit",
		Description: "Find-and-replace edit on a file on the active remote target. Reads via SFTP, patches in-process, writes back.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args EditInput) (*gomcp.CallToolResult, any, error) {
		if err := svc.RemoteEdit(ctx, args.FilePath, args.OldString, args.NewString, args.ReplaceAll); err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(fmt.Sprintf("Edited %s on %s.", args.FilePath, svc.Manager.CurrentName())), nil, nil
	})

	type GlobInput struct {
		Pattern string `json:"pattern" jsonschema:"glob pattern to match files"`
		Path    string `json:"path" jsonschema:"directory to search in (defaults to target cwd)"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "glob",
		Description: "Find files matching a glob pattern on the active remote target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args GlobInput) (*gomcp.CallToolResult, any, error) {
		result, err := svc.RemoteGlob(ctx, args.Pattern, args.Path)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(result), nil, nil
	})

	type GrepInput struct {
		Pattern    string `json:"pattern" jsonschema:"regex pattern to search for"`
		Path       string `json:"path" jsonschema:"file or directory to search in (defaults to target cwd)"`
		Type       string `json:"type" jsonschema:"file type filter (e.g. js, py, go)"`
		Glob       string `json:"glob" jsonschema:"glob pattern to filter files"`
		OutputMode string `json:"output_mode" jsonschema:"content, files_with_matches, or count"`
		Context    int    `json:"context" jsonschema:"lines of context around matches"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "grep",
		Description: "Search file contents on the active remote target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args GrepInput) (*gomcp.CallToolResult, any, error) {
		result, err := svc.RemoteGrep(ctx, args.Pattern, args.Path, args.Type, args.Glob, args.OutputMode, args.Context)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(result), nil, nil
	})

	type LSInput struct {
		Path string `json:"path" jsonschema:"directory path to list (defaults to target cwd)"`
	}
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "ls",
		Description: "List directory contents on the active remote target.",
	}, func(ctx context.Context, req *gomcp.CallToolRequest, args LSInput) (*gomcp.CallToolResult, any, error) {
		result, err := svc.RemoteLS(ctx, args.Path)
		if err != nil {
			return toolError(err.Error()), nil, nil
		}
		return toolText(result), nil, nil
	})
}
