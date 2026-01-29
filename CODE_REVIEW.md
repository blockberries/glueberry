# Code Review Report - Glueberry

## Review Date: January 2026

## Summary

This document tracks issues found during code review and their resolutions.

---

## Bug Fixes Applied

### 1. Cipher Convenience Functions Missing Key Zeroing (SECURITY)

**File:** `/pkg/crypto/cipher.go`

**Issue:** The convenience functions `Encrypt()` and `Decrypt()` created temporary `Cipher` instances but did not call `Close()` on them, leaving key material in memory. This violated the security requirement that all cryptographic key material should be zeroed after use.

**Fix:** Added `defer c.Close()` calls to both functions to ensure key material is properly zeroed.

**Impact:** Medium - Security best practice violation. Key material could persist in memory longer than necessary.

---

## Linting Fixes Applied

### 2. gofmt Formatting Issues

**Files:**
- `otel/tracing.go`
- `otel/tracing_test.go`
- `chaos_test.go`
- `health_test.go`
- `prometheus/metrics.go`

**Issue:** Files were not properly formatted according to gofmt standards.

**Fix:** Ran `gofmt -w` on affected files.

### 3. Ineffectual Assignments in Test Code

**File:** `otel/tracing_test.go`

**Issue:** Multiple `ctx, span = ...` assignments where `ctx` was never used, causing ineffassign warnings.

**Fix:** Changed to `_, span = ...` for subsequent assignments where context is not needed.

### 4. Unchecked Error Returns in HTTP Handlers

**File:** `health.go`

**Issue:** `json.NewEncoder(w).Encode()` and `w.Write()` return values were not checked, causing errcheck warnings.

**Fix:** Added explicit `_ =` assignments to acknowledge intentionally ignoring errors (standard practice for HTTP response writes where client disconnection errors are unrecoverable).

### 5. Integer Overflow Warning

**File:** `node.go`

**Issue:** `int64` to `uint64` conversion without bounds checking caused gosec G115 warning.

**Fix:** Added defensive check to ensure value is non-negative before conversion, with nolint directive.

### 6. Nil Pointer Check Without Return

**File:** `otel/tracing_test.go`

**Issue:** Nil check on line 18 with `t.Error()` didn't return, allowing potential nil dereference on line 21.

**Fix:** Changed `t.Error()` to `t.Fatal()` to ensure test exits early on nil.

### 7. Unused Test Utilities

**Files:**
- `chaos_test.go` - `chaosStream` type and methods, `flakyReader` type and method
- `benchmark/load_test.go` - `loadToCryptoKey`, `generateLoadPeerID`

**Issue:** Test utilities were defined but not currently used.

**Fix:** Added `//nolint:unused` directives as these are intentional test utilities for future chaos/load tests.

---

## Architecture Compliance

The codebase follows the architecture defined in `ARCHITECTURE.md`:
- Proper separation between libp2p transport and application-level handshaking
- App-controlled handshake design with unencrypted handshake stream
- Post-handshake encryption with ChaCha20-Poly1305
- Thread-safe APIs with proper mutex usage
- Secure key material handling with zeroing

---

## Test Results

All tests pass with race detection enabled:
- `go test -race ./...` - All packages pass
- `go build ./...` - Clean build
- `golangci-lint run` - No issues

## Dependency Status

- `github.com/blockberries/cramberry v1.5.3` - Latest version (verified January 2026)

## Verification Pass (Second Iteration)

A second review iteration was performed to verify:
- All previously identified bugs were fixed
- No new issues were introduced
- All tests continue to pass
- All linting passes

**Result:** Codebase was marked production-ready.

---

## Third Iteration - January 2026

A comprehensive deep-dive review was performed using parallel specialized agents focusing on different components (crypto, connection, streams, node, observability, flow control, address book).

### Bug Fixes Applied

#### 8. Missing Key Material Zeroing in ECDH (SECURITY - CRITICAL)

**File:** `/pkg/crypto/ecdh.go`

