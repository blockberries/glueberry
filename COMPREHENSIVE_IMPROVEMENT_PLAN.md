# Comprehensive Improvement Plan for Glueberry

**Version:** 1.0.0
**Created:** 2026-01-26
**Based On:** Code review findings + ROADMAP.md items

This document provides a unified, prioritized action plan combining critical bug fixes, security hardening, and planned enhancements.

---

## Priority Legend

| Priority | Description | Timeline |
|----------|-------------|----------|
| **P0** | Critical bugs/security issues requiring immediate attention | Within 1 week |
| **P1** | High-priority fixes affecting stability/security | Within 2 weeks |
| **P2** | Medium-priority improvements | Within 1 month |
| **P3** | Lower-priority enhancements | As scheduled |

---

## Phase 0: Critical Bug Fixes (P0)

**Target:** Immediate - before any production use
**Theme:** Fix show-stopping bugs and critical security issues

### 0.1 Fix SecureZero Compiler Optimization Issue

**Priority:** P0 - CRITICAL
**Complexity:** Low
**Location:** `pkg/crypto/secure.go`

**Problem:** The `SecureZero` function uses a simple loop that Go's compiler can legally optimize away as a "dead store" if the memory is never read afterward.

**Implementation:**
```go
// Option 1: Use runtime.KeepAlive pattern
func SecureZero(b []byte) {
    for i := range b {
        b[i] = 0
    }
    runtime.KeepAlive(&b[0]) // Prevent optimization
}

// Option 2: Use crypto/subtle (preferred)
func SecureZero(b []byte) {
    subtle.XORBytes(b, b, b) // XOR with itself = zero, not optimized
}

// Option 3: Use volatile memory semantics via assembly
// See golang.org/x/crypto/internal/subtle for reference
```

**Tasks:**
- [ ] Research best practice for Go version 1.25.6
- [ ] Implement secure zeroing with compiler barrier
- [ ] Add test to verify zeroing actually occurs (memory inspection)
- [ ] Update documentation with security guarantees
- [ ] Apply to all key material handling

**Acceptance Criteria:**
- Memory inspection test confirms zeroing occurs
- No compiler warnings about unused writes
- Works on Go 1.22+

---

### 0.2 Fix Flow Controller Multi-Waiter Deadlock

**Priority:** P0 - CRITICAL
**Complexity:** Medium
**Location:** `internal/flow/controller.go`

**Problem:** When multiple goroutines are blocked on backpressure, `Release()` only unblocks ONE waiter via a size-1 buffered channel. Other waiters remain blocked indefinitely.

**Current Code:**
```go
// Only one waiter gets unblocked!
select {
case fc.unblockCh <- struct{}{}:
default:
}
```

**Implementation:**
```go
// Option 1: Close and recreate channel (broadcast)
func (fc *Controller) Release() {
    fc.mu.Lock()
    defer fc.mu.Unlock()

    if fc.pending > 0 {
        fc.pending--
    }

    if fc.blocked && fc.pending <= fc.lowWatermark {
        fc.blocked = false
        // Broadcast to all waiters by closing
        close(fc.unblockCh)
        fc.unblockCh = make(chan struct{})
    }
}

// Option 2: Use sync.Cond (cleaner)
type Controller struct {
    cond *sync.Cond
    // ... other fields
}

func (fc *Controller) Acquire(ctx context.Context) error {
    fc.mu.Lock()
    for fc.blocked && fc.pending >= fc.highWatermark {
        fc.mu.Unlock()
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            fc.mu.Lock()
            fc.cond.Wait()
        }
    }
    fc.pending++
    fc.mu.Unlock()
    return nil
}

func (fc *Controller) Release() {
    fc.mu.Lock()
    fc.pending--
    if fc.blocked && fc.pending <= fc.lowWatermark {
        fc.blocked = false
        fc.cond.Broadcast() // Wake ALL waiters
    }
    fc.mu.Unlock()
}
```

**Tasks:**
- [ ] Choose implementation approach (sync.Cond preferred)
- [ ] Implement fix with proper synchronization
- [ ] Add stress test with 100+ concurrent senders
- [ ] Verify no goroutine leaks
- [ ] Update flow controller documentation

