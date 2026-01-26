# Glueberry Prerelease Progress Report

This document tracks the implementation progress for the Glueberry prerelease preparation.

---

## Phase: Security Hardening - Key Material Zeroing

**Status**: ✅ Completed
**Priority**: P0 (Critical - Security)

### Summary

Implemented secure zeroing of cryptographic key material to prevent sensitive data from lingering in memory after use. This addresses the security concern identified in the SECURITY_REVIEW.md regarding key material not being explicitly wiped.

### Files Created

- `pkg/crypto/secure.go` - Secure zeroing utility functions
- `pkg/crypto/secure_test.go` - Comprehensive tests for zeroing functionality

### Files Modified

- `pkg/crypto/module.go`:
  - Updated `RemovePeerKey()` to zero cached shared key before deletion
  - Updated `ClearPeerKeys()` to zero all cached keys before clearing the map
  - Added `Close()` method to zero private keys on shutdown

- `pkg/connection/peer.go`:
  - Updated `Cleanup()` to securely zero the shared key before releasing

- `pkg/streams/manager.go`:
  - Updated `CloseStreams()` to zero shared key before removing from map
  - Updated `Shutdown()` to zero all shared keys before clearing

- `pkg/streams/encrypted.go`:
  - Note: Key zeroing NOT added to `markClosed()` because:
    1. The key is owned by the Manager which handles zeroing
    2. Multiple streams share the same key reference
    3. The cipher has its own internal copy of the key

### Key Functionality Implemented

1. **SecureZero([]byte)** - Overwrites byte slice with zeros
2. **SecureZeroMultiple(...[]byte)** - Zeros multiple slices at once
3. **Module.Close()** - Securely cleans up all private key material
4. Automatic key zeroing on peer disconnection and node shutdown

### Test Coverage

- `TestSecureZero` - Basic zeroing functionality
- `TestSecureZero_NilSlice` - Edge case handling
- `TestSecureZeroMultiple` - Multiple slice zeroing
- `TestSecureZeroMultiple_WithNil` - Mixed nil/non-nil slices
- `TestModule_Close_ZerosPrivateKeys` - Verifies private keys are zeroed
- `TestModule_RemovePeerKey_ZerosKey` - Verifies cached keys are zeroed on removal
- `TestModule_ClearPeerKeys_ZerosAllKeys` - Verifies all keys zeroed on clear

All tests pass with race detection enabled.

### Design Decisions

1. **Simple zeroing loop vs memguard**: Used a simple zeroing loop rather than external dependencies like memguard, as Go's memory model makes more complex solutions (like mlock) less reliable without significant additional complexity.

2. **Manager owns key lifecycle**: The stream Manager is responsible for zeroing shared keys since multiple encrypted streams may share references to the same key. This prevents race conditions and ensures clean ownership.

3. **Defense in depth**: While Go's garbage collector doesn't guarantee memory zeroing, explicit zeroing provides defense in depth against memory disclosure attacks.

---

## Phase: P2-4 - Connection Direction Tracking

**Status**: ✅ Completed
**Priority**: P0 (Blockberry Integration Blocker)

### Summary

Implemented connection direction tracking to distinguish between outbound (we initiated) and inbound (peer initiated) connections. This is critical for Blockberry integration, which requires the initiator to send HelloRequest first.

### Files Modified

- `pkg/connection/peer.go`:
  - Added `IsOutbound bool` field to `PeerConnection` struct
  - Added `GetIsOutbound()` getter method
  - Added `SetIsOutbound()` setter method

- `pkg/connection/manager.go`:
  - Updated `Connect()` to mark connections as outbound (`SetIsOutbound(true)`)
  - Updated `RegisterIncomingConnection()` to mark connections as inbound (`SetIsOutbound(false)`)
  - Added `IsOutbound(peerID)` method to query connection direction

- `pkg/connection/peer_test.go`:
  - Added `TestNewPeerConnection_IsOutbound_DefaultFalse`
  - Added `TestPeerConnection_SetIsOutbound`
  - Added `TestPeerConnection_IsOutbound_ConcurrentAccess`

- `node.go`:
  - Added `IsOutbound(peerID peer.ID) (bool, error)` public API method

### Key Functionality Implemented

1. **IsOutbound field** - Tracks whether connection was initiated by us
2. **Thread-safe access** - Concurrent get/set operations are safe
3. **Automatic tracking** - Direction is set automatically when connections are established
4. **Public API** - Node exposes `IsOutbound()` for application use

### Test Coverage

- Default value test (false for new connections)
- Set/Get roundtrip test
- Concurrent access test (race detection)

All tests pass with race detection enabled.

### Usage Example

```go
// In handshake logic
isOutbound, err := node.IsOutbound(peerID)
if err != nil {
    return err
}

if isOutbound {
    // We initiated - send HelloRequest first
    node.Send(peerID, "handshake", helloRequest)
} else {
    // They initiated - wait for their HelloRequest
    // ... receive and respond
}
```

---

