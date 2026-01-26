# Glueberry Roadmap

This roadmap outlines planned improvements, enhancements, and new features for Glueberry. Items are prioritized based on security impact, user demand, and implementation complexity.

**Current Version:** 1.0.1
**Protocol Version:** 1.0.0
**Last Updated:** 2026-01-26

---

## Version 1.1.0 - Security Hardening Release

**Target:** Q1 2026
**Theme:** Address security review findings and harden cryptographic operations

### High Priority

#### 1.1.1 Key Material Memory Protection
**Status:** Planned
**Priority:** HIGH
**Complexity:** Medium

Implement secure memory handling for sensitive key material:

- [ ] Add `SecureZero()` function using `memguard` or manual memory clearing
- [ ] Zero shared keys on `RemovePeerKey()` and peer disconnect
- [ ] Zero private keys on `Node.Stop()`
- [ ] Implement `SecureBytes` type that auto-zeros on GC finalization
- [ ] Add optional `mlock()` support for critical memory regions (Linux/macOS)
- [ ] Document memory security guarantees

**Rationale:** Keys currently remain in memory after use. While only exploitable with memory access, this matters for high-security deployments.

#### 1.1.2 Custom Decryption Error Callback
**Status:** Planned
**Priority:** HIGH
**Complexity:** Low

Expose decryption error callback to applications:

- [ ] Add `WithDecryptionErrorCallback(func(peer.ID, error))` config option
- [ ] Allow applications to implement custom tampering detection/response
- [ ] Document use cases (alerting, rate limiting, peer banning)

**Rationale:** Currently hardcoded internally. Applications need visibility into tampering attempts.

#### 1.1.3 Connection Limits and Rate Limiting
**Status:** Planned
**Priority:** HIGH
**Complexity:** Medium

Prevent resource exhaustion attacks:

- [ ] Add `MaxPeers` config option (limit concurrent connections)
- [ ] Add `MaxPendingConnections` for connection queue limits
- [ ] Implement per-peer message rate limiting
- [ ] Add `MaxStreamsPerPeer` limit
- [ ] Add connection attempt rate limiting per peer

**Rationale:** No limits exist for connection or message rates, enabling potential DoS.

### Medium Priority

#### 1.1.4 Nonce Exhaustion Warning
**Status:** Planned
**Priority:** MEDIUM
**Complexity:** Low

Add warnings for long-lived connections:

- [ ] Track message count per peer connection
- [ ] Emit warning at configurable threshold (default: 2^40 messages)
- [ ] Add `MessagesSent(peer.ID) uint64` API method
- [ ] Document nonce collision risk and mitigation

**Rationale:** Random nonces have birthday bound at ~2^48. Long-lived connections need monitoring.

#### 1.1.5 Enhanced Input Validation
**Status:** Planned
**Priority:** MEDIUM
**Complexity:** Low

Strengthen input validation:

- [ ] Add stream name validation (alphanumeric + hyphen only)
- [ ] Validate metadata keys/values length limits
- [ ] Add configurable `MaxMetadataSize`
- [ ] Reject oversized public keys in handshake

---

## Version 1.2.0 - Observability & Operations Release

**Target:** Q2 2026
**Theme:** Production-grade monitoring and operational tooling

### High Priority

#### 1.2.1 OpenTelemetry Integration
**Status:** Planned
**Priority:** HIGH
**Complexity:** Medium

Add distributed tracing support:

- [ ] Define span hierarchy (Connection → Handshake → Message)
- [ ] Add `WithTracerProvider()` config option
- [ ] Trace connection lifecycle events
- [ ] Trace message send/receive with timing
- [ ] Propagate trace context in messages (opt-in)

#### 1.2.2 Prometheus Metrics Adapter
**Status:** Planned
**Priority:** HIGH
**Complexity:** Low

Provide ready-to-use Prometheus implementation:

- [ ] Create `prometheus/` subpackage
- [ ] Implement all Metrics interface methods with prometheus types
- [ ] Add standard metric naming (glueberry_connections_total, etc.)
- [ ] Include Grafana dashboard JSON template
- [ ] Document metrics and alert recommendations

#### 1.2.3 Health Check API
**Status:** Planned
**Priority:** MEDIUM
**Complexity:** Low

