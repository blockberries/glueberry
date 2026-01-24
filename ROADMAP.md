# Glueberry Future Roadmap

This document outlines planned improvements and future directions for Glueberry. Items are organized by category and priority level.

---

## Executive Summary

Glueberry v1.0 is a production-ready P2P communication library with:
- Secure Ed25519/X25519/ChaCha20-Poly1305 encryption
- Symmetric event-driven handshake API
- Lazy stream opening with automatic reconnection
- Comprehensive test coverage (217+ tests)

This roadmap identifies opportunities to enhance **observability**, **performance**, **reliability**, and **developer experience** while maintaining the library's core design principles.

---

## Priority Levels

- **P0 (Critical)**: Essential for production deployments
- **P1 (High)**: Significant value for most users
- **P2 (Medium)**: Nice-to-have improvements
- **P3 (Low)**: Future considerations

---

## 1. Observability & Debugging

### 1.1 Structured Logging (P1)
**Current State:** No built-in logging mechanism.

**Proposed Improvements:**
- [ ] Add pluggable structured logging interface (compatible with slog, zap, zerolog)
- [ ] Log key lifecycle events: connections, handshakes, stream operations
- [ ] Support log levels: Debug, Info, Warn, Error
- [ ] Ensure no sensitive data (keys, plaintext) in logs
- [ ] Add correlation IDs for tracing related operations

**Example API:**
```go
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Info(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
    Error(msg string, keysAndValues ...any)
}

cfg := glueberry.NewConfig(
    privateKey, addrBookPath, listenAddrs,
    glueberry.WithLogger(myLogger),
)
```

**Benefits:**
- Production debugging capabilities
- Troubleshooting connection issues
- Audit trail for security events

---

### 1.2 Prometheus Metrics (P1)
**Current State:** No metrics collection.

**Proposed Metrics:**
- [ ] **Connection Metrics:**
  - `glueberry_connections_total{state}` - Connections by state
  - `glueberry_connection_attempts_total{result}` - Success/failure counts
  - `glueberry_reconnection_attempts_total` - Reconnection attempt counter
  - `glueberry_handshake_duration_seconds` - Histogram of handshake times
  - `glueberry_handshake_timeouts_total` - Handshake timeout counter

- [ ] **Stream Metrics:**
  - `glueberry_streams_active{stream_name}` - Active streams gauge
  - `glueberry_messages_sent_total{stream_name}` - Messages sent
  - `glueberry_messages_received_total{stream_name}` - Messages received
  - `glueberry_message_bytes_sent_total{stream_name}` - Bytes sent
  - `glueberry_message_bytes_received_total{stream_name}` - Bytes received

- [ ] **Crypto Metrics:**
  - `glueberry_key_cache_hits_total` - Shared key cache hits
  - `glueberry_key_cache_misses_total` - Shared key cache misses
  - `glueberry_encryption_errors_total` - Encryption failures
  - `glueberry_decryption_errors_total` - Decryption failures

- [ ] **Event Metrics:**
  - `glueberry_events_emitted_total{state}` - Events by type
  - `glueberry_events_dropped_total` - Dropped events (buffer full)
  - `glueberry_messages_dropped_total` - Dropped messages (buffer full)

**Implementation:**
```go
type Metrics interface {
    CounterVec(name string, labels []string) CounterVec
    GaugeVec(name string, labels []string) GaugeVec
    HistogramVec(name string, labels []string, buckets []float64) HistogramVec
}

cfg := glueberry.NewConfig(
    privateKey, addrBookPath, listenAddrs,
    glueberry.WithMetrics(prometheusMetrics),
)
```

**Benefits:**
- Production monitoring dashboards
- Alerting on connection failures
- Performance analysis

---

### 1.3 OpenTelemetry Tracing (P2)
**Current State:** No distributed tracing support.

**Proposed Improvements:**
- [ ] Add optional OpenTelemetry instrumentation
- [ ] Trace spans for: Connect, Handshake, Send, Receive
- [ ] Propagate trace context across handshake messages
- [ ] Link traces between connected peers

**Benefits:**
- End-to-end visibility in distributed systems
- Performance bottleneck identification
- Cross-peer correlation

---

### 1.4 Debug Mode (P2)
**Current State:** No debugging utilities.

**Proposed Improvements:**
- [ ] Add `WithDebugMode(enabled bool)` configuration option
- [ ] Capture detailed timing information
- [ ] Dump connection state on demand
- [ ] Export diagnostics endpoint for inspection