## Phase: P1-1 - Logger Interface

**Status**: ✅ Completed
**Priority**: P0 (Production Readiness)

### Summary

Implemented a structured logging interface for Glueberry that is compatible with standard logging libraries like slog, zap, and zerolog. The interface provides Debug, Info, Warn, and Error methods with key-value pair support.

### Files Created

- `logging.go` - Logger interface definition and NopLogger implementation
- `logging_test.go` - Comprehensive tests for logging functionality

### Files Modified

- `config.go`:
  - Added `Logger Logger` field to Config struct
  - Added `WithLogger(Logger)` configuration option
  - Updated `applyDefaults()` to set NopLogger as default

- `node.go`:
  - Added `logger Logger` field to Node struct
  - Added logging for node start/stop events
  - Added logging for peer added/blacklisted events
  - Added logging for handshake preparation and completion
  - Fixed key zeroing order (host must close before zeroing keys)

### Key Functionality Implemented

1. **Logger Interface**:
   - `Debug(msg string, keysAndValues ...any)`
   - `Info(msg string, keysAndValues ...any)`
   - `Warn(msg string, keysAndValues ...any)`
   - `Error(msg string, keysAndValues ...any)`

2. **NopLogger** - Default no-op implementation for when logging is disabled

3. **WithLogger(Logger)** - Configuration option for setting a custom logger

4. **Strategic Log Points**:
   - Node started (Info): peer_id, listen_addrs
   - Node stopped (Info): peer_id
   - Peer added (Info): peer_id
   - Peer blacklisted (Warn): peer_id
   - Handshake preparing (Debug): peer_id, streams
   - Handshake failed (Warn): peer_id, error
   - Handshake complete (Info): peer_id

### Test Coverage

- `TestNopLogger_Implements_Logger` - Interface compliance
- `TestNopLogger_Methods_DoNotPanic` - Edge case handling
- `TestTestLogger_RecordsCalls` - Custom logger verification
- `TestLogger_IsThreadSafe` - Concurrent access safety
- `TestWithLogger_SetsLogger` - Config option
- `TestConfig_DefaultsToNopLogger` - Default behavior
- `TestConfig_WithLogger_OverridesDefault` - Custom logger preservation

All tests pass with race detection enabled.

### Security Considerations

Following CLAUDE.md guidelines:
- No private keys, shared secrets, or plaintext message contents are logged
- Peer IDs are logged as strings (safe)
- Error messages are logged without sensitive data

### Usage Example

```go
// Using with slog
logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithLogger(slogAdapter{logger}),
)
```

---

## Phase: P1-2 - Metrics Interface

**Status**: ✅ Completed
**Priority**: P0 (Production Readiness)

### Summary

Implemented a comprehensive metrics interface for Glueberry that is compatible with Prometheus and other metrics systems. The interface provides connection, stream, crypto, and event metrics with labeled dimensions.

### Files Created

- `metrics.go` - Metrics interface definition and NopMetrics implementation
- `metrics_test.go` - Comprehensive tests for metrics functionality

### Files Modified

- `config.go`:
  - Added `Metrics Metrics` field to Config struct
  - Added `WithMetrics(Metrics)` configuration option
  - Updated `applyDefaults()` to set NopMetrics as default

- `node.go`:
  - Added `metrics Metrics` field to Node struct
  - Stored metrics reference from config

### Key Functionality Implemented

**Metrics Interface Methods:**

Connection Metrics:
- `ConnectionOpened(direction string)` - Records new connections (inbound/outbound)
- `ConnectionClosed(direction string)` - Records closed connections
- `ConnectionAttempt(result string)` - Records attempt results (success/failure)
- `HandshakeDuration(seconds float64)` - Histogram of handshake times
- `HandshakeResult(result string)` - Records handshake outcomes

Stream Metrics:
- `MessageSent(stream string, bytes int)` - Records outgoing messages and bytes
- `MessageReceived(stream string, bytes int)` - Records incoming messages and bytes
- `StreamOpened(stream string)` - Records stream creation
- `StreamClosed(stream string)` - Records stream closure

Crypto Metrics:
- `EncryptionError()` - Records encryption failures
- `DecryptionError()` - Records decryption failures
- `KeyDerivation(cached bool)` - Records key derivation operations

Event Metrics:
- `EventEmitted(state string)` - Records emitted events by state
- `EventDropped()` - Records dropped events (buffer full)
- `MessageDropped()` - Records dropped messages (buffer full)

### Test Coverage

- `TestNopMetrics_Implements_Metrics` - Interface compliance
- `TestNopMetrics_Methods_DoNotPanic` - Edge case handling
- `TestTestMetrics_RecordsCalls` - Recording verification
- `TestTestMetrics_IsThreadSafe` - Concurrent access safety
- `TestWithMetrics_SetsMetrics` - Config option
- `TestConfig_DefaultsToNopMetrics` - Default behavior
- `TestConfig_WithMetrics_OverridesDefault` - Custom metrics preservation

