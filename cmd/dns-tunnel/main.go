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
	"path/filepath"

	"github.com/chjkh8113/dns-tunnel-vpn/internal/app"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/config"
)

var (
	// Version is set at build time
	Version = "dev"
	// BuildTime is set at build time
	BuildTime = "unknown"
)

// findConfigFile searches for config file in common locations
func findConfigFile() string {
	// Get executable directory
	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)

		// Search paths relative to executable
		searchPaths := []string{
			filepath.Join(exeDir, "configs", "dns-tunnel.yaml"),
			filepath.Join(exeDir, "config.yaml"),
			filepath.Join(exeDir, "dns-tunnel.yaml"),
		}

		for _, p := range searchPaths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	// Search in current working directory
	cwd, err := os.Getwd()
	if err == nil {
		cwdPaths := []string{
			filepath.Join(cwd, "configs", "dns-tunnel.yaml"),
			filepath.Join(cwd, "config.yaml"),
			filepath.Join(cwd, "dns-tunnel.yaml"),
		}

		for _, p := range cwdPaths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	// Search in user home directory
	home, err := os.UserHomeDir()
	if err == nil {
		homePaths := []string{
			filepath.Join(home, ".dns-tunnel", "config.yaml"),
			filepath.Join(home, ".config", "dns-tunnel", "config.yaml"),
		}

		for _, p := range homePaths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	return ""
}

func main() {
	var (
		configPath  string
		showVersion bool
	)

	flag.StringVar(&configPath, "config", "", "Path to configuration file (optional, auto-detected)")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "dns-tunnel - Unified DNS Tunnel VPN Client\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nConfig file search order:\n")
		fmt.Fprintf(os.Stderr, "  1. -config flag (if provided)\n")
		fmt.Fprintf(os.Stderr, "  2. <exe_dir>/configs/dns-tunnel.yaml\n")
		fmt.Fprintf(os.Stderr, "  3. <exe_dir>/config.yaml\n")
		fmt.Fprintf(os.Stderr, "  4. <cwd>/configs/dns-tunnel.yaml\n")
		fmt.Fprintf(os.Stderr, "  5. ~/.dns-tunnel/config.yaml\n")
	}
	flag.Parse()

	// Show version and exit
	if showVersion {
		fmt.Printf("dns-tunnel version %s (built %s)\n", Version, BuildTime)
		os.Exit(0)
	}

	// Auto-detect config file if not provided
	if configPath == "" {
		configPath = findConfigFile()
		if configPath == "" {
			fmt.Fprintf(os.Stderr, "Error: No config file found\n\n")
			flag.Usage()
			os.Exit(1)
		}
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