**Example API:**
```go
node.DumpState() // Returns JSON with all connection states, stream info, etc.
node.DumpPeerConnections(peerID) // Detailed info for specific peer
```

---

## 2. Performance & Scalability

### 2.1 Benchmarking Suite (P1)
**Current State:** Only cipher benchmarks exist (`cipher_test.go`).

**Proposed Benchmarks:**
- [ ] **Micro-benchmarks:**
  - Key derivation (Ed25519 → X25519, ECDH, HKDF)
  - Message encryption/decryption at various sizes (1B, 1KB, 64KB, 1MB)
  - Cramberry serialization/deserialization

- [ ] **Macro-benchmarks:**
  - Full connection establishment time
  - Handshake completion time
  - Message throughput (messages/second)
  - Latency distribution (p50, p90, p99)

- [ ] **Scalability benchmarks:**
  - Concurrent connections (10, 100, 1000 peers)
  - Concurrent message sends
  - Memory usage under load

**Deliverables:**
- `make benchmark` target
- Benchmark results in CI
- Performance regression detection

---

### 2.2 Connection Pooling (P2)
**Current State:** One connection per peer.

**Proposed Improvements:**
- [ ] Optional multiple connections per peer for higher throughput
- [ ] Load balancing across connections
- [ ] Graceful connection rotation

**Use Cases:**
- High-throughput data transfer
- Redundancy for critical peers

---

### 2.3 Message Batching (P2)
**Current State:** Each message sent individually.

**Proposed Improvements:**
- [ ] Optional message batching with configurable flush interval
- [ ] Nagle-style algorithm for small message coalescing
- [ ] Explicit flush API for latency-sensitive messages

**Benefits:**
- Reduced syscall overhead
- Improved throughput for small messages
- Lower CPU usage

---

### 2.4 Zero-Copy Optimizations (P3)
**Current State:** Standard buffer copying throughout.

**Proposed Improvements:**
- [ ] Investigate buffer pooling for message buffers
- [ ] Reduce allocations in hot paths
- [ ] Profile and optimize garbage collection pressure

---

### 2.5 Compression (P2)
**Current State:** No message compression.

**Proposed Improvements:**
- [ ] Optional per-stream compression (LZ4, zstd, snappy)
- [ ] Compression negotiation during handshake
- [ ] Configurable compression threshold (skip for small messages)

**Example API:**
```go
node.PrepareStreams(peerID, pubKey, []glueberry.StreamConfig{
    {Name: "bulk-data", Compression: glueberry.CompressionLZ4},
    {Name: "control", Compression: glueberry.CompressionNone},
})
```

---

## 3. Reliability & Resilience

### 3.1 Message Acknowledgments (P1)
**Current State:** Fire-and-forget messaging only.

**Proposed Improvements:**
- [ ] Optional per-message acknowledgments
- [ ] Configurable timeout for ack reception
- [ ] Automatic retransmission on timeout
- [ ] Delivery status callbacks

**Example API:**
```go
// Option 1: Blocking with timeout
err := node.SendWithAck(peerID, stream, data, 5*time.Second)

// Option 2: Async with callback
node.SendAsync(peerID, stream, data, func(ack *AckResult) {
    if ack.Err != nil {
        // Handle failure
    }
})

// Option 3: Context-based
ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
err := node.SendWithAckCtx(ctx, peerID, stream, data)
```

**Benefits:**
- Guaranteed delivery for critical messages
- Better error handling
- Idempotency support

---

### 3.2 Flow Control & Backpressure (P1)
**Current State:** Unbounded message buffering; events dropped if channel full.

**Proposed Improvements:**
- [ ] Implement per-stream flow control
- [ ] Configurable high/low watermarks
- [ ] Backpressure signaling to sender
- [ ] Optional blocking send when buffer full
- [ ] Metrics for buffer utilization

**Benefits:**
- Prevents memory exhaustion
- Better handling of slow consumers
- More predictable behavior under load

---

### 3.3 Circuit Breaker (P2)
**Current State:** Basic exponential backoff for reconnection.

**Proposed Improvements:**
- [ ] Circuit breaker pattern for repeated failures
- [ ] States: Closed (normal), Open (failing), Half-Open (testing)
- [ ] Configurable thresholds and timeouts
- [ ] Per-peer circuit breakers