**Acceptance Criteria:**
- Stress test with 100 blocked goroutines all unblock correctly
- No race conditions detected with `-race`
- Benchmark shows acceptable performance

---

### 0.3 Add Receive-Path Message Size Validation

**Priority:** P0 - CRITICAL
**Complexity:** Low
**Location:** `pkg/streams/encrypted.go`, `pkg/streams/unencrypted.go`

**Problem:** `MaxMessageSize` is only validated on send. A malicious peer can send arbitrarily large messages, causing OOM.

**Implementation:**
```go
// In readLoop(), after reading:
if len(data) > maxMessageSize {
    // Log and skip message, optionally close connection
    if es.onOversizedMessage != nil {
        es.onOversizedMessage(es.peerID, len(data))
    }
    continue // or return with error
}
```

**Tasks:**
- [ ] Add `MaxMessageSize` field to EncryptedStream/UnencryptedStream
- [ ] Pass max size from config through Manager
- [ ] Add size check in both readLoop implementations
- [ ] Add callback for oversized message events
- [ ] Add test with oversized message injection
- [ ] Document behavior (drop message vs close connection)

**Acceptance Criteria:**
- Oversized messages are rejected before full allocation
- Memory usage bounded under attack
- Metrics/logging for rejected messages

---

## Phase 1: Security Hardening (P1)

**Target:** Within 2 weeks
**Theme:** Address remaining security issues before production

### 1.1 Add Cipher.Close() for Key Material Zeroing

**Priority:** P1
**Complexity:** Low
**Location:** `pkg/crypto/cipher.go`, `pkg/streams/encrypted.go`

**Problem:** The cipher's internal AEAD copy of the key is never zeroed.

**Implementation:**
```go
type Cipher struct {
    aead cipher
    key  []byte // Keep copy for zeroing
}

func NewCipher(key []byte) (*Cipher, error) {
    keyCopy := make([]byte, len(key))
    copy(keyCopy, key)

    aead, err := chacha20poly1305.New(keyCopy)
    // ...
    return &Cipher{aead: aead, key: keyCopy}, nil
}

func (c *Cipher) Close() {
    SecureZero(c.key)
    c.aead = nil
}
```

**Tasks:**
- [ ] Store key copy in Cipher struct
- [ ] Implement Close() method
- [ ] Call Close() in EncryptedStream.Close()
- [ ] Update Manager.CloseStreams to close ciphers
- [ ] Add test verifying key is zeroed

---

### 1.2 Make EncryptWithNonce Internal-Only

**Priority:** P1
**Complexity:** Low
**Location:** `pkg/crypto/cipher.go`

**Problem:** Public API allows nonce reuse which breaks ChaCha20-Poly1305 security entirely.

**Tasks:**
- [ ] Rename to `encryptWithNonce` (unexported)
- [ ] Or add build tag `//go:build testing`
- [ ] Update any tests that use it
- [ ] Add documentation warning about nonce uniqueness

---

### 1.3 Fix Handshake Timeout Race Condition

**Priority:** P1
**Complexity:** Medium
**Location:** `pkg/connection/manager.go:308-327`

**Problem:** Timer can fire before HandshakeCancel/HandshakeTimer fields are set.

**Implementation:**
```go
func (m *Manager) setupHandshakeTimeout(conn *PeerConnection) {
    conn.mu.Lock()
    defer conn.mu.Unlock()

    timeoutCtx, cancel := context.WithCancel(m.ctx)
    conn.HandshakeCancel = cancel // Set BEFORE creating timer

    timer := time.AfterFunc(m.config.HandshakeTimeout, func() {
        select {
        case <-timeoutCtx.Done():
            return
        default:
            m.handleHandshakeTimeout(conn.PeerID)
        }
    })
    conn.HandshakeTimer = timer
}
```

**Tasks:**
- [ ] Reorder to set cancel function before timer creation
- [ ] Hold lock during entire setup
- [ ] Add test for rapid handshake completion
- [ ] Add test for timeout during setup

---

### 1.4 Fix Stream Manager Lock During Network I/O

**Priority:** P1
**Complexity:** Medium
**Location:** `pkg/streams/manager.go:257-289`

**Problem:** Lock held during `NewStream()` network call blocks all stream operations.