Add liveness/readiness probes:

- [ ] `IsHealthy() bool` - node operational check
- [ ] `ReadinessChecks() []CheckResult` - detailed component status
- [ ] Include crypto module, address book, connection manager status
- [ ] Add HTTP handler wrapper for Kubernetes integration

### Medium Priority

#### 1.2.4 Structured Event Streaming
**Status:** Planned
**Priority:** MEDIUM
**Complexity:** Medium

Enhanced event system for monitoring:

- [ ] Add event severity levels (Info, Warning, Error)
- [ ] Add event categories (Connection, Crypto, Stream, Security)
- [ ] Add `StreamEvents()` for high-volume event streaming
- [ ] Add event sampling for high-throughput deployments
- [ ] Add event JSON serialization for log aggregation

#### 1.2.5 Connection Diagnostics
**Status:** Planned
**Priority:** MEDIUM
**Complexity:** Low

Enhanced debugging capabilities:

- [ ] Add `PingPeer(peer.ID) (latency, error)` method
- [ ] Add `NetworkInfo()` returning NAT type, relay status
- [ ] Add `BandwidthStats(peer.ID)` for throughput monitoring
- [ ] Expose libp2p identify info for connected peers

---

## Version 1.3.0 - Protocol Enhancements Release

**Target:** Q3 2026
**Theme:** Advanced protocol features for demanding use cases

### High Priority

#### 1.3.1 Session Resumption
**Status:** Research
**Priority:** HIGH
**Complexity:** High

Resume connections without full handshake:

- [ ] Design session ticket format with expiration
- [ ] Implement session ticket encryption (per-node secret)
- [ ] Add `WithSessionCache()` config option
- [ ] Define session resumption protocol extension
- [ ] Implement abbreviated handshake flow
- [ ] Add session ticket rotation

**Rationale:** Full ECDH on every reconnect adds latency. Session resumption enables faster reconnects.

#### 1.3.2 Message Priorities and QoS
**Status:** Planned
**Priority:** MEDIUM
**Complexity:** Medium

Quality of service for messages:

- [ ] Add `SendWithPriority(peer, stream, data, priority)` method
- [ ] Implement priority queues in flow controller
- [ ] Add configurable priority levels (High, Normal, Low)
- [ ] Preemptive scheduling for high-priority messages
- [ ] Document priority semantics and guarantees

#### 1.3.3 Multicast/Broadcast Support
**Status:** Research
**Priority:** MEDIUM
**Complexity:** High

Send to multiple peers efficiently:

- [ ] Add `Broadcast(streamName, data) []error` method
- [ ] Add `SendToMany([]peer.ID, streamName, data) []error`
- [ ] Implement parallel sends with configurable concurrency
- [ ] Add broadcast result aggregation
- [ ] Consider gossip-based propagation for large peer sets

### Medium Priority

#### 1.3.4 Protocol Negotiation
**Status:** Planned
**Priority:** MEDIUM
**Complexity:** Medium

Flexible protocol version handling:

- [ ] Add protocol capability advertisement
- [ ] Implement feature negotiation during handshake
- [ ] Support optional feature extensions
- [ ] Add `RequiredCapabilities` config option
- [ ] Document capability system

#### 1.3.5 Message Acknowledgments
**Status:** Research
**Priority:** LOW
**Complexity:** Medium

Optional delivery confirmation:

- [ ] Design ack protocol (optional per-stream)
- [ ] Add `SendWithAck(ctx, peer, stream, data) error`
- [ ] Implement timeout-based retry
- [ ] Track delivery statistics
- [ ] Document reliability guarantees

---

## Version 1.4.0 - Enterprise Features Release

**Target:** Q4 2026
**Theme:** Features for large-scale and enterprise deployments

### High Priority

#### 1.4.1 Circuit Breaker Pattern
**Status:** Planned
**Priority:** HIGH
**Complexity:** Medium

Prevent cascading failures:

- [ ] Implement per-peer circuit breaker
- [ ] Configure failure threshold, recovery time
- [ ] Add circuit breaker states (Closed, Open, Half-Open)
- [ ] Emit circuit breaker state change events
- [ ] Integrate with metrics system

