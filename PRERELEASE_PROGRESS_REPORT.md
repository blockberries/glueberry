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

## Phase: P2-3 - Handshake Robustness

**Status**: ✅ Completed
**Priority**: P1 (Production Readiness)

### Summary

Implemented a handshake state machine utility type that applications can use to track handshake progress and implement retry logic. This helps prevent hangs when handshake messages are lost (such as HelloFinalize in the Blockberry integration).

### Files Created

- `handshake_state.go` - HandshakeStateMachine type and HandshakeState enum
- `handshake_state_test.go` - Comprehensive tests for state machine

### Key Functionality Implemented

1. **HandshakeState enum**:
   - `HandshakeStateInit` - Initial state
   - `HandshakeStateSentRequest` - Request sent, awaiting response
   - `HandshakeStateReceivedResponse` - Response received
   - `HandshakeStateSentFinalize` - Final message sent
   - `HandshakeStateComplete` - Handshake successful
   - `HandshakeStateFailed` - Handshake failed

2. **HandshakeStateMachine**:
   - Tracks current state with valid transition enforcement
   - Retry counting with configurable max retries
   - Thread-safe for concurrent access
   - Error tracking for failed handshakes
   - Duration tracking for metrics

3. **State Machine Methods**:
   - `Transition(to)` - Attempt state transition
   - `TransitionWithRetry(to)` - Transition with retry tracking
   - `Fail(err)` - Mark handshake as failed
   - `CanRetry()` - Check if retries available
   - `IsComplete()`, `IsFailed()`, `IsTerminal()` - State queries
   - `Reset()` - Reset for retry from scratch

4. **Valid Transitions**:
   - Init → SentRequest, Failed
   - SentRequest → ReceivedResponse, SentRequest (retry), Failed
   - ReceivedResponse → SentFinalize, ReceivedResponse (retry), Failed
   - SentFinalize → Complete, SentFinalize (retry), Failed

5. **Error Types**:
   - `ErrInvalidStateTransition` - Invalid transition attempted
   - `ErrHandshakeMaxRetries` - Max retries exceeded

### Test Coverage

- `TestHandshakeState_String` - State string formatting
- `TestNewHandshakeStateMachine` - Initialization
- `TestNewHandshakeStateMachine_NegativeRetries` - Edge case
- `TestHandshakeStateMachine_ValidTransitions` - Valid transition sequences
- `TestHandshakeStateMachine_InvalidTransitions` - Invalid transitions rejected
- `TestHandshakeStateMachine_TransitionWithRetry` - Retry counting and limits
- `TestHandshakeStateMachine_Fail` - Failure handling
- `TestHandshakeStateMachine_CanRetry` - Retry availability
- `TestHandshakeStateMachine_CanRetry_TerminalStates` - Terminal state behavior
- `TestHandshakeStateMachine_Reset` - Reset functionality
- `TestHandshakeStateMachine_Duration` - Duration tracking
- `TestHandshakeStateMachine_IsTerminal` - Terminal state detection
- `TestHandshakeStateMachine_TransitionToFailed` - Fail from any state
- `TestHandshakeStateMachine_Concurrent` - Thread safety

All tests pass with race detection enabled.

### Usage Example

```go
// Create state machine with 3 retries
hsm := glueberry.NewHandshakeStateMachine(3)

// Track handshake progress
func doHandshake(node *glueberry.Node, peerID peer.ID, hsm *glueberry.HandshakeStateMachine) error {
    // Send request
    if err := hsm.Transition(glueberry.HandshakeStateSentRequest); err != nil {
        return err
    }
    if err := sendHelloRequest(peerID); err != nil {
        hsm.Fail(err)
        return err
    }

    // Receive response
    response, err := receiveHelloResponse(peerID)
    if err != nil {
        // Timeout? Try retrying the request
        if hsm.CanRetry() {
            hsm.TransitionWithRetry(glueberry.HandshakeStateSentRequest)
            // ... retry logic
        }
        hsm.Fail(err)
        return err
    }
    hsm.Transition(glueberry.HandshakeStateReceivedResponse)

    // Send finalize
    if err := hsm.Transition(glueberry.HandshakeStateSentFinalize); err != nil {
        return err
    }
    if err := sendHelloFinalize(peerID); err != nil {
        hsm.Fail(err)
        return err
    }

    // Complete
    if err := hsm.Transition(glueberry.HandshakeStateComplete); err != nil {
        return err
    }

    log.Printf("Handshake completed in %v with %d retries", hsm.Duration(), hsm.Retries())
    return nil
}
```

### Design Decisions

1. **App-level state machine**: Since Glueberry's handshake is app-controlled, the state machine is a utility type that apps can optionally use. It doesn't change the fundamental handshake model.

2. **Self-transition for retries**: Transitioning to the same state counts as a retry. This allows simple retry logic: `hsm.TransitionWithRetry(currentState)`.

3. **Max retries enforcement**: When max retries is exceeded, the state automatically transitions to Failed. This prevents infinite retry loops.

4. **Thread-safe design**: All state access is mutex-protected, allowing the state machine to be shared across goroutines (e.g., timeout goroutine and main handshake goroutine).

5. **Reset capability**: Failed handshakes can be reset and retried from scratch, useful for reconnection scenarios.

---

## Phase: P4-1 - Benchmark Suite

**Status**: ✅ Completed
**Priority**: P2 (Performance Baseline)

### Summary

Implemented a comprehensive benchmark suite for measuring crypto performance, message serialization, and stream throughput. The benchmarks provide performance baselines for key operations and support performance regression testing.

### Files Created

- `benchmark/crypto_bench_test.go` - Cryptographic operation benchmarks
- `benchmark/stream_bench_test.go` - Message path and serialization benchmarks

### Files Modified

- `Makefile`:
  - Added `benchmark` and `bench` targets for running benchmarks
  - Added `bench-crypto` target for crypto-only benchmarks
  - Added `bench-all` target for running benchmarks across entire codebase
  - Added `bench-compare` target with instructions for using benchstat

### Key Functionality Implemented

1. **Crypto Benchmarks** (`benchmark/crypto_bench_test.go`):
   - `BenchmarkEd25519PrivateToX25519` - Ed25519 to X25519 private key conversion
   - `BenchmarkEd25519PublicToX25519` - Ed25519 to X25519 public key conversion
   - `BenchmarkComputeX25519SharedSecret` - Raw ECDH shared secret computation
   - `BenchmarkDeriveSharedKey` - Full key derivation (ECDH + HKDF)
   - `BenchmarkEncrypt_*` - Encryption at 64B, 256B, 1KB, 4KB, 16KB, 64KB, 1MB
   - `BenchmarkDecrypt_*` - Decryption at 64B, 256B, 1KB, 4KB, 16KB, 64KB, 1MB
   - `BenchmarkModule_DeriveSharedKey` - Module-level key derivation (cached/uncached)
   - `BenchmarkModule_EncryptDecrypt` - Module-level encrypt/decrypt operations

2. **Message Path Benchmarks** (`benchmark/stream_bench_test.go`):
   - `BenchmarkMessagePath_*` - Full send path (cramberry serialize + encrypt) at 64B to 64KB
   - `BenchmarkReceivePath_*` - Full receive path (decrypt + cramberry deserialize) at 64B to 64KB
   - `BenchmarkCramberry_Marshal_*` - Cramberry serialization alone
   - `BenchmarkCramberry_Unmarshal_*` - Cramberry deserialization alone

3. **Makefile Targets**:
   - `make benchmark` / `make bench` - Run benchmark suite
   - `make bench-crypto` - Run crypto benchmarks only (skips unit tests)
   - `make bench-all` - Run benchmarks across entire codebase
   - `make bench-compare` - Instructions for performance comparison with benchstat

### Benchmark Results (Reference)

Crypto operations:
- Encrypt/Decrypt 1MB: ~1.2 GB/s throughput
- Encrypt/Decrypt 64KB: ~1.0 GB/s throughput
- Key derivation (cached): ~35 ns/op
- Key derivation (uncached): ~125 µs/op (includes key generation)

Message path:
- Send path (serialize+encrypt) 1KB: ~685 MB/s
- Receive path (decrypt+deserialize) 1KB: ~440 MB/s
- Cramberry Marshal 64KB: ~3.5 GB/s (essentially memory copy)

### Test Coverage

All benchmarks run successfully with proper setup/teardown:
- Random key/data generation for realistic measurements
- Proper use of `b.ResetTimer()` to exclude setup
- `b.SetBytes()` for throughput calculations
- Memory allocation tracking with `-benchmem`

### Usage Example

