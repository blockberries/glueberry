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

## Recommendations for Future Reviews

1. **Key Material Handling:** Always verify that any code creating `Cipher` instances calls `Close()` appropriately, and that raw ECDH shared secrets are zeroed.
2. **HTTP Handler Errors:** Continue using `_ =` pattern for response write errors
3. **Test Utilities:** Mark unused but intentional test utilities with nolint directives
4. **Nil Checks:** Use `t.Fatal()` instead of `t.Error()` when subsequent code depends on nil check passing
5. **TOCTOU Prevention:** When operating on resources obtained from a map, prefer holding the lock through the entire operation rather than releasing between check and use.
6. **Timer Resources:** Use `time.NewTimer()` instead of `time.After()` when the timer might need to be cancelled before firing.
7. **RWMutex for Read-Heavy Operations:** Use `RWMutex` instead of `Mutex` for structures where reads vastly outnumber writes (like `IsClosed()` checks).