**Benefits:**
- Faster failure detection
- Prevents cascade failures
- Resource protection

---

### 3.4 Graceful Degradation (P2)
**Current State:** All-or-nothing stream establishment.

**Proposed Improvements:**
- [ ] Continue operation when some streams fail
- [ ] Partial stream recovery
- [ ] Health status per stream

---

### 3.5 Request/Response Pattern (P1)
**Current State:** Message-oriented only; no built-in request/response.

**Proposed Improvements:**
- [ ] Optional request/response layer
- [ ] Automatic request ID generation and correlation
- [ ] Timeout handling for pending requests
- [ ] Concurrent request limiting

**Example API:**
```go
// Synchronous request/response
resp, err := node.Request(peerID, stream, request, 5*time.Second)

// Async request/response
node.RequestAsync(peerID, stream, request, func(resp []byte, err error) {
    // Handle response
})

// Server-side: Register request handler
node.HandleRequests(stream, func(req IncomingRequest) ([]byte, error) {
    return processRequest(req.Data)
})
```

**Benefits:**
- RPC-style communication
- Automatic response correlation
- Timeout handling

---

## 4. Security Enhancements

### 4.1 Protocol Versioning (P1)
**Current State:** Protocol IDs hardcoded (e.g., `/glueberry/handshake/1.0.0`).

**Proposed Improvements:**
- [ ] Formal protocol version negotiation during handshake
- [ ] Backwards compatibility mechanism
- [ ] Version mismatch detection and error handling
- [ ] Minimum supported version configuration

**Benefits:**
- Safe protocol upgrades
- Gradual rollout support
- Clear error messages for version mismatches

---

### 4.2 Peer Identity Verification (P2)
**Current State:** App responsible for peer authentication.

**Proposed Improvements:**
- [ ] Optional built-in peer verification callback
- [ ] Certificate chain validation hooks
- [ ] Signature verification helpers

**Example API:**
```go
cfg := glueberry.NewConfig(
    privateKey, addrBookPath, listenAddrs,
    glueberry.WithPeerVerifier(func(peerID peer.ID, pubKey ed25519.PublicKey) error {
        // Verify peer identity against trusted source
        if !trustedKeys.Contains(pubKey) {
            return errors.New("untrusted peer")
        }
        return nil
    }),
)
```

---

### 4.3 Key Rotation (P2)
**Current State:** Keys are static for node lifetime.

**Proposed Improvements:**
- [ ] Support for session key rotation
- [ ] Configurable rotation interval
- [ ] Seamless key transition without connection interruption

**Benefits:**
- Limit exposure from key compromise
- Forward secrecy improvements
- Compliance requirements

---

### 4.4 Replay Protection (P2)
**Current State:** No replay protection (app responsibility).

**Proposed Improvements:**
- [ ] Optional sequence numbers for messages
- [ ] Sliding window for sequence validation
- [ ] Configurable window size
- [ ] Replay detection callbacks

**Benefits:**
- Protection against replay attacks
- Message ordering guarantees (optional)

---

### 4.5 Rate Limiting (P2)
**Current State:** No rate limiting.

**Proposed Improvements:**
- [ ] Per-peer rate limiting
- [ ] Per-stream rate limiting
- [ ] Configurable limits (messages/sec, bytes/sec)
- [ ] Soft limits with backpressure vs hard limits with rejection

**Benefits:**
- DoS protection
- Fair resource allocation
- Bandwidth management

---

### 4.6 Security Audit (P1)
**Current State:** No formal security audit.

**Proposed:**
- [ ] Engage third-party security auditors
- [ ] Focus areas: cryptographic implementation, input validation, resource exhaustion
- [ ] Address findings before v1.0 release
- [ ] Publish audit report

---

## 5. Peer Management

### 5.1 Peer Scoring (P2)
**Current State:** Binary peer status (connected/blacklisted).

**Proposed Improvements:**
- [ ] Numeric peer score based on behavior
- [ ] Scoring factors: latency, uptime, message success rate
- [ ] Automatic score decay over time
- [ ] Configurable score thresholds for actions (disconnect, deprioritize)

**Example API:**
```go
score := node.PeerScore(peerID)

cfg := glueberry.NewConfig(
    privateKey, addrBookPath, listenAddrs,
    glueberry.WithPeerScoring(glueberry.ScoringConfig{
        DecayInterval: 1 * time.Hour,
        DisconnectThreshold: -100,
        InitialScore: 0,
    }),
)
```