```bash
# Run all benchmarks
make bench

# Run crypto benchmarks only
make bench-crypto

# Compare performance between versions
# First, save baseline:
go test -bench=. -benchmem ./benchmark/... > baseline.txt

# After changes, save new results:
go test -bench=. -benchmem ./benchmark/... > current.txt

# Compare using benchstat:
benchstat baseline.txt current.txt
```

### Design Decisions

1. **Separate benchmark package**: Benchmarks are in their own `benchmark/` directory to avoid polluting the main package and to allow independent execution.

2. **Multiple message sizes**: Benchmarks test various message sizes (64B to 1MB) to show how performance scales with data size. This helps identify any non-linear performance characteristics.

3. **Hot path focus**: The message path benchmarks focus on the serialization + encryption path (the "hot path" for sending/receiving messages) rather than trying to mock full network streams.

4. **Cached vs uncached key derivation**: The Module benchmarks explicitly test both cached (repeat peer) and uncached (new peer) key derivation to show the benefit of the internal cache.

5. **Memory allocation tracking**: All benchmarks are run with `-benchmem` to track allocations per operation, which is important for garbage collection pressure in high-throughput scenarios.

---

## Phase: P3-3 - Address Book Persistence Improvements

**Status**: ✅ Completed
**Priority**: P2 (Reliability)

### Summary

Implemented file locking for the address book to ensure safe concurrent access from multiple processes. This prevents file corruption when multiple instances of an application use the same address book file.

### Files Modified

- `pkg/addressbook/storage.go`:
  - Added `lockFileSuffix` constant for lock file naming
  - Added `lockPath` field to storage struct
  - Added `acquireFileLock()` method using `syscall.Flock` for exclusive file locking
  - Added `releaseFileLock()` method for releasing locks
  - Updated `load()` to acquire file lock before reading
  - Updated `save()` to acquire file lock before writing
  - Added `Sync()` call before atomic rename for durability

- `pkg/addressbook/book_test.go`:
  - Added `TestFileLocking` - Verifies concurrent writes from multiple Book instances
  - Added `TestFileLocking_LockFileCreated` - Verifies lock file is created

### Key Functionality Implemented

1. **Inter-Process File Locking**:
   - Uses `syscall.Flock` with `LOCK_EX` for exclusive locks
   - Lock file created at `{addressbook_path}.lock`
   - Blocking lock acquisition ensures serialized access

2. **Durability Improvements**:
   - Added `Sync()` call after writing temp file
   - Ensures data is flushed to disk before atomic rename
   - Protects against data loss on system crash

3. **Lock Lifecycle**:
   - Lock acquired at start of load/save operations
   - Lock released after operation completes (via defer)
   - Lock file persists between operations (reused)

### Test Coverage

- `TestFileLocking` - Concurrent writes from two Book instances
- `TestFileLocking_LockFileCreated` - Lock file existence verification
- All existing tests pass with new locking

All tests pass with race detection enabled.

### Usage Notes

The file locking is automatic and transparent to users. When multiple processes or Book instances access the same address book file:

1. Operations are serialized via the lock
2. No data corruption occurs
3. Each operation sees a consistent view of the data

```go
// Safe to use from multiple processes
book1, _ := addressbook.New("/shared/addressbook.json")
book2, _ := addressbook.New("/shared/addressbook.json")

// Concurrent operations are safe
go book1.AddPeer(peer1, addrs, nil)
go book2.AddPeer(peer2, addrs, nil)
```

### Design Decisions

1. **Separate lock file**: Using a `.lock` file rather than locking the main file allows atomic rename to work correctly and avoids issues with file handle inheritance.

2. **Blocking locks**: Operations block until the lock is acquired. This is appropriate for address book operations which should be quick.

3. **Lock per operation**: Each load/save operation acquires and releases the lock. This provides fine-grained concurrency while ensuring consistency.

4. **Unix-only flock**: Uses `syscall.Flock` which is available on Unix systems. For cross-platform support in the future, could add build tags for Windows.

---

## Phase: P4-2 - Buffer Pooling

**Status**: ✅ Completed
**Priority**: P2 (Performance)

### Summary

Implemented a buffer pooling utility to reduce GC pressure for high-throughput scenarios. The pool provides size-classed buffer allocation that reuses memory instead of allocating new buffers for each operation.

### Files Created

- `internal/pool/buffer.go` - Buffer pool implementation with size classes
- `internal/pool/buffer_test.go` - Comprehensive tests and benchmarks

### Key Functionality Implemented

1. **BufferPool Type**:
   - Three size classes: small (≤1KB), medium (≤4KB), large (≤64KB)
   - Very large buffers (>64KB) allocated directly (not pooled)
   - Thread-safe using `sync.Pool`

2. **Pool Methods**:
   - `Get(size int) *[]byte` - Get buffer with at least specified capacity
   - `Put(buf *[]byte)` - Return buffer to pool
   - `GetExact(size int) *[]byte` - Get buffer with exact length (zeroed)

3. **Global Pool Functions**:
   - `GetBuffer(size int)` - Get from global pool
   - `PutBuffer(buf *[]byte)` - Return to global pool
   - `GetExactBuffer(size int)` - Get exact-sized from global pool

4. **Size Classes**:
   - Small: ≤ 1024 bytes (SmallBufferSize)
   - Medium: ≤ 4096 bytes (DefaultBufferSize)
   - Large: ≤ 65536 bytes (LargeBufferSize)
   - Direct: > 65536 bytes (not pooled)

### Test Coverage

- `TestNewBufferPool` - Pool creation
- `TestBufferPool_Get_Small/Medium/Large/VeryLarge` - Size class selection
- `TestBufferPool_Put_ReturnsToPool` - Buffer reuse
- `TestBufferPool_Put_Nil` - Nil safety
- `TestBufferPool_GetExact` - Exact-size allocation with zeroing
- `TestGlobalPool` - Global pool functions
- `TestBufferPool_Concurrent` - Thread safety
- `TestBufferPool_SizeClasses` - Size class boundaries

Benchmarks included comparing pool vs direct allocation.

All tests pass with race detection enabled.

### Usage Example

```go
import "github.com/blockberries/glueberry/internal/pool"

// Using the global pool
buf := pool.GetBuffer(1024)
*buf = append(*buf, data...)
// ... use buffer ...
pool.PutBuffer(buf)

// Using a dedicated pool
p := pool.NewBufferPool()
buf := p.Get(4096)
*buf = append(*buf, moreData...)
p.Put(buf)

// Getting exact-sized, zeroed buffer
zeroed := pool.GetExactBuffer(256)
// Buffer is guaranteed to be 256 bytes and zeroed
```

### Design Decisions

1. **Size classes**: Using multiple size classes prevents wasting memory when small buffers are requested but large ones are returned.

2. **Pointer to slice**: Using `*[]byte` allows the slice header itself to be reused, not just the backing array.

3. **Not pooling very large buffers**: Buffers > 64KB are not pooled to avoid holding large amounts of memory. Let GC handle these.

4. **Global pool**: Provides convenient access for simple use cases. Applications can also create dedicated pools.

5. **Internal package**: Currently an internal utility. Can be exposed as public API if needed by applications.

---

## Phase: P5-2 - Testing Utilities

**Status**: ✅ Completed
**Priority**: P2 (Developer Experience)

### Summary

Implemented a MockNode for unit testing applications that use Glueberry. The mock provides a complete simulation of the Node API with helpers for injecting test scenarios.

### Files Created

- `testing/mock.go` - MockNode implementation
- `testing/mock_test.go` - Comprehensive tests for MockNode

### Key Functionality Implemented

1. **MockNode Type**:
   - Simulates Glueberry Node API
   - Thread-safe implementation
   - Tracks all sent messages for assertions

2. **Core Node Methods**:
   - `Start()` / `Stop()` - Lifecycle simulation
   - `AddPeer()` / `RemovePeer()` / `GetPeer()` / `ListPeers()` - Peer management
   - `BlacklistPeer()` / `UnblacklistPeer()` - Blacklist management
   - `Connect()` / `ConnectCtx()` / `Disconnect()` - Connection simulation
   - `Send()` / `SendCtx()` - Message sending with tracking
   - `Messages()` / `Events()` - Channel access

3. **Test Simulation Helpers**:
   - `SimulateConnect(peerID, addrs)` - Simulate inbound connection
   - `SimulateDisconnect(peerID)` - Simulate peer disconnect
   - `SimulateMessage(peerID, stream, data)` - Simulate incoming message
   - `SimulateEvent(peerID, state, err)` - Simulate connection event