**Implementation:**
```go
func (m *Manager) openStreamLazilyCtx(ctx context.Context, peerID peer.ID, streamName string) (*EncryptedStream, error) {
    // Check prerequisites under lock
    m.mu.Lock()
    if existing := m.streams[peerID][streamName]; existing != nil && !existing.IsClosed() {
        m.mu.Unlock()
        return existing, nil
    }

    sharedKey, ok := m.sharedKeys[peerID]
    if !ok {
        m.mu.Unlock()
        return nil, fmt.Errorf("no shared key")
    }
    keyCopy := make([]byte, len(sharedKey))
    copy(keyCopy, sharedKey)
    m.mu.Unlock() // Release lock before network I/O!

    // Network call without lock
    rawStream, err := m.host.NewStream(ctx, peerID, streamName)
    if err != nil {
        return nil, err
    }

    // Re-acquire lock to store
    m.mu.Lock()
    defer m.mu.Unlock()

    // Double-check another goroutine didn't create it
    if existing := m.streams[peerID][streamName]; existing != nil && !existing.IsClosed() {
        rawStream.Close()
        return existing, nil
    }

    // Create and store
    encStream, err := NewEncryptedStream(...)
    // ...
}
```

**Tasks:**
- [ ] Implement lock-release-reacquire pattern
- [ ] Handle race where two goroutines create same stream
- [ ] Add stress test for concurrent stream creation
- [ ] Benchmark before/after

---

### 1.5 Connection Rate Limiting (from ROADMAP 1.1.3)

**Priority:** P1
**Complexity:** Medium
**Location:** `pkg/protocol/gater.go`, new `pkg/ratelimit/` package

**Tasks:**
- [ ] Add `MaxPeers` config option
- [ ] Add `MaxPendingConnections` config
- [ ] Implement per-IP rate limiting in ConnectionGater
- [ ] Add `ConnectionRateLimit` (connections per second per peer)
- [ ] Add metrics for rejected connections
- [ ] Document rate limiting behavior

---

### 1.6 Key Material Memory Protection (from ROADMAP 1.1.1)

**Priority:** P1
**Complexity:** Medium

This extends 0.1 and 1.1 with additional hardening:

**Tasks:**
- [ ] Implement `SecureBytes` type with finalizer-based zeroing
- [ ] Add optional `mlock()` support for Linux/macOS
- [ ] Zero all key copies in all locations
- [ ] Add memory security documentation
- [ ] Consider memguard integration for high-security deployments

---

## Phase 2: Stability & Performance (P2)

**Target:** Within 1 month
**Theme:** Fix performance issues and improve stability

### 2.1 Optimize Address Book Persistence

**Priority:** P2
**Complexity:** Medium
**Location:** `pkg/addressbook/book.go`, `pkg/addressbook/storage.go`

**Problem:** Every mutation triggers disk I/O. `UpdateLastSeen` is called on every connection.

**Implementation Options:**

**Option A: Batch Writes**
```go
type Book struct {
    dirty     bool
    flushChan chan struct{}
}

func (b *Book) markDirty() {
    b.dirty = true
    select {
    case b.flushChan <- struct{}{}:
    default:
    }
}

func (b *Book) flushLoop() {
    ticker := time.NewTicker(5 * time.Second)
    for {
        select {
        case <-ticker.C:
        case <-b.flushChan:
        }
        b.mu.Lock()
        if b.dirty {
            b.saveLocked()
            b.dirty = false
        }
        b.mu.Unlock()
    }
}
```

**Option B: Separate Hot/Cold Data**
- Keep LastSeen updates in memory only
- Persist critical data (addresses, keys) immediately
- Flush LastSeen periodically

**Tasks:**
- [ ] Choose batching strategy
- [ ] Implement deferred persistence
- [ ] Add `Flush()` method for explicit persistence
- [ ] Ensure flush on shutdown
- [ ] Benchmark before/after
- [ ] Update documentation

---

### 2.2 Fix Goroutine Leak in ConnectCtx

**Priority:** P2
**Complexity:** Low
**Location:** `pkg/connection/manager.go:179-185`

