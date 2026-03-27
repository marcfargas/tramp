// internal/target/manager_test.go
package target

import (
	"testing"
)

func TestManagerAddAndList(t *testing.T) {
	m := NewManager()
	cfg := TargetConfig{Type: "ssh", Host: "user@host", Shell: "bash"}

	if err := m.Add("dev", cfg); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	targets := m.List()
	if len(targets) != 1 {
		t.Fatalf("List() len = %d, want 1", len(targets))
	}
	if targets[0].Name != "dev" {
		t.Errorf("Name = %q, want %q", targets[0].Name, "dev")
	}
}

func TestManagerAddRejectsLocal(t *testing.T) {
	m := NewManager()
	err := m.Add("local", TargetConfig{Type: "ssh", Host: "h", Shell: "bash"})
	if err == nil {
		t.Error("expected error for reserved name 'local'")
	}
}

func TestManagerAddRejectsDuplicate(t *testing.T) {
	m := NewManager()
	cfg := TargetConfig{Type: "ssh", Host: "h", Shell: "bash"}
	m.Add("dev", cfg)
	err := m.Add("dev", cfg)
	if err == nil {
		t.Error("expected error for duplicate name")
	}
}

func TestManagerSwitch(t *testing.T) {
	m := NewManager()
	m.Add("dev", TargetConfig{Type: "ssh", Host: "h", Shell: "bash"})

	if err := m.Switch("dev"); err != nil {
		t.Fatalf("Switch failed: %v", err)
	}
	if m.CurrentName() != "dev" {
		t.Errorf("CurrentName = %q, want %q", m.CurrentName(), "dev")
	}
}

func TestManagerSwitchLocal(t *testing.T) {
	m := NewManager()
	m.Add("dev", TargetConfig{Type: "ssh", Host: "h", Shell: "bash"})
	m.Switch("dev")

	if err := m.Switch("local"); err != nil {
		t.Fatalf("Switch to local failed: %v", err)
	}
	if m.CurrentName() != "" {
		t.Errorf("CurrentName after local = %q, want empty", m.CurrentName())
	}
	if m.Current() != nil {
		t.Error("Current after local should be nil")
	}
}

func TestManagerSwitchNonexistent(t *testing.T) {
	m := NewManager()
	err := m.Switch("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent target")
	}
}

func TestManagerRemove(t *testing.T) {
	m := NewManager()
	m.Add("dev", TargetConfig{Type: "ssh", Host: "h", Shell: "bash"})
	m.Switch("dev")

	if err := m.Remove("dev"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if m.CurrentName() != "" {
		t.Error("current should be cleared after removing active target")
	}
	if len(m.List()) != 0 {
		t.Error("target should be removed from list")
	}
}

func TestManagerGet(t *testing.T) {
	m := NewManager()
	m.Add("dev", TargetConfig{Type: "docker", Container: "c1"})

	tgt := m.Get("dev")
	if tgt == nil {
		t.Fatal("Get returned nil")
	}
	if tgt.Config.Container != "c1" {
		t.Errorf("Container = %q, want %q", tgt.Config.Container, "c1")
	}
}

func TestManagerLoadConfig(t *testing.T) {
	m := NewManager()
	cfg := &Config{
		Default: "staging",
		Targets: map[string]TargetConfig{
			"staging": {Type: "ssh", Host: "user@staging", Shell: "bash"},
		},
	}
	m.LoadFromConfig(cfg)

	targets := m.List()
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Name != "staging" {
		t.Errorf("Name = %q, want %q", targets[0].Name, "staging")
	}
}
