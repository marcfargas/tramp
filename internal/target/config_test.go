// internal/target/config_test.go
package target

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfig(t *testing.T) {
	content := `{
		"default": "dev",
		"targets": {
			"dev": {
				"type": "ssh",
				"host": "user@dev.example.com",
				"cwd": "/home/user/project",
				"shell": "bash"
			},
			"docker-dev": {
				"type": "docker",
				"container": "my-container",
				"cwd": "/workspace"
			}
		}
	}`

	cfg, err := ParseConfig([]byte(content))
	if err != nil {
		t.Fatalf("ParseConfig failed: %v", err)
	}
	if cfg.Default != "dev" {
		t.Errorf("Default = %q, want %q", cfg.Default, "dev")
	}
	if len(cfg.Targets) != 2 {
		t.Errorf("len(Targets) = %d, want 2", len(cfg.Targets))
	}

	dev := cfg.Targets["dev"]
	if dev.Type != "ssh" {
		t.Errorf("dev.Type = %q, want %q", dev.Type, "ssh")
	}
	if dev.Host != "user@dev.example.com" {
		t.Errorf("dev.Host = %q, want %q", dev.Host, "user@dev.example.com")
	}
}

func TestParseConfigRejectsLocalName(t *testing.T) {
	content := `{
		"targets": {
			"local": {
				"type": "ssh",
				"host": "user@host",
				"shell": "bash"
			}
		}
	}`
	_, err := ParseConfig([]byte(content))
	if err == nil {
		t.Error("expected error for reserved name 'local'")
	}
}

func TestMergeConfigs(t *testing.T) {
	global := &Config{
		Default: "global-default",
		Targets: map[string]TargetConfig{
			"shared":      {Type: "ssh", Host: "global@host", Shell: "bash"},
			"only-global": {Type: "ssh", Host: "global@host2", Shell: "bash"},
		},
	}
	project := &Config{
		Default: "project-default",
		Targets: map[string]TargetConfig{
			"shared": {Type: "ssh", Host: "project@host", Shell: "bash"},
		},
	}

	merged := MergeConfigs(global, project)
	if merged.Default != "project-default" {
		t.Errorf("Default = %q, want %q", merged.Default, "project-default")
	}
	if merged.Targets["shared"].Host != "project@host" {
		t.Error("project should override global for same target name")
	}
	if _, ok := merged.Targets["only-global"]; !ok {
		t.Error("global-only target should be preserved")
	}
}

func TestLoadConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tramp.json")
	content := `{"targets": {"test": {"type": "docker", "container": "c1"}}}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFromFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFromFile failed: %v", err)
	}
	if cfg.Targets["test"].Container != "c1" {
		t.Error("expected container c1")
	}
}

func TestLoadConfigFromFileMissing(t *testing.T) {
	cfg, err := LoadConfigFromFile("/nonexistent/tramp.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected empty config, got nil")
	}
	if len(cfg.Targets) != 0 {
		t.Error("expected empty targets for missing file")
	}
}
