// Package app provides the main application orchestrator for dns-tunnel.
package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/chjkh8113/dns-tunnel-vpn/internal/api"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/cloudflare"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/config"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/health"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/resolver"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/scanner"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/tunnel"
)

// App is the main application orchestrator that coordinates all components.
type App struct {
	config       *config.Config
	scanner      *scanner.Scanner
	tunnelMgr    *tunnel.Manager
	healthMon    *health.Monitor
	resolverPool *resolver.Pool
	cfClient     *cloudflare.Client
	apiServer    *api.Server

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new App instance with all components wired together.
func New(cfg *config.Config) *App {
	ctx, cancel := context.WithCancel(context.Background())

	// Create resolver pool
	pool := resolver.NewPool()

	// Create components
	scannerInst := scanner.New(&cfg.Scanner, pool)
	tunnelMgr := tunnel.New(&cfg.Tunnel, pool)
	healthMon := health.New(&cfg.Health, tunnelMgr, pool)
	cfClient := cloudflare.New(&cfg.Cloudflare)
	apiServer := api.New(pool, healthMon)

	return &App{
		config:       cfg,
		scanner:      scannerInst,
		tunnelMgr:    tunnelMgr,
		healthMon:    healthMon,
		resolverPool: pool,
		cfClient:     cfClient,
		apiServer:    apiServer,
		ctx:          ctx,
		cancel:       cancel,
	}
}

// Run starts the application and blocks until shutdown.
func (a *App) Run() error {
	log.Printf("Starting dns-tunnel application")
	log.Printf("Domain: %s", a.config.Tunnel.Domain)
	log.Printf("Local address: %s", a.config.Tunnel.LocalAddr)

	// Step 1: Start API server if enabled
	if a.config.API.Enabled {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			if err := a.apiServer.Start(a.config.API.Port); err != nil {
				log.Printf("API server stopped: %v", err)
			}
		}()
	}

	// Step 2: Try to fetch resolvers from TXT record (fallback source)
	if a.cfClient.IsEnabled() {
		log.Printf("Attempting to fetch resolvers from Cloudflare TXT record...")
		resolvers, err := a.cfClient.FetchResolvers(a.ctx)
		if err != nil {
			log.Printf("Failed to fetch resolvers from TXT: %v", err)
		} else {
			for _, r := range resolvers {
				a.resolverPool.Add(r, a.config.Tunnel.ResolverType)
			}
			log.Printf("Loaded %d resolvers from TXT record", len(resolvers))
		}
	}

	// Step 3: If pool is empty or has few resolvers, run initial scan
	if a.config.Scanner.Enabled && a.resolverPool.Count() < a.config.Scanner.MinResolvers {
		log.Printf("Running initial resolver scan...")
		working, err := a.scanner.ScanFromSources(a.ctx)
		if err != nil {
			log.Printf("Scan error: %v", err)
		} else {
			log.Printf("Found %d working resolvers", working)
		}
	}

	// Step 4: Connect to first available resolver
	currentResolver := a.resolverPool.Get()
	if currentResolver == nil {
		return fmt.Errorf("no resolvers available, cannot start tunnel")
	}

	if err := a.tunnelMgr.Connect(currentResolver); err != nil {
		return fmt.Errorf("failed to connect tunnel: %w", err)
	}

	// Step 5: Start health monitor in goroutine
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.healthMon.Start(a.ctx); err != nil {
			log.Printf("Health monitor stopped: %v", err)
		}
	}()

	// Step 6: Start background scanner if interval configured
	if a.config.Scanner.Enabled && a.config.Scanner.BackgroundInterval > 0 {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			a.scanner.StartBackground(a.ctx, a.config.Scanner.BackgroundInterval)
		}()
	}

	// Step 7: Start periodic Cloudflare TXT refresh
	if a.cfClient.IsEnabled() {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			a.periodicTXTRefresh()
		}()
	}

	// Step 8: Start disconnect handler
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.handleDisconnects()
	}()

	// Step 9: Block until shutdown signal
	return a.waitForShutdown()
}

