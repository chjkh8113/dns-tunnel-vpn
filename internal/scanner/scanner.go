// Package scanner provides DNS resolver scanning functionality.
package scanner

import (
	"context"
	"fmt"
	"log"
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

// ScanFromSources fetches resolver lists from configured sources and scans them.
func (s *Scanner) ScanFromSources(ctx context.Context) (int, error) {
	// Start with default public DNS resolvers
	candidates := []string{
		"8.8.8.8:53",
		"8.8.4.4:53",
		"1.1.1.1:53",
		"1.0.0.1:53",
		"9.9.9.9:53",
		"208.67.222.222:53",
		"208.67.220.220:53",
	}

	// Fetch country IP ranges if configured
	if s.config.CountryCode != "" {
		log.Printf("Fetching IP ranges for country: %s", s.config.CountryCode)
		countryIPs, err := s.fetchCountryIPRanges(ctx, s.config.CountryCode)
		if err != nil {
			log.Printf("Failed to fetch country IP ranges: %v", err)
		} else {
			log.Printf("Fetched %d IP candidates from country ranges", len(countryIPs))
			candidates = append(candidates, countryIPs...)
		}
	}

	// Limit candidates to MaxCandidates
	if s.config.MaxCandidates > 0 && len(candidates) > s.config.MaxCandidates {
		candidates = candidates[:s.config.MaxCandidates]
	}

	log.Printf("Scanning %d resolver candidates", len(candidates))
	results := s.Scan(ctx, candidates, "udp")

	working := 0
	for _, r := range results {
		if r.Working {
			working++
		}
	}

	return working, nil
}