4. **Error Injection**:
   - `SetConnectError(err)` - Make Connect return error
   - `SetSendError(err)` - Make Send return error
   - `SetDisconnectError(err)` - Make Disconnect return error

5. **Assertions**:
   - `SentMessages()` - Get all sent messages
   - `AssertSent(peerID, stream)` - Get messages sent to peer/stream
   - `AssertNotSent(peerID, stream)` - Assert no messages sent
   - `ClearSentMessages()` - Clear sent message history

### Test Coverage

Comprehensive tests for all MockNode functionality:
- Creation and lifecycle
- Peer management (add, remove, blacklist)
- Connection simulation
- Message sending and tracking
- Error injection
- Event simulation
- Reset functionality

All tests pass with race detection enabled.

### Usage Example

```go
import gtesting "github.com/blockberries/glueberry/testing"

func TestMyApp(t *testing.T) {
    // Create mock node
    node := gtesting.NewMockNode()
    node.Start()
    defer node.Stop()

    // Simulate a peer connecting
    peerID := peer.ID("test-peer")
    node.SimulateConnect(peerID, nil)

    // Run your application code
    myApp := NewMyApp(node)
    myApp.HandleConnection(peerID)

    // Verify messages were sent
    msgs := node.AssertSent(peerID, "mystream")
    if len(msgs) != 1 {
        t.Errorf("expected 1 message, got %d", len(msgs))
    }

    // Simulate receiving a message
    node.SimulateMessage(peerID, "mystream", []byte("response"))

    // Test error handling
    node.SetSendError(errors.New("network error"))
    err := myApp.SendData(peerID, []byte("test"))
    if err == nil {
        t.Error("expected error")
    }
}
```

### Design Decisions

1. **Separate testing package**: The mock is in a dedicated `testing` package to avoid polluting the main package and to clearly indicate its purpose.

2. **Channel-based events**: Uses buffered channels for events and messages, matching the real Node API.

3. **Data copying**: All data passed to/from the mock is copied to prevent accidental mutation.

4. **Thread-safe**: All operations are protected by mutexes for safe concurrent use.

5. **Error injection**: Allows testing error handling paths by setting errors that will be returned.

---

## Phase: P5-3 - Debug Utilities

**Status**: ✅ Completed
**Priority**: P2 (Developer Experience)

### Summary

Implemented debug utilities for troubleshooting Glueberry nodes. The utilities provide node state inspection in multiple formats (struct, JSON, human-readable).

### Files Created

- `debug.go` - Debug utilities implementation
- `debug_test.go` - Comprehensive tests for debug utilities

### Key Functionality Implemented

1. **DumpState()** - Returns complete node state as a `DebugState` struct:
   - Node identity (peer ID, public key)
   - Listen addresses
   - Protocol version
   - Address book summary
   - Configuration
   - Statistics summary
   - Flow controller states

2. **DumpStateJSON()** - Returns node state as formatted JSON string

3. **DumpStateString()** - Returns human-readable formatted state

4. **ConnectionSummary()** - Returns brief summary of connection states

5. **ListKnownPeers()** - Returns list of known peer IDs from address book

6. **PeerInfo(peerID)** - Returns detailed info about a specific peer

### Test Coverage

Comprehensive tests for all debug utilities. All tests pass with race detection enabled.

### Usage Example

```go
// Get structured state
state := node.DumpState()
fmt.Printf("Node %s has %d known peers\n", state.PeerID, state.AddressBook.ActivePeers)

// Get JSON for logging or API responses
jsonStr, _ := node.DumpStateJSON()
log.Printf("Node state: %s", jsonStr)

// Get human-readable output for CLI/debugging
fmt.Println(node.DumpStateString())
```

### Design Decisions

1. **Multiple output formats**: Provides struct, JSON, and string formats to suit different use cases.

2. **Non-invasive**: Uses only public methods and existing state, doesn't affect node operation.

3. **Thread-safe**: All state access is properly locked to prevent data races.

4. **Timestamp included**: Each state capture includes a timestamp for correlation with logs.

---

## Phase: P5-1 - Additional Examples

**Status**: ✅ Completed
**Priority**: P2 (Developer Experience)

### Summary

Created comprehensive examples demonstrating various Glueberry usage patterns. The examples show real-world integration scenarios including file transfer, RPC patterns, cluster coordination, and blockchain integration.

### Files Created

- `examples/file-transfer/main.go` - Large data transfer with progress reporting
- `examples/rpc/main.go` - Request/response (RPC) communication pattern
- `examples/cluster/main.go` - Multi-node cluster with gossip-based discovery
- `examples/blockberry-integration/main.go` - Blockchain node integration pattern

### Key Functionality Demonstrated

1. **File Transfer Example** (`examples/file-transfer/`):
   - Chunked file transmission (64KB chunks)
   - Progress reporting during transfer
   - SHA-256 checksum verification
   - Acknowledgment-based flow
   - Interactive CLI for sending files

2. **RPC Pattern Example** (`examples/rpc/`):
   - Request/response messaging over encrypted streams
   - Request ID tracking for response correlation
   - Multiple concurrent pending requests
   - Timeout support with context
   - Example methods: echo, add, getTime, getPeers
   - Error handling with structured error responses

3. **Multi-Node Cluster Example** (`examples/cluster/`):
   - Gossip-based peer discovery
   - Automatic connection to discovered peers
   - Broadcast messaging with deduplication
   - Ping/pong heartbeat for liveness
   - Member join/leave tracking
   - Interactive CLI for cluster operations

4. **Blockchain Integration Example** (`examples/blockberry-integration/`):
   - Custom handshake protocol with chain validation
   - Multiple stream types (consensus, mempool, sync)
   - Connection direction handling (important for protocol roles)
   - Genesis/ChainID verification during handshake
   - Transaction gossip simulation
   - Block proposal simulation
   - Peer scoring concept
   - Two-phase handshake using PrepareStreams/FinalizeHandshake

### Usage Examples

```bash
# File Transfer
# Terminal 1 (receiver):
go run ./examples/file-transfer/ -port 9000

# Terminal 2 (sender):
go run ./examples/file-transfer/ -port 9001 -peer /ip4/127.0.0.1/tcp/9000/p2p/... -send myfile.txt

# RPC
# Terminal 1 (server):
go run ./examples/rpc/ -port 9000

# Terminal 2 (client):
go run ./examples/rpc/ -port 9001 -peer /ip4/127.0.0.1/tcp/9000/p2p/...
> echo hello
> add 5 3
> time

# Cluster
# Terminal 1 (first node):
go run ./examples/cluster/ -port 9000 -name node1

# Terminal 2 (join cluster):
go run ./examples/cluster/ -port 9001 -name node2 -bootstrap /ip4/127.0.0.1/tcp/9000/p2p/...

# Terminal 3 (join cluster):
go run ./examples/cluster/ -port 9002 -name node3 -bootstrap /ip4/127.0.0.1/tcp/9001/p2p/...

# Blockchain Integration
# Terminal 1:
go run ./examples/blockberry-integration/ -port 9000 -chain testnet-1

# Terminal 2:
go run ./examples/blockberry-integration/ -port 9001 -chain testnet-1 -peer /ip4/127.0.0.1/tcp/9000/p2p/...
> tx hello world
> propose
> peers
```

### Test Coverage

All examples build successfully with `go build ./examples/...`. No tests required for examples as they are demonstration code.

### Design Decisions

1. **Interactive CLI**: All examples include interactive command-line interfaces for easy experimentation and demonstration.

2. **Self-contained**: Each example is self-contained with its own handshake protocol, demonstrating how applications would implement the full connection flow.

3. **Realistic patterns**: Examples demonstrate realistic patterns (chunking, progress, checksums) rather than trivial hello-world examples.

4. **Two-phase handshake**: The blockchain example demonstrates the recommended two-phase handshake (PrepareStreams + FinalizeHandshake) for avoiding race conditions.

5. **Connection direction**: The blockchain example shows how to use IsOutbound() to determine protocol roles (initiator vs responder).

---

## Phase CP-2: Fuzz Testing Infrastructure

**Status**: ✅ Completed
**Priority**: P0 (Critical - Security)

### Summary

Implemented comprehensive fuzz testing infrastructure to identify panics, buffer overflows, and other issues when handling malformed or malicious input. This addresses the security audit preparation requirement for systematic testing of untrusted input handling.

### Files Created

- `fuzz/crypto_fuzz_test.go` - Fuzz tests for cryptographic operations
- `fuzz/addressbook_fuzz_test.go` - Fuzz tests for address book JSON parsing
- `fuzz/cramberry_fuzz_test.go` - Fuzz tests for Cramberry message framing
- `fuzz/handshake_fuzz_test.go` - Fuzz tests for handshake protocol parsing