All tests pass with race detection enabled.

### Usage Example

```go
// Using with Prometheus
promMetrics := NewPrometheusMetrics("glueberry")
cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithMetrics(promMetrics),
)

// Later, expose metrics endpoint
http.Handle("/metrics", promhttp.Handler())
```

### Notes

The Metrics interface is now available for integration. Instrumentation points will be added throughout the codebase as a follow-up task (can be done incrementally). The interface is designed to be zero-overhead when using NopMetrics.

---

## Phase: P1-3 - Context Support

**Status**: ✅ Completed
**Priority**: P1 (Production Readiness)

### Summary

Implemented context support for all blocking operations in the public API. This allows callers to control timeouts and cancellation for connection, send, and disconnect operations.

### Files Created

- `context_test.go` - Comprehensive tests for context-aware methods

### Files Modified

- `node.go`:
  - Added `ConnectCtx(ctx context.Context, peerID peer.ID) error` public API
  - Added `DisconnectCtx(ctx context.Context, peerID peer.ID) error` public API
  - Added `SendCtx(ctx context.Context, peerID peer.ID, stream string, data []byte) error` public API
  - Updated `Connect()`, `Disconnect()`, `Send()` to call context variants with `context.Background()`

- `pkg/connection/manager.go`:
  - Added `ConnectCtx(ctx context.Context, peerID peer.ID) error`
  - Added `DisconnectCtx(ctx context.Context, peerID peer.ID) error`
  - Updated `Connect()` and `Disconnect()` to call context variants

- `pkg/streams/manager.go`:
  - Added `SendCtx(ctx context.Context, peerID peer.ID, streamName string, data []byte) error`
  - Added `sendUnencryptedCtx()` for unencrypted stream sending with context
  - Added `openUnencryptedStreamLazilyCtx()` for lazy stream opening with context
  - Added `openStreamLazilyCtx()` for encrypted stream opening with context
  - Updated `Send()` to call `SendCtx` with `context.Background()`

- `pkg/streams/encrypted.go`:
  - Added `SendCtx(ctx context.Context, data []byte) error`
  - Respects context deadline by setting stream write deadline
  - Updated `Send()` to call `SendCtx` with `context.Background()`

- `pkg/streams/unencrypted.go`:
  - Added `SendCtx(ctx context.Context, data []byte) error`
  - Respects context deadline by setting stream write deadline
  - Updated `Send()` to call `SendCtx` with `context.Background()`

### Key Functionality Implemented

1. **Context-aware Connect**:
   - Respects context cancellation and deadline
   - Propagates context to libp2p dial operation
   - Uses 30-second default timeout if context has no deadline

2. **Context-aware Send**:
   - Checks context before starting operation
   - Checks context after acquiring write lock
   - Sets stream write deadline from context deadline
   - Clears deadline after operation completes

3. **Context-aware Disconnect**:
   - Checks context before starting operation
   - Allows cancellation of cleanup operations

4. **Backwards Compatibility**:
   - Original methods (`Connect`, `Send`, `Disconnect`) still work
   - They internally call context variants with `context.Background()`

### Test Coverage

- `TestConnectCtx_ContextCancelled` - Verifies immediate cancellation
- `TestSendCtx_ContextCancelled` - Verifies send cancellation
- `TestDisconnectCtx_ContextCancelled` - Verifies disconnect cancellation
- `TestConnectCtx_Timeout` - Verifies deadline expiration
- `TestNodeNotStarted_Ctx` - Verifies error handling when node not started
- `TestConnect_WrapsConnectCtx` - Verifies backwards compatibility
- `TestSend_WrapsSendCtx` - Verifies backwards compatibility

All tests pass with race detection enabled.

### Usage Example

```go
// Connect with 5-second timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

if err := node.ConnectCtx(ctx, peerID); err != nil {
    if err == context.DeadlineExceeded {
        log.Println("Connection timed out")
    }
    return err
}

// Send with cancellation support
ctx, cancel = context.WithCancel(context.Background())
go func() {
    <-shutdownSignal
    cancel()
}()

if err := node.SendCtx(ctx, peerID, "data", message); err != nil {
    if err == context.Canceled {
        log.Println("Send cancelled")
    }
    return err
}
```

### Design Decisions

1. **Context propagation pattern**: Context is checked at multiple points (before acquiring locks, after acquiring locks, before I/O) to ensure responsiveness to cancellation.

2. **Stream deadline integration**: When context has a deadline, it's set as the stream's write deadline, allowing the underlying network operations to respect the timeout.

3. **Backwards compatibility**: Original non-context methods are preserved as thin wrappers, ensuring existing code continues to work without changes.

---

## Phase: P1-4 - Rich Error Types

**Status**: ✅ Completed
**Priority**: P1 (Production Readiness)

### Summary

Implemented rich error types with error codes for programmatic error handling. The Error type provides structured information including error code, message, peer ID, stream name, underlying cause, and retriability status.

### Files Created

- `errors_rich_test.go` - Comprehensive tests for rich error types

