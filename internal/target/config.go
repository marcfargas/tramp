// internal/target/config.go
package target

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// TargetConfig holds the configuration for a single target.
type TargetConfig struct {
	Type         string `json:"type"`                   // "ssh" or "docker"
	Host         string `json:"host,omitempty"`         // SSH: user@hostname
	Port         int    `json:"port,omitempty"`         // SSH: port (default 22)
	IdentityFile string `json:"identityFile,omitempty"` // SSH: path to key
	Container    string `json:"container,omitempty"`    // Docker: container name/ID
	Cwd          string `json:"cwd,omitempty"`          // Remote working directory
	Shell        string `json:"shell,omitempty"`        // "bash" or "pwsh" (optional for docker)
	Timeout      int    `json:"timeout,omitempty"`      // Connection timeout in ms
}

// Config holds the full tramp configuration from a tramp.json file.
type Config struct {
	Default string                  `json:"default,omitempty"`
	Targets map[string]TargetConfig `json:"targets"`
}

// ParseConfig parses a tramp.json file's contents.
func ParseConfig(data []byte) (*Config, error) {
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid tramp.json: %w", err)
	}
	if cfg.Targets == nil {
		cfg.Targets = make(map[string]TargetConfig)
	}

	// Validate reserved name
	if _, ok := cfg.Targets["local"]; ok {
		return nil, errors.New(`target name "local" is reserved`)
	}

	// Validate target types
	for name, tc := range cfg.Targets {
		switch tc.Type {
		case "ssh":
			if tc.Host == "" {
				return nil, fmt.Errorf("target %q: ssh target requires host", name)
			}
		case "docker":
			if tc.Container == "" {
				return nil, fmt.Errorf("target %q: docker target requires container", name)
			}
		default:
			return nil, fmt.Errorf("target %q: unknown type %q (expected ssh or docker)", name, tc.Type)
		}
	}

	return &cfg, nil
}

// MergeConfigs merges global and project configs. Project overrides global.
func MergeConfigs(global, project *Config) *Config {
	merged := &Config{
		Targets: make(map[string]TargetConfig),
	}

	// Start with global
	if global != nil {
		merged.Default = global.Default
		for k, v := range global.Targets {
			merged.Targets[k] = v
		}
	}

	// Project overrides
	if project != nil {
		if project.Default != "" {
			merged.Default = project.Default
		}
		for k, v := range project.Targets {
			merged.Targets[k] = v
		}
	}

	return merged
}

// LoadConfigFromFile loads config from a file path. Returns empty config if file doesn't exist.
func LoadConfigFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Targets: make(map[string]TargetConfig)}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return ParseConfig(data)
}
