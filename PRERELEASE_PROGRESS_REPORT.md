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