### Files Modified

- `errors.go`:
  - Added `ErrorCode` type and constants for all error categories
  - Added `Error` struct with Code, Message, PeerID, Stream, Cause, and Retriable fields
  - Implemented `Error()`, `Unwrap()`, and `Is()` methods for standard error interface
  - Added `IsRetriable()` and `IsPermanent()` helper functions
  - Added factory functions: `NewError()`, `NewErrorWithCause()`, `NewPeerError()`, `NewStreamError()`
  - Preserved existing sentinel errors for backwards compatibility

### Key Functionality Implemented

1. **ErrorCode constants**:
   - `ErrCodeUnknown` - Unknown/unclassified error
   - `ErrCodeConnectionFailed` - Connection attempt failed
   - `ErrCodeHandshakeFailed` - Handshake protocol failed
   - `ErrCodeHandshakeTimeout` - Handshake timed out
   - `ErrCodeStreamClosed` - Stream has been closed
   - `ErrCodeEncryptionFailed` - Encryption failed
   - `ErrCodeDecryptionFailed` - Decryption failed
   - `ErrCodePeerNotFound` - Peer not found
   - `ErrCodePeerBlacklisted` - Peer is blacklisted
   - `ErrCodeBufferFull` - Buffer (event/message) is full
   - `ErrCodeContextCanceled` - Context was cancelled
   - `ErrCodeInvalidConfig` - Configuration is invalid
   - `ErrCodeNodeNotStarted` - Node not started
   - `ErrCodeNodeAlreadyStarted` - Node already running

2. **Error struct**:
   - Rich context including peer ID and stream name
   - Supports error wrapping with Cause field
   - Retriable flag for retry logic

3. **Standard error interface**:
   - `Error()` - Returns human-readable message
   - `Unwrap()` - Returns underlying cause
   - `Is()` - Compares by error code for `errors.Is()` support

4. **Helper functions**:
   - `IsRetriable(err)` - Check if error can be retried
   - `IsPermanent(err)` - Check if error is permanent (blacklisted, invalid config)

5. **Factory functions**:
   - `NewError(code, message)` - Basic error
   - `NewErrorWithCause(code, message, cause)` - Error with underlying cause
   - `NewPeerError(code, message, peerID)` - Peer-specific error
   - `NewStreamError(code, message, peerID, stream)` - Stream-specific error

### Test Coverage

- `TestErrorCode_String` - All error code string representations
- `TestError_Error` - Error message formatting with/without cause
- `TestError_Unwrap` - Error unwrapping
- `TestError_Is` - Error code matching
- `TestError_ErrorsIs` - Integration with `errors.Is()`
- `TestError_ErrorsAs` - Integration with `errors.As()`
- `TestIsRetriable` - Retriability checking
- `TestIsPermanent` - Permanent error detection
- `TestNewError` - Factory function
- `TestNewErrorWithCause` - Factory function with cause
- `TestNewPeerError` - Peer error factory
- `TestNewStreamError` - Stream error factory
- `TestError_Fields` - All struct fields

All tests pass with race detection enabled.

### Usage Example

```go
// Creating errors
err := &Error{
    Code:      ErrCodeConnectionFailed,
    Message:   "connection refused",
    PeerID:    peerID,
    Cause:     dialErr,
    Retriable: true,
}

// Checking error types
if errors.Is(err, &Error{Code: ErrCodeConnectionFailed}) {
    // Handle connection failure
}

// Using helper functions
if IsRetriable(err) {
    // Retry the operation
}

if IsPermanent(err) {
    // Don't retry, remove peer
}

// Extracting error details
var gErr *Error
if errors.As(err, &gErr) {
    log.Printf("Error on peer %s: %s", gErr.PeerID, gErr.Message)
}
```

### Design Decisions

1. **Backwards compatibility**: Existing sentinel errors are preserved. The rich Error type can coexist with them.

2. **Error code based Is()**: Two Glueberry errors are equal if they have the same error code, regardless of other fields. This allows `errors.Is(err, &Error{Code: ErrCodeFoo})` pattern.

3. **Permanent vs retriable**: PeerBlacklisted and InvalidConfig are permanent errors. Other errors may be retriable depending on context.

---

## Phase: Security Medium - Decryption Failure Observability

**Status**: ✅ Completed
**Priority**: Security Medium

### Summary

Implemented decryption failure observability through a callback mechanism. When message decryption fails (potentially indicating tampering or wrong key), the failure is logged and metrics are recorded. This provides visibility into potential security events.

### Files Modified

- `pkg/streams/encrypted.go`:
  - Added `onDecryptionError DecryptionErrorCallback` field to `EncryptedStream` struct
  - Updated `NewEncryptedStream` to accept callback parameter
  - Updated `readLoop()` to invoke callback on decryption failure