**Benefits:**
- Quality-based peer selection
- Automatic bad peer detection
- Network health improvement

---

### 5.2 Peer Discovery Integration (P2)
**Current State:** Peer discovery is app responsibility.

**Proposed Improvements:**
- [ ] Optional built-in mDNS discovery for local networks
- [ ] Optional DHT integration for global discovery
- [ ] Discovery event callbacks
- [ ] Configurable discovery intervals

**Note:** Keep as optional to maintain library's minimal philosophy.

**Example API:**
```go
cfg := glueberry.NewConfig(
    privateKey, addrBookPath, listenAddrs,
    glueberry.WithMDNSDiscovery(true),
    glueberry.WithDHTDiscovery(dhtClient),
)

for peer := range node.DiscoveredPeers() {
    // New peer discovered
}
```

---

### 5.3 Peer Groups (P3)
**Current State:** Flat peer list.

**Proposed Improvements:**
- [ ] Group peers by app-defined categories
- [ ] Group-level operations (connect all, disconnect all)
- [ ] Group-based message broadcasting

---

### 5.4 Peer Statistics (P1)
**Current State:** Basic `LastSeen` timestamp only.

**Proposed Improvements:**
- [ ] Track per-peer statistics:
  - Total connection time
  - Messages sent/received
  - Bytes sent/received
  - Connection failures
  - Average latency
- [ ] Persist statistics in address book
- [ ] API to query statistics

**Example API:**
```go
stats := node.PeerStats(peerID)
// stats.MessagesReceived, stats.TotalBytesReceived, stats.AvgLatency, etc.
```

---

## 6. API Improvements

### 6.1 Context Support (P1)
**Current State:** Limited context support; timeout via configuration.

**Proposed Improvements:**
- [ ] Add context parameter to all blocking operations
- [ ] Support cancellation via context
- [ ] Respect context deadlines

**Example API:**
```go
ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
defer cancel()

err := node.ConnectCtx(ctx, peerID)
err := node.SendCtx(ctx, peerID, stream, data)
```

---

### 6.2 Streaming API (P2)
**Current State:** Discrete message API only.

**Proposed Improvements:**
- [ ] Optional streaming interface for large data transfers
- [ ] io.Reader/io.Writer wrappers for streams
- [ ] Chunked transfer support

**Example API:**
```go
stream, err := node.OpenStream(peerID, "file-transfer")
defer stream.Close()

// io.Reader/io.Writer interface
io.Copy(stream, fileReader)
io.Copy(fileWriter, stream)
```

---

### 6.3 Broadcast/Multicast (P2)
**Current State:** Point-to-point only.

**Proposed Improvements:**
- [ ] Broadcast to all connected peers
- [ ] Multicast to peer groups
- [ ] Configurable delivery semantics (best-effort, all-or-none)

**Example API:**
```go
results := node.Broadcast(streamName, data)
for peerID, err := range results {
    if err != nil {
        // Handle per-peer failure
    }
}

node.Multicast(peerGroup, streamName, data)
```

---

### 6.4 Typed Message Support (P2)
**Current State:** Raw byte slices for messages.

**Proposed Improvements:**
- [ ] Generic typed message wrapper using Cramberry
- [ ] Type-safe send/receive with automatic serialization
- [ ] Message type registry per stream

**Example API:**
```go
type ChatMessage struct {
    From    string
    Content string
}

// Type-safe send
node.SendTyped(peerID, "chat", &ChatMessage{From: "alice", Content: "hello"})

// Type-safe receive
for msg := range node.TypedMessages[ChatMessage]("chat") {
    fmt.Printf("%s: %s\n", msg.Data.From, msg.Data.Content)
}
```

---

### 6.5 Event Filtering (P1)
**Current State:** All events sent to single channel.

**Proposed Improvements:**
- [ ] Filter events by peer ID
- [ ] Filter events by state
- [ ] Multiple event subscriber support

**Example API:**
```go
// Only events for specific peer
events := node.EventsFor(peerID)

// Only specific state changes
events := node.Events(glueberry.FilterStates(StateConnected, StateEstablished))
```

---

## 7. Testing & Quality

### 7.1 Fuzz Testing (P1)
**Current State:** No fuzz testing.

