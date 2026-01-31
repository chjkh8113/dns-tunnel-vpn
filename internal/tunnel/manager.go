// Package tunnel provides DNS tunnel connection management via dnstt-client.
package tunnel

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/chjkh8113/dns-tunnel-vpn/internal/config"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/resolver"
)

// Manager manages the dnstt-client subprocess
type Manager struct {
	config     *config.TunnelConfig
	pool       *resolver.Pool
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	mu         sync.RWMutex
	resolverIP string

	// Event channels
	disconnectCh chan struct{}
}

// New creates a new tunnel Manager
func New(cfg *config.TunnelConfig, pool *resolver.Pool) *Manager {
	return &Manager{
		config:       cfg,
		pool:         pool,
		disconnectCh: make(chan struct{}, 1),
	}
}

// Connect establishes a tunnel connection using the provided resolver
func (m *Manager) Connect(r *resolver.Resolver) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmd != nil && m.isProcessRunning() {
		// Stop existing tunnel first
		m.stopInternal()
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	// Build command arguments
	args := m.buildArgs(r.Address)

	log.Printf("[tunnel] Starting: %s %v", m.config.DnsttPath, args)

	m.cmd = exec.CommandContext(ctx, m.config.DnsttPath, args...)
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr

	// Set process group for proper cleanup
	setProcAttr(m.cmd)

	if err := m.cmd.Start(); err != nil {
		m.cancel = nil
		m.cmd = nil
		return fmt.Errorf("failed to start dnstt-client: %w", err)
	}

	m.resolverIP = r.Address
	log.Printf("[tunnel] Process started with PID: %d", m.cmd.Process.Pid)

	// Start goroutine to wait for process completion
	go func() {
		err := m.cmd.Wait()
		if err != nil {
			log.Printf("[tunnel] Process exited with error: %v", err)
		} else {
			log.Printf("[tunnel] Process exited normally")
		}
		// Notify disconnect
		select {
		case m.disconnectCh <- struct{}{}:
		default:
		}
	}()

	// Wait for port to become available
	localPort := 7000
	if m.config.LocalAddr != "" {
		fmt.Sscanf(m.config.LocalAddr, "127.0.0.1:%d", &localPort)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", localPort)
	log.Printf("[tunnel] Waiting for port %d to open...", localPort)

	portOpen := false
	for i := 0; i < 20; i++ { // 20 * 500ms = 10 seconds max
		time.Sleep(500 * time.Millisecond)

		if !m.isProcessRunning() {
			return fmt.Errorf("dnstt-client exited unexpectedly")
		}

		conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
		if err == nil {
			conn.Close()
			portOpen = true
			log.Printf("[tunnel] Port %d is now open (after %dms)", localPort, (i+1)*500)
			break
		}
	}

	if !portOpen {
		log.Printf("[tunnel] WARNING: Port %d never opened, but process is running", localPort)
	}

	return nil
}

// buildArgs constructs the command line arguments for dnstt-client
func (m *Manager) buildArgs(resolverAddr string) []string {
	args := []string{}

	// Add resolver (UDP mode)
	if !hasPort(resolverAddr) {
		resolverAddr = resolverAddr + ":53"
	}
	args = append(args, "-udp", resolverAddr)

	// Add public key
	args = append(args, "-pubkey", m.config.PubKey)

	// Add domain
	args = append(args, m.config.Domain)

	// Add local listener
	args = append(args, m.config.LocalAddr)

	return args
}

// hasPort checks if address includes a port
func hasPort(addr string) bool {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return true
		}
		if addr[i] == '.' {
			return false
		}
	}
	return false
}

// Disconnect stops the tunnel
func (m *Manager) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopInternal()
}

// stopInternal stops the process without locking
func (m *Manager) stopInternal() error {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}

	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}

	if err := terminateProcess(m.cmd); err != nil {
		return fmt.Errorf("failed to terminate process: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- m.cmd.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = m.cmd.Process.Kill()
	}

	m.cmd = nil
	m.resolverIP = ""
	return nil
}

// IsConnected checks if the tunnel is running
func (m *Manager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isProcessRunning()
}

// isProcessRunning checks process status without locking
func (m *Manager) isProcessRunning() bool {
	if m.cmd == nil || m.cmd.Process == nil {
		return false
	}
	if m.cmd.ProcessState != nil {
		return false
	}
	return checkProcessAlive(m.cmd.Process.Pid)
}

// CurrentResolver returns the current resolver
func (m *Manager) CurrentResolver() *resolver.Resolver {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.resolverIP == "" {
		return nil
	}
	return &resolver.Resolver{Address: m.resolverIP, Type: "udp"}
}

// OnDisconnect returns a channel that receives when tunnel disconnects
func (m *Manager) OnDisconnect() <-chan struct{} {
	return m.disconnectCh
}

// Shutdown gracefully shuts down the tunnel
func (m *Manager) Shutdown() error {
	return m.Disconnect()
}

// terminateProcess attempts graceful termination
func terminateProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		return cmd.Process.Kill()
	}
	return cmd.Process.Signal(os.Interrupt)
}

// checkProcessAlive checks if a process is running
func checkProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	if runtime.GOOS == "windows" {
		return checkWindowsProcess(pid)
	}

	err = process.Signal(nil)
	return err == nil
}