**Issue:** The `DeriveSharedKey` function computed a raw X25519 shared secret but did not zero it after deriving the final key via HKDF. The raw shared secret is highly sensitive cryptographic material that could persist in memory, potentially exposing it through memory dumps or garbage collection.

**Fix:** Added `defer SecureZero(sharedSecret)` immediately after computing the shared secret to ensure it's always zeroed regardless of whether `deriveKeyFromSecret` succeeds or fails.

**Impact:** Critical - Violation of ARCHITECTURE.md security requirement that all key material must be zeroed after use.

---

#### 9. Race Condition in UnencryptedStream.Close() (CONCURRENCY - HIGH)

**File:** `/pkg/streams/unencrypted.go`

**Issue:** The `Close()` method had a TOCTOU race condition where it checked `us.closed`, released the lock, then performed close operations. Multiple concurrent goroutines could pass the closed check and all attempt to close the stream.

**Fix:** Modified `Close()` to set `us.closed = true` atomically with the check before releasing the lock. Also changed `closeMu` from `sync.Mutex` to `sync.RWMutex` for better concurrency on `IsClosed()` reads.

**Impact:** High - Could cause double-close issues and undefined behavior on concurrent close attempts.

---

#### 10. TOCTOU Race in MarkEstablished (CONCURRENCY - HIGH)

**File:** `/pkg/connection/manager.go`

**Issue:** The `MarkEstablished()` function acquired RLock, got the connection, released RLock, then operated on the connection (CancelHandshakeTimeout) without holding any lock. Between releasing RLock and acquiring Lock for state transition, the connection could be cleaned up by another goroutine.

**Fix:** Changed to hold Lock through the entire operation including `CancelHandshakeTimeout()` to make the operation atomic.

**Impact:** High - Could cause operations on stale/cleaned-up connections during shutdown races.

---

#### 11. Goroutine Leak in ConnectCtx (RESOURCE LEAK - MEDIUM)

**File:** `/pkg/connection/manager.go`

**Issue:** The goroutine spawned to respect manager context only called `cancel()` when `m.ctx.Done()` fired, not when `connectCtx.Done()` fired. This left the context's cancel func uncalled on the normal success path.

**Fix:** Modified the goroutine to call `cancel()` in all exit paths, regardless of which context fires first.

**Impact:** Medium - Unnecessary goroutine accumulation until manager shutdown.

---

#### 12. Timer Leak in scheduleReconnectAfterCooldown (RESOURCE LEAK - MEDIUM)

**File:** `/pkg/connection/manager.go`

**Issue:** Used `time.After()` which creates a timer that cannot be stopped. If the manager context is cancelled during cooldown, the timer resources aren't released until the timer fires.

**Fix:** Changed to `time.NewTimer()` with `defer timer.Stop()` to ensure timer resources are properly released when context is cancelled.

**Impact:** Medium - Timer resource leaks during shutdown with long cooldown periods.

---

#### 13. TOCTOU Race in CancelHandshakeTimeout (CONCURRENCY - MEDIUM)

**File:** `/pkg/connection/manager.go`

**Issue:** Same pattern as MarkEstablished - released RLock before operating on the connection.

**Fix:** Changed to hold Lock through the entire operation.

**Impact:** Medium - Consistency fix matching MarkEstablished pattern.

---

#### 14. Config Validation Gap with Watermarks (CONFIGURATION - LOW)

**File:** `/config.go`

**Issue:** Validation only checked watermark relationships when BOTH values were > 0. If a user set only one watermark (e.g., `HighWatermark=50`) and left the other at 0, `applyDefaults()` would set `LowWatermark=100`, creating an invalid relationship (`LowWatermark >= HighWatermark`).

**Fix:** Added post-defaults validation in `applyDefaults()` to ensure watermark relationships are always valid, automatically adjusting low watermark if needed.

**Impact:** Low - Could cause confusing behavior when partially configuring watermarks.

---

### Additional Issues Identified (Not Fixed - Lower Priority)