- `pkg/streams/manager.go`:
  - Added `DecryptionErrorCallback` type definition
  - Added `onDecryptionError DecryptionErrorCallback` field to `Manager` struct
  - Added `SetDecryptionErrorCallback(callback DecryptionErrorCallback)` method
  - Updated `createEncryptedStream()` and `HandleIncomingStream()` to pass callback to encrypted streams

- `pkg/streams/encrypted_test.go`:
  - Added `TestEncryptedStream_DecryptionErrorCallback` test
  - Updated all `NewEncryptedStream` calls to include callback parameter

- `node.go`:
  - Added callback wiring in `New()` to log decryption failures and record metrics
  - Uses `Logger.Warn()` for logging with peer_id and error context
  - Uses `Metrics.DecryptionError()` for metrics recording

- `internal/eventdispatch/dispatcher.go`:
  - Fixed race condition in `EmitEvent()` by holding mutex during channel send
  - Previously, the mutex was released before sending to channel, allowing `Close()` to race

### Key Functionality Implemented

1. **DecryptionErrorCallback type**: `func(peerID peer.ID, err error)` - callback invoked on decryption failure

2. **Manager.SetDecryptionErrorCallback()**: Allows setting the callback for all encrypted streams

3. **Automatic wiring in Node**: When Logger or Metrics are configured, decryption failures are automatically logged and recorded

4. **Race condition fix**: Fixed concurrent access bug in event dispatcher between `EmitEvent()` and `Close()`

### Test Coverage

- `TestEncryptedStream_DecryptionErrorCallback` - Verifies callback is invoked when decryption fails with mismatched keys
- Tests verify callback receives correct peer ID and error
- Tests verify message is NOT delivered on decryption failure
- All existing tests updated to pass nil callback parameter

All tests pass with race detection enabled.

### Security Considerations

- Decryption failures are logged at Warn level (not Error) as they may indicate network issues or misconfiguration
- Peer ID is logged to help identify problematic peers
- Error message is logged for debugging but does NOT contain sensitive data
- Metrics increment allows monitoring for unusual patterns (potential attack detection)

### Usage Example

```go
// Automatic with configured Logger and Metrics
cfg := glueberry.Config{
    PrivateKey:      privateKey,
    AddressBookPath: "/path/to/addressbook.json",
    ListenAddrs:     listenAddrs,
    Logger:          myLogger,
    Metrics:         myMetrics,
}

node, _ := glueberry.New(&cfg)
// Decryption failures will automatically:
// 1. Log: "decryption failed" with peer_id and error
// 2. Record: metrics.DecryptionError()
```

### Design Decisions

1. **Callback pattern**: Using a callback allows flexibility - the streams package doesn't depend on logging or metrics packages.

2. **Continue on failure**: Decryption failures don't close the stream. This allows recovery from transient issues while still providing visibility.

3. **Warn level logging**: Decryption failures are logged at Warn level since they're not necessarily errors (could be network corruption or misconfiguration).

---

## Phase: P3-2 - Peer Statistics API

**Status**: ✅ Completed
**Priority**: P1 (Production Readiness)

### Summary

Implemented a comprehensive peer statistics API that tracks connection, message, and stream statistics per peer. The API provides real-time visibility into peer activity and connection health.

### Files Created

- `stats.go` - PeerStats, StreamStats, and PeerStatsTracker types
- `stats_test.go` - Comprehensive tests for statistics tracking

### Files Modified

- `node.go`:
  - Added `peerStats` map and mutex for tracking
  - Added `internalMsgs` channel for message interception
  - Added `PeerStatistics(peerID)` and `AllPeerStatistics()` public APIs
  - Added `getOrCreatePeerStats()` helper
  - Added `forwardMessagesWithStats()` goroutine for message tracking
  - Modified `SendCtx()` to record sent message stats

- `pkg/connection/peer.go`:
  - Added `StatsRecorder` interface
  - Added `Stats` field to `PeerConnection`
  - Added `SetStats()` and `GetStats()` methods

- `pkg/connection/manager.go`:
  - Added `GetConnection()` method for stats access

### Key Functionality Implemented

1. **PeerStats struct**:
   - PeerID, Connected, IsOutbound, ConnectedAt, TotalConnectTime
   - MessagesSent, MessagesReceived, BytesSent, BytesReceived
   - StreamStats map for per-stream statistics
   - LastMessageAt, ConnectionCount, FailureCount

2. **StreamStats struct**:
   - Name, MessagesSent, MessagesReceived
   - BytesSent, BytesReceived
   - LastSentAt, LastReceivedAt

3. **PeerStatsTracker**:
   - Thread-safe mutable stats tracker
   - Implements connection.StatsRecorder interface
   - RecordConnectionStart(), RecordConnectionEnd()
   - RecordFailure()
   - RecordMessageSent(), RecordMessageReceived()
   - Snapshot() for creating immutable copies

4. **Public API**:
   - `PeerStatistics(peerID peer.ID) *PeerStats` - Get stats for a peer
   - `AllPeerStatistics() map[peer.ID]*PeerStats` - Get all peer stats

