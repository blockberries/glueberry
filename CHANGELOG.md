# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.2.1] - 2026-01-27

### Changed
- Updated Cramberry dependency to v1.4.1

## [1.2.0] - 2026-01-27

### Added

#### Observability
- **Prometheus Metrics Adapter** (`prometheus/` subpackage): Complete implementation of the Metrics interface with standard Prometheus naming conventions, histograms, counters, and gauges
- **Grafana Dashboard Template** (`prometheus/grafana-dashboard.json`): Ready-to-import dashboard with connection, message, and flow control panels
- **OpenTelemetry Tracing** (`otel/` subpackage): Distributed tracing support with span hierarchy for all major operations (connect, handshake, send, receive, encrypt, decrypt)
- **Health Check API**: `IsHealthy()` for liveness probes, `ReadinessChecks()` for detailed health status, `HealthHandler()` and `LivenessHandler()` HTTP handlers for Kubernetes integration

#### Security
- **Decryption Error Callback**: `WithDecryptionErrorCallback()` config option for custom handling of decryption failures (e.g., peer banning)
- **Nonce Exhaustion Warning**: Automatic warning when message count approaches collision risk threshold (2^40 messages)
- **Enhanced Input Validation**: Stream name validation (alphanumeric, hyphens, underscores), metadata size limits, configurable via `WithMaxStreamNameLength()` and `WithMaxMetadataSize()`
- **Connection Rate Limiting**: Configurable connection manager watermarks via `WithConnMgrLowWatermark()` and `WithConnMgrHighWatermark()`

#### API Improvements
- **Event Unsubscription**: `EventSubscription` type with `Unsubscribe()` method for proper cleanup of event listeners
- **Debug Utilities**: `DumpState()`, `DumpStateJSON()`, `DumpStateString()` for node state inspection
- **Error Hints**: All errors now include troubleshooting hints via `Hint` field

#### Testing Infrastructure
- **Chaos Engineering Tests**: 11 chaos tests covering connection drops, rapid connect/disconnect, concurrent operations, event overflow, simultaneous shutdown, network partition simulation
- **Load Testing**: Benchmarks for node operations, peer management at scale (10-1000 peers), concurrent throughput measurement
- **Flow Controller Stress Tests**: 100+ goroutine concurrent access verification

#### Platform Support
- **Windows File Locking**: Address book now supports Windows via `LockFileEx`/`UnlockFileEx`

### Changed
- **Address Book Persistence**: Non-critical updates (LastSeen) are now batched and flushed periodically (5s) for improved performance
- **Stream Manager Locking**: Network I/O no longer blocks other stream operations (lock released during `NewStream()` calls)
- **Message Drop Visibility**: All message drops now tracked via metrics and optional logging

### Fixed
- **SecureZero Optimization**: Fixed potential compiler optimization of key zeroing using function variable indirection
- **Flow Controller Deadlock**: Fixed multi-waiter deadlock where only one goroutine would unblock (now uses broadcast pattern)
- **Receive-Path Size Validation**: Added `MaxMessageSize` enforcement on receive path to prevent OOM from malicious peers
- **Cipher Key Zeroing**: Added `Close()` method to Cipher for proper key material cleanup
- **EncryptWithNonce Exposure**: Made nonce-setting API internal-only to prevent accidental nonce reuse
- **Handshake Timeout Race**: Fixed race condition between timeout firing and handshake completion using atomic state transition
- **EstablishEncryptedStreams**: Removed premature `MarkEstablished()` call that violated two-phase handshake design
- **Peer Stats Memory Leak**: Added periodic cleanup of stale peer stats (24h inactivity threshold)

### Security
- All critical security issues from security audit addressed
- Receive-side message size validation prevents memory exhaustion attacks
- Key material properly zeroed in all code paths
- Nonce reuse prevention via internal-only API

## [1.0.1] - 2026-01-26

### Changed
- Updated Cramberry dependency to v1.2.0 for zero-copy safety improvements

### Added
- Comprehensive stress tests for concurrent connection handling
- Additional connection lifecycle tests

### Fixed
- Removed unreachable datarace in connection state machine (verified via stress testing)

## [1.0.0] - 2026-01-26

### Added
- Initial public release of Glueberry P2P library

#### Core Features
- Secure P2P communications over libp2p transport
- ChaCha20-Poly1305 AEAD encryption for all post-handshake traffic
- X25519 ECDH key exchange (derived from Ed25519 identity keys)
- Named stream multiplexing over single connections
- Two-phase handshake API (`PrepareStreams` + `FinalizeHandshake`) for race-free setup
- Automatic reconnection with exponential backoff

#### Connection Management
- 6-state connection state machine (Disconnected, Connecting, Connected, Preparing, Established, Failed)
- Connection event system with filtering support
- Peer blacklisting with optional expiry
- Per-peer address tracking with last-seen timestamps

#### Address Book
- Persistent address book with JSON storage
- Thread-safe peer management
- Automatic address updates on connection

#### Observability
- Pluggable Logger interface for structured logging
- Pluggable Metrics interface for custom metrics collection
- Flow controller with high/low watermarks
- Peer statistics tracking (messages sent/received, bytes, latency)
- Debug utilities for connection state inspection

#### Protocol
- Protocol version negotiation (v1.0.0)
- Graceful shutdown with connection draining
- Context-aware APIs for cancellation support

#### Configuration
- Functional options pattern for flexible configuration
- Sensible defaults with `DefaultConfig()`
- Configurable timeouts, intervals, and limits

#### Testing Infrastructure
- MockNode for unit testing applications
- Comprehensive fuzz tests for crypto, address book, and message parsing
- Integration tests for connection lifecycle
- Stress tests for concurrent operations
- Benchmark suite for performance testing

#### Examples
- simple-chat: Basic two-peer chat application

### Security
- AEAD encryption with random nonces
- Automatic rejection of tampered messages
- Secure key derivation from ECDH shared secret
- Input validation on all network data
- Decryption errors logged and counted (messages silently dropped)

### Performance Highlights
- Zero-allocation message framing with Cramberry
- Buffer pooling for reduced GC pressure
- Efficient stream multiplexing
- Flow control prevents memory exhaustion

---

## Version Compatibility

| Glueberry Version | Protocol Version | Cramberry Version | Go Version |
|-------------------|------------------|-------------------|------------|
| 1.2.1             | 1.2.1            | 1.4.1             | 1.21+      |
| 1.2.0             | 1.2.0            | 1.2.0             | 1.21+      |
| 1.0.1             | 1.0.0            | 1.2.0             | 1.21+      |
| 1.0.0             | 1.0.0            | 1.2.0             | 1.21+      |

## Upgrade Guide

### From Pre-release to 1.0.0

If upgrading from a pre-release version:

1. **API Changes**: The handshake API has been split into two phases:
   - Old: `EstablishEncryptedStreams(peerID, pubKey, streams)`
   - New: `PrepareStreams(peerID, streams)` then `FinalizeHandshake(peerID, pubKey)`

2. **Event Filtering**: Connection events now support filtering:
   ```go
   // Filter for specific peer and state
   filter := glueberry.EventFilter{
       PeerID: somePeerID,
       States: []glueberry.ConnectionState{glueberry.StateEstablished},
   }
   events := node.SubscribeFiltered(filter)
   ```

3. **Context Support**: Most APIs now accept context for cancellation:
   ```go
   err := node.ConnectWithContext(ctx, peerID)
   err := node.SendWithContext(ctx, peerID, stream, data)
   ```