The following issues were identified but not fixed in this iteration due to lower severity:

1. **Flow Controller Cleanup**: The `flowControllers` map in node.go grows unbounded as different stream names are used. Consider adding periodic cleanup.

2. **Prometheus Label Cardinality**: Stream names as metric labels could cause cardinality explosion with many unique stream names. Consider documenting this risk or adding limits.

3. **Address Book flushLoop/Close() Race**: Minor race between background flush and Close() - could miss final dirty writes in edge cases.

---

## Test Results

All tests pass with race detection enabled:
- `go test -race ./...` - All packages pass
- `go build ./...` - Clean build
- `golangci-lint run` - No issues

## Dependency Status

- `github.com/blockberries/cramberry v1.5.3` - Latest version (verified January 2026)

---

## Fourth Iteration - January 2026

A follow-up deep-dive review was performed focusing on edge cases and security patterns.

### Bug Fixes Applied

#### 15. Race Condition in RegisterIncomingConnection (CONCURRENCY - HIGH)

**File:** `/pkg/connection/manager.go`

**Issue:** The `RegisterIncomingConnection()` function called `GetOrCreateConnection()` which released the lock, then performed multiple operations (SetIsOutbound, GetState, TransitionTo, setupHandshakeTimeout) without holding the manager lock. This created a race condition where concurrent operations (like Disconnect) could interfere.

**Fix:** Refactored to hold the manager lock through all operations, only releasing before emitting the event to prevent deadlocks.

**Impact:** High - Could cause race conditions with concurrent connection management.

---

#### 16. Missing Handshake Timeout on Reconnection (FUNCTIONALITY - MEDIUM)

**File:** `/pkg/connection/manager.go`

**Issue:** When `attemptReconnect()` successfully reconnected a peer, it transitioned to `StateConnected` but did NOT set up a handshake timeout. This meant reconnected peers could stay in StateConnected indefinitely without completing the handshake, unlike initial and incoming connections which have timeouts.

**Fix:** Added `m.setupHandshakeTimeout(conn)` call after transitioning reconnected peers to StateConnected.

**Impact:** Medium - Reconnected peers could hang indefinitely without completing handshake.

---

#### 17. Decrypted Data Not Zeroed on Drop/Cancellation (SECURITY - HIGH)

**File:** `/pkg/streams/encrypted.go`

**Issue:** In the `readLoop()` method, when a decrypted message was dropped (channel full) or the context was cancelled during send, the plaintext `decrypted` buffer was not zeroed. Sensitive decrypted message contents could persist in memory.

**Fix:** Added `crypto.SecureZero(decrypted)` calls when messages are dropped and when returning due to context cancellation.

**Impact:** High - Plaintext message data could leak in memory dumps.

---

#### 18. Encrypted Buffer Not Zeroed After Decryption (SECURITY - MEDIUM)

**File:** `/pkg/streams/encrypted.go`

**Issue:** After successful or failed decryption, the `encrypted` ciphertext buffer was not zeroed (except for oversized messages). While ciphertext is less sensitive than plaintext, defense-in-depth principles dictate minimizing data exposure.

**Fix:** Added `crypto.SecureZero(encrypted)` immediately after decryption attempt, regardless of success or failure.

**Impact:** Medium - Defense in depth improvement for security.

---

### Issues Reviewed and Deemed Not Bugs

1. **FilteredEvents subscription tracking:** Initially flagged as potential leak, but verified that cleanup works correctly via the base subscription being tracked in eventSubs. When node shuts down, baseSub's channel is closed, which causes the filtering goroutine to exit and close the filtered channel.

### Additional Issues Identified (Not Fixed - Lower Priority)

The following issues remain from previous iterations:

1. **Flow Controller Cleanup**: The `flowControllers` map grows unbounded.
2. **Prometheus Label Cardinality**: Stream names as metric labels could cause cardinality explosion.
3. **Address Book flushLoop/Close() Race**: Minor race - goroutine not waited for.

