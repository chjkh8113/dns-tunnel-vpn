// Package tunnel provides DNS tunnel connection management.
package tunnel

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/chjkh8113/dns-tunnel-vpn/internal/config"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/resolver"
)

// State represents the current state of the tunnel.
type State int

const (
	// StateDisconnected indicates the tunnel is not connected.
	StateDisconnected State = iota
	// StateConnecting indicates the tunnel is being established.
	StateConnecting
	// StateConnected indicates the tunnel is active.
	StateConnected
	// StateReconnecting indicates the tunnel is reconnecting.
	StateReconnecting
)

// Manager manages the DNS tunnel connection.
type Manager struct {
	config   *config.TunnelConfig
	pool     *resolver.Pool
	state    State
	stateMu  sync.RWMutex

	// Connection details
	currentResolver *resolver.Resolver
	localListener   net.Listener

	// Event channels
	onDisconnect chan struct{}
	onConnect    chan struct{}

	// Shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new tunnel Manager.
func New(cfg *config.TunnelConfig, pool *resolver.Pool) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		config:       cfg,
		pool:         pool,
		state:        StateDisconnected,
		onDisconnect: make(chan struct{}, 1),
		onConnect:    make(chan struct{}, 1),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Connect establishes a connection using the provided resolver.
func (m *Manager) Connect(r *resolver.Resolver) error {
	m.stateMu.Lock()
	m.state = StateConnecting
	m.stateMu.Unlock()

	log.Printf("Connecting to tunnel via resolver: %s (%s)", r.Address, r.Type)

	// Create local listener
	ln, err := net.Listen("tcp", m.config.LocalAddr)
	if err != nil {
		m.stateMu.Lock()
		m.state = StateDisconnected
		m.stateMu.Unlock()
		return fmt.Errorf("failed to create local listener: %w", err)
	}

	m.localListener = ln
	m.currentResolver = r

	m.stateMu.Lock()
	m.state = StateConnected
	m.stateMu.Unlock()

	// Notify connection established
	select {
	case m.onConnect <- struct{}{}:
	default:
	}

	log.Printf("Tunnel connected via %s, listening on %s", r.Address, m.config.LocalAddr)
	return nil
}

// Disconnect closes the current tunnel connection.
func (m *Manager) Disconnect() error {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	if m.state == StateDisconnected {
		return nil
	}

	if m.localListener != nil {
		m.localListener.Close()
		m.localListener = nil
	}

	m.currentResolver = nil
	m.state = StateDisconnected

	// Notify disconnection
	select {
	case m.onDisconnect <- struct{}{}:
	default:
	}

	log.Printf("Tunnel disconnected")
	return nil
}

// Reconnect attempts to reconnect using the next available resolver.
func (m *Manager) Reconnect() error {
	m.stateMu.Lock()
	m.state = StateReconnecting
	m.stateMu.Unlock()

	// Close existing connection
	if m.localListener != nil {
		m.localListener.Close()
	}

	// Mark current resolver as blocked if it exists
	if m.currentResolver != nil {
		m.pool.MarkBlocked(m.currentResolver.Address)
		log.Printf("Marked resolver %s as blocked", m.currentResolver.Address)
	}

	// Get next resolver
	next := m.pool.Next()
	if next == nil {
		m.stateMu.Lock()
		m.state = StateDisconnected
		m.stateMu.Unlock()
		return fmt.Errorf("no resolvers available")
	}

	// Connect with new resolver
	return m.Connect(next)
}

// State returns the current tunnel state.
func (m *Manager) State() State {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return m.state
}

// IsConnected returns true if the tunnel is connected.
func (m *Manager) IsConnected() bool {
	return m.State() == StateConnected
}

// CurrentResolver returns the currently active resolver.
func (m *Manager) CurrentResolver() *resolver.Resolver {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return m.currentResolver
}

// OnDisconnect returns a channel that receives when the tunnel disconnects.
func (m *Manager) OnDisconnect() <-chan struct{} {
	return m.onDisconnect
}

// OnConnect returns a channel that receives when the tunnel connects.
func (m *Manager) OnConnect() <-chan struct{} {
	return m.onConnect
}

// Run starts the tunnel manager and handles connections.
func (m *Manager) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return m.Disconnect()
		case <-m.onDisconnect:
			log.Printf("Tunnel disconnected, attempting reconnect...")
			time.Sleep(time.Second) // Brief delay before reconnect
			if err := m.Reconnect(); err != nil {
				log.Printf("Reconnect failed: %v", err)
			}
		}
	}
}

// Shutdown gracefully shuts down the tunnel manager.
func (m *Manager) Shutdown() error {
	m.cancel()
	return m.Disconnect()
}

// AcceptLoop accepts incoming connections on the local listener.
// This should be called in a goroutine after Connect.
func (m *Manager) AcceptLoop(handler func(net.Conn)) error {
	if m.localListener == nil {
		return fmt.Errorf("tunnel not connected")
	}

	for {
		conn, err := m.localListener.Accept()
		if err != nil {
			// Check if we're shutting down
			m.stateMu.RLock()
			state := m.state
			m.stateMu.RUnlock()

			if state == StateDisconnected {
				return nil
			}
			return fmt.Errorf("accept failed: %w", err)
		}

		go handler(conn)
	}
}