**Implementation:**
```go
// Use merged context instead of goroutine
func (m *Manager) ConnectCtx(ctx context.Context, peerID peer.ID) error {
    // Create context that cancels on either
    connectCtx, cancel := context.WithCancel(ctx)
    defer cancel()

    // Watch manager context in same goroutine
    go func() {
        select {
        case <-m.ctx.Done():
            cancel()
        case <-connectCtx.Done():
        }
    }()

    // Better: use errgroup or similar
}
```

**Better approach:**
```go
import "context"

func mergeContexts(ctx1, ctx2 context.Context) (context.Context, context.CancelFunc) {
    ctx, cancel := context.WithCancel(ctx1)
    go func() {
        select {
        case <-ctx2.Done():
            cancel()
        case <-ctx.Done():
        }
    }()
    return ctx, cancel
}
```

**Tasks:**
- [ ] Implement proper context merging
- [ ] Ensure goroutine exits when connection completes
- [ ] Add test for goroutine cleanup
- [ ] Check for similar patterns elsewhere

---

### 2.3 Add Peer Stats Cleanup

**Priority:** P2
**Complexity:** Low
**Location:** `node.go`

**Problem:** `peerStats` map grows unboundedly.

**Implementation:**
```go
// Option 1: LRU cache with size limit
type Node struct {
    peerStats *lru.Cache[peer.ID, *PeerStatsTracker]
}

// Option 2: Periodic cleanup
func (n *Node) cleanupStaleStats() {
    ticker := time.NewTicker(1 * time.Hour)
    for range ticker.C {
        n.peerStatsMu.Lock()
        for peerID, tracker := range n.peerStats {
            if time.Since(tracker.LastActivity()) > 24*time.Hour {
                delete(n.peerStats, peerID)
            }
        }
        n.peerStatsMu.Unlock()
    }
}
```

**Tasks:**
- [ ] Add LastActivity timestamp to PeerStatsTracker
- [ ] Implement periodic cleanup goroutine
- [ ] Add `MaxTrackedPeers` config option
- [ ] Add metrics for cleanup operations

---

### 2.4 Add Message Drop Metrics and Logging

**Priority:** P2
**Complexity:** Low
**Location:** Multiple stream files, `node.go`

**Problem:** Silent message dropping provides no visibility.

**Tasks:**
- [ ] Add `MessageDropped(peerID, streamName, reason)` to Metrics interface
- [ ] Add drop reason enum (BufferFull, Oversized, Decryption, etc.)
- [ ] Implement metric calls at all drop locations
- [ ] Add optional logging at DEBUG level
- [ ] Document buffer sizing recommendations

---

### 2.5 Fix Read Loop Context Handling

**Priority:** P2
**Complexity:** Low
**Location:** `pkg/streams/encrypted.go`, `pkg/streams/unencrypted.go`

**Problem:** Context cancellation only checked before blocking read.

**Implementation:**
```go
func (es *EncryptedStream) readLoop() {
    defer es.markClosed(nil)

    for {
        select {
        case <-es.ctx.Done():
            return
        default:
        }

        // Set read deadline to enable periodic context checks
        _ = es.stream.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

        var encrypted []byte
        if !es.reader.Next(&encrypted) {
            if err := es.reader.Err(); err != nil {
                if isTimeout(err) {
                    continue // Check context again
                }
                // Handle real error
            }
            return
        }

        // Clear deadline for normal processing
        _ = es.stream.SetReadDeadline(time.Time{})

        // Process message...
    }
}
```

**Tasks:**
- [ ] Add periodic read deadline
- [ ] Check context between deadline expirations
- [ ] Handle timeout errors specially
- [ ] Test shutdown latency improvement

---

### 2.6 Add Event Unsubscription

**Priority:** P2
**Complexity:** Low
**Location:** `node.go`

**Implementation:**
```go
type EventSubscription struct {
    ch     chan ConnectionEvent
    cancel func()
}

func (n *Node) SubscribeEvents(filter EventFilter) *EventSubscription {
    ch := make(chan ConnectionEvent, n.config.EventBufferSize)
    ctx, cancel := context.WithCancel(n.ctx)

    sub := &EventSubscription{ch: ch, cancel: cancel}

    // Register
    n.eventSubsMu.Lock()
    n.eventSubs[sub] = struct{}{}
    n.eventSubsMu.Unlock()

    // Start forwarding
    go func() {
        defer close(ch)
        for {
            select {
            case <-ctx.Done():
                return
            case evt := <-n.subscribeEvents():
                if filter.matches(evt) {
                    ch <- evt
                }
            }
        }
    }()

    return sub
}

func (s *EventSubscription) Unsubscribe() {
    s.cancel()
}
```

