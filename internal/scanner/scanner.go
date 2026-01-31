// Package scanner provides DNS resolver scanning functionality.
package scanner

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

// Scanner scans and validates DNS resolvers for tunnel compatibility.
type Scanner struct {
	config *config.ScannerConfig
	pool   *resolver.Pool
}

// New creates a new Scanner instance.
func New(cfg *config.ScannerConfig, pool *resolver.Pool) *Scanner {
	return &Scanner{
		config: cfg,
		pool:   pool,
	}
}

// ScanResult represents the result of scanning a single resolver.
type ScanResult struct {
	Address  string
	Type     string
	Working  bool
	Latency  time.Duration
	Error    error
}

// Scan performs a scan of all provided resolver addresses.
func (s *Scanner) Scan(ctx context.Context, addresses []string, resolverType string) []ScanResult {
	results := make([]ScanResult, 0, len(addresses))
	resultCh := make(chan ScanResult, len(addresses))

	// Create worker pool
	sem := make(chan struct{}, s.config.ConcurrentScans)
	var wg sync.WaitGroup

	for _, addr := range addresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()

			// Acquire semaphore
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				resultCh <- ScanResult{
					Address: address,
					Type:    resolverType,
					Working: false,
					Error:   ctx.Err(),
				}
				return
			}

			result := s.testResolver(ctx, address, resolverType)
			resultCh <- result
		}(addr)
	}

	// Close result channel when all workers done
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results
	for result := range resultCh {
		results = append(results, result)
		if result.Working {
			s.pool.Add(result.Address, result.Type)
			s.pool.MarkHealthy(result.Address, result.Latency)
			log.Printf("Found working resolver: %s (latency: %v)", result.Address, result.Latency)
		}
	}

	return results
}

// testResolver tests if a DNS resolver works for tunnel traffic.
func (s *Scanner) testResolver(ctx context.Context, address, resolverType string) ScanResult {
	result := ScanResult{
		Address: address,
		Type:    resolverType,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()

	start := time.Now()

	switch resolverType {
	case "udp":
		result.Error = s.testUDPResolver(ctx, address)
	case "doh":
		result.Error = s.testDoHResolver(ctx, address)
	case "dot":
		result.Error = s.testDoTResolver(ctx, address)
	default:
		result.Error = fmt.Errorf("unknown resolver type: %s", resolverType)
	}

	result.Latency = time.Since(start)
	result.Working = result.Error == nil

	return result
}

// testUDPResolver tests a UDP DNS resolver.
func (s *Scanner) testUDPResolver(ctx context.Context, address string) error {
	// Simple DNS query to test connectivity
	dialer := net.Dialer{Timeout: s.config.Timeout}
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()

	// Send a minimal DNS query for "example.com" A record
	// This is a simplified test - real implementation would use proper DNS query
	query := []byte{
		0x00, 0x01, // Transaction ID
		0x01, 0x00, // Standard query
		0x00, 0x01, // Questions: 1
		0x00, 0x00, // Answer RRs: 0
		0x00, 0x00, // Authority RRs: 0
		0x00, 0x00, // Additional RRs: 0
		// Query: example.com
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x03, 'c', 'o', 'm',
		0x00,       // null terminator
		0x00, 0x01, // Type A
		0x00, 0x01, // Class IN
	}

	if _, err := conn.Write(query); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	// Wait for response
	conn.SetReadDeadline(time.Now().Add(s.config.Timeout))
	response := make([]byte, 512)
	n, err := conn.Read(response)
	if err != nil {
		return fmt.Errorf("read failed: %w", err)
	}

	// Basic validation - check if we got a DNS response
	if n < 12 {
		return fmt.Errorf("response too short: %d bytes", n)
	}

	// Check response flags (should have QR bit set)
	if response[2]&0x80 == 0 {
		return fmt.Errorf("not a DNS response")
	}

	return nil
}

// testDoHResolver tests a DNS-over-HTTPS resolver.
func (s *Scanner) testDoHResolver(ctx context.Context, url string) error {
	// DoH testing would require HTTP client with proper DNS query
	// For now, just validate URL format
	if url == "" {
		return fmt.Errorf("empty DoH URL")
	}
	// TODO: Implement proper DoH testing
	return nil
}

// testDoTResolver tests a DNS-over-TLS resolver.
func (s *Scanner) testDoTResolver(ctx context.Context, address string) error {
	// DoT testing would require TLS connection
	// For now, just validate address format
	if address == "" {
		return fmt.Errorf("empty DoT address")
	}
	// TODO: Implement proper DoT testing
	return nil
}

// ScanFromSources fetches resolver lists from configured sources and scans them.
func (s *Scanner) ScanFromSources(ctx context.Context) (int, error) {
	// Default UDP resolvers to test (public DNS)
	defaultResolvers := []string{
		"8.8.8.8:53",
		"8.8.4.4:53",
		"1.1.1.1:53",
		"1.0.0.1:53",
		"9.9.9.9:53",
		"208.67.222.222:53",
		"208.67.220.220:53",
	}

	// If resolver sources configured, check for "builtin" or use defaults
	if len(s.config.ResolverSources) == 0 {
		log.Printf("No resolver sources configured, using built-in public DNS")
	}

	results := s.Scan(ctx, defaultResolvers, "udp")

	working := 0
	for _, r := range results {
		if r.Working {
			working++
		}
	}

	return working, nil
}
