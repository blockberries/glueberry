# Glueberry Prerelease Implementation Plan

This document provides a comprehensive, prioritized implementation plan for preparing Glueberry for production release. It synthesizes findings from the code review, ROADMAP items, and integration requirements with the broader Blockberries ecosystem.

**Current Status**: 100% feature complete, production-ready for core functionality
**Target**: v1.0.0 stable release with full production observability

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Critical Path Items](#critical-path-items)
3. [Phase 1: Production Readiness](#phase-1-production-readiness)
4. [Phase 2: Reliability & Robustness](#phase-2-reliability--robustness)
5. [Phase 3: Integration Hardening](#phase-3-integration-hardening)
6. [Phase 4: Performance & Scale](#phase-4-performance--scale)
7. [Phase 5: Developer Experience](#phase-5-developer-experience)
8. [Testing Requirements](#testing-requirements)
9. [Release Checklist](#release-checklist)

---

## Executive Summary

Glueberry is a mature P2P networking library with solid cryptographic foundations. The prerelease focus is on **observability**, **security hardening**, and **integration reliability** rather than new features.

### Key Prerelease Priorities

| Priority | Category | Items | Effort |
|----------|----------|-------|--------|
| P0 | Security | Security audit, fuzz testing | High |
| P0 | Observability | Structured logging, Prometheus metrics | High |
| P1 | Reliability | Flow control, context support, error handling | Medium |
| P1 | Integration | Blockberry connection direction API, handshake robustness | Medium |
| P2 | Performance | Benchmarking suite, connection pooling | Medium |
| P2 | DX | Better error messages, more examples | Low |

### Dependencies

- **Cramberry**: Used for message serialization; must be stable
- **Blockberry**: Primary consumer; requires connection direction API

---

## Critical Path Items

These items must be completed before any production deployment.

### CP-1: Security Audit Preparation

**Priority**: P0
**Effort**: 1-2 weeks preparation, external audit timeline varies

Before engaging external auditors, complete internal preparation:

#### CP-1.1: Cryptographic Implementation Review
- [ ] Verify Ed25519 → X25519 key conversion follows RFC 8032
- [ ] Audit HKDF key derivation parameters against best practices
- [ ] Verify ChaCha20-Poly1305 nonce handling (no reuse)
- [ ] Review constant-time operations for secret comparisons
- [ ] Document cryptographic assumptions and guarantees

**Files to audit**:
- `internal/crypto/keys.go`
- `internal/crypto/cipher.go`
- `internal/crypto/kdf.go`

#### CP-1.2: Input Validation Audit
- [ ] Review all network message parsing for bounds checking
- [ ] Verify Cramberry deserialization can't cause panics
- [ ] Check for integer overflow in size calculations
- [ ] Audit handshake message validation

**Files to audit**:
- `internal/connection/handshake.go`
- `internal/streams/encrypted_stream.go`

#### CP-1.3: Resource Exhaustion Review
- [ ] Verify connection limits are enforced
- [ ] Check message size limits throughout pipeline
- [ ] Review buffer allocation patterns for DoS potential
- [ ] Audit channel buffer sizes for memory exhaustion

---

### CP-2: Fuzz Testing Infrastructure

**Priority**: P0
**Effort**: 1 week

Implement comprehensive fuzz testing before release.

#### CP-2.1: Cramberry Message Fuzzing
```go
// fuzz/cramberry_fuzz.go
func FuzzHandshakeMessage(data []byte) int {
    var msg schema.HandshakeMessage
    if err := cramberry.UnmarshalInterface(data, &msg); err != nil {
        return 0
    }
    // Verify no panic, memory corruption
    return 1
}
```

**Fuzz targets**:
- [ ] Handshake message parsing (HelloRequest, HelloResponse, HelloFinalize)
- [ ] Encrypted message decryption with malformed ciphertext
- [ ] Varint decoding (potential integer overflow)
- [ ] Address book JSON parsing

#### CP-2.2: Handshake Protocol Fuzzing
- [ ] Random byte injection during handshake
- [ ] Out-of-order message delivery
- [ ] Truncated messages
- [ ] Oversized messages

#### CP-2.3: CI Integration
- [ ] Add go-fuzz targets to repository
- [ ] Configure OSS-Fuzz or continuous fuzzing
- [ ] Create corpus of interesting inputs
- [ ] Set up crash alerting

---

## Phase 1: Production Readiness

Focus: Essential observability for production operations.

### P1-1: Structured Logging

**Priority**: P0
**Effort**: 3-4 days
**Files**: New `logging.go`, updates throughout

#### P1-1.1: Logger Interface Definition
```go
// pkg/glueberry/logging.go

// Logger defines the logging interface for Glueberry.
// Compatible with slog, zap, zerolog.
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Info(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
    Error(msg string, keysAndValues ...any)
}

// NopLogger is a no-op logger for testing or when logging is disabled.
type NopLogger struct{}

func (NopLogger) Debug(msg string, keysAndValues ...any) {}
func (NopLogger) Info(msg string, keysAndValues ...any)  {}
func (NopLogger) Warn(msg string, keysAndValues ...any)  {}
func (NopLogger) Error(msg string, keysAndValues ...any) {}
```

#### P1-1.2: Configuration Integration
```go
// Add to Config
type Config struct {
    // ... existing fields
    Logger Logger // Optional; defaults to NopLogger
}

func NewConfig(..., opts ...Option) *Config {
    cfg := &Config{Logger: NopLogger{}}
    // ...
}

func WithLogger(l Logger) Option {
    return func(c *Config) { c.Logger = l }
}
```

#### P1-1.3: Strategic Log Points

Add logging at these locations:

| Component | Log Level | Event | Fields |
|-----------|-----------|-------|--------|
| Node | Info | Started/Stopped | listen_addrs |
| Connection | Info | Connected | peer_id, addr |
| Connection | Info | Disconnected | peer_id, reason |
| Connection | Debug | Handshake started | peer_id |
| Connection | Info | Handshake complete | peer_id, streams |
| Connection | Warn | Handshake failed | peer_id, error |
| Stream | Debug | Message sent | peer_id, stream, size |
| Stream | Debug | Message received | peer_id, stream, size |
| Crypto | Error | Decryption failed | peer_id, error |
| AddressBook | Info | Peer added/removed | peer_id |
| AddressBook | Warn | Peer blacklisted | peer_id, reason |

#### P1-1.4: Security Considerations
- [ ] NEVER log private keys, shared secrets, or plaintext message contents
- [ ] Sanitize peer IDs to prevent log injection
- [ ] Rate-limit error logs to prevent log flooding attacks

---

### P1-2: Prometheus Metrics

**Priority**: P0
**Effort**: 4-5 days
**Files**: New `metrics.go`, updates throughout

#### P1-2.1: Metrics Interface Definition
```go
// pkg/glueberry/metrics.go

// Metrics defines the metrics collection interface.
type Metrics interface {
    // Connection metrics
    ConnectionsTotal() CounterVec    // labels: state
    ConnectionAttempts() CounterVec  // labels: result (success, failure)
    HandshakeDuration() Histogram

    // Stream metrics
    MessagesSent() CounterVec     // labels: stream
    MessagesReceived() CounterVec // labels: stream
    BytesSent() CounterVec        // labels: stream
    BytesReceived() CounterVec    // labels: stream
    ActiveStreams() GaugeVec      // labels: stream

    // Crypto metrics
    EncryptionErrors() Counter
    DecryptionErrors() Counter
    KeyCacheHits() Counter
    KeyCacheMisses() Counter

    // Event metrics
    EventsEmitted() CounterVec // labels: state
    EventsDropped() Counter
    MessagesDropped() Counter
}

// NopMetrics provides a no-op implementation for when metrics are disabled.
type NopMetrics struct{}
```

#### P1-2.2: Prometheus Implementation
```go
// pkg/glueberry/prometheus/metrics.go

type PrometheusMetrics struct {
    connectionsTotal   *prometheus.CounterVec
    handshakeDuration  prometheus.Histogram
    messagesSent       *prometheus.CounterVec
    // ... etc
}

func NewPrometheusMetrics(namespace string) *PrometheusMetrics {
    m := &PrometheusMetrics{
        connectionsTotal: prometheus.NewCounterVec(
            prometheus.CounterOpts{
                Namespace: namespace,
                Name:      "connections_total",
                Help:      "Total number of connections by state",
            },
            []string{"state"},
        ),
        handshakeDuration: prometheus.NewHistogram(
            prometheus.HistogramOpts{
                Namespace: namespace,
                Name:      "handshake_duration_seconds",
                Help:      "Handshake completion time in seconds",
                Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
            },
        ),
        // ... etc
    }
    return m
}
```

#### P1-2.3: Metric Instrumentation Points

| Metric | Location | When |
|--------|----------|------|
| connections_total{state} | connection/manager.go | State transitions |
| connection_attempts_total{result} | connection/dialer.go | Connect() returns |
| handshake_duration_seconds | connection/handshake.go | Handshake complete |
| messages_sent_total{stream} | streams/encrypted_stream.go | Send() |
| messages_received_total{stream} | streams/encrypted_stream.go | readLoop() |
| bytes_sent_total{stream} | streams/encrypted_stream.go | After encryption |
| bytes_received_total{stream} | streams/encrypted_stream.go | After decryption |
| encryption_errors_total | crypto/cipher.go | Encrypt() error |
| decryption_errors_total | crypto/cipher.go | Decrypt() error |
| events_dropped_total | node.go | Channel full |
| messages_dropped_total | streams/router.go | Channel full |

---

### P1-3: Context Support

**Priority**: P1
**Effort**: 2-3 days
**Files**: Public API updates, internal propagation

#### P1-3.1: API Updates
```go
// Add context variants to all blocking operations

// Current
func (n *Node) Connect(peerID peer.ID) error

// New (keep old for backwards compatibility)
func (n *Node) ConnectCtx(ctx context.Context, peerID peer.ID) error
func (n *Node) Connect(peerID peer.ID) error {
    return n.ConnectCtx(context.Background(), peerID)
}

// Similarly for:
func (n *Node) SendCtx(ctx context.Context, peerID peer.ID, stream string, data []byte) error
func (n *Node) DisconnectCtx(ctx context.Context, peerID peer.ID) error
```

#### P1-3.2: Internal Context Propagation
- [ ] Pass context through connection establishment
- [ ] Respect context cancellation in dial operations
- [ ] Propagate context to handshake timeout
- [ ] Add context to stream write operations

#### P1-3.3: Cancellation Handling
```go
func (n *Node) ConnectCtx(ctx context.Context, peerID peer.ID) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // Create derived context for this operation
    connCtx, cancel := context.WithCancel(ctx)
    defer cancel()

    // ... connection logic with connCtx
}
```

---

### P1-4: Enhanced Error Handling

**Priority**: P1
**Effort**: 2 days
**Files**: `errors.go`, updates throughout

#### P1-4.1: Rich Error Types
```go
// pkg/glueberry/errors.go

// ErrorCode identifies the type of error for programmatic handling.
type ErrorCode int

const (
    ErrCodeUnknown ErrorCode = iota
    ErrCodeConnectionFailed
    ErrCodeHandshakeFailed
    ErrCodeHandshakeTimeout
    ErrCodeStreamClosed
    ErrCodeEncryptionFailed
    ErrCodeDecryptionFailed
    ErrCodePeerNotFound
    ErrCodePeerBlacklisted
    ErrCodeBufferFull
    ErrCodeContextCanceled
)

// Error represents a Glueberry error with rich context.
type Error struct {
    Code      ErrorCode
    Message   string
    PeerID    peer.ID
    Stream    string
    Cause     error
    Retriable bool
}

func (e *Error) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("glueberry: %s: %v", e.Message, e.Cause)
    }
    return fmt.Sprintf("glueberry: %s", e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func (e *Error) Is(target error) bool {
    t, ok := target.(*Error)
    return ok && t.Code == e.Code
}
```

#### P1-4.2: Error Classification
```go
// IsRetriable returns true if the operation can be retried.
func IsRetriable(err error) bool {
    var gErr *Error
    if errors.As(err, &gErr) {
        return gErr.Retriable
    }
    return false
}

// IsPermanent returns true if the error indicates a permanent failure.
func IsPermanent(err error) bool {
    var gErr *Error
    if errors.As(err, &gErr) {
        return gErr.Code == ErrCodePeerBlacklisted
    }
    return false
}
```

#### P1-4.3: Error Wrapping Standards
Update all error returns to use rich errors:
```go
// Before
return fmt.Errorf("handshake failed: %w", err)

// After
return &Error{
    Code:      ErrCodeHandshakeFailed,
    Message:   "handshake failed",
    PeerID:    peerID,
    Cause:     err,
    Retriable: true,
}
```

---

## Phase 2: Reliability & Robustness

Focus: Improve behavior under stress and failure conditions.

### P2-1: Flow Control and Backpressure

**Priority**: P1
**Effort**: 4-5 days
**Files**: `internal/streams/flow.go` (new), updates to stream handling

#### P2-1.1: Per-Stream Flow Control
```go
// internal/streams/flow.go

type FlowController struct {
    mu            sync.Mutex
    highWatermark int
    lowWatermark  int
    pending       int
    blocked       bool
    unblockCh     chan struct{}
}

func NewFlowController(high, low int) *FlowController {
    return &FlowController{
        highWatermark: high,
        lowWatermark:  low,
        unblockCh:     make(chan struct{}, 1),
    }
}

// Acquire blocks if flow control is engaged.
func (fc *FlowController) Acquire(ctx context.Context) error {
    fc.mu.Lock()
    if fc.pending >= fc.highWatermark {
        fc.blocked = true
    }
    blocked := fc.blocked
    fc.mu.Unlock()

    if blocked {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-fc.unblockCh:
            return nil
        }
    }
    return nil
}

// Release decrements pending count and potentially unblocks.
func (fc *FlowController) Release() {
    fc.mu.Lock()
    defer fc.mu.Unlock()

    fc.pending--
    if fc.blocked && fc.pending <= fc.lowWatermark {
        fc.blocked = false
        select {
        case fc.unblockCh <- struct{}{}:
        default:
        }
    }
}
```

#### P2-1.2: Configuration
```go
type StreamConfig struct {
    Name             string
    HighWatermark    int  // Messages; default 1000
    LowWatermark     int  // Messages; default 100
    MaxMessageSize   int  // Bytes; default 1MB
    EnableBackpressure bool // Default true
}
```

#### P2-1.3: Metrics Integration
- [ ] Add `glueberry_flow_control_blocked_total{stream}` counter
- [ ] Add `glueberry_pending_messages{stream}` gauge
- [ ] Add `glueberry_backpressure_events_total{stream}` counter

---

### P2-2: Event Filtering

**Priority**: P1
**Effort**: 2 days
**Files**: `node.go`, `internal/events/filter.go` (new)

#### P2-2.1: Filtered Event Channels
```go
// EventFilter specifies which events to receive.
type EventFilter struct {
    PeerIDs []peer.ID      // nil = all peers
    States  []ConnectionState // nil = all states
}

// Events returns a channel of all events.
func (n *Node) Events() <-chan ConnectionEvent {
    return n.events
}

// FilteredEvents returns events matching the filter.
func (n *Node) FilteredEvents(filter EventFilter) <-chan ConnectionEvent {
    filtered := make(chan ConnectionEvent, n.cfg.EventBufferSize)

    go func() {
        defer close(filtered)
        for event := range n.events {
            if filter.matches(event) {
                select {
                case filtered <- event:
                default:
                    // Drop if buffer full
                }
            }
        }
    }()

    return filtered
}

// EventsFor returns events for a specific peer.
func (n *Node) EventsFor(peerID peer.ID) <-chan ConnectionEvent {
    return n.FilteredEvents(EventFilter{PeerIDs: []peer.ID{peerID}})
}
```

---

### P2-3: Handshake Robustness

**Priority**: P1
**Effort**: 3 days
**Integration Issue**: Blockberry HelloFinalize loss can cause hangs

#### P2-3.1: Handshake State Machine Hardening
```go
// internal/connection/handshake_state.go

type HandshakeState int

const (
    HandshakeStateInit HandshakeState = iota
    HandshakeStateSentRequest
    HandshakeStateReceivedResponse
    HandshakeStateSentFinalize
    HandshakeStateComplete
    HandshakeStateFailed
)

type HandshakeStateMachine struct {
    mu       sync.Mutex
    state    HandshakeState
    timeout  time.Duration
    timer    *time.Timer
    retries  int
    maxRetries int
}

// Transition validates and performs state transition.
func (hsm *HandshakeStateMachine) Transition(to HandshakeState) error {
    hsm.mu.Lock()
    defer hsm.mu.Unlock()

    if !hsm.validTransition(hsm.state, to) {
        return fmt.Errorf("invalid transition %v -> %v", hsm.state, to)
    }

    hsm.state = to
    hsm.resetTimer()
    return nil
}
```

#### P2-3.2: Handshake Timeout with Retry
```go
func (c *Connection) handleHandshakeTimeout() {
    c.hsm.mu.Lock()
    state := c.hsm.state
    retries := c.hsm.retries
    c.hsm.mu.Unlock()

    if retries < c.hsm.maxRetries {
        switch state {
        case HandshakeStateSentFinalize:
            // Retry HelloFinalize
            c.hsm.retries++
            c.sendHelloFinalize()
            return
        }
    }

    // Max retries exceeded; fail handshake
    c.failHandshake(ErrHandshakeTimeout)
}
```

#### P2-3.3: Configuration
```go
type Config struct {
    // ... existing
    HandshakeTimeout     time.Duration // Default: 10s
    HandshakeMaxRetries  int           // Default: 3
}
```

---

### P2-4: Connection Direction Tracking

**Priority**: P1
**Effort**: 1 day
**Integration Issue**: Blockberry hardcodes isOutbound=true

#### P2-4.1: Track Connection Direction
```go
// internal/connection/connection.go

type Connection struct {
    // ... existing fields
    isOutbound bool
}

// IsOutbound returns true if we initiated this connection.
func (c *Connection) IsOutbound() bool {
    return c.isOutbound
}
```

#### P2-4.2: Public API
```go
// node.go

// IsOutbound returns true if the connection to the peer was initiated by us.
func (n *Node) IsOutbound(peerID peer.ID) (bool, error) {
    conn, ok := n.connections.Get(peerID)
    if !ok {
        return false, ErrPeerNotFound
    }
    return conn.IsOutbound(), nil
}
```

#### P2-4.3: Update Connection Manager
```go
func (cm *ConnectionManager) handleInbound(stream network.Stream) {
    conn := cm.createConnection(stream.Conn(), false) // isOutbound=false
    // ...
}

func (cm *ConnectionManager) dialPeer(peerID peer.ID) (*Connection, error) {
    // ...
    conn := cm.createConnection(rawConn, true) // isOutbound=true
    return conn, nil
}
```

---

## Phase 3: Integration Hardening

Focus: Ensure seamless integration with Blockberry and ecosystem.

### P3-1: Protocol Versioning

**Priority**: P1
**Effort**: 2-3 days

#### P3-1.1: Version Negotiation
```go
// protocol/version.go

const (
    ProtocolVersionMajor = 1
    ProtocolVersionMinor = 0
    ProtocolVersionPatch = 0
)

type ProtocolVersion struct {
    Major uint8
    Minor uint8
    Patch uint8
}

func (v ProtocolVersion) Compatible(other ProtocolVersion) bool {
    // Major version must match
    return v.Major == other.Major
}

func (v ProtocolVersion) String() string {
    return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}
```

#### P3-1.2: Handshake Version Exchange
```go
// Update HelloRequest to include version
type HelloRequest struct {
    NodeID        []byte
    Version       string // Semantic version
    ProtocolVersion ProtocolVersion
    // ... existing fields
}

// Validate version compatibility
func (c *Connection) validateVersion(peerVersion ProtocolVersion) error {
    if !ourVersion.Compatible(peerVersion) {
        return &Error{
            Code:    ErrCodeVersionMismatch,
            Message: fmt.Sprintf("incompatible protocol version: %s vs %s", ourVersion, peerVersion),
        }
    }
    return nil
}
```

---

### P3-2: Peer Statistics API

**Priority**: P1
**Effort**: 2 days

#### P3-2.1: Statistics Structure
```go
// PeerStats contains statistics for a peer connection.
type PeerStats struct {
    PeerID           peer.ID
    Connected        bool
    IsOutbound       bool
    ConnectedAt      time.Time
    TotalConnectTime time.Duration

    // Message stats
    MessagesSent     int64
    MessagesReceived int64
    BytesSent        int64
    BytesReceived    int64

    // Per-stream stats
    StreamStats      map[string]*StreamStats

    // Health
    LastMessageAt    time.Time
    ConnectionCount  int // Total connections (including reconnects)
    FailureCount     int
    AvgLatency       time.Duration
}

type StreamStats struct {
    MessagesSent     int64
    MessagesReceived int64
    BytesSent        int64
    BytesReceived    int64
}
```

#### P3-2.2: Statistics Collection
```go
// Update connection to track stats
func (c *Connection) recordMessageSent(stream string, size int) {
    c.stats.mu.Lock()
    defer c.stats.mu.Unlock()

    c.stats.MessagesSent++
    c.stats.BytesSent += int64(size)

    ss := c.stats.StreamStats[stream]
    if ss == nil {
        ss = &StreamStats{}
        c.stats.StreamStats[stream] = ss
    }
    ss.MessagesSent++
    ss.BytesSent += int64(size)
}
```

#### P3-2.3: Public API
```go
// PeerStats returns statistics for a peer.
func (n *Node) PeerStats(peerID peer.ID) (*PeerStats, error) {
    conn, ok := n.connections.Get(peerID)
    if !ok {
        return nil, ErrPeerNotFound
    }
    return conn.Stats(), nil
}

// AllPeerStats returns statistics for all connected peers.
func (n *Node) AllPeerStats() map[peer.ID]*PeerStats {
    stats := make(map[peer.ID]*PeerStats)
    n.connections.Range(func(peerID peer.ID, conn *Connection) bool {
        stats[peerID] = conn.Stats()
        return true
    })
    return stats
}
```

---

### P3-3: Address Book Persistence Improvements

**Priority**: P2
**Effort**: 2 days
**Issue**: Concurrent access safety

#### P3-3.1: File Locking
```go
// internal/addressbook/addressbook.go

func (ab *AddressBook) Save() error {
    ab.mu.Lock()
    defer ab.mu.Unlock()

    // Create temp file
    tmpFile := ab.path + ".tmp"

    f, err := os.OpenFile(tmpFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
    if err != nil {
        return fmt.Errorf("create temp file: %w", err)
    }

    // Acquire exclusive lock
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
        f.Close()
        return fmt.Errorf("acquire lock: %w", err)
    }
    defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

    // Write and sync
    enc := json.NewEncoder(f)
    if err := enc.Encode(ab.entries); err != nil {
        f.Close()
        return fmt.Errorf("encode: %w", err)
    }

    if err := f.Sync(); err != nil {
        f.Close()
        return fmt.Errorf("sync: %w", err)
    }
    f.Close()

    // Atomic rename
    return os.Rename(tmpFile, ab.path)
}
```

---

## Phase 4: Performance & Scale

Focus: Optimize for high-throughput scenarios.

### P4-1: Comprehensive Benchmark Suite

**Priority**: P1
**Effort**: 3-4 days

#### P4-1.1: Micro-benchmarks
```go
// benchmark/crypto_bench_test.go

func BenchmarkEd25519ToX25519(b *testing.B) {
    _, priv, _ := ed25519.GenerateKey(rand.Reader)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        crypto.Ed25519ToX25519Private(priv)
    }
}

func BenchmarkEncrypt(b *testing.B) {
    key := make([]byte, 32)
    rand.Read(key)
    cipher := crypto.NewCipher(key)
    plaintext := make([]byte, 1024)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cipher.Encrypt(plaintext)
    }
}

func BenchmarkDecrypt(b *testing.B) {
    // Similar setup
}
```

#### P4-1.2: Macro-benchmarks
```go
// benchmark/connection_bench_test.go

func BenchmarkHandshake(b *testing.B) {
    // Measure full handshake time
}

func BenchmarkMessageRoundTrip(b *testing.B) {
    // Measure send + receive latency
}

func BenchmarkThroughput(b *testing.B) {
    // Measure messages/second
}
```

#### P4-1.3: Scalability benchmarks
```go
func BenchmarkConcurrentConnections(b *testing.B) {
    for _, n := range []int{10, 100, 500, 1000} {
        b.Run(fmt.Sprintf("peers_%d", n), func(b *testing.B) {
            // Connect n peers, measure resource usage
        })
    }
}
```

#### P4-1.4: CI Integration
- [ ] Add `make benchmark` target
- [ ] Store benchmark baselines
- [ ] Fail CI on >10% regression
- [ ] Generate benchmark report

---

### P4-2: Buffer Pooling

**Priority**: P2
**Effort**: 2 days

#### P4-2.1: Message Buffer Pool
```go
// internal/pool/buffers.go

var messagePool = sync.Pool{
    New: func() interface{} {
        buf := make([]byte, 0, 4096)
        return &buf
    },
}

func GetBuffer() *[]byte {
    return messagePool.Get().(*[]byte)
}

func PutBuffer(buf *[]byte) {
    *buf = (*buf)[:0]
    messagePool.Put(buf)
}
```

#### P4-2.2: Integration Points
- [ ] Use pool in `Send()` for serialization buffer
- [ ] Use pool in read loop for receive buffer
- [ ] Use pool in cipher for encryption buffer

---

### P4-3: Message Batching (Optional)

**Priority**: P3
**Effort**: 3-4 days

For high-throughput scenarios, optional message batching:

```go
type BatchingConfig struct {
    Enabled       bool
    MaxBatchSize  int           // Messages
    MaxBatchBytes int           // Bytes
    FlushInterval time.Duration // Max delay
}

type Batcher struct {
    config    BatchingConfig
    pending   [][]byte
    pendingBytes int
    mu        sync.Mutex
    flushCh   chan struct{}
}

func (b *Batcher) Add(msg []byte) {
    b.mu.Lock()
    b.pending = append(b.pending, msg)
    b.pendingBytes += len(msg)

    shouldFlush := len(b.pending) >= b.config.MaxBatchSize ||
        b.pendingBytes >= b.config.MaxBatchBytes
    b.mu.Unlock()

    if shouldFlush {
        b.Flush()
    }
}
```

---

## Phase 5: Developer Experience

Focus: Make Glueberry easy to use and debug.

### P5-1: Additional Examples

**Priority**: P1
**Effort**: 3 days

Create comprehensive examples:

#### P5-1.1: File Transfer Example
```go
// examples/file-transfer/main.go
// Demonstrates large data transfer with progress reporting
```

#### P5-1.2: Request/Response Pattern
```go
// examples/rpc/main.go
// Demonstrates building RPC-style communication on top of Glueberry
```

#### P5-1.3: Multi-Node Cluster
```go
// examples/cluster/main.go
// Demonstrates peer discovery and cluster coordination
```

#### P5-1.4: Integration with Blockberry
```go
// examples/blockberry-integration/main.go
// Demonstrates how Blockberry uses Glueberry
```

---

### P5-2: Testing Utilities

**Priority**: P2
**Effort**: 2-3 days

Provide testing utilities for consumers:

```go
// pkg/glueberry/testing/mock.go

// MockNode provides a mock implementation for unit testing.
type MockNode struct {
    mu            sync.Mutex
    connections   map[peer.ID]bool
    sentMessages  []SentMessage
    eventQueue    []ConnectionEvent
}

func NewMockNode() *MockNode {
    return &MockNode{
        connections:  make(map[peer.ID]bool),
        sentMessages: make([]SentMessage, 0),
        eventQueue:   make([]ConnectionEvent, 0),
    }
}

// SimulateConnect simulates a peer connecting.
func (m *MockNode) SimulateConnect(peerID peer.ID) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.connections[peerID] = true
    m.eventQueue = append(m.eventQueue, ConnectionEvent{
        PeerID: peerID,
        State:  StateConnected,
    })
}

// SimulateMessage simulates receiving a message.
func (m *MockNode) SimulateMessage(peerID peer.ID, stream string, data []byte) {
    // Queue message for Messages() channel
}

// AssertSent verifies a message was sent.
func (m *MockNode) AssertSent(t *testing.T, peerID peer.ID, stream string) {
    // Assertion helper
}
```

---

### P5-3: Debug Utilities

**Priority**: P2
**Effort**: 1-2 days

```go
// DumpState returns a JSON representation of node state for debugging.
func (n *Node) DumpState() ([]byte, error) {
    state := struct {
        ListenAddrs   []string
        Connections   []ConnectionInfo
        AddressBook   []AddressBookEntry
        Streams       map[peer.ID][]string
        BufferStats   BufferStats
    }{
        ListenAddrs: n.ListenAddresses(),
        // ... populate
    }
    return json.MarshalIndent(state, "", "  ")
}

type ConnectionInfo struct {
    PeerID     string
    State      string
    IsOutbound bool
    ConnectedAt time.Time
    Streams    []string
    Stats      *PeerStats
}
```

---

## Testing Requirements

### Unit Test Coverage Targets

| Package | Current | Target |
|---------|---------|--------|
| internal/crypto | 85% | 95% |
| internal/connection | 70% | 85% |
| internal/streams | 75% | 85% |
| internal/addressbook | 80% | 90% |
| pkg/glueberry | 65% | 80% |

### Test Categories

#### Security Tests
- [ ] Cryptographic operation correctness
- [ ] Key derivation determinism
- [ ] Nonce uniqueness verification
- [ ] Input validation edge cases

#### Concurrency Tests
- [ ] Race detector clean (`go test -race`)
- [ ] Concurrent connect/disconnect
- [ ] Concurrent send from multiple goroutines
- [ ] Connection state machine under load

#### Integration Tests
- [ ] Multi-node (3+) cluster formation
- [ ] Network partition and recovery
- [ ] Long-running stability (24h soak test)
- [ ] High message throughput

#### Chaos Tests
- [ ] Random connection drops
- [ ] Message corruption/truncation
- [ ] Slow network simulation
- [ ] Resource exhaustion

---

## Release Checklist

### Pre-Release

- [ ] All P0 items complete
- [ ] All P1 items complete (or documented as post-release)
- [ ] Security audit scheduled/complete
- [ ] Fuzz testing runs clean for 24h
- [ ] Unit test coverage meets targets
- [ ] Integration tests pass
- [ ] Benchmarks documented
- [ ] No known security vulnerabilities
- [ ] Dependencies up to date
- [ ] API documentation complete
- [ ] CHANGELOG updated
- [ ] Migration guide (if breaking changes)

### Release Process

1. [ ] Create release branch `release/v1.0.0`
2. [ ] Run full test suite
3. [ ] Run benchmarks and document results
4. [ ] Update version constants
5. [ ] Generate and review CHANGELOG
6. [ ] Tag release `v1.0.0`
7. [ ] Create GitHub release with notes
8. [ ] Announce release

### Post-Release

- [ ] Monitor for issues
- [ ] Address critical bugs in patch releases
- [ ] Collect performance feedback
- [ ] Plan v1.1.0 based on feedback

---

## Appendix: File Change Summary

### New Files

| File | Purpose |
|------|---------|
| `pkg/glueberry/logging.go` | Logger interface and NopLogger |
| `pkg/glueberry/metrics.go` | Metrics interface and NopMetrics |
| `pkg/glueberry/prometheus/metrics.go` | Prometheus implementation |
| `pkg/glueberry/errors.go` | Rich error types |
| `internal/streams/flow.go` | Flow control implementation |
| `internal/events/filter.go` | Event filtering |
| `internal/connection/handshake_state.go` | Handshake state machine |
| `pkg/glueberry/testing/mock.go` | Mock node for testing |
| `fuzz/` | Fuzz testing targets |
| `benchmark/` | Benchmark tests |
| `examples/file-transfer/` | File transfer example |
| `examples/rpc/` | RPC pattern example |
| `examples/cluster/` | Multi-node example |

### Modified Files

| File | Changes |
|------|---------|
| `pkg/glueberry/config.go` | Add Logger, Metrics, context options |
| `pkg/glueberry/node.go` | Add context methods, stats API |
| `internal/connection/connection.go` | Add isOutbound, stats tracking |
| `internal/connection/manager.go` | Track connection direction |
| `internal/streams/encrypted_stream.go` | Add metrics, flow control |
| `internal/addressbook/addressbook.go` | Add file locking |

---

*Last Updated: January 2026*
*Version: Prerelease 1.0*