// handleDisconnects monitors for disconnection events and handles reconnection.
func (a *App) handleDisconnects() {
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-a.healthMon.OnUnhealthy():
			log.Printf("Health monitor detected unhealthy connection")
			a.handleDisconnect()
		case <-a.tunnelMgr.OnDisconnect():
			log.Printf("Tunnel disconnected")
			a.handleDisconnect()
		}
	}
}

// handleDisconnect handles a tunnel disconnection by attempting reconnection.
func (a *App) handleDisconnect() {
	// Step 1: Mark current resolver as blocked
	current := a.tunnelMgr.CurrentResolver()
	if current != nil {
		a.resolverPool.MarkBlocked(current.Address)
		log.Printf("Marked resolver %s as blocked", current.Address)
	}

	// Step 2: Get next resolver from pool
	next := a.resolverPool.Next()
	if next == nil || a.resolverPool.IsExhausted() {
		// Step 3: Pool exhausted, trigger scan
		log.Printf("Resolver pool exhausted, triggering new scan...")
		if a.config.Scanner.Enabled {
			working, err := a.scanner.ScanFromSources(a.ctx)
			if err != nil {
				log.Printf("Scan failed: %v", err)
				return
			}
			if working == 0 {
				log.Printf("No working resolvers found")
				return
			}
			next = a.resolverPool.Get()
		}
	}

	if next == nil {
		log.Printf("No resolvers available for reconnection")
		return
	}

	// Step 4: Reconnect with new resolver
	log.Printf("Attempting reconnection with resolver: %s", next.Address)
	if err := a.tunnelMgr.Connect(next); err != nil {
		log.Printf("Reconnection failed: %v", err)
		// Try again with next resolver
		a.handleDisconnect()
		return
	}

	// Step 5: Reset health monitor after successful reconnection
	a.healthMon.Reset()
	log.Printf("Successfully reconnected to %s", next.Address)
}

// waitForShutdown blocks until a shutdown signal is received.
func (a *App) waitForShutdown() error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("Received signal: %v, initiating shutdown...", sig)
	case <-a.ctx.Done():
		log.Printf("Context cancelled, initiating shutdown...")
	}

	return a.Shutdown()
}

// periodicTXTRefresh periodically fetches resolvers from Cloudflare TXT record.
func (a *App) periodicTXTRefresh() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			log.Printf("Refreshing resolvers from Cloudflare TXT record...")
			resolvers, err := a.cfClient.FetchResolvers(a.ctx)
			if err != nil {
				log.Printf("TXT refresh failed: %v", err)
				continue
			}
			for _, r := range resolvers {
				a.resolverPool.Add(r, a.config.Tunnel.ResolverType)
			}
			log.Printf("TXT refresh: added %d resolvers", len(resolvers))
		}
	}
}

// Shutdown gracefully shuts down all components.
func (a *App) Shutdown() error {
	log.Printf("Shutting down dns-tunnel...")

	// Cancel context to stop all goroutines
	a.cancel()

	// Stop health monitor
	a.healthMon.Stop()

	// Stop API server
	if a.config.API.Enabled {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.apiServer.Stop(ctx); err != nil {
			log.Printf("Error stopping API server: %v", err)
		}
	}

	// Disconnect tunnel
	if err := a.tunnelMgr.Shutdown(); err != nil {
		log.Printf("Error shutting down tunnel: %v", err)
	}

	// Wait for all goroutines to finish
	a.wg.Wait()

	log.Printf("Shutdown complete")
	return nil
}

// Config returns the application configuration.
func (a *App) Config() *config.Config {
	return a.config
}

// ResolverPool returns the resolver pool.
func (a *App) ResolverPool() *resolver.Pool {
	return a.resolverPool
}

// TunnelManager returns the tunnel manager.
func (a *App) TunnelManager() *tunnel.Manager {
	return a.tunnelMgr
}

// HealthMonitor returns the health monitor.
func (a *App) HealthMonitor() *health.Monitor {
	return a.healthMon
}
