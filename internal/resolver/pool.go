// Package resolver provides a pool of DNS resolvers with status tracking.
package resolver

import (
	"sync"
	"time"
)

// Status represents the current status of a resolver.
type Status int

const (
	// StatusUnknown indicates the resolver hasn't been tested yet.
	StatusUnknown Status = iota
	// StatusHealthy indicates the resolver is working correctly.
	StatusHealthy
	// StatusDegraded indicates the resolver is experiencing issues.
	StatusDegraded
	// StatusBlocked indicates the resolver is blocked or not working.
	StatusBlocked
)

// Resolver represents a DNS resolver with its status.
type Resolver struct {
	// Address is the resolver address (e.g., "8.8.8.8:53" or "https://dns.google/dns-query")
	Address string

	// Type is the resolver type: "udp", "doh", or "dot"
	Type string

	// Status is the current health status
	Status Status

	// LastCheck is the time of the last health check
	LastCheck time.Time

	// FailCount is the number of consecutive failures
	FailCount int

	// Latency is the average response latency
	Latency time.Duration

	// BlockedAt is when the resolver was marked as blocked
	BlockedAt time.Time
}

// Pool manages a collection of DNS resolvers.
type Pool struct {
	mu        sync.RWMutex
	resolvers []*Resolver
	current   int
}

// NewPool creates a new resolver pool.
func NewPool() *Pool {
	return &Pool{
		resolvers: make([]*Resolver, 0),
		current:   0,
	}
}

// Add adds a new resolver to the pool.
func (p *Pool) Add(address, resolverType string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if resolver already exists
	for _, r := range p.resolvers {
		if r.Address == address {
			return
		}
	}

	p.resolvers = append(p.resolvers, &Resolver{
		Address: address,
		Type:    resolverType,
		Status:  StatusUnknown,
	})
}

// AddMultiple adds multiple resolvers of the same type.
func (p *Pool) AddMultiple(addresses []string, resolverType string) {
	for _, addr := range addresses {
		p.Add(addr, resolverType)
	}
}

// Get returns the current active resolver.
func (p *Pool) Get() *Resolver {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.resolvers) == 0 {
		return nil
	}

	return p.resolvers[p.current]
}

// Next moves to the next available resolver.
func (p *Pool) Next() *Resolver {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.resolvers) == 0 {
		return nil
	}

	// Find next healthy or unknown resolver
	startIdx := p.current
	for {
		p.current = (p.current + 1) % len(p.resolvers)

		// If we've gone full circle, return current (even if blocked)
		if p.current == startIdx {
			return p.resolvers[p.current]
		}

		r := p.resolvers[p.current]
		if r.Status != StatusBlocked {
			return r
		}
	}
}

// MarkBlocked marks a resolver as blocked.
func (p *Pool) MarkBlocked(address string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, r := range p.resolvers {
		if r.Address == address {
			r.Status = StatusBlocked
			r.BlockedAt = time.Now()
			return
		}
	}
}

// MarkHealthy marks a resolver as healthy.
func (p *Pool) MarkHealthy(address string, latency time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, r := range p.resolvers {
		if r.Address == address {
			r.Status = StatusHealthy
			r.LastCheck = time.Now()
			r.FailCount = 0
			r.Latency = latency
			return
		}
	}
}

// MarkFailed increments the fail count for a resolver.
func (p *Pool) MarkFailed(address string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, r := range p.resolvers {
		if r.Address == address {
			r.FailCount++
			r.LastCheck = time.Now()
			if r.FailCount >= 3 {
				r.Status = StatusDegraded
			}
			return
		}
	}
}

// GetHealthy returns all healthy resolvers.
func (p *Pool) GetHealthy() []*Resolver {
	p.mu.RLock()
	defer p.mu.RUnlock()

	healthy := make([]*Resolver, 0)
	for _, r := range p.resolvers {
		if r.Status == StatusHealthy {
			healthy = append(healthy, r)
		}
	}
	return healthy
}

// Count returns the total number of resolvers.
func (p *Pool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.resolvers)
}

// CountHealthy returns the number of healthy resolvers.
func (p *Pool) CountHealthy() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, r := range p.resolvers {
		if r.Status == StatusHealthy {
			count++
		}
	}
	return count
}

// IsExhausted returns true if all resolvers are blocked.
func (p *Pool) IsExhausted() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, r := range p.resolvers {
		if r.Status != StatusBlocked {
			return false
		}
	}
	return true
}

// Clear removes all resolvers from the pool.
func (p *Pool) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.resolvers = make([]*Resolver, 0)
	p.current = 0
}

// All returns a copy of all resolvers.
func (p *Pool) All() []*Resolver {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*Resolver, len(p.resolvers))
	copy(result, p.resolvers)
	return result
}