#### 1.4.2 Graceful Degradation
**Status:** Planned
**Priority:** HIGH
**Complexity:** Medium

Handle overload gracefully:

- [ ] Implement adaptive backpressure
- [ ] Add load shedding for excess connections
- [ ] Priority-based connection acceptance
- [ ] Document degradation behavior

#### 1.4.3 Multi-Region Support
**Status:** Research
**Priority:** MEDIUM
**Complexity:** High

Geographic awareness:

- [ ] Add peer region/zone metadata
- [ ] Implement region-aware peer selection
- [ ] Add latency-based routing hints
- [ ] Support relay selection by region

### Medium Priority

#### 1.4.4 Hot Reload Configuration
**Status:** Planned
**Priority:** MEDIUM
**Complexity:** Medium

Runtime configuration updates:

- [ ] Add `Reconfigure(opts...)` method
- [ ] Support updating timeouts without restart
- [ ] Support updating rate limits dynamically
- [ ] Emit configuration change events

#### 1.4.5 Audit Logging
**Status:** Planned
**Priority:** MEDIUM
**Complexity:** Low

Security audit trail:

- [ ] Add structured audit log interface
- [ ] Log all connection attempts with metadata
- [ ] Log handshake successes/failures
- [ ] Log blacklist operations
- [ ] Include tamper attempt details

---

## Version 2.0.0 - Next Generation

**Target:** 2027
**Theme:** Breaking changes for long-term improvements

### Planned Breaking Changes

#### 2.0.1 API Simplification
- Remove deprecated `EstablishEncryptedStreams()` method
- Consolidate error types
- Simplify config with builder pattern
- Rename ambiguous methods

#### 2.0.2 Protocol Version 2.0
- Mandatory capabilities exchange
- Improved handshake with forward secrecy rotation
- Optional post-quantum key exchange hybrid
- Message compression support

#### 2.0.3 Async/Await Pattern
- Consider context-first API design
- Evaluate Promise/Future patterns for Go 2.x
- Improve cancelation propagation

---

## Testing Improvements (Ongoing)

### Planned Testing Enhancements

#### Chaos Engineering
- [ ] Network partition simulation
- [ ] Random connection drops
- [ ] Latency injection
- [ ] Message corruption injection (detected by AEAD)
- [ ] Clock skew simulation

#### Property-Based Testing
- [ ] Add property tests for state machine
- [ ] Add property tests for message ordering
- [ ] Add property tests for reconnection logic

#### Load Testing
- [ ] Benchmark with 1000+ concurrent peers
- [ ] Measure message throughput limits
- [ ] Profile memory under sustained load
- [ ] Document performance characteristics

#### Fuzz Testing Expansion
- [ ] Fuzz protocol message parsing
- [ ] Fuzz state machine transitions
- [ ] Fuzz configuration validation
- [ ] Add continuous fuzzing in CI

---

## Documentation Improvements (Ongoing)

### Planned Documentation

- [ ] API reference with godoc examples
- [ ] Migration guide for version upgrades
- [ ] Troubleshooting guide
- [ ] Performance tuning guide
- [ ] Security best practices guide
- [ ] Example: Building a chat application
- [ ] Example: Building a file sync service
- [ ] Example: Integration with existing protocols

---

## Community & Ecosystem

### Planned Integrations

- [ ] gRPC transport adapter
- [ ] WebSocket bridge for browser clients
- [ ] NATS-like pub/sub layer (community)
- [ ] Kubernetes operator for peer management (community)

### Developer Experience

- [ ] CLI tool for debugging connections
- [ ] VS Code extension for config validation
- [ ] Improved error messages with suggestions

---

## Contributing

We welcome contributions! Priority areas:

1. **Security fixes** - Always highest priority
2. **Documentation improvements** - Help others adopt Glueberry
3. **Test coverage** - More edge cases, more scenarios
4. **Performance optimizations** - With benchmarks
5. **New features** - Discuss in issues first

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

## Version History

| Version | Date | Highlights |
|---------|------|------------|
| 1.0.0 | 2026-01-26 | Initial release |
| 1.0.1 | 2026-01-26 | Stress tests, cramberry v1.2.0 |

---

## Feedback

Have suggestions for the roadmap? Open an issue with the `roadmap` label or discuss in our community channels.