**Tasks:**
- [ ] Create EventSubscription type with Unsubscribe()
- [ ] Track subscriptions with cleanup capability
- [ ] Update FilteredEvents/EventsForPeer to return subscriptions
- [ ] Add test for subscription lifecycle
- [ ] Update documentation

---

## Phase 3: API Improvements (P2-P3)

**Target:** 1-2 months
**Theme:** Improve developer experience and API clarity

### 3.1 Consolidate Handshake APIs

**Priority:** P2
**Complexity:** Medium
**Location:** `node.go`

**Problem:** Two ways to complete handshake causes confusion.

**Plan:**
1. Deprecate `EstablishEncryptedStreams()` with clear warning
2. Document migration path to `PrepareStreams()` + `FinalizeHandshake()`
3. Remove in v2.0.0

**Tasks:**
- [ ] Add deprecation warning to EstablishEncryptedStreams
- [ ] Add `// Deprecated:` godoc comment
- [ ] Update all examples to use two-phase API
- [ ] Add migration guide in documentation
- [ ] Plan removal for v2.0.0

---

### 3.2 Add Windows Compatibility for File Locking

**Priority:** P2
**Complexity:** Medium
**Location:** `pkg/addressbook/storage.go`

**Implementation:**
```go
// storage_unix.go
//go:build !windows

func (s *storage) acquireFileLock() (*os.File, error) {
    // Current implementation with syscall.Flock
}

// storage_windows.go
//go:build windows

func (s *storage) acquireFileLock() (*os.File, error) {
    // Use LockFileEx from golang.org/x/sys/windows
}
```

**Tasks:**
- [ ] Split storage.go into platform-specific files
- [ ] Implement Windows locking with LockFileEx
- [ ] Add Windows CI testing
- [ ] Test cross-platform behavior

---

### 3.3 Improve Error Messages

**Priority:** P3
**Complexity:** Low
**Location:** Throughout codebase

**Tasks:**
- [ ] Audit error messages for information leakage
- [ ] Add error codes to all errors for programmatic handling
- [ ] Create user-friendly vs debug-level error messages
- [ ] Add troubleshooting suggestions to common errors

---

## Phase 4: ROADMAP Feature Integration

### 4.1 Version 1.1.0 Features (from ROADMAP)

**Target:** Q1 2026

Items already covered in Phases 0-2:
- ✅ 1.1.1 Key Material Memory Protection → Phase 0.1, 1.1, 1.6
- ✅ 1.1.3 Connection Limits and Rate Limiting → Phase 1.5

**Remaining items:**

#### 4.1.1 Custom Decryption Error Callback (ROADMAP 1.1.2)
**Status:** Partially implemented (internal callback exists)

**Tasks:**
- [ ] Expose via `WithDecryptionErrorCallback()` config
- [ ] Document tampering detection patterns
- [ ] Add example showing peer banning on repeated failures

#### 4.1.2 Nonce Exhaustion Warning (ROADMAP 1.1.4)

**Tasks:**
- [ ] Add message counter per peer connection
- [ ] Emit warning at 2^40 messages
- [ ] Add `MessagesSent(peer.ID) uint64` API
- [ ] Document nonce collision risk

#### 4.1.3 Enhanced Input Validation (ROADMAP 1.1.5)

**Tasks:**
- [ ] Validate stream names (alphanumeric + hyphen)
- [ ] Add `MaxMetadataSize` config
- [ ] Add `MaxStreamNameLength` validation
- [ ] Reject malformed public keys early

---

### 4.2 Version 1.2.0 Features (from ROADMAP)

**Target:** Q2 2026

#### 4.2.1 Prometheus Metrics Adapter (ROADMAP 1.2.2)

**Tasks:**
- [ ] Create `prometheus/` subpackage
- [ ] Implement Metrics interface
- [ ] Add standard metric names
- [ ] Create Grafana dashboard template
- [ ] Document all metrics

#### 4.2.2 OpenTelemetry Integration (ROADMAP 1.2.1)