5. **Automatic tracking**:
   - Message sends recorded via SendCtx()
   - Message receives recorded via forwarding goroutine

### Test Coverage

- `TestPeerStatsTracker_RecordMessageSent` - Message send tracking
- `TestPeerStatsTracker_RecordMessageReceived` - Message receive tracking
- `TestPeerStatsTracker_RecordConnectionStartEnd` - Connection duration tracking
- `TestPeerStatsTracker_RecordFailure` - Failure counting
- `TestPeerStatsTracker_Concurrent` - Thread safety verification
- `TestPeerStatsTracker_LastMessageAt` - Timestamp tracking
- `TestPeerStatsTracker_TotalConnectTimeDuringConnection` - Active connection time
- `TestPeerStats_Fields` / `TestStreamStats_Fields` - Struct verification

All tests pass with race detection enabled.

### Usage Example

```go
// Get statistics for a specific peer
stats := node.PeerStatistics(peerID)
if stats != nil {
    fmt.Printf("Peer %s: %d messages sent, %d received\n",
        stats.PeerID, stats.MessagesSent, stats.MessagesReceived)
    fmt.Printf("Total connection time: %v\n", stats.TotalConnectTime)

    // Check per-stream stats
    for stream, ss := range stats.StreamStats {
        fmt.Printf("  Stream %s: %d KB sent\n", stream, ss.BytesSent/1024)
    }
}

// Get all peer statistics
allStats := node.AllPeerStatistics()
for peerID, stats := range allStats {
    if stats.Connected {
        fmt.Printf("Connected to %s for %v\n", peerID, stats.TotalConnectTime)
    }
}
```

### Design Decisions

1. **Separate internal/external channels**: Messages flow through an internal channel where stats are recorded before forwarding to the external channel for the application. This provides transparent stats collection.

2. **Snapshot pattern**: The `Snapshot()` method returns an immutable copy of stats, allowing safe concurrent access without holding locks while the caller processes the data.

3. **StatsRecorder interface**: The connection package defines a `StatsRecorder` interface, allowing the glueberry package to provide the implementation without circular dependencies.

4. **Non-blocking message forwarding**: If the external channel is full, messages are dropped to prevent blocking the stats collection. This matches the existing behavior elsewhere in the codebase.

---

## Phase: P2-2 - Event Filtering

**Status**: ✅ Completed
**Priority**: P1 (Production Readiness)

### Summary

Implemented event filtering to allow applications to receive only specific events they care about. The implementation uses a pub-sub pattern with broadcast support, allowing multiple consumers to receive events without conflict.

### Files Modified

- `node.go`:
  - Added `eventSubs` slice and mutex for subscriber management
  - Added `internalEvents` field for internal event channel
  - Refactored `forwardEvents()` to broadcast to all subscribers
  - Added `subscribeEvents()` internal method
  - Added `EventFilter` type with peer and state filtering
  - Added `FilteredEvents(filter EventFilter)` method
  - Added `EventsForPeer(peerID)` convenience method
  - Added `EventsForStates(states...)` convenience method
  - Fixed `Events()` to use pre-created external channel

- `events_filter_test.go` (new):
  - Tests for EventFilter.matches() with all filters
  - Tests for peer, state, and combined filtering
  - Tests for ConnectionEvent fields

### Key Functionality Implemented

1. **EventFilter struct**:
   - `PeerIDs []peer.ID` - Filter by specific peers (nil = all)
   - `States []ConnectionState` - Filter by specific states (nil = all)
   - `matches(evt)` method for filter evaluation

2. **Event subscription mechanism**:
   - Broadcast pattern: events sent to all subscribers
   - Main `Events()` channel and filtered subscribers receive same events
   - No conflict between `Events()` and `FilteredEvents()`

3. **Public API**:
   - `Events() <-chan ConnectionEvent` - All events
   - `FilteredEvents(filter EventFilter) <-chan ConnectionEvent` - Filtered events
   - `EventsForPeer(peerID peer.ID) <-chan ConnectionEvent` - Events for one peer
   - `EventsForStates(states ...ConnectionState) <-chan ConnectionEvent` - Events for specific states

### Test Coverage

- `TestEventFilter_Matches_AllEvents` - Empty filter matches all
- `TestEventFilter_Matches_PeerFilter` - Peer ID filtering
- `TestEventFilter_Matches_StateFilter` - Connection state filtering
- `TestEventFilter_Matches_CombinedFilter` - Combined peer and state filtering
- `TestConnectionEvent_Fields` - Event struct verification
- `TestConnectionState_StringInFilter` - State string representation

All tests pass with race detection enabled.

### Usage Example