**Proposed Improvements:**
- [ ] Fuzz test Cramberry message parsing
- [ ] Fuzz test handshake protocol
- [ ] Fuzz test encrypted message handling
- [ ] Add to CI pipeline

**Benefits:**
- Find edge cases and crashes
- Security vulnerability detection
- Robustness improvement

---

### 7.2 Chaos Testing (P2)
**Current State:** No chaos/fault injection testing.

**Proposed Improvements:**
- [ ] Simulate network partitions
- [ ] Inject message corruption
- [ ] Simulate slow networks (latency, packet loss)
- [ ] Test reconnection under various failure modes

---

### 7.3 Load Testing (P2)
**Current State:** No load testing suite.

**Proposed Improvements:**
- [ ] Create load testing scenarios
- [ ] Test with 100+ concurrent peers
- [ ] Test with high message throughput
- [ ] Memory and CPU profiling under load

---

### 7.4 Property-Based Testing (P3)
**Current State:** Traditional example-based tests.

**Proposed Improvements:**
- [ ] Add property-based tests for crypto operations
- [ ] Test serialization round-trip properties
- [ ] Test state machine invariants

---

### 7.5 Integration Test Coverage (P1)
**Current State:** One integration test for basic flow.

**Proposed Improvements:**
- [ ] Add multi-peer scenarios (3+ nodes)
- [ ] Test network topology changes
- [ ] Test long-running connections
- [ ] Test graceful shutdown under load
- [ ] Test handler cleanup on disconnect (noted TODO in PROGRESS_REPORT.md)

---

## 8. Developer Experience

### 8.1 CLI Tool (P2)
**Current State:** No CLI tool.

**Proposed Tool:**
- [ ] `glueberry-cli` for testing and debugging
- [ ] Commands: `keygen`, `connect`, `send`, `listen`, `peers`
- [ ] Interactive mode for testing handshake protocols

**Example Usage:**
```bash
# Generate identity key
glueberry-cli keygen -o key.pem

# Connect and send message
glueberry-cli connect --key key.pem --peer /ip4/127.0.0.1/tcp/9000/p2p/QmXXX

# Listen for connections
glueberry-cli listen --key key.pem --addr /ip4/0.0.0.0/tcp/9000
```

---

### 8.2 Better Error Messages (P1)
**Current State:** Basic sentinel errors.

**Proposed Improvements:**
- [ ] Rich error types with additional context
- [ ] Error codes for programmatic handling
- [ ] Suggested remediation in error messages
- [ ] Error classification (retriable, fatal, configuration)

**Example:**
```go
type GlueberryError struct {
    Code      ErrorCode
    Message   string
    PeerID    peer.ID
    Stream    string
    Cause     error
    Retriable bool
}

err := node.Send(peerID, stream, data)
if gErr, ok := err.(*GlueberryError); ok {
    if gErr.Retriable {
        // Retry logic
    }
}
```

---

### 8.3 Mock/Fake for Testing (P2)
**Current State:** No testing utilities provided.

**Proposed Improvements:**
- [ ] Provide mock `Node` implementation for unit testing
- [ ] In-memory transport for integration testing without network
- [ ] Event/message injection utilities

**Example API:**
```go
import "github.com/blockberries/glueberry/testing"

func TestMyApp(t *testing.T) {
    node := testing.NewMockNode()
    node.SimulateConnect(peerID)
    node.SimulateMessage(peerID, "stream", []byte("test"))

    // Verify app behavior
}
```

---

### 8.4 Migration Guides (P2)
**Current State:** Basic documentation.

**Proposed Improvements:**
- [ ] Version migration guides (when breaking changes occur)
- [ ] Common pitfalls documentation
- [ ] Best practices guide

---

### 8.5 More Examples (P1)
**Current State:** Two examples (basic, simple-chat).

**Proposed Examples:**
- [ ] **file-transfer**: Large file transfer with progress
- [ ] **pubsub**: Simple publish/subscribe pattern
- [ ] **discovery**: Peer discovery with mDNS
- [ ] **rpc**: Request/response pattern
- [ ] **cluster**: Multi-node cluster coordination

---

## 9. Infrastructure & Build

### 9.1 CI/CD Pipeline (P1)
**Current State:** No CI configuration visible.

**Proposed Improvements:**
- [ ] GitHub Actions workflow
- [ ] Run tests on multiple Go versions
- [ ] Run tests on multiple platforms (Linux, macOS, Windows)
- [ ] Lint check on PR
- [ ] Coverage reporting
- [ ] Benchmark comparison on PR