**Tasks:**
- [ ] Add `WithTracerProvider()` config
- [ ] Define span hierarchy
- [ ] Instrument connection lifecycle
- [ ] Add trace context propagation

#### 4.2.3 Health Check API (ROADMAP 1.2.3)

**Tasks:**
- [ ] Implement `IsHealthy() bool`
- [ ] Implement `ReadinessChecks() []CheckResult`
- [ ] Add HTTP handler wrapper
- [ ] Document Kubernetes integration

---

### 4.3 Version 1.3.0+ Features (from ROADMAP)

**Target:** Q3-Q4 2026

See ROADMAP.md for detailed plans:
- Session Resumption (1.3.1)
- Message Priorities (1.3.2)
- Protocol Negotiation (1.3.4)
- Circuit Breaker (1.4.1)
- Graceful Degradation (1.4.2)

---

## Phase 5: Testing Improvements

### 5.1 Add Missing Test Coverage

**Priority:** P2
**Complexity:** Medium

**Tasks:**
- [ ] Add test for SecureZero actually zeroing memory
- [ ] Add flow controller stress test (100+ goroutines)
- [ ] Add test for oversized message rejection
- [ ] Add test for handshake timeout race
- [ ] Add test for stream manager concurrent access
- [ ] Add Windows CI testing

### 5.2 Chaos Engineering (from ROADMAP)

**Tasks:**
- [ ] Network partition simulation
- [ ] Random connection drops
- [ ] Latency injection
- [ ] Message corruption injection

### 5.3 Load Testing (from ROADMAP)

**Tasks:**
- [ ] Benchmark with 1000+ peers
- [ ] Measure throughput limits
- [ ] Profile memory under load
- [ ] Document performance characteristics

---

## Implementation Order Summary

```
Week 1 (P0 - Critical):
├── 0.1 Fix SecureZero optimization
├── 0.2 Fix Flow Controller deadlock
└── 0.3 Add receive message size validation

Week 2 (P1 - Security):
├── 1.1 Add Cipher.Close()
├── 1.2 Make EncryptWithNonce internal
├── 1.3 Fix handshake timeout race
└── 1.4 Fix stream manager locking

Week 3-4 (P1 - Security cont.):
├── 1.5 Connection rate limiting
└── 1.6 Key material memory protection

Month 2 (P2 - Stability):
├── 2.1 Optimize address book persistence
├── 2.2 Fix goroutine leak
├── 2.3 Add peer stats cleanup
├── 2.4 Add message drop metrics
├── 2.5 Fix read loop context handling
└── 2.6 Add event unsubscription

Month 3+ (P2-P3 - API/Features):
├── 3.x API improvements
└── 4.x ROADMAP features
```

---

## Tracking Progress

Create GitHub issues for each item with labels:
- `priority:P0`, `priority:P1`, `priority:P2`, `priority:P3`
- `type:bug`, `type:security`, `type:performance`, `type:feature`
- `component:crypto`, `component:streams`, `component:connection`, etc.

Use milestones:
- `v1.0.2` - Phase 0 (Critical fixes)
- `v1.1.0` - Phase 1-2 (Security + Stability)
- `v1.2.0` - Phase 3-4 (API + Features)

---

## Success Criteria

### Phase 0 Complete When:
- [ ] Memory inspection tests pass for SecureZero
- [ ] Flow controller stress test passes (100 goroutines)
- [ ] Oversized message test rejects without OOM

### Phase 1 Complete When:
- [ ] All cipher key material properly zeroed
- [ ] No public nonce-setting APIs
- [ ] No race conditions detected under stress
- [ ] Rate limiting active and tested

### Phase 2 Complete When:
- [ ] Address book benchmark shows <10ms per update
- [ ] No goroutine leaks under connection churn
- [ ] All message drops visible in metrics
- [ ] Clean shutdown within 1 second

---

## Notes

1. **Breaking Changes:** Phases 0-2 should be backward compatible. Phase 3 may deprecate APIs.

2. **Testing:** Each fix MUST include regression tests before merge.

3. **Documentation:** Update CHANGELOG.md, ARCHITECTURE.md, and API docs for each phase.

4. **Security Disclosure:** If P0 issues are found in production use, consider security advisory.
