package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// UnifiedConfig combines client and server configs for dns-tunnel service
type UnifiedConfig struct {
	Tunnel     TunnelConfig     `json:"tunnel"`
	Scanner    ScannerConfig    `json:"scanner"`
	Health     HealthConfig     `json:"health"`
	Cloudflare CloudflareConfig `json:"cloudflare"`
	Fallback   FallbackConfig   `json:"fallback"`
}

// TunnelConfig holds DNS tunnel connection settings
type TunnelConfig struct {
	DnsttPath string `json:"dnstt_path"`
	PubKey    string `json:"pubkey"`
	Domain    string `json:"domain"`
	LocalPort int    `json:"local_port"`
}

// ScannerConfig holds DNS resolver scanner settings
type ScannerConfig struct {
	Country      string `json:"country"`
	Workers      int    `json:"workers"`
	TimeoutSec   int    `json:"timeout_sec"`
	MinResolvers int    `json:"min_resolvers"`
}

// HealthConfig holds health check settings
type HealthConfig struct {
	CheckIntervalSec int `json:"check_interval_sec"`
	FailThreshold    int `json:"fail_threshold"`
}

// CloudflareConfig holds Cloudflare API settings (optional)
type CloudflareConfig struct {
	APIToken   string `json:"api_token"`
	ZoneID     string `json:"zone_id"`
	RecordName string `json:"record_name"`
}

// FallbackConfig holds fallback resolver settings
type FallbackConfig struct {
	TXTRecord string `json:"txt_record"`
}

// IsEnabled returns true if Cloudflare is configured
func (c *CloudflareConfig) IsEnabled() bool {
	return c.APIToken != "" && c.ZoneID != ""
}

// LoadUnifiedConfig loads config from JSON file
func LoadUnifiedConfig(path string) (*UnifiedConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultUnifiedConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks if the config has required fields set
func (c *UnifiedConfig) Validate() error {
	// Tunnel validation
	if c.Tunnel.DnsttPath == "" {
		return fmt.Errorf("tunnel.dnstt_path is required")
	}
	if c.Tunnel.PubKey == "" {
		return fmt.Errorf("tunnel.pubkey is required")
	}
	if c.Tunnel.Domain == "" {
		return fmt.Errorf("tunnel.domain is required")
	}
	if c.Tunnel.LocalPort <= 0 || c.Tunnel.LocalPort > 65535 {
		return fmt.Errorf("tunnel.local_port must be 1-65535")
	}

	// Scanner validation
	if c.Scanner.Workers <= 0 {
		return fmt.Errorf("scanner.workers must be positive")
	}
	if c.Scanner.TimeoutSec <= 0 {
		return fmt.Errorf("scanner.timeout_sec must be positive")
	}
	if c.Scanner.MinResolvers <= 0 {
		return fmt.Errorf("scanner.min_resolvers must be positive")
	}

	// Health validation
	if c.Health.CheckIntervalSec <= 0 {
		return fmt.Errorf("health.check_interval_sec must be positive")
	}
	if c.Health.FailThreshold <= 0 {
		return fmt.Errorf("health.fail_threshold must be positive")
	}

	// Cloudflare is optional - only validate if partially configured
	if c.Cloudflare.APIToken != "" && c.Cloudflare.ZoneID == "" {
		return fmt.Errorf("cloudflare.zone_id required when api_token is set")
	}
	if c.Cloudflare.ZoneID != "" && c.Cloudflare.APIToken == "" {
		return fmt.Errorf("cloudflare.api_token required when zone_id is set")
	}

	return nil
}

// DefaultUnifiedConfig returns config with sensible defaults
func DefaultUnifiedConfig() *UnifiedConfig {
	return &UnifiedConfig{
		Tunnel: TunnelConfig{
			DnsttPath: "dnstt-client.exe",
			LocalPort: 7000,
		},
		Scanner: ScannerConfig{
			Country:      "ir",
			Workers:      100,
			TimeoutSec:   2,
			MinResolvers: 10,
		},
		Health: HealthConfig{
			CheckIntervalSec: 20,
			FailThreshold:    2,
		},
		Cloudflare: CloudflareConfig{
			RecordName: "",
		},
		Fallback: FallbackConfig{
			TXTRecord: "",
		},
	}
}

// SaveToFile saves config to JSON file
func (c *UnifiedConfig) SaveToFile(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