---

## Test Results

All tests pass with race detection enabled:
- `go test -race ./...` - All packages pass
- `go build ./...` - Clean build
- `golangci-lint run` - No issues

## Dependency Status

- `github.com/blockberries/cramberry v1.5.3` - Latest version (verified January 2026)

---

## Fifth Iteration - January 2026

A comprehensive deep-dive review was performed using parallel specialized agents focusing on crypto, connection, streams, node/config, addressbook, protocol handlers, and internal utilities.

### Bug Fixes Applied

#### 19. Closure Variable Capture Bug in Stream Handler Registration (CRITICAL)

**File:** `/node.go`

**Issue:** The `registerIncomingStreamHandlers` function had a classic Go closure bug where the loop variable `streamName` was captured by reference in the handler closure. All handlers would use the value from the last iteration, causing all incoming streams to be handled with the wrong stream name.

**Fix:** Added explicit variable capture with `name := streamName` before creating the closure.

**Impact:** Critical - All incoming encrypted stream handlers would use the wrong stream name, breaking multi-stream communication.

---

#### 20. TOCTOU Race in Module.Encrypt/Decrypt (CRITICAL)

**File:** `/pkg/crypto/module.go`

**Issue:** The `Encrypt` and `Decrypt` methods retrieved the key under lock but used it after releasing the lock. If `RemovePeerKey` was called concurrently, it would zero the key via `SecureZero(key)` while the encryption/decryption was in progress, corrupting the operation.

**Fix:** Copy the key before releasing the lock, then zero the copy after use with `defer SecureZero(keyCopy)`.

**Impact:** Critical - Could cause encryption with zeroed key, completely compromising confidentiality.

---

#### 21. Data Race in Module.Close() (CRITICAL)

**File:** `/pkg/crypto/module.go`

**Issue:** The `Close()` method zeroed `x25519Private` without holding any lock, while `DeriveSharedKeyFromX25519` could be reading it concurrently. This violated the data race safety guarantees documented for the Module type.

**Fix:** Added `closed` flag to Module struct. `Close()` now acquires the lock before setting `closed = true` and zeroing private keys. All methods that access private keys check the `closed` flag and copy private key data while holding the lock.

**Impact:** Critical - Data race on private key material, undefined behavior.

---

#### 22. Nil Map Panic in SubscribeEvents After Stop (CRITICAL)

**File:** `/node.go`

**Issue:** When the node stopped, `forwardEvents()` set `n.eventSubs = nil`. If `SubscribeEvents()` was called after the node stopped, it would attempt to write to a nil map, causing a panic.

**Fix:** Added nil check for `n.eventSubs` in `SubscribeEvents()`. Returns nil if the node has stopped. Updated `FilteredEvents()` to handle nil return.

**Impact:** Critical - Application crash if subscriptions created after node stops.

---

#### 23. SHA-512 Hash Not Zeroed After Key Derivation (HIGH)

**File:** `/pkg/crypto/keys.go`

**Issue:** In `Ed25519PrivateToX25519`, the SHA-512 hash containing sensitive key material was not zeroed after use. The first 32 bytes are the X25519 private key.

**Fix:** Added `defer SecureZero(h[:])` to zero the hash after extracting key material.

**Impact:** High - Key material persisted in memory longer than necessary.

---

#### 24. X25519 Private Key Not Zeroed on Error (HIGH)

**File:** `/pkg/crypto/module.go`

**Issue:** In `NewModule`, if `Ed25519PublicToX25519` failed after successfully deriving the X25519 private key, the private key was not zeroed before returning the error.

**Fix:** Added `SecureZero(x25519Priv)` before returning the error.

**Impact:** High - Key material leaked on error path.

---

#### 25. Data Race in ConnectionGater.SetStateChecker (HIGH)

**File:** `/pkg/protocol/gater.go`

**Issue:** `SetStateChecker` wrote to `g.stateChecker` without synchronization, while `InterceptSecured` read it concurrently. This is a data race.

