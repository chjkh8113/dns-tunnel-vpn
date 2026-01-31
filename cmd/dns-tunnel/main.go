// Package main provides the unified dns-tunnel entry point.
// This executable merges the functionality of dns-client and dns-resolver-svc
// into a single, configurable application.
//
// Usage:
//
//	dns-tunnel -config config.yaml
//
// The application will:
// 1. Load configuration from the specified YAML file
// 2. Initialize all components (scanner, tunnel, health monitor)
// 3. Attempt to fetch resolvers from Cloudflare TXT record (if configured)
// 4. Run initial resolver scan if needed
// 5. Connect to the first available resolver
// 6. Start health monitoring
// 7. Handle reconnection on failures
// 8. Gracefully shutdown on SIGINT/SIGTERM
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/chjkh8113/dns-tunnel-vpn/internal/app"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/config"
)

var (
	// Version is set at build time
	Version = "dev"
	// BuildTime is set at build time
	BuildTime = "unknown"
)

func main() {
	var (
		configPath  string
		showVersion bool
	)

	flag.StringVar(&configPath, "config", "", "Path to configuration file (required)")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "dns-tunnel - Unified DNS Tunnel VPN Client\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s -config <config.yaml>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -config /etc/dns-tunnel/config.yaml\n", os.Args[0])
	}
	flag.Parse()

	// Show version and exit
	if showVersion {
		fmt.Printf("dns-tunnel version %s (built %s)\n", Version, BuildTime)
		os.Exit(0)
	}

	// Require config file
	if configPath == "" {
		fmt.Fprintf(os.Stderr, "Error: -config flag is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Configure logging
	log.SetFlags(log.LstdFlags | log.LUTC | log.Lmicroseconds)
	log.SetPrefix("[dns-tunnel] ")

	log.Printf("dns-tunnel version %s starting...", Version)

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded from: %s", configPath)

	// Create and run the application
	application := app.New(cfg)
	if err := application.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