### Files Modified

- `Makefile` - Added fuzz testing targets

### Fuzz Tests Implemented

1. **Crypto Fuzz Tests** (`fuzz/crypto_fuzz_test.go`):
   - `FuzzDecrypt` - Tests decryption with malformed ciphertext
   - `FuzzDecryptRoundTrip` - Tests encrypt/decrypt consistency
   - `FuzzEd25519PublicToX25519` - Tests key conversion with invalid keys
   - `FuzzEd25519PrivateToX25519` - Tests private key conversion
   - `FuzzNewCipher` - Tests cipher creation with various key sizes

2. **Address Book Fuzz Tests** (`fuzz/addressbook_fuzz_test.go`):
   - `FuzzAddressBookJSON` - Tests address book JSON parsing with malformed data
   - `FuzzPeerEntryJSON` - Tests peer entry JSON parsing
   - `FuzzMultiaddrParsing` - Tests multiaddr string handling

3. **Cramberry Message Fuzz Tests** (`fuzz/cramberry_fuzz_test.go`):
   - `FuzzMessageIterator` - Tests delimited message reading with corrupted data
   - `FuzzStreamReaderVarint` - Tests varint parsing with truncated/overflow data
   - `FuzzStreamReaderString` - Tests string parsing with invalid UTF-8
   - `FuzzStreamReaderBytes` - Tests byte slice parsing
   - `FuzzStreamWriterReader` - Tests write/read round-trip consistency
   - `FuzzDelimitedMessages` - Tests delimited message format
   - `FuzzMarshalUnmarshal` - Tests type serialization round-trip

4. **Handshake Protocol Fuzz Tests** (`fuzz/handshake_fuzz_test.go`):
   - `FuzzHandshakeMessageParsing` - Tests handshake message parsing
   - `FuzzHandshakeDelimited` - Tests delimited handshake messages
   - `FuzzHandshakeMessageRoundTrip` - Tests handshake serialization consistency
   - `FuzzCryptoMaterial` - Tests cryptographic field handling
   - `FuzzMultipleHandshakeMessages` - Tests sequential message parsing

### Makefile Targets Added

```makefile
# Run all fuzz tests (default 30s each)
make fuzz

# Run specific fuzz test categories
make fuzz-crypto
make fuzz-addressbook
make fuzz-cramberry
make fuzz-handshake

# Override fuzz duration
FUZZTIME=60s make fuzz

# List available fuzz targets
make fuzz-list
```

### Seed Corpus

Each fuzz test includes a comprehensive seed corpus covering:
- Valid inputs (baseline coverage)
- Edge cases (empty, truncated, oversized)
- Malformed inputs (invalid encoding, wrong types)
- Attack vectors (overflow attempts, deeply nested structures)
- Unicode edge cases (invalid UTF-8, surrogate halves)

### Test Coverage

All fuzz tests pass with no panics detected during initial fuzzing runs. Example output:
```
=== RUN   FuzzHandshakeMessageParsing
fuzz: elapsed: 3s, execs: 543676, new interesting: 96
--- PASS: FuzzHandshakeMessageParsing (3.01s)
```

### Design Decisions

1. **Mirror internal types**: For address book fuzzing, internal types are mirrored in the fuzz package to avoid import cycles while still testing JSON parsing behavior.

2. **Focus on boundaries**: Fuzz tests focus on system boundaries where untrusted data enters (network messages, file parsing) rather than internal APIs.

3. **Configurable duration**: The FUZZTIME variable allows adjusting fuzz duration for different CI/local testing scenarios.

4. **Comprehensive seed corpus**: Each test includes 10-20 seed inputs covering valid data, edge cases, and known attack patterns to guide the fuzzer effectively.

5. **No panic tolerance**: All fuzz tests verify that malformed input results in errors, not panics, ensuring robust error handling.

---

## Phase CP-1: Security Audit Preparation

**Status:** ✅ Completed
**Priority:** P0 (Critical - Security)

### Summary

Performed comprehensive security audit of the Glueberry codebase covering cryptographic implementation correctness, input validation, and resource exhaustion prevention.

### Files Modified

- `SECURITY_REVIEW.md` - Added CP-1 audit addendum with detailed findings

### CP-1.1: Cryptographic Implementation Review

**Status:** ✅ PASSED

**Verified:**
- Ed25519 → X25519 key conversion follows RFC 8032
- HKDF-SHA256 key derivation with domain separation (`"glueberry-v1-stream-key"`)
- ChaCha20-Poly1305 random nonce generation via `crypto/rand`
- Low-order point attack defense (all-zeros check)
- Key material zeroing implemented (`SecureZero`, `Close()`)
- Constant-time operations delegated to `golang.org/x/crypto`

**Key Files Reviewed:**
- `pkg/crypto/keys.go` - Key conversion
- `pkg/crypto/ecdh.go` - ECDH and HKDF
- `pkg/crypto/cipher.go` - ChaCha20-Poly1305
- `pkg/crypto/secure.go` - Key zeroing
- `pkg/crypto/module.go` - Crypto module lifecycle

### CP-1.2: Input Validation Audit

**Status:** ✅ PASSED

**Validated:**
- Key size validation (32 bytes for symmetric, 64/32 for Ed25519)
- Nonce size validation (12 bytes)
- Ciphertext minimum length validation
- Ed25519 public key curve point validation
- MaxMessageSize enforcement on send
- Configuration validation in `Config.Validate()`
- Address book JSON parsing with corruption handling

**Key Files Reviewed:**
- `pkg/crypto/*.go` - All crypto input validation
- `pkg/addressbook/storage.go` - JSON parsing
- `pkg/streams/handshake.go` - Handshake message handling
- `config.go` - Configuration validation
- `node.go` - Message size enforcement

**Observations (Non-Blocking):**
- MaxMessageSize only enforced on send, not receive
- Receive-side size validation recommended for future

### CP-1.3: Resource Exhaustion Review

**Status:** ✅ PASSED

**Verified:**
- Connection limits via libp2p ConnManager (100-400)
- Message buffer sizes configurable
- Flow control with high/low watermarks
- Buffer pooling for GC pressure reduction
- Handshake timeout enforcement (default 30s)
- Reconnection backoff with max attempts
- Blacklist enforcement via ConnectionGater

**Key Files Reviewed:**
- `config.go` - All resource limit defaults
- `pkg/protocol/host.go` - Connection manager setup
- `pkg/protocol/gater.go` - Connection gating
- `pkg/connection/manager.go` - Timeout enforcement
- `internal/flow/controller.go` - Flow control
- `internal/pool/buffer.go` - Buffer pooling

**Observations (Non-Blocking):**
- No per-stream receive buffer limit
- Silent message drops when channel full (documented behavior)
- No stream rate limiting (consider for high-security deployments)

### Security Review Document Updated

Added comprehensive CP-1 audit addendum to `SECURITY_REVIEW.md` including:
- Detailed validation point tables
- Resource limit inventory
- DoS mitigation mechanisms review
- Observations and recommendations

### Audit Summary

| Category | Status | Critical Issues |
|----------|--------|-----------------|
| CP-1.1 Cryptography | ✅ PASS | None |
| CP-1.2 Input Validation | ✅ PASS | None |
| CP-1.3 Resource Exhaustion | ✅ PASS | None |

**Overall CP-1 Status:** ✅ READY FOR RELEASE

---

## Race Condition Verification (Post-Release)

**Date:** 2026-01-26

### Task Description

Verify and fix race condition claims described in SECURITY_REVIEW.md Section 4.1:
1. Claim 1: Double locking in `peer.go` TransitionTo() method
2. Claim 2: TOCTOU race in `manager.go` ConnectCtx() method

### Verification Results

#### Claim 1: Double Locking - ❌ FALSE POSITIVE

**Original Claim:** The security review stated that `pc.State.ValidateTransition(newState)` was "also locked by caller".

**Analysis:**
- `ConnectionState` is defined as `type ConnectionState int` in `state.go`
- `ValidateTransition()` is a method on an integer type, not a struct with a mutex
- No double locking is possible because there is no second lock

**Verdict:** The claim was based on an incorrect assumption that `ValidateTransition()` operated on a mutex-protected struct.

#### Claim 2: TOCTOU Race - ❌ FALSE POSITIVE

**Original Claim:** State could change between `GetState()` check and `IsInCooldown()` check.

**Analysis:**
- `ConnectCtx()` holds `m.mu` (manager mutex) throughout state checks
- Lock ordering is consistent: Manager.mu → PeerConnection.mu
- The `GetState()` call acquires `conn.mu.RLock()` while `m.mu` is held
- Intentional lock release/reacquire in `handleHandshakeTimeout()` is a deliberate design pattern

