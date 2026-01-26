# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
