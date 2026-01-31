// Package health provides health monitoring for DNS tunnel connections.
package health

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/chjkh8113/dns-tunnel-vpn/internal/config"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/resolver"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/tunnel"
)

// Status represents the health status.
type Status int

const (
	// StatusHealthy indicates the connection is healthy.
	StatusHealthy Status = iota
	// StatusDegraded indicates the connection is experiencing issues.
	StatusDegraded
	// StatusUnhealthy indicates the connection is not working.
	StatusUnhealthy
)

// Monitor continuously monitors the health of the tunnel connection.
type Monitor struct {
	config     *config.HealthConfig
	tunnelMgr  *tunnel.Manager
	pool       *resolver.Pool

	status     Status
	statusMu   sync.RWMutex
	failCount  int

	// Event channels
	onUnhealthy chan struct{}
	onHealthy   chan struct{}

	// Shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new health Monitor.
func New(cfg *config.HealthConfig, tunnelMgr *tunnel.Manager, pool *resolver.Pool) *Monitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Monitor{
		config:      cfg,
		tunnelMgr:   tunnelMgr,
		pool:        pool,
		status:      StatusHealthy,
		onUnhealthy: make(chan struct{}, 1),
		onHealthy:   make(chan struct{}, 1),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start begins the health monitoring loop.
func (m *Monitor) Start(ctx context.Context) error {
	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	log.Printf("Health monitor started (interval: %v)", m.config.CheckInterval)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-m.ctx.Done():
			return nil
		case <-ticker.C:
			m.check()
		}
	}
}

// check performs a single health check.
func (m *Monitor) check() {
	if !m.tunnelMgr.IsConnected() {
		m.handleFailure("tunnel not connected")
		return
	}

	r := m.tunnelMgr.CurrentResolver()
	if r == nil {
		m.handleFailure("no current resolver")
		return
	}

	// Perform health check on current resolver
	start := time.Now()
	err := m.checkResolver(r)
	latency := time.Since(start)

	if err != nil {
		m.handleFailure(err.Error())
		m.pool.MarkFailed(r.Address)
	} else {
		m.handleSuccess(latency)
		m.pool.MarkHealthy(r.Address, latency)
	}
}

// checkResolver performs an ACTIVE connectivity check through the SOCKS proxy.
func (m *Monitor) checkResolver(r *resolver.Resolver) error {
	// First check if tunnel process is running
	if !m.tunnelMgr.IsConnected() {
		return &HealthError{message: "tunnel disconnected"}
	}

	// Active check: Try to connect through SOCKS5 proxy
	proxyAddr := m.tunnelMgr.LocalAddr()
	if proxyAddr == "" {
		proxyAddr = "127.0.0.1:7000"
	}

	// Test by connecting to a known endpoint through the SOCKS proxy
	err := m.testSOCKS5Connection(proxyAddr)
	if err != nil {
		return &HealthError{message: fmt.Sprintf("SOCKS5 test failed: %v", err)}
	}

	return nil
}

// testSOCKS5Connection tests if SOCKS5 proxy is working by connecting to a test endpoint.
func (m *Monitor) testSOCKS5Connection(proxyAddr string) error {
	// Connect to the SOCKS5 proxy
	conn, err := net.DialTimeout("tcp", proxyAddr, m.config.Timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to proxy: %w", err)
	}
	defer conn.Close()

	// Set deadline for the entire handshake
	deadline := time.Now().Add(m.config.Timeout)
	conn.SetDeadline(deadline)

	// SOCKS5 handshake - no auth
	// Send: version(1) + nmethods(1) + methods(1)
	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		return fmt.Errorf("failed to send SOCKS5 greeting: %w", err)
	}

	// Read response: version(1) + method(1)
	resp := make([]byte, 2)
	_, err = io.ReadFull(conn, resp)
	if err != nil {
		return fmt.Errorf("failed to read SOCKS5 response: %w", err)
	}

	if resp[0] != 0x05 {
		return fmt.Errorf("invalid SOCKS5 version: %d", resp[0])
	}

	if resp[1] == 0xFF {
		return fmt.Errorf("SOCKS5 no acceptable auth method")
	}

	// SOCKS5 connect request to a known endpoint (Google DNS)
	// cmd=CONNECT(0x01), rsv=0, atyp=IPv4(0x01), addr=8.8.8.8, port=53
	connectReq := []byte{
		0x05, 0x01, 0x00, 0x01, // version, cmd=connect, rsv, atyp=ipv4
		8, 8, 8, 8, // 8.8.8.8
		0x00, 0x35, // port 53
	}
	_, err = conn.Write(connectReq)
	if err != nil {
		return fmt.Errorf("failed to send SOCKS5 connect: %w", err)
	}

	// Read connect response
	respBuf := make([]byte, 10) // minimum for IPv4 response
	_, err = io.ReadFull(conn, respBuf)
	if err != nil {
		return fmt.Errorf("failed to read SOCKS5 connect response: %w", err)
	}

	if respBuf[0] != 0x05 {
		return fmt.Errorf("invalid SOCKS5 version in response: %d", respBuf[0])
	}

	if respBuf[1] != 0x00 {
		return fmt.Errorf("SOCKS5 connect failed with code: %d", respBuf[1])
	}

	// Connection through proxy successful!
	return nil
}

// handleFailure handles a failed health check.
func (m *Monitor) handleFailure(reason string) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()

	m.failCount++
	log.Printf("Health check failed (%d/%d): %s", m.failCount, m.config.FailThreshold, reason)

	if m.failCount >= m.config.FailThreshold {
		if m.status != StatusUnhealthy {
			m.status = StatusUnhealthy
			log.Printf("Connection marked as unhealthy")
			select {
			case m.onUnhealthy <- struct{}{}:
			default:
			}
		}
	} else if m.failCount > 0 && m.status == StatusHealthy {
		m.status = StatusDegraded
	}
}

// handleSuccess handles a successful health check.
func (m *Monitor) handleSuccess(latency time.Duration) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()

	if m.status == StatusUnhealthy || m.status == StatusDegraded {
		m.failCount--
		if m.failCount <= -m.config.RecoveryThreshold {
			m.status = StatusHealthy
			m.failCount = 0
			log.Printf("Connection recovered (latency: %v)", latency)
			select {
			case m.onHealthy <- struct{}{}:
			default:
			}
		}
	} else {
		m.failCount = 0
	}
}

// Status returns the current health status.
func (m *Monitor) Status() Status {
	m.statusMu.RLock()
	defer m.statusMu.RUnlock()
	return m.status
}

// IsHealthy returns true if the connection is healthy.
func (m *Monitor) IsHealthy() bool {
	return m.Status() == StatusHealthy
}

// OnUnhealthy returns a channel that receives when connection becomes unhealthy.
func (m *Monitor) OnUnhealthy() <-chan struct{} {
	return m.onUnhealthy
}

// OnHealthy returns a channel that receives when connection recovers.
func (m *Monitor) OnHealthy() <-chan struct{} {
	return m.onHealthy
}

// Stop stops the health monitor.
func (m *Monitor) Stop() {
	m.cancel()
}

// HealthError represents a health check error.
type HealthError struct {
	message string
}

func (e *HealthError) Error() string {
	return e.message
}