**Verdict:** The state machine has proper mutex protection with consistent lock ordering.

### Testing Performed

1. **Race Detector Tests:**
   ```bash
   go test -race -count=1 ./...  # All pass
   go test -race -tags=integration ./...  # All pass
   ```

2. **Stress Tests Added:**
   Created `pkg/connection/stress_test.go` with 12 concurrent stress tests:
   - `TestPeerConnection_ConcurrentStateTransitions` (50 goroutines × 100 iterations)
   - `TestPeerConnection_ConcurrentReadWrite` (100 concurrent readers/writers)
   - `TestPeerConnection_ConcurrentCooldownOperations`
   - `TestPeerConnection_ConcurrentHandshakeTimeout`
   - `TestPeerConnection_StressCleanup`
   - `TestStateTransition_ConcurrentValidation`
   - `TestBackoffCalculator_ConcurrentAccess`
   - `TestMultiplePeerConnections_ConcurrentOperations`
   - `TestPeerConnection_RapidStateChanges`
   - `TestPeerConnection_ConcurrentSetAndCleanup`

### Files Modified

- `pkg/connection/stress_test.go` - NEW: Comprehensive concurrent stress tests
- `SECURITY_REVIEW.md` - Updated Section 4.1 and added verification addendum

### Conclusion

Both race condition claims from the security review were **false positives**. The implementation has:
- Proper mutex protection on all shared state
- Consistent lock ordering (Manager.mu → PeerConnection.mu)
- No data races detected under stress testing

**Status:** ✅ VERIFIED - No race conditions

---

## Phase 0: Critical Bug Fixes (P0)

### Phase 0.1: Fix SecureZero Compiler Optimization Issue

**Status:** ✅ Completed
**Priority:** P0 (Critical - Security)

**Issue:** The `SecureZero` function used a direct loop that could theoretically be optimized away by the compiler as a dead store elimination.

**Fix:** Changed implementation to use function variable indirection pattern, a technique used in security-conscious Go code to prevent compiler optimization:

```go
var memclrFunc = func(b []byte) {
    for i := range b {
        b[i] = 0
    }
}

func SecureZero(b []byte) {
    memclrFunc(b)
}
```

**Files Modified:** `pkg/crypto/secure.go`

---

### Phase 0.2: Fix Flow Controller Multi-Waiter Deadlock

**Status:** ✅ Completed
**Priority:** P0 (Critical - Correctness)

**Issue:** When multiple goroutines blocked waiting for flow control to unblock, only one would be woken up. The others would remain blocked forever (deadlock).

**Root Cause:** Two issues:
1. Waiters incremented `pending` before blocking, counting blocked waiters as pending work
2. Used a buffered channel that only unblocked one waiter instead of broadcasting

**Fix:**
1. Changed to not increment `pending` until permission is actually granted
2. Changed from buffered channel to close/recreate pattern that broadcasts to all waiters

**Files Modified:** `internal/flow/controller.go`
**Files Created:** `internal/flow/stress_test.go` (regression test)

---

### Phase 0.3: Add Receive-Path Message Size Validation

**Status:** ✅ Completed
**Priority:** P0 (Critical - Security)

**Issue:** `MaxMessageSize` was only validated on the send path, not the receive path. A malicious peer could send oversized messages to exhaust memory.

**Fix:** Added size validation in `EncryptedStream.readLoop()` before decryption with configurable callback:

```go
type OversizedMessageCallback func(peerID peer.ID, streamName string, size int)

type EncryptedStreamConfig struct {
    OnDecryptionError   DecryptionErrorCallback
    OnOversizedMessage  OversizedMessageCallback
    MaxMessageSize      int
}
```

**Files Modified:**
- `pkg/streams/encrypted.go` - Added config struct and size check in readLoop
- `pkg/streams/manager.go` - Updated to pass config to streams
- `pkg/streams/encrypted_test.go` - Added `TestEncryptedStream_OversizedMessageRejection`

---

## Phase 1: Security Hardening (P1)

### Phase 1.1: Add Cipher.Close() for Key Material Zeroing

**Status:** ✅ Completed
**Priority:** P1 (Security)

**Issue:** The `Cipher` struct didn't store a copy of the key, making it impossible to zero the key material when the cipher was no longer needed.

**Fix:**
1. Added `key []byte` field to store a copy of the key
2. Added `closed bool` field to track closure state
3. Added `Close()` method that zeros the key and nils the AEAD
4. Added `IsClosed()` method to check closure state
5. Updated `EncryptedStream.Close()` to call `cipher.Close()`

**Note:** The underlying chacha20poly1305 library stores its own internal copy of the key which cannot be zeroed from outside the package. This is documented in the code comments. The `Close()` method zeros our copy for defense in depth.

**Files Modified:**
- `pkg/crypto/cipher.go` - Added key field, Close(), IsClosed()
- `pkg/streams/encrypted.go` - Call cipher.Close() in stream Close()
- `pkg/crypto/cipher_test.go` - Added TestCipher_Close and TestCipher_Close_ZerosKey

---

### Phase 1.2: Make EncryptWithNonce Internal-Only

**Status:** ✅ Completed
**Priority:** P1 (Security)

**Issue:** The `EncryptWithNonce` method was publicly exported, allowing users to provide their own nonces. Nonce reuse with ChaCha20-Poly1305 is catastrophic - it completely breaks the encryption, allowing plaintext recovery.

**Fix:** Renamed `EncryptWithNonce` to `encryptWithNonce` (unexported). Since tests are in the same package (`package crypto`), they can still access the method for deterministic testing.

**Files Modified:**
- `pkg/crypto/cipher.go` - Renamed method to `encryptWithNonce`, enhanced warning comment
- `pkg/crypto/cipher_test.go` - Updated all references to use `encryptWithNonce`

---

### Phase 1.3: Fix Handshake Timeout Race Condition

**Status:** ✅ Completed
**Priority:** P1 (Security/Correctness)

**Issue:** A race condition existed between handshake timeout firing and handshake completing. The sequence was:
1. Timeout timer fires
2. `handleHandshakeTimeout` checks state == StateConnected ✓
3. Mutex unlocked
4. Meanwhile, `MarkEstablished` completes handshake, transitions to StateEstablished
5. `handleHandshakeTimeout` continues, disconnects an **established** connection!

**Fix:** Changed `handleHandshakeTimeout` to atomically transition to StateCooldown while holding the lock. If the transition fails (because another goroutine changed the state), we bail out without disconnecting:

```go
// Atomically transition to cooldown while holding the lock.
// This prevents the race where handshake completes between our state check
// and the disconnect call.
if err := conn.StartCooldown(m.config.FailedHandshakeCooldown); err != nil {
    m.mu.Unlock()
    return // Another goroutine is handling this or state changed
}
m.mu.Unlock()
// Now we "own" the cleanup
```

**Files Modified:**
- `pkg/connection/manager.go` - Fixed `handleHandshakeTimeout` to use atomic state transition
- `pkg/connection/stress_test.go` - Added `TestHandshakeTimeoutVsCompletion` stress test

---

### Phase 1.4: Fix Stream Manager Lock During Network I/O

**Status:** ✅ Completed
**Priority:** P1 (Performance/Correctness)

**Issue:** Both `openUnencryptedStreamLazilyCtx` and `openStreamLazilyCtx` held the manager mutex while calling `m.host.NewStream()`, which performs network I/O. This could:
1. Block all other stream operations while one stream is being opened
2. Cause deadlocks if network operations hang
3. Severely degrade performance under load

**Fix:** Restructured both functions to:
1. Acquire lock, check if stream exists, get required data (shared key, config)
2. Release lock before network I/O
3. Open the libp2p stream without holding the lock
4. Re-acquire lock
5. Check if another goroutine created a stream while we were waiting (race handling)
6. Store our stream or use the existing one

The fix uses double-checked locking pattern with proper race handling - if two goroutines try to open the same stream simultaneously, one will win and the other will close its stream and use the winner's stream.

**Files Modified:**
- `pkg/streams/manager.go` - Restructured `openUnencryptedStreamLazilyCtx` and `openStreamLazilyCtx` to release lock before network I/O

---

### Phase 1.5: Connection Rate Limiting

**Status:** ✅ Completed
**Priority:** P1 (Security/Resource Management)

**Issue:** No configurable connection limits were exposed to applications. The hardcoded libp2p connection manager watermarks (100/400) couldn't be adjusted.

