// internal/target/manager.go
package target

import (
	"errors"
	"fmt"
	"sync"
)

// Target represents a named target with its configuration.
type Target struct {
	Name      string
	Config    TargetConfig
	IsDynamic bool // true = added at runtime, not from config file
}

// Manager manages targets and tracks the current active target.
// Thread-safe via mutex.
type Manager struct {
	mu      sync.RWMutex
	targets map[string]*Target
	current string // empty means no active target ("local")
}

// NewManager creates a new target manager.
func NewManager() *Manager {
	return &Manager{
		targets: make(map[string]*Target),
	}
}

// Add adds a dynamic target. Returns error if name is reserved or duplicate.
func (m *Manager) Add(name string, config TargetConfig) error {
	if name == "local" {
		return errors.New(`target name "local" is reserved`)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.targets[name]; exists {
		return fmt.Errorf("target %q already exists", name)
	}

	m.targets[name] = &Target{
		Name:      name,
		Config:    config,
		IsDynamic: true,
	}
	return nil
}

// Remove removes a target. If it's the current target, clears current.
func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.targets[name]; !exists {
		return fmt.Errorf("target %q not found", name)
	}

	delete(m.targets, name)
	if m.current == name {
		m.current = ""
	}
	return nil
}

// Switch sets the current target. "local" clears the current target.
func (m *Manager) Switch(name string) error {
	if name == "local" {
		m.mu.Lock()
		m.current = ""
		m.mu.Unlock()
		return nil
	}

	m.mu.RLock()
	_, exists := m.targets[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("target %q not found", name)
	}

	m.mu.Lock()
	m.current = name
	m.mu.Unlock()
	return nil
}

// CurrentName returns the current target name, or empty if local.
func (m *Manager) CurrentName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Current returns the current target, or nil if local.
func (m *Manager) Current() *Target {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.current == "" {
		return nil
	}
	return m.targets[m.current]
}

// Get returns a target by name, or nil.
func (m *Manager) Get(name string) *Target {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.targets[name]
}

// List returns all targets.
func (m *Manager) List() []Target {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Target, 0, len(m.targets))
	for _, t := range m.targets {
		result = append(result, *t)
	}
	return result
}

// LoadFromConfig populates targets from a parsed config. Existing dynamic targets are preserved.
func (m *Manager) LoadFromConfig(cfg *Config) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, tc := range cfg.Targets {
		m.targets[name] = &Target{
			Name:      name,
			Config:    tc,
			IsDynamic: false,
		}
	}
}
