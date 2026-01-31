// Package api provides a REST API server for external access to resolver pool data.
package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/chjkh8113/dns-tunnel-vpn/internal/health"
	"github.com/chjkh8113/dns-tunnel-vpn/internal/resolver"
)

// ResolverInfo represents a resolver in JSON responses.
type ResolverInfo struct {
	Address   string `json:"address"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	FailCount int    `json:"fail_count,omitempty"`
}

// ResolversResponse is the response for GET /resolvers.
type ResolversResponse struct {
	Resolvers []ResolverInfo `json:"resolvers"`
	Count     int            `json:"count"`
	Healthy   int            `json:"healthy"`
}

// HealthResponse is the response for GET /health.
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// StatsResponse is the response for GET /stats.
type StatsResponse struct {
	ResolverCount  int    `json:"resolver_count"`
	HealthyCount   int    `json:"healthy_count"`
	DegradedCount  int    `json:"degraded_count"`
	BlockedCount   int    `json:"blocked_count"`
	UnknownCount   int    `json:"unknown_count"`
	PoolExhausted  bool   `json:"pool_exhausted"`
	MonitorStatus  string `json:"monitor_status"`
	MonitorHealthy bool   `json:"monitor_healthy"`
}

// Server is the REST API server.
type Server struct {
	pool    *resolver.Pool
	monitor *health.Monitor
	server  *http.Server
	mu      sync.RWMutex
}

// New creates a new API server.
func New(pool *resolver.Pool, monitor *health.Monitor) *Server {
	return &Server{pool: pool, monitor: monitor}
}

// Start starts the API server on the specified port.
func (s *Server) Start(port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/resolvers", s.handleResolvers)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)

	s.mu.Lock()
	s.server = &http.Server{
		Addr:         ":" + strconv.Itoa(port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	s.mu.Unlock()

	log.Printf("[api] Server starting on port %d", port)
	return s.server.ListenAndServe()
}

// Stop gracefully stops the API server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.RLock()
	srv := s.server
	s.mu.RUnlock()
	if srv == nil {
		return nil
	}
	log.Printf("[api] Server shutting down")
	return srv.Shutdown(ctx)
}

func (s *Server) handleResolvers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resolvers := s.pool.All()
	infos := make([]ResolverInfo, 0, len(resolvers))
	for _, res := range resolvers {
		infos = append(infos, ResolverInfo{
			Address:   res.Address,
			Type:      res.Type,
			Status:    statusToString(res.Status),
			LatencyMs: res.Latency.Milliseconds(),
			FailCount: res.FailCount,
		})
	}
	writeJSON(w, ResolversResponse{
		Resolvers: infos,
		Count:     s.pool.Count(),
		Healthy:   s.pool.CountHealthy(),
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := "healthy"
	if s.monitor != nil && !s.monitor.IsHealthy() {
		status = "unhealthy"
	}
	writeJSON(w, HealthResponse{
		Status:    status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resolvers := s.pool.All()
	var healthy, degraded, blocked, unknown int
	for _, res := range resolvers {
		switch res.Status {
		case resolver.StatusHealthy:
			healthy++
		case resolver.StatusDegraded:
			degraded++
		case resolver.StatusBlocked:
			blocked++
		default:
			unknown++
		}
	}
	monitorStatus, monitorHealthy := "unknown", false
	if s.monitor != nil {
		monitorHealthy = s.monitor.IsHealthy()
		switch s.monitor.Status() {
		case health.StatusHealthy:
			monitorStatus = "healthy"
		case health.StatusDegraded:
			monitorStatus = "degraded"
		case health.StatusUnhealthy:
			monitorStatus = "unhealthy"
		}
	}
	writeJSON(w, StatsResponse{
		ResolverCount:  len(resolvers),
		HealthyCount:   healthy,
		DegradedCount:  degraded,
		BlockedCount:   blocked,
		UnknownCount:   unknown,
		PoolExhausted:  s.pool.IsExhausted(),
		MonitorStatus:  monitorStatus,
		MonitorHealthy: monitorHealthy,
	})
}

func statusToString(s resolver.Status) string {
	switch s {
	case resolver.StatusHealthy:
		return "healthy"
	case resolver.StatusDegraded:
		return "degraded"
	case resolver.StatusBlocked:
		return "blocked"
	default:
		return "unknown"
	}
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[api] Error encoding JSON: %v", err)
	}
}