**Fix:** Added configurable connection manager watermarks:
- `ConnMgrLowWatermark` - Low watermark for connection manager pruning (default: 100)
- `ConnMgrHighWatermark` - High watermark that triggers pruning (default: 400)
- Added corresponding `WithConnMgrLowWatermark()` and `WithConnMgrHighWatermark()` config options
- Wired config values to libp2p host creation

**Note:** The libp2p connection manager automatically prunes connections when the high watermark is exceeded, down to the low watermark. This provides effective peer limiting.

**Files Modified:**
- `config.go` - Added `ConnMgrLowWatermark` and `ConnMgrHighWatermark` fields, validation, defaults, and config options
- `config_test.go` - Added validation, default, and config option tests
- `node.go` - Wired config values to host creation

---

### Phase 1.6: Key Material Memory Protection - Consistency Fix

**Status:** ✅ Completed
**Priority:** P1 (Security)

**Issue:** Manual key zeroing (`for i := range key { key[i] = 0 }`) was used in some places instead of the `crypto.SecureZero()` function, which uses function variable indirection to prevent compiler optimization.

**Fix:** Replaced all manual key zeroing with `crypto.SecureZero()`:
- `pkg/streams/manager.go` - Updated `CloseStreams()` and `Shutdown()` to use `crypto.SecureZero()`
- `pkg/connection/peer.go` - Updated `Cleanup()` to use `crypto.SecureZero()`

This ensures consistent use of the secure zeroing function across the entire codebase, providing uniform protection against compiler optimization of dead stores.

**Files Modified:**
- `pkg/streams/manager.go` - Use `crypto.SecureZero()` instead of manual zeroing
- `pkg/connection/peer.go` - Added crypto import, use `crypto.SecureZero()` instead of manual zeroing

---

## Phase 2: Stability & Performance (P2)

### Phase 2.3: Add Peer Stats Cleanup

**Status:** ✅ Completed
**Priority:** P2 (Resource Management)

**Issue:** The `peerStats` map in Node grows unboundedly as peers connect over time, even after they disconnect. This can lead to memory growth in long-running nodes.

**Fix:**
1. Added `LastActivity()` method to `PeerStatsTracker` that returns the time of last activity
2. Added `cleanupStalePeerStats()` goroutine to Node that runs hourly
3. Cleanup removes stats for peers that:
   - Are not currently connected (StateEstablished)
   - Haven't had any activity in the last 24 hours

**Configuration:** The cleanup interval (1 hour) and stale threshold (24 hours) are hardcoded constants. They can be made configurable if needed.

**Files Modified:**
- `stats.go` - Added `LastActivity()` method to `PeerStatsTracker`
- `node.go` - Added `cleanupStalePeerStats()` goroutine

---

### Phase 2.1: Optimize Address Book Persistence

**Status:** ✅ Completed
**Priority:** P2 (Performance)

**Issue:** Every address book mutation triggered immediate disk I/O. `UpdateLastSeen()` is called on every connection, causing excessive disk writes that can impact performance, especially on slower storage or with many connections.

**Solution:** Implemented batched persistence for non-critical operations:
- Critical changes (AddPeer, RemovePeer, Blacklist, UpdatePublicKey, UpdateMetadata, Clear) still persist immediately
- Non-critical changes (UpdateLastSeen) are batched and flushed periodically
- Background goroutine flushes dirty changes every 5 seconds
- Explicit `Flush()` method for immediate persistence when needed
- `Close()` method flushes pending changes and stops the background goroutine

**Design Decisions:**
1. **5-second flush interval**: Balances data safety with I/O reduction. Loss of at most 5 seconds of LastSeen updates on crash.
2. **Background flush errors ignored**: Errors in periodic flush are silently ignored since they'll retry next interval. This prevents log spam.
3. **Close() flushes synchronously**: Ensures no data loss on graceful shutdown.
4. **Reload() clears dirty flag**: When reloading from disk, any pending in-memory changes are discarded.

**Files Modified:**
- `pkg/addressbook/book.go`:
  - Added `dirty` flag, `ctx`, and `cancel` fields to Book struct
  - Added `flushInterval` constant (5 seconds)
  - Modified `New()` to start background flush goroutine
  - Modified `UpdateLastSeen()` to mark dirty instead of saving immediately
  - Modified `saveLocked()` to clear dirty flag after successful save
  - Modified `Reload()` to clear dirty flag
  - Added `flushLoop()` method for periodic background flushing
  - Added `Flush()` method for explicit persistence
  - Added `Close()` method to stop goroutine and flush pending changes

- `pkg/addressbook/book_test.go`:
  - Updated `TestPersistence` to call `Close()` before reopening
  - Added `TestBatchedPersistence` to verify batched persistence behavior
  - Added `TestClose` to verify Close flushes pending changes

**Test Coverage:**
- `TestBatchedPersistence` - Verifies UpdateLastSeen doesn't persist immediately, Flush() persists
- `TestClose` - Verifies Close() flushes pending changes before exiting

---

### Phase 2.4: Add Message Drop Metrics and Logging

**Status:** ✅ Completed
**Priority:** P2 (Observability)

**Issue:** Messages were being silently dropped when incoming channels were full, providing no visibility into potential issues. The existing `MessageDropped()` metric was defined but never called.

**Solution:** Added comprehensive message drop tracking:
1. **Added `MessageDroppedCallback`** - New callback type for tracking dropped messages
2. **Added `OnMessageDropped` to EncryptedStreamConfig** - Optional callback when messages are dropped
3. **Added `SetOnMessageDropped` to UnencryptedStream** - Setter for drop callback (preserves API compatibility)
4. **Added `SetMessageDroppedCallback` to stream Manager** - Wires callback to all streams
5. **Wired callbacks in Node** - Logs warnings and calls `metrics.MessageDropped()`
6. **Added logging at node-level drops** - Event and message drops now logged with context

**Drop Locations Covered:**
- `pkg/streams/encrypted.go` - Encrypted stream incoming channel full
- `pkg/streams/unencrypted.go` - Unencrypted stream incoming channel full
- `node.go` - External message channel full
- `node.go` - External event channel full

**Log Format:**
```
level=WARN msg="message dropped" peer_id=<id> stream=<name> reason=buffer_full
level=WARN msg="event dropped" peer_id=<id> state=<state> reason=buffer_full
```

**Files Modified:**
- `pkg/streams/encrypted.go`:
  - Added `MessageDroppedCallback` type
  - Added `onMessageDropped` field to `EncryptedStream`
  - Added `OnMessageDropped` field to `EncryptedStreamConfig`
  - Updated `readLoop()` to call callback on drop

- `pkg/streams/unencrypted.go`:
  - Added `onMessageDropped` field to `UnencryptedStream`
  - Added `SetOnMessageDropped()` method
  - Updated `readLoop()` to call callback on drop

- `pkg/streams/manager.go`:
  - Added `onMessageDropped` field
  - Added `SetMessageDroppedCallback()` method
  - Wired callback to all stream creations

- `node.go`:
  - Added message dropped callback wiring to stream manager
  - Added logging and metrics at external message channel drop
  - Added logging at external event channel drop

- `pkg/streams/encrypted_test.go`:
  - Added `TestEncryptedStream_MessageDroppedCallback`

**Test Coverage:**
- `TestEncryptedStream_MessageDroppedCallback` - Verifies callback is invoked when channel is full

---

### Phase 2.5: Fix Read Loop Context Handling

**Status:** ✅ Verified (No Change Needed)
**Priority:** P2

**Analysis:** The proposed issue was that context cancellation is only checked before blocking read, potentially causing delayed shutdown.

**Verification:** After analyzing the code, this is a non-issue:
1. When `Close()` is called on any stream, it both cancels the context AND closes the underlying stream
2. Closing the underlying stream causes `reader.Next()` to return immediately with an error
3. The read loop handles this error and exits promptly

The proposed fix (adding read deadlines) would add unnecessary complexity without solving a real problem, since stream closure already unblocks the reader.

---

### Phase 2.6: Add Event Unsubscription

**Status:** ✅ Completed
**Priority:** P2 (Resource Management)

**Issue:** Once you subscribe to filtered events, there was no clean way to unsubscribe. The subscription goroutine would run forever until the node stopped, potentially leaking resources.

**Solution:** Implemented proper subscription lifecycle management:
1. Created `EventSubscription` type that wraps a channel and cancel function
2. Changed `eventSubs` from slice to map for efficient removal
3. Added `Unsubscribe()` method to stop receiving events and clean up
4. Updated `SubscribeEvents()`, `FilteredEvents()`, `EventsForPeer()`, `EventsForStates()` to return `*EventSubscription`

