// internal/pool/pool.go
package pool

import (
	"context"
	"fmt"
	"sync"

	"github.com/marcfargas/tramp/internal/target"
	"github.com/marcfargas/tramp/internal/transport"
)

// TransportFactory creates a new transport for a given target config.
type TransportFactory func(cfg target.TargetConfig) (transport.Transport, error)

// Pool manages cached connections to remote targets.
type Pool struct {
	mu          sync.Mutex
	connections map[string]transport.Transport
	connecting  map[string]chan struct{} // prevents duplicate connect attempts
	factory     TransportFactory
}

// New creates a new connection pool with the given transport factory.
func New(factory TransportFactory) *Pool {
	return &Pool{
		connections: make(map[string]transport.Transport),
		connecting:  make(map[string]chan struct{}),
		factory:     factory,
	}
}

// Get returns a healthy connection for the given target, creating one if needed.
func (p *Pool) Get(ctx context.Context, name string, cfg target.TargetConfig) (transport.Transport, error) {
	p.mu.Lock()

	// Check cache
	if conn, ok := p.connections[name]; ok {
		p.mu.Unlock()

		// Health check
		if err := conn.HealthCheck(ctx); err == nil {
			return conn, nil
		}

		// Unhealthy — evict and reconnect
		p.mu.Lock()
		if existing, ok := p.connections[name]; ok && existing == conn {
			delete(p.connections, name)
			conn.Close()
		}
		p.mu.Unlock()

		return p.connect(ctx, name, cfg)
	}

	p.mu.Unlock()
	return p.connect(ctx, name, cfg)
}

// connect creates and caches a new connection.
func (p *Pool) connect(ctx context.Context, name string, cfg target.TargetConfig) (transport.Transport, error) {
	conn, err := p.factory(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating transport for %q: %w", name, err)
	}

	if err := conn.Connect(ctx); err != nil {
		return nil, fmt.Errorf("connecting to %q: %w", name, err)
	}

	p.mu.Lock()
	p.connections[name] = conn
	p.mu.Unlock()

	return conn, nil
}

// Close closes a specific connection.
func (p *Pool) Close(name string) {
	p.mu.Lock()
	conn, ok := p.connections[name]
	if ok {
		delete(p.connections, name)
	}
	p.mu.Unlock()

	if ok {
		conn.Close()
	}
}

// CloseAll closes all connections.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	conns := make(map[string]transport.Transport, len(p.connections))
	for k, v := range p.connections {
		conns[k] = v
	}
	p.connections = make(map[string]transport.Transport)
	p.mu.Unlock()

	for _, conn := range conns {
		conn.Close()
	}
}

// Status returns the connection state for all cached connections.
func (p *Pool) Status() map[string]transport.TransportState {
	p.mu.Lock()
	defer p.mu.Unlock()

	status := make(map[string]transport.TransportState, len(p.connections))
	for name, conn := range p.connections {
		status[name] = conn.State()
	}
	return status
}
