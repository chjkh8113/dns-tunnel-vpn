package pool

import (
	"sync"

	"github.com/chjkh8113/dns-tunnel-vpn/internal/resolver"
)

// Pool manages a list of DNS resolvers with rotation and blocking support
type Pool struct {
	resolvers []resolver.Resolver
	blocked   map[string]bool
	current   int
	mu        sync.RWMutex
}

// NewPool creates a new resolver pool
func NewPool() *Pool {
	return &Pool{
		resolvers: make([]resolver.Resolver, 0),
		blocked:   make(map[string]bool),
		current:   0,
	}
}

// SetResolvers replaces the resolver list and resets state
func (p *Pool) SetResolvers(resolvers []resolver.Resolver) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.resolvers = resolvers
	p.blocked = make(map[string]bool)
	p.current = 0
}

// Current returns the current resolver, or nil if pool is empty or exhausted
func (p *Pool) Current() *resolver.Resolver {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.resolvers) == 0 {
		return nil
	}

	// Check if current resolver is blocked
	if p.current >= 0 && p.current < len(p.resolvers) {
		r := &p.resolvers[p.current]
		if !p.blocked[r.IP] {
			return r
		}
	}

	// Current is blocked, try to find a non-blocked one
	for i := 0; i < len(p.resolvers); i++ {
		r := &p.resolvers[i]
		if !p.blocked[r.IP] {
			return r
		}
	}

	return nil
}

// Next moves to the next non-blocked resolver
// Returns nil if all resolvers are blocked
func (p *Pool) Next() *resolver.Resolver {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.resolvers) == 0 {
		return nil
	}

	// Try each resolver starting from current+1
	startIdx := p.current
	for i := 0; i < len(p.resolvers); i++ {
		nextIdx := (startIdx + 1 + i) % len(p.resolvers)
		r := &p.resolvers[nextIdx]
		if !p.blocked[r.IP] {
			p.current = nextIdx
			return r
		}
	}

	return nil
}

// MarkBlocked marks a resolver as blocked by its IP address
func (p *Pool) MarkBlocked(ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.blocked[ip] = true
}

// IsExhausted returns true if all resolvers are blocked
func (p *Pool) IsExhausted() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.resolvers) == 0 {
		return true
	}

	for _, r := range p.resolvers {
		if !p.blocked[r.IP] {
			return false
		}
	}

	return true
}

// Reset clears the blocked list and resets to the first resolver
func (p *Pool) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.blocked = make(map[string]bool)
	p.current = 0
}

// Count returns the total number of resolvers in the pool
func (p *Pool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return len(p.resolvers)
}

// Available returns the count of non-blocked resolvers
func (p *Pool) Available() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, r := range p.resolvers {
		if !p.blocked[r.IP] {
			count++
		}
	}

	return count
}