---

### 9.2 Release Automation (P1)
**Current State:** No release process documented.

**Proposed Improvements:**
- [ ] Semantic versioning
- [ ] Automated changelog generation
- [ ] Release notes template
- [ ] Go module versioning (v1.x.x)

---

### 9.3 Documentation Generation (P2)
**Current State:** Manual documentation.

**Proposed Improvements:**
- [ ] Automated godoc hosting
- [ ] API reference website
- [ ] Diagram generation from code

---

### 9.4 Dependency Management (P1)
**Current State:** Local replace directive for cramberry.

**Proposed Improvements:**
- [ ] Publish cramberry as separate module
- [ ] Remove local replace directives before v1.0
- [ ] Regular dependency updates
- [ ] Vulnerability scanning

---

## 10. Protocol Enhancements

### 10.1 Multiple Protocol Versions (P2)
**Current State:** Single protocol version.

**Proposed Improvements:**
- [ ] Support multiple concurrent protocol versions
- [ ] Version negotiation during handshake
- [ ] Deprecation warnings for old versions

---

### 10.2 Custom Handshake Extensions (P2)
**Current State:** Fixed handshake message types.

**Proposed Improvements:**
- [ ] Extensible handshake message format
- [ ] App-defined extension fields
- [ ] Negotiate extensions during handshake

---

### 10.3 Stream Priorities (P3)
**Current State:** All streams treated equally.

**Proposed Improvements:**
- [ ] Assign priority levels to streams
- [ ] Priority-based scheduling
- [ ] Quality of Service (QoS) support

---

### 10.4 Keep-Alive Mechanism (P2)
**Current State:** No explicit keep-alive.

**Proposed Improvements:**
- [ ] Configurable keep-alive interval
- [ ] Detect dead connections faster
- [ ] Optional keep-alive pings

---

## Implementation Phases

### Phase A: Production Readiness (P0-P1 items)
**Goal:** Ensure Glueberry is production-ready with essential observability.

1. Structured Logging (1.1)
2. Prometheus Metrics (1.2)
3. Benchmarking Suite (2.1)
4. Protocol Versioning (4.1)
5. Security Audit (4.6)
6. Fuzz Testing (7.1)
7. Integration Test Coverage (7.5)
8. CI/CD Pipeline (9.1)
9. Release Automation (9.2)
10. Better Error Messages (8.2)
11. Peer Statistics (5.4)
12. Context Support (6.1)
13. Event Filtering (6.5)
14. More Examples (8.5)
15. Dependency Management (9.4)

### Phase B: Reliability Features (P1-P2 items)
**Goal:** Improve reliability for high-stakes applications.

1. Message Acknowledgments (3.1)
2. Flow Control & Backpressure (3.2)
3. Request/Response Pattern (3.5)
4. Circuit Breaker (3.3)

### Phase C: Performance & Scale (P2 items)
**Goal:** Optimize for high-throughput scenarios.

1. Connection Pooling (2.2)
2. Message Batching (2.3)
3. Compression (2.5)
4. Load Testing (7.3)

### Phase D: Advanced Features (P2-P3 items)
**Goal:** Add advanced capabilities for power users.

1. Peer Scoring (5.1)
2. Peer Discovery Integration (5.2)
3. Key Rotation (4.3)
4. Rate Limiting (4.5)
5. Streaming API (6.2)
6. Broadcast/Multicast (6.3)
7. CLI Tool (8.1)

---

## Success Metrics

### Adoption Metrics
- GitHub stars and forks
- Number of dependent projects
- Community contributions

### Quality Metrics
- Test coverage (target: >85%)
- Zero critical/high vulnerabilities
- Benchmark regression detection

### Performance Metrics
- Handshake latency < 100ms (local network)
- Message throughput > 10,000 msg/sec (single peer)
- Support 1,000+ concurrent connections

---

## Contributing

We welcome contributions to any roadmap item. Before starting work:

1. Check existing issues for the item
2. Create an issue if none exists
3. Discuss approach in the issue
4. Submit PR referencing the issue

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

---

## Revision History

| Date | Version | Changes |
|------|---------|---------|
| 2026-01-24 | 1.0 | Initial roadmap created |

---

*This roadmap is a living document and will be updated as priorities evolve.*