```go
// Get all events
allEvents := node.Events()
go func() {
    for evt := range allEvents {
        log.Printf("Event: %s -> %s", evt.PeerID, evt.State)
    }
}()

// Get filtered events for specific peer
peerEvents := node.EventsForPeer(targetPeerID)
go func() {
    for evt := range peerEvents {
        log.Printf("Peer %s: %s", evt.PeerID, evt.State)
    }
}()

// Get events for specific states
connectEvents := node.EventsForStates(StateConnected, StateEstablished)
go func() {
    for evt := range connectEvents {
        log.Printf("Connection event: %s", evt.State)
    }
}()

// Custom filter - specific peers in specific states
filter := EventFilter{
    PeerIDs: []peer.ID{peer1, peer2},
    States:  []ConnectionState{StateEstablished},
}
filtered := node.FilteredEvents(filter)
```

### Design Decisions

1. **Broadcast pattern**: All event subscribers receive all events (independently filtered). This allows using both `Events()` and `FilteredEvents()` without conflict.

2. **Non-blocking delivery**: If a subscriber's channel is full, events are dropped for that subscriber. This prevents slow consumers from blocking the event system.

3. **Subscriber cleanup**: All subscriber channels are closed when the node stops, ensuring goroutines don't leak.

4. **Filter composition**: Filters use AND logic - events must match both peer and state filters if both are specified. Empty filter slices match all.

---

## Phase: P3-1 - Protocol Versioning

**Status**: ✅ Completed
**Priority**: P1 (Production Readiness)

### Summary

Implemented protocol versioning support to allow applications to detect incompatible peers early during handshake. The implementation uses semantic versioning (major.minor.patch) with well-defined compatibility rules.

### Files Created

- `version.go` - ProtocolVersion type and version constants
- `version_test.go` - Comprehensive tests for version functionality

### Files Modified

- `errors.go`:
  - Added `ErrCodeVersionMismatch` error code
  - Added `ErrVersionMismatch` sentinel error

- `node.go`:
  - Added `Version() ProtocolVersion` public API method

### Key Functionality Implemented

1. **Protocol version constants**:
   - `ProtocolVersionMajor` = 1 (breaking changes)
   - `ProtocolVersionMinor` = 0 (new features)
   - `ProtocolVersionPatch` = 0 (bug fixes)

2. **ProtocolVersion struct**:
   - `Major uint8` - Breaking changes require matching major
   - `Minor uint8` - New features; backwards compatible within major
   - `Patch uint8` - Bug fixes; always compatible within major.minor

3. **Version methods**:
   - `String()` - Returns "major.minor.patch" format
   - `Compatible(other)` - Checks compatibility with another version
   - `IsNewer(other)` - Checks if this version is newer
   - `Equal(other)` - Checks exact equality

4. **Compatibility rules**:
   - Major versions must match
   - Peer's minor version must not exceed ours (they can't use features we don't have)
   - Patch versions are always compatible within same major.minor

5. **Utility functions**:
   - `CurrentVersion()` - Returns current protocol version
   - `ParseVersion(s string)` - Parses "major.minor.patch" string

6. **Public API**:
   - `node.Version() ProtocolVersion` - Get current protocol version

### Test Coverage

- `TestProtocolVersion_String` - String formatting
- `TestProtocolVersion_Compatible` - All compatibility scenarios:
  - Same version (compatible)
  - Same with patch difference (compatible)
  - Older minor version (compatible)
  - Newer minor version (NOT compatible)
  - Different major version (NOT compatible)
- `TestProtocolVersion_IsNewer` - Version comparison
- `TestProtocolVersion_Equal` - Exact equality
- `TestParseVersion` - String parsing with edge cases
- `TestCurrentVersion` - Constants verification
- `TestErrCodeVersionMismatch` - Error code string

All tests pass with race detection enabled.

### Usage Example

```go
// Get current version
version := node.Version()
fmt.Printf("Running Glueberry protocol %s\n", version.String())

// During handshake, exchange and check versions
myVersion := glueberry.CurrentVersion()
// ... send myVersion to peer, receive peerVersion ...

if !myVersion.Compatible(peerVersion) {
    return glueberry.ErrVersionMismatch
}

// Parse version from string (e.g., from config or header)
v, err := glueberry.ParseVersion("1.2.3")
if err != nil {
    return fmt.Errorf("invalid version: %w", err)
}
```

### Design Decisions

1. **Semantic versioning**: Following standard semver conventions makes compatibility rules intuitive and familiar to developers.

2. **Minor version compatibility**: We can communicate with peers that have OLDER minor versions (they're using a subset of our features), but NOT with peers that have NEWER minor versions (they might use features we don't support).

3. **uint8 for version numbers**: Protocol versions don't need to exceed 255. Using uint8 saves bytes in wire format and makes the version fit in 3 bytes.

4. **Application-level checking**: The library provides the tools but doesn't enforce version checking. Applications can decide whether to reject connections, warn, or accept incompatible versions.

---

## Phase: P2-1 - Flow Control and Backpressure

**Status**: ✅ Completed
**Priority**: P1 (Production Readiness)

### Summary

Implemented flow control and backpressure to prevent overwhelming peers or buffers when sending messages. The system uses high/low watermark thresholds to block sends when too many messages are pending, and automatically unblocks when the backlog is processed.

### Files Created

