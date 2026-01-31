// Package config provides unified configuration for the dns-tunnel application.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the unified configuration for dns-tunnel.
type Config struct {
	// Tunnel configuration
	Tunnel TunnelConfig `yaml:"tunnel"`

	// Scanner configuration
	Scanner ScannerConfig `yaml:"scanner"`

	// Health monitoring configuration
	Health HealthConfig `yaml:"health"`

	// Cloudflare DNS configuration
	Cloudflare CloudflareConfig `yaml:"cloudflare"`

	// Logging configuration
	Log LogConfig `yaml:"log"`
}

// TunnelConfig contains tunnel-specific settings.
type TunnelConfig struct {
	// DnsttPath is the path to dnstt-client executable
	DnsttPath string `yaml:"dnstt_path"`

	// Domain is the tunnel domain (e.g., t.example.com)
	Domain string `yaml:"domain"`

	// PubKey is the server's public key (hex string)
	PubKey string `yaml:"pubkey"`

	// PubKeyFile is the path to the server's public key file
	PubKeyFile string `yaml:"pubkey_file"`

	// LocalAddr is the local address to listen on (e.g., 127.0.0.1:7000)
	LocalAddr string `yaml:"local_addr"`

	// ResolverType is the DNS resolver type: "doh", "dot", or "udp"
	ResolverType string `yaml:"resolver_type"`

	// UTLSFingerprint is the uTLS fingerprint distribution
	UTLSFingerprint string `yaml:"utls_fingerprint"`

	// IdleTimeout is the timeout for idle connections
	IdleTimeout time.Duration `yaml:"idle_timeout"`
}

// ScannerConfig contains scanner-specific settings.
type ScannerConfig struct {
	// Enabled determines if scanner should run on startup
	Enabled bool `yaml:"enabled"`

	// ConcurrentScans is the number of concurrent resolver scans
	ConcurrentScans int `yaml:"concurrent_scans"`

	// Timeout is the timeout for each resolver scan
	Timeout time.Duration `yaml:"timeout"`

	// MinResolvers is the minimum number of working resolvers to find
	MinResolvers int `yaml:"min_resolvers"`

	// ResolverSources is a list of sources to fetch resolver lists from
	ResolverSources []string `yaml:"resolver_sources"`
}

// HealthConfig contains health monitoring settings.
type HealthConfig struct {
	// CheckInterval is the interval between health checks
	CheckInterval time.Duration `yaml:"check_interval"`

	// FailThreshold is the number of consecutive failures before marking unhealthy
	FailThreshold int `yaml:"fail_threshold"`

	// RecoveryThreshold is the number of successes needed to mark healthy again
	RecoveryThreshold int `yaml:"recovery_threshold"`

	// Timeout is the timeout for each health check
	Timeout time.Duration `yaml:"timeout"`
}

// CloudflareConfig contains Cloudflare DNS settings.
type CloudflareConfig struct {
	// APIToken is the Cloudflare API token
	APIToken string `yaml:"api_token"`

	// ZoneID is the Cloudflare zone ID
	ZoneID string `yaml:"zone_id"`

	// TXTRecord is the TXT record name for storing resolver list
	TXTRecord string `yaml:"txt_record"`

	// Enabled determines if Cloudflare integration is enabled
	Enabled bool `yaml:"enabled"`
}

// LogConfig contains logging settings.
type LogConfig struct {
	// Level is the log level (debug, info, warn, error)
	Level string `yaml:"level"`

	// Format is the log format (text, json)
	Format string `yaml:"format"`

	// File is the optional log file path
	File string `yaml:"file"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Tunnel: TunnelConfig{
			LocalAddr:       "127.0.0.1:7000",
			ResolverType:    "udp",
			UTLSFingerprint: "4*random,3*Firefox_120,1*Firefox_105,3*Chrome_120",
			IdleTimeout:     2 * time.Minute,
		},
		Scanner: ScannerConfig{
			Enabled:         true,
			ConcurrentScans: 10,
			Timeout:         5 * time.Second,
			MinResolvers:    3,
		},
		Health: HealthConfig{
			CheckInterval:     5 * time.Second,
			FailThreshold:     2,
			RecoveryThreshold: 1,
			Timeout:           5 * time.Second,
		},
		Cloudflare: CloudflareConfig{
			Enabled: false,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

// Load reads configuration from a YAML file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Resolve relative paths
	cfg.resolvePaths()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// resolvePaths converts relative paths to absolute paths based on executable location
func (c *Config) resolvePaths() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	exeDir := filepath.Dir(exePath)

	// Resolve dnstt_path if relative
	if c.Tunnel.DnsttPath != "" && !filepath.IsAbs(c.Tunnel.DnsttPath) {
		c.Tunnel.DnsttPath = filepath.Join(exeDir, c.Tunnel.DnsttPath)
	}

	// Resolve pubkey_file if relative
	if c.Tunnel.PubKeyFile != "" && !filepath.IsAbs(c.Tunnel.PubKeyFile) {
		c.Tunnel.PubKeyFile = filepath.Join(exeDir, c.Tunnel.PubKeyFile)
	}

	// Resolve log file if relative
	if c.Log.File != "" && !filepath.IsAbs(c.Log.File) {
		c.Log.File = filepath.Join(exeDir, c.Log.File)
	}
}

// Validate checks the configuration for required fields and valid values.
func (c *Config) Validate() error {
	if c.Tunnel.Domain == "" {
		return fmt.Errorf("tunnel.domain is required")
	}

	if c.Tunnel.PubKey == "" && c.Tunnel.PubKeyFile == "" {
		return fmt.Errorf("tunnel.pubkey or tunnel.pubkey_file is required")
	}

	if c.Tunnel.LocalAddr == "" {
		return fmt.Errorf("tunnel.local_addr is required")
	}

	switch c.Tunnel.ResolverType {
	case "doh", "dot", "udp":
		// valid
	default:
		return fmt.Errorf("tunnel.resolver_type must be 'doh', 'dot', or 'udp'")
	}

	return nil
}
