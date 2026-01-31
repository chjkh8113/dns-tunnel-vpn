package resolver

import "fmt"

// Resolver represents a DNS resolver configuration
type Resolver struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"` // "udp", "tcp", "doh", "dot"
	Name     string `json:"name"`     // Optional friendly name
}

// Address returns the resolver address in IP:Port format
func (r *Resolver) Address() string {
	if r.Port == 0 {
		r.Port = 53
	}
	return fmt.Sprintf("%s:%d", r.IP, r.Port)
}

// String returns a string representation of the resolver
func (r *Resolver) String() string {
	if r.Name != "" {
		return r.Name + " (" + r.IP + ")"
	}
	return r.IP
}