- `internal/flow/controller.go` - FlowController type with acquire/release semantics
- `internal/flow/controller_test.go` - Comprehensive tests for flow controller
- `flow_control_test.go` - Tests for config options and error codes

### Files Modified

- `config.go`:
  - Added `DefaultHighWatermark`, `DefaultLowWatermark`, `DefaultMaxMessageSize` constants
  - Added `HighWatermark`, `LowWatermark`, `MaxMessageSize`, `DisableBackpressure` fields to Config
  - Added validation for flow control config values
  - Added `WithHighWatermark()`, `WithLowWatermark()`, `WithMaxMessageSize()`, `WithBackpressureDisabled()` options

- `errors.go`:
  - Added `ErrCodeMessageTooLarge` and `ErrCodeBackpressure` error codes
  - Added `ErrMessageTooLarge` and `ErrBackpressureTimeout` sentinel errors

- `metrics.go`:
  - Added `BackpressureEngaged(stream)` method for backpressure events
  - Added `BackpressureWait(stream, seconds)` method for wait time metrics
  - Added `PendingMessages(stream, count)` method for pending message gauge

- `node.go`:
  - Added `flowControllers` map and mutex for per-stream flow control
  - Updated `SendCtx()` to check max message size and apply flow control
  - Added `getOrCreateFlowController()` method with metrics callbacks

- `metrics_test.go`:
  - Added fields and methods for new metrics to TestMetrics

### Key Functionality Implemented

1. **Flow Controller**:
   - High/low watermark thresholds (default 1000/100 messages)
   - Context-aware Acquire() with cancellation support
   - Non-blocking Release() with automatic unblock
   - Thread-safe with proper mutex handling

2. **Per-Stream Flow Control**:
   - Each stream name gets its own flow controller
   - Flow controllers created lazily on first send
   - Metrics callback on backpressure engagement

3. **Max Message Size**:
   - Default 1MB maximum message size
   - Configurable via `WithMaxMessageSize()`
   - Returns `ErrCodeMessageTooLarge` for oversized messages

4. **Backpressure Configuration**:
   - Enabled by default (`DisableBackpressure = false`)
   - Can be disabled with `WithBackpressureDisabled()`
   - Configurable watermarks via `WithHighWatermark()` and `WithLowWatermark()`

5. **Metrics Integration**:
   - `BackpressureEngaged(stream)` - counter for backpressure events
   - `BackpressureWait(stream, seconds)` - histogram of wait times
   - `PendingMessages(stream, count)` - gauge of pending messages

### Test Coverage

Flow Controller tests:
- `TestNewController_Defaults` - Default watermark values
- `TestNewController_CustomValues` - Custom watermark values
- `TestNewController_LowWatermarkAdjustment` - Auto-adjustment when low >= high
- `TestController_AcquireRelease_Basic` - Basic acquire/release
- `TestController_BlocksAtHighWatermark` - Blocking at threshold
- `TestController_UnblocksAtLowWatermark` - Unblocking at threshold
- `TestController_ContextCancelled` - Cancellation support
- `TestController_BlockedCallback` - Metrics callback
- `TestController_Close` - Graceful shutdown
- `TestController_Concurrent` - Thread safety

Config tests:
- `TestConfig_FlowControl_Defaults` - Default values
- `TestConfig_FlowControl_Validate` - Validation of flow control fields
- `TestWithHighWatermark`, `TestWithLowWatermark`, `TestWithMaxMessageSize` - Config options

All tests pass with race detection enabled.

### Usage Example

```go
// Configure custom flow control
cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithHighWatermark(500),    // Block at 500 pending
    glueberry.WithLowWatermark(50),      // Unblock at 50 pending
    glueberry.WithMaxMessageSize(512*1024), // 512KB max
)

// Send with context timeout (backpressure-aware)
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

err := node.SendCtx(ctx, peerID, "data", message)
if err != nil {
    var gErr *glueberry.Error
    if errors.As(err, &gErr) {
        switch gErr.Code {
        case glueberry.ErrCodeMessageTooLarge:
            log.Printf("Message too large: %s", gErr.Message)
        case glueberry.ErrCodeBackpressure:
            log.Printf("Send blocked by backpressure, retry later")
        }
    }
}

// Disable backpressure for fire-and-forget scenarios
cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithBackpressureDisabled(),
)
```

### Design Decisions

1. **Per-stream flow control**: Each stream has its own flow controller, allowing different streams to have independent backpressure. This prevents a slow stream from blocking all communication.

2. **High/low watermark**: Using two thresholds prevents oscillation (thrashing). Sends block at high watermark and unblock at low watermark, providing hysteresis.

3. **Backpressure enabled by default**: Production systems should have backpressure to prevent resource exhaustion. Applications can opt-out if needed.

4. **Context-aware blocking**: The Acquire() method respects context deadlines and cancellation, allowing applications to set timeouts on send operations.

5. **Metrics for observability**: Flow control events are tracked via the Metrics interface, allowing operators to monitor backpressure in production.

---