**API Changes (Breaking):**
- `FilteredEvents()` now returns `*EventSubscription` instead of `<-chan ConnectionEvent`
- `EventsForPeer()` now returns `*EventSubscription` instead of `<-chan ConnectionEvent`
- `EventsForStates()` now returns `*EventSubscription` instead of `<-chan ConnectionEvent`
- Added new `SubscribeEvents()` method that returns `*EventSubscription`

**Usage Example:**
```go
// Old API (no longer works):
// ch := node.EventsForPeer(peerID)
// for evt := range ch { ... }  // Runs forever

// New API:
sub := node.EventsForPeer(peerID)
defer sub.Unsubscribe()  // Clean up when done
for evt := range sub.Events() {
    // Process events
    if done {
        break
    }
}
```

**Files Modified:**
- `node.go`:
  - Added `EventSubscription` type with `Events()` and `Unsubscribe()` methods
  - Changed `eventSubs` from `[]chan ConnectionEvent` to `map[*EventSubscription]struct{}`
  - Added `SubscribeEvents()` public method
  - Updated `FilteredEvents()` to return `*EventSubscription`
  - Updated `EventsForPeer()` to return `*EventSubscription`
  - Updated `EventsForStates()` to return `*EventSubscription`
  - Updated `forwardEvents()` to iterate over map

- `events_filter_test.go`:
  - Added `TestEventSubscription_Unsubscribe`
  - Added `TestEventSubscription_Events`

**Test Coverage:**
- `TestEventSubscription_Unsubscribe` - Verifies unsubscribe removes subscription and calls cancel
- `TestEventSubscription_Events` - Verifies Events() returns the subscription channel

---

### Phase 3.1: Fix EstablishEncryptedStreams Two-Phase Behavior

**Status:** ✅ Completed
**Priority:** P1 (Bug Fix)

**Issue:** `EstablishEncryptedStreams` was incorrectly calling `MarkEstablished()` at the end, which was leftover from an earlier one-phase iteration. This violated the intended two-phase handshake design where:

1. `EstablishEncryptedStreams` / `PrepareStreams` - sets up encryption when receiving peer's pubkey
2. `CompleteHandshake` / `FinalizeHandshake` - finalizes when receiving peer's "complete" message

With `MarkEstablished` in `EstablishEncryptedStreams`, the connection would transition to `StateEstablished` prematurely before the peer confirmed readiness.

**Solution:**
1. Removed the redundant `MarkEstablished()` call from `EstablishEncryptedStreams`
2. Updated docstrings to clarify that `EstablishEncryptedStreams` prepares streams but does NOT finalize the handshake
3. Removed misleading "race condition" comment from `PrepareStreams` docstring

**Files Modified:**
- `node.go`:
  - Removed `_ = n.connections.MarkEstablished(peerID)` from `EstablishEncryptedStreams`
  - Updated `EstablishEncryptedStreams` docstring to clarify two-phase usage
  - Cleaned up `PrepareStreams` docstring

**API Clarification:**
- `EstablishEncryptedStreams` ≈ `PrepareStreams` (both set up encryption, neither mark established)
- `CompleteHandshake` = `PrepareStreams` + `FinalizeHandshake` (convenience wrapper)
- Call `EstablishEncryptedStreams` when receiving pubkey, then `CompleteHandshake` when receiving confirmation

---

### Phase 3.2: Add Windows Compatibility for File Locking

**Status:** ✅ Completed
**Priority:** P2 (Cross-Platform Support)

**Issue:** The address book file locking used `syscall.Flock()` which is not available on Windows. This prevented the library from compiling on Windows.

**Solution:** Split the file locking implementation into platform-specific files:

1. **storage.go** - Platform-independent code (storage struct, load, save)
2. **storage_unix.go** - Unix implementation using `syscall.Flock()`
3. **storage_windows.go** - Windows implementation using `windows.LockFileEx()` / `windows.UnlockFileEx()`

**Files Created:**
- `pkg/addressbook/storage_unix.go`:
  - Build tag: `//go:build !windows`
  - Uses `syscall.Flock()` with `LOCK_EX` for exclusive locking
  - Uses `syscall.Flock()` with `LOCK_UN` to release

- `pkg/addressbook/storage_windows.go`:
  - Build tag: `//go:build windows`
  - Uses `windows.LockFileEx()` with `LOCKFILE_EXCLUSIVE_LOCK`
  - Uses `windows.UnlockFileEx()` to release

**Files Modified:**
- `pkg/addressbook/storage.go`:
  - Removed `syscall` import
  - Removed `acquireFileLock()` and `releaseFileLock()` methods (moved to platform files)

- `go.mod`:
  - `golang.org/x/sys` promoted from indirect to direct dependency

**Notes:**
- Windows implementation uses overlapped I/O structures as required by the Windows API
- Both implementations block until lock is acquired (no timeout)
- Lock scope is 1 byte which is sufficient for advisory locking

---

### Phase 3.3: Improve Error Messages

**Status:** ✅ Completed
**Priority:** P3 (Developer Experience)

**Issue:** While the error system had error codes for programmatic handling, errors lacked troubleshooting guidance to help developers resolve issues quickly.

**Solution:** Added a `Hint` field to the `Error` struct with default troubleshooting hints for all known error codes.

**Changes:**

1. **Added `Hint` field to Error struct** - Provides context-specific troubleshooting guidance

2. **Added `WithHint()` method** - Allows overriding the default hint with a custom one

3. **Added `defaultHint()` function** - Returns appropriate troubleshooting hints for each error code:
   - `ErrCodeConnectionFailed`: Network connectivity guidance
   - `ErrCodeHandshakeFailed`: Protocol compatibility advice
   - `ErrCodeHandshakeTimeout`: Timeout tuning suggestions
   - `ErrCodeDecryptionFailed`: Key mismatch troubleshooting
   - `ErrCodePeerNotFound`: Address book usage reminder
   - `ErrCodePeerBlacklisted`: Blacklist management guidance
   - `ErrCodeBufferFull`: Buffer sizing recommendations
   - `ErrCodeNodeNotStarted`: Lifecycle reminder
   - And more...

4. **Updated all error constructors** - `NewError`, `NewErrorWithCause`, `NewPeerError`, `NewStreamError` now automatically include default hints

**Files Modified:**
- `errors.go`:
  - Added `Hint` field to `Error` struct
  - Added `WithHint()` method
  - Added `defaultHint()` function with hints for all error codes
  - Updated all constructors to set default hints

- `errors_rich_test.go`:
  - Added `TestError_Hint` with comprehensive coverage
  - Updated `TestError_Fields` to include Hint

**Example Usage:**
```go
err := NewError(ErrCodeConnectionFailed, "connection refused")
fmt.Println(err.Hint)
// Output: "Check that the peer is online and reachable. Verify the multiaddress is correct..."

// Custom hint
err = err.WithHint("Try port 9001 instead")
```

---

### Phase 4.1.1: Custom Decryption Error Callback

**Status:** ✅ Completed
**Priority:** P2 (Security Observability)

**Issue:** While decryption errors were logged and counted via metrics internally, applications had no way to implement custom responses like banning peers with repeated failures.

**Solution:** Exposed the decryption error callback via configuration:

1. **Added `DecryptionErrorCallback` type** at package level with documentation
2. **Added `OnDecryptionError` field** to Config struct
3. **Added `WithDecryptionErrorCallback()` config option** with usage examples
4. **Updated node initialization** to call user callback in addition to logging/metrics

**Files Modified:**
- `config.go`:
  - Added `DecryptionErrorCallback` type definition
  - Added `OnDecryptionError DecryptionErrorCallback` field to Config
  - Added `WithDecryptionErrorCallback(cb DecryptionErrorCallback) ConfigOption`

- `node.go`:
  - Updated decryption error callback to invoke user's custom callback

- `config_test.go`:
  - Added test case for `WithDecryptionErrorCallback`

**Example Usage:**
```go
failureCounts := make(map[peer.ID]int)

cfg := glueberry.NewConfig(key, path, addrs,
    glueberry.WithDecryptionErrorCallback(func(peerID peer.ID, err error) {
        failureCounts[peerID]++
        if failureCounts[peerID] > 10 {
            log.Printf("Banning peer %s after repeated decryption failures", peerID)
            node.Blacklist(peerID)
        }
    }),
)
```

---

### Phase 4.1.2: Nonce Exhaustion Warning

**Status:** ✅ Completed
**Priority:** P2 (Security Observability)