**Fix:** Added `sync.RWMutex` to ConnectionGater. `SetStateChecker` acquires write lock, `InterceptSecured` acquires read lock when accessing `stateChecker`.

**Impact:** High - Data race, potential crashes or undefined behavior.

---

#### 26. AddPeer Storing External References Without Copying (HIGH)

**File:** `/pkg/addressbook/book.go`

**Issue:** `AddPeer` stored the provided `addrs` slice and `metadata` map directly without making defensive copies. External callers could modify these after the call, corrupting the internal state of the address book.

**Fix:** Added defensive copies of both the `addrs` slice and `metadata` map before storing.

**Impact:** High - Internal state corruption possible via external modification.

---

#### 27. Flow Controller Callback Invoked While Holding Lock (HIGH)

**File:** `/internal/flow/controller.go`

**Issue:** The `onBlocked` callback was invoked while holding `fc.mu`. If the callback attempted to access any Controller method, it would deadlock.

**Fix:** Capture callback and shouldCallback flag while holding lock, then invoke callback after releasing lock.

**Impact:** High - Potential deadlock if callback accesses controller.

---

#### 28. Timer Leak in reconnectionLoop (MEDIUM)

**File:** `/pkg/connection/manager.go`

**Issue:** Used `time.After()` in a select statement with cancellation. When context was cancelled, the timer resources weren't released until the timer fired.

**Fix:** Changed to `time.NewTimer()` with explicit `timer.Stop()` when context is cancelled.

**Impact:** Medium - Timer resource leaks during shutdown.

---

### Issues Reviewed and Deemed Not Fixed (Lower Priority)

The following issues remain from previous iterations:

1. **Flow Controller Cleanup**: The `flowControllers` map grows unbounded.
2. **Prometheus Label Cardinality**: Stream names as metric labels could cause cardinality explosion.
3. **Address Book flushLoop/Close() Race**: Minor race - goroutine not waited for.
4. **Unbounded m.connections map**: Connection entries never removed except on shutdown.
5. **PeerStatsTracker StreamStats map unbounded**: Similar to flowControllers.

---

## Test Results

All tests pass with race detection enabled:
- `go test -race ./...` - All packages pass
- `go build ./...` - Clean build
- `golangci-lint run` - No issues

## Dependency Status

- `github.com/blockberries/cramberry v1.5.3` - Latest version (verified January 2026)

---

## Recommendations for Future Reviews

1. **Key Material Handling:** Always verify that any code creating `Cipher` instances calls `Close()` appropriately, and that raw ECDH shared secrets are zeroed.
2. **HTTP Handler Errors:** Continue using `_ =` pattern for response write errors
3. **Test Utilities:** Mark unused but intentional test utilities with nolint directives
4. **Nil Checks:** Use `t.Fatal()` instead of `t.Error()` when subsequent code depends on nil check passing
5. **TOCTOU Prevention:** When operating on resources obtained from a map, prefer holding the lock through the entire operation rather than releasing between check and use.
6. **Timer Resources:** Use `time.NewTimer()` instead of `time.After()` when the timer might need to be cancelled before firing.
7. **RWMutex for Read-Heavy Operations:** Use `RWMutex` instead of `Mutex` for structures where reads vastly outnumber writes (like `IsClosed()` checks).
8. **Plaintext Zeroing:** Always zero decrypted/plaintext data when it's no longer needed, especially on error paths and when messages are dropped.
9. **Lock Scope in Registration:** When registering connections, hold the lock through state transitions to prevent race conditions with concurrent operations.
10. **Closure Variable Capture:** Always explicitly capture loop variables in closures with a local variable assignment.
11. **Key Copy Before Use:** When using cached key material, copy it before releasing the lock and zero the copy after use.
12. **Defensive Copies:** Make defensive copies of slices and maps passed to public APIs to prevent external mutation.
13. **Callback Invocation:** Invoke callbacks outside of locks to prevent deadlocks.
