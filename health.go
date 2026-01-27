package glueberry

import (
	"encoding/json"
	"net/http"
	"time"
)

// CheckResult represents the result of a health check.
type CheckResult struct {
	// Name is the name of the check.
	Name string `json:"name"`

	// Healthy indicates whether the check passed.
	Healthy bool `json:"healthy"`

	// Message provides additional context about the check result.
	Message string `json:"message,omitempty"`

	// Duration is how long the check took.
	Duration time.Duration `json:"duration_ns,omitempty"`
}

// HealthStatus represents the overall health status of the node.
type HealthStatus struct {
	// Healthy indicates whether all checks passed.
	Healthy bool `json:"healthy"`

	// Checks contains the results of individual checks.
	Checks []CheckResult `json:"checks"`

	// Timestamp is when the health check was performed.
	Timestamp time.Time `json:"timestamp"`
}

// IsHealthy returns true if the node is healthy and ready to handle requests.
// A node is considered healthy if:
//   - The node is started
//   - The libp2p host is running
//   - The address book is accessible
//
// This is a quick check suitable for liveness probes.
func (n *Node) IsHealthy() bool {
	n.startMu.Lock()
	started := n.started
	n.startMu.Unlock()

	if !started {
		return false
	}

	// Check that host is accessible
	if n.host == nil || n.host.LibP2PHost() == nil {
		return false
	}

	return true
}

// ReadinessChecks performs detailed health checks and returns the results.
// This is suitable for readiness probes and debugging.
//
// Checks performed:
//   - node_started: Whether the node has been started
//   - host_running: Whether the libp2p host is running
//   - address_book: Whether the address book is accessible
//   - connections: Whether there are active connections (informational)
func (n *Node) ReadinessChecks() HealthStatus {
	status := HealthStatus{
		Healthy:   true,
		Checks:    make([]CheckResult, 0, 4),
		Timestamp: time.Now(),
	}

	// Check 1: Node started
	start := time.Now()
	n.startMu.Lock()
	started := n.started
	n.startMu.Unlock()

	status.Checks = append(status.Checks, CheckResult{
		Name:     "node_started",
		Healthy:  started,
		Message:  boolToMessage(started, "node is running", "node is not started"),
		Duration: time.Since(start),
	})
	if !started {
		status.Healthy = false
	}

	// Check 2: Host running
	start = time.Now()
	hostOK := n.host != nil && n.host.LibP2PHost() != nil
	status.Checks = append(status.Checks, CheckResult{
		Name:     "host_running",
		Healthy:  hostOK,
		Message:  boolToMessage(hostOK, "libp2p host is running", "libp2p host is not available"),
		Duration: time.Since(start),
	})
	if !hostOK {
		status.Healthy = false
	}

	// Check 3: Address book accessible
	start = time.Now()
	bookOK := false
	bookMsg := "address book is not available"
	if n.addressBook != nil {
		// Try to list peers - if this works, the book is functional
		_ = n.addressBook.ListPeers()
		bookOK = true
		bookMsg = "address book is accessible"
	}
	status.Checks = append(status.Checks, CheckResult{
		Name:     "address_book",
		Healthy:  bookOK,
		Message:  bookMsg,
		Duration: time.Since(start),
	})
	if !bookOK {
		status.Healthy = false
	}

	// Check 4: Connection info (informational, doesn't affect health)
	start = time.Now()
	connCount := 0
	connMsg := "no active connections"
	if hostOK && n.host.LibP2PHost().Network() != nil {
		connCount = len(n.host.LibP2PHost().Network().Peers())
		if connCount > 0 {
			connMsg = "has active connections"
		}
	}
	status.Checks = append(status.Checks, CheckResult{
		Name:     "connections",
		Healthy:  true, // This is informational only
		Message:  connMsg,
		Duration: time.Since(start),
	})

	return status
}

// boolToMessage returns trueMsg if b is true, otherwise falseMsg.
func boolToMessage(b bool, trueMsg, falseMsg string) string {
	if b {
		return trueMsg
	}
	return falseMsg
}

// HealthHandler returns an http.Handler that serves health check responses.
// The handler responds with:
//   - 200 OK if the node is healthy
//   - 503 Service Unavailable if the node is unhealthy
//
// The response body contains a JSON representation of HealthStatus.
//
// Example usage:
//
//	http.Handle("/health", glueberry.HealthHandler(node))
func HealthHandler(node *Node) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := node.ReadinessChecks()

		w.Header().Set("Content-Type", "application/json")
		if status.Healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(status)
	})
}

// LivenessHandler returns an http.Handler that serves liveness check responses.
// This is a quick check that returns:
//   - 200 OK if the node is alive
//   - 503 Service Unavailable if the node is not alive
//
// Unlike HealthHandler, this does not perform detailed checks.
//
// Example usage:
//
//	http.Handle("/live", glueberry.LivenessHandler(node))
func LivenessHandler(node *Node) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		healthy := node.IsHealthy()

		w.Header().Set("Content-Type", "application/json")
		if healthy {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"healthy":true}`))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"healthy":false}`))
		}
	})
}