**Issue:** ChaCha20-Poly1305 uses 96-bit random nonces. Due to the birthday paradox, after ~2^48 messages there's a 50% chance of nonce collision. Applications had no visibility into message counts to detect when key rotation might be advisable.

**Solution:** Implemented a warning system that fires once per peer when message count exceeds a conservative threshold:

1. **Added `NonceExhaustionWarningThreshold` constant** (2^40 = ~1 trillion messages) - provides safety margin well before collision probability becomes significant

2. **Added `NonceExhaustionCallback` type** for user-defined warning handlers

3. **Updated `PeerStatsTracker`**:
   - Added `peerID` field to track which peer for callback invocation
   - Added `nonceWarningEmitted` flag to ensure warning fires only once
   - Added `onNonceExhaustion` callback field
   - Updated `RecordMessageSent()` to check threshold and invoke callback outside lock

4. **Updated `NewPeerStatsTracker()` signature** to accept peer ID and callback

5. **Wired callback in node.go** - logs warning when threshold exceeded

**Files Modified:**
- `stats.go`:
  - Added `NonceExhaustionWarningThreshold` constant with documentation
  - Added `NonceExhaustionCallback` type
  - Updated `PeerStatsTracker` struct with warning tracking fields
  - Updated `NewPeerStatsTracker(peerID peer.ID, callback NonceExhaustionCallback)`
  - Updated `RecordMessageSent()` to check threshold and emit warning

- `stats_test.go`:
  - Updated all `NewPeerStatsTracker()` calls to use new signature
  - Added `TestPeerStatsTracker_NonceExhaustionWarning`
  - Added `TestPeerStatsTracker_NonceExhaustionWarning_NilCallback`

- `node.go`:
  - Updated `getOrCreatePeerStats()` to pass nonce exhaustion callback

**Example Usage:**
```go
// The warning is logged automatically by the node when threshold is exceeded.
// Applications can also monitor message counts directly:
messageCount := node.MessagesSent(peerID)

// The callback receives the peer ID and current count when threshold crossed:
// "nonce exhaustion warning: peer QmXyz has sent 1099511627776 messages, consider key rotation"
```

**Design Notes:**
- Callback invoked outside mutex to prevent deadlocks
- Warning fires exactly once per peer (flag prevents repeated warnings)
- Threshold is conservative (2^40) - actual collision risk starts around 2^48
- Nil callback is safe - no panic, just skips notification

---

### Phase 4.1.3: Enhanced Input Validation

**Status:** ✅ Completed
**Priority:** P2 (Security)

**Issue:** The library accepted arbitrary stream names and metadata without validation, potentially allowing injection attacks, memory exhaustion from oversized metadata, or confusing error messages from invalid characters.

**Solution:** Implemented comprehensive input validation:

1. **Stream Name Validation**:
   - Must be non-empty
   - Only allows alphanumeric characters, hyphens, and underscores
   - Enforces maximum length (default 64 characters)
   - Validation in `PrepareStreams()`, `EstablishEncryptedStreams()`, and `SendCtx()`

2. **Metadata Size Validation**:
   - Limits total metadata size (sum of key+value lengths)
   - Default maximum of 4KB
   - Validation in `AddPeer()`

3. **Configuration Options**:
   - Added `MaxStreamNameLength` config (default 64)
   - Added `MaxMetadataSize` config (default 4096 bytes)
   - Added `WithMaxStreamNameLength()` and `WithMaxMetadataSize()` options

4. **New Error Types**:
   - `ErrInvalidStreamName` - invalid characters in stream name
   - `ErrStreamNameTooLong` - exceeds max length
   - `ErrMetadataTooLarge` - metadata exceeds size limit

**Note:** Public key validation was already implemented in `pkg/crypto/keys.go` via `ValidateEd25519PublicKey()` which validates both size and curve point validity.

**Files Created:**
- `validation.go`: `ValidateStreamName()`, `ValidateStreamNames()`, `ValidateMetadataSize()`, `isValidStreamNameChar()`
- `validation_test.go`: Comprehensive tests for all validation functions

**Files Modified:**
- `errors.go`:
  - Added `ErrInvalidStreamName`, `ErrStreamNameTooLong`, `ErrMetadataTooLarge`

- `config.go`:
  - Added `DefaultMaxStreamNameLength` (64) and `DefaultMaxMetadataSize` (4096)
  - Added `MaxStreamNameLength` and `MaxMetadataSize` fields to Config
  - Added validation and default application for new fields
  - Added `WithMaxStreamNameLength()` and `WithMaxMetadataSize()` options

- `node.go`:
  - Added stream name validation in `PrepareStreams()`
  - Added stream name validation in `EstablishEncryptedStreams()`
  - Added stream name validation in `SendCtx()`
  - Added metadata size validation in `AddPeer()`

- `config_test.go`:
  - Added tests for new validation options
  - Added tests for defaults

**Example Usage:**
```go
// Invalid stream name rejected
err := node.Send(peerID, "my stream", data) // Error: invalid character ' '

// Stream name too long rejected
err := node.Send(peerID, "very-long-name-exceeding-max", data) // Error: exceeds maximum

// Metadata too large rejected
largeMetadata := map[string]string{"key": strings.Repeat("x", 5000)}
err := node.AddPeer(peerID, addrs, largeMetadata) // Error: metadata too large

// Custom limits via config
cfg := glueberry.NewConfig(key, path, addrs,
    glueberry.WithMaxStreamNameLength(128),
    glueberry.WithMaxMetadataSize(8192),
)
```

---

### Phase 4.2.1: Prometheus Metrics Adapter

**Status:** ✅ Completed
**Priority:** P2 (Observability)

**Issue:** While Glueberry defined a Metrics interface with a NopMetrics implementation, there was no ready-to-use Prometheus implementation. Users had to implement the entire interface themselves.

**Solution:** Created a complete Prometheus metrics adapter in a new `prometheus/` subpackage:

1. **Created `prometheus/` subpackage** with full Prometheus implementation
2. **Implemented all 17 metrics methods** from the glueberry.Metrics interface
3. **Standard Prometheus naming conventions**:
   - Counters: `*_total` suffix
   - Histograms: `*_seconds` or `*_bytes` suffix
   - Gauges: descriptive names
4. **Configurable namespace** (default: "glueberry")
5. **Custom registerer support** for testing and avoiding conflicts
6. **Grafana dashboard template** with pre-built panels

**Metrics Exposed:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `connections_opened_total` | Counter | direction | Connections opened |
| `connections_closed_total` | Counter | direction | Connections closed |
| `connection_attempts_total` | Counter | result | Connection attempts |
| `handshake_duration_seconds` | Histogram | - | Handshake duration |
| `handshake_results_total` | Counter | result | Handshake outcomes |
| `messages_sent_total` | Counter | stream | Messages sent |
| `messages_received_total` | Counter | stream | Messages received |
| `bytes_sent_total` | Counter | stream | Bytes sent |
| `bytes_received_total` | Counter | stream | Bytes received |
| `streams_opened_total` | Counter | stream | Streams opened |
| `streams_closed_total` | Counter | stream | Streams closed |
| `encryption_errors_total` | Counter | - | Encryption failures |
| `decryption_errors_total` | Counter | - | Decryption failures |
| `key_derivations_total` | Counter | cached | Key derivations |
| `events_emitted_total` | Counter | state | Events emitted |
| `events_dropped_total` | Counter | - | Events dropped |
| `messages_dropped_total` | Counter | - | Messages dropped |
| `backpressure_engaged_total` | Counter | stream | Backpressure events |
| `backpressure_wait_seconds` | Histogram | stream | Backpressure wait time |
| `pending_messages` | Gauge | stream | Current pending msgs |

**Files Created:**
- `prometheus/metrics.go`: Complete Prometheus implementation with full documentation
- `prometheus/metrics_test.go`: Comprehensive tests for all metrics
- `prometheus/grafana-dashboard.json`: Ready-to-import Grafana dashboard

**Example Usage:**
```go
import (
    "github.com/blockberries/glueberry"
    prommetrics "github.com/blockberries/glueberry/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    metrics := prommetrics.NewMetrics("myapp")

    cfg := glueberry.NewConfig(key, path, addrs,
        glueberry.WithMetrics(metrics),
    )

    node, err := glueberry.New(cfg)
    // ...

    // Expose metrics endpoint
    http.Handle("/metrics", promhttp.Handler())
    http.ListenAndServe(":9090", nil)
}
```

**Grafana Dashboard Features:**
- Connection overview (rate, failure rate, handshake p95)
- Message throughput by stream
- Bytes sent/received over time
- Security metrics (encryption/decryption errors)
- Flow control (pending messages, backpressure)
- Configurable namespace variable

---
