// Package scanner provides background scanning functionality.
package scanner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// StartBackground starts a background scanner that runs at the configured interval.
// It respects context cancellation and logs scan results.
func (s *Scanner) StartBackground(ctx context.Context, interval time.Duration) {
	log.Printf("Starting background scanner with interval: %v", interval)

	// Run initial scan immediately
	working, err := s.ScanFromSources(ctx)
	if err != nil {
		log.Printf("Initial scan error: %v", err)
	} else {
		log.Printf("Initial scan complete: %d working resolvers found", working)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("Background scanner stopped: %v", ctx.Err())
			return
		case <-ticker.C:
			working, err := s.ScanFromSources(ctx)
			if err != nil {
				log.Printf("Background scan error: %v", err)
			} else {
				log.Printf("Background scan complete: %d working resolvers found", working)
			}
		}
	}
}

// fetchCountryIPRanges fetches IP ranges for a country from ipdeny.com.
// The endpoint returns CIDR blocks, one per line.
func (s *Scanner) fetchCountryIPRanges(ctx context.Context, countryCode string) ([]string, error) {
	url := fmt.Sprintf("https://www.ipdeny.com/ipblocks/data/countries/%s.zone",
		strings.ToLower(countryCode))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "dns-tunnel-scanner/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching IP ranges: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return s.parseCIDRList(resp.Body)
}

// parseCIDRList parses a list of CIDR ranges and extracts the first IP from each.
// Returns addresses in format "ip:53" ready for DNS scanning.
func (s *Scanner) parseCIDRList(r io.Reader) ([]string, error) {
	var candidates []string
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		ip := s.extractFirstIP(line)
		if ip != "" {
			candidates = append(candidates, ip+":53")
		}

		// Early exit if we have enough candidates
		if s.config.MaxCandidates > 0 && len(candidates) >= s.config.MaxCandidates {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading CIDR list: %w", err)
	}

	return candidates, nil
}

// extractFirstIP extracts the first usable IP from a CIDR range.
// For example, "2.144.0.0/14" returns "2.144.0.1" (first host address).
func (s *Scanner) extractFirstIP(cidr string) string {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		// Try parsing as plain IP
		if ip := net.ParseIP(cidr); ip != nil {
			return ip.String()
		}
		return ""
	}

	// Get the network address and convert to IPv4
	ip := ipNet.IP.To4()
	if ip == nil {
		return "" // Skip IPv6 for now
	}

	// Increment to get first host (network address + 1)
	// Handle overflow properly
	ip = incrementIP(ip)
	if ip == nil {
		return ""
	}

	return ip.String()
}

// incrementIP adds 1 to an IP address.
func incrementIP(ip net.IP) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)

	for i := len(result) - 1; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			return result
		}
	}
	return nil // Overflow
}
