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

**Result:** Codebase is production-ready with no remaining issues.

---

## Recommendations for Future Reviews

1. **Key Material Handling:** Always verify that any code creating `Cipher` instances calls `Close()` appropriately
2. **HTTP Handler Errors:** Continue using `_ =` pattern for response write errors
3. **Test Utilities:** Mark unused but intentional test utilities with nolint directives
4. **Nil Checks:** Use `t.Fatal()` instead of `t.Error()` when subsequent code depends on nil check passing
