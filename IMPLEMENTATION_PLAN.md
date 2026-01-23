# Glueberry Implementation Plan

This document outlines the implementation tasks for Glueberry, organized by phase. Tasks are ordered by dependency - earlier tasks must be completed before later ones that depend on them.

## Phase 1: Project Setup & Foundation

### 1.1 Project Initialization
- [ ] Initialize Go module (`go mod init`)
- [ ] Set up directory structure:
  ```
  glueberry/
  ├── glueberry.go
  ├── config.go
  ├── errors.go
  ├── node.go
  ├── pkg/
  │   ├── addressbook/
  │   ├── connection/
  │   ├── crypto/
  │   ├── streams/
  │   └── protocol/
  └── internal/
  ```
- [ ] Add core dependencies to `go.mod`:
  - `github.com/libp2p/go-libp2p`
  - `golang.org/x/crypto`
  - Cramberry (local module reference `../cramberry`)
- [ ] Create `errors.go` with all error types defined in architecture

### 1.2 Configuration Types
- [ ] Create `config.go` with `Config` struct
- [ ] Implement config validation function
- [ ] Define sensible defaults for all optional fields
- [ ] Add `ConfigOption` functional options pattern for ergonomic configuration

## Phase 2: Crypto Module

### 2.1 Key Conversion
- [ ] Create `pkg/crypto/keys.go`
- [ ] Implement Ed25519 → X25519 private key conversion
  - SHA-512 hash of Ed25519 seed
  - Clamp first 32 bytes per X25519 spec
- [ ] Implement Ed25519 → X25519 public key conversion
  - Use `edwards25519` package for point conversion
- [ ] Add comprehensive tests for key conversion (test vectors)

### 2.2 ECDH Key Exchange
- [ ] Create `pkg/crypto/ecdh.go`
- [ ] Implement X25519 ECDH shared secret computation
- [ ] Implement HKDF-SHA256 key derivation from shared secret
  - Use context string: `"glueberry-v1-stream-key"`
  - Derive 256-bit symmetric key
- [ ] Add tests with known test vectors

### 2.3 Symmetric Encryption
- [ ] Create `pkg/crypto/cipher.go`
- [ ] Implement ChaCha20-Poly1305 encryption
  - Generate random 96-bit nonce
  - Return `nonce || ciphertext || tag`
- [ ] Implement ChaCha20-Poly1305 decryption
  - Parse nonce from message prefix
  - Verify authentication tag
  - Return plaintext or error
- [ ] Add tests including:
  - Round-trip encryption/decryption
  - Tamper detection (modified ciphertext)
  - Invalid nonce handling

### 2.4 Crypto Module Interface
- [ ] Create `pkg/crypto/module.go`
- [ ] Implement `Module` struct with:
  - Constructor taking Ed25519 private key
  - `DeriveSharedKey(remotePubKey) ([]byte, error)`
  - `Encrypt(key, plaintext) ([]byte, error)`
  - `Decrypt(key, ciphertext) ([]byte, error)`
  - `PublicKey() ed25519.PublicKey`
  - `X25519PublicKey() []byte`
- [ ] Ensure private key material is not exposed
- [ ] Add integration tests for full key exchange flow

## Phase 3: Address Book

### 3.1 Data Structures
- [ ] Create `pkg/addressbook/types.go`
- [ ] Define `PeerEntry` struct with all fields:
  - PeerID, Multiaddrs, PublicKey, Metadata
  - LastSeen, Blacklisted, CreatedAt, UpdatedAt
- [ ] Implement JSON marshaling/unmarshaling
- [ ] Handle multiaddr JSON serialization (string format)

### 3.2 Persistence Layer
- [ ] Create `pkg/addressbook/storage.go`
- [ ] Implement atomic file writes (temp file + rename)
- [ ] Implement file locking for concurrent access
- [ ] Add `load()` and `save()` internal methods
- [ ] Handle missing file (create empty book)
- [ ] Handle corrupted file (backup + error)

### 3.3 Address Book Operations
- [ ] Create `pkg/addressbook/book.go`
- [ ] Implement `Book` struct with:
  - Constructor taking file path
  - Internal mutex for thread safety
  - In-memory cache of entries
- [ ] Implement CRUD operations:
  - `AddPeer(peerID, addrs, metadata) error`
  - `RemovePeer(peerID) error`
  - `GetPeer(peerID) (*PeerEntry, error)`
  - `ListPeers() []PeerEntry` (excludes blacklisted)
  - `ListAllPeers() []PeerEntry` (includes blacklisted)
- [ ] Implement blacklist operations:
  - `BlacklistPeer(peerID) error`
  - `UnblacklistPeer(peerID) error`
  - `IsBlacklisted(peerID) bool`
- [ ] Implement update operations:
  - `UpdatePublicKey(peerID, pubKey) error`
  - `UpdateLastSeen(peerID) error`
  - `UpdateMetadata(peerID, metadata) error`
- [ ] Add tests for all operations including persistence

## Phase 4: libp2p Integration

### 4.1 Host Management
- [ ] Create `pkg/protocol/host.go`
- [ ] Implement libp2p host creation with:
  - Ed25519 identity from app-provided key
  - Configurable listen addresses
  - Connection manager with limits
  - Enable hole punching (AutoNAT, relay)
- [ ] Implement host lifecycle (start/stop)
- [ ] Add connection gater for blacklist enforcement

### 4.2 Protocol Registration
- [ ] Create `pkg/protocol/handlers.go`
- [ ] Define protocol IDs:
  - `/glueberry/handshake/1.0.0`
  - `/glueberry/stream/{name}/1.0.0`
- [ ] Implement stream handler registration
- [ ] Implement incoming connection handling
- [ ] Add protocol negotiation

### 4.3 Connection Gater (Blacklist Enforcement)
- [ ] Create `pkg/protocol/gater.go`
- [ ] Implement `ConnectionGater` interface:
  - `InterceptPeerDial` - check blacklist before dialing
  - `InterceptAccept` - check blacklist on incoming
  - `InterceptSecured` - verify peer ID against blacklist
- [ ] Wire gater to address book blacklist

## Phase 5: Handshake Stream

### 5.1 Handshake Stream Implementation
- [ ] Create `pkg/streams/handshake.go`
- [ ] Implement `HandshakeStream` struct:
  - Wrap libp2p network.Stream
  - Cramberry MessageIterator for reading
  - Cramberry StreamWriter for writing
  - Deadline/timeout tracking
- [ ] Implement methods:
  - `Send(msg any) error` - serialize with Cramberry, write
  - `Receive(msg any) error` - read, deserialize with Cramberry
  - `Close() error`
  - `TimeRemaining() time.Duration`
- [ ] Handle deadline expiration gracefully
- [ ] Add tests with mock streams

### 5.2 Handshake Protocol Handler
- [ ] Create incoming handshake stream handler
- [ ] Emit events when handshake stream opened
- [ ] Implement timeout enforcement
- [ ] Handle handshake stream closure

## Phase 6: Connection Manager

### 6.1 Connection State Machine
- [ ] Create `pkg/connection/state.go`
- [ ] Define `ConnectionState` enum
- [ ] Implement state transition validation
- [ ] Add state change callbacks

### 6.2 Peer Connection
- [ ] Create `pkg/connection/peer.go`
- [ ] Implement `PeerConnection` struct:
  - Current state
  - Handshake stream (when applicable)
  - Encrypted streams (when established)
  - Shared key
  - Reconnection state
- [ ] Implement state transition methods

### 6.3 Connection Manager Core
- [ ] Create `pkg/connection/manager.go`
- [ ] Implement `Manager` struct with:
  - Active connections map
  - Reconnection state map
  - Cancellation functions map
- [ ] Implement `Connect(peerID) (*HandshakeStream, error)`:
  - Check not blacklisted
  - Get multiaddrs from address book
  - Dial peer via libp2p
  - Open handshake stream
  - Set handshake timeout
  - Return HandshakeStream to app
- [ ] Implement `Disconnect(peerID) error`:
  - Close all streams
  - Close libp2p connection
  - Clean up state

### 6.4 Reconnection Logic
- [ ] Create `pkg/connection/reconnect.go`
- [ ] Implement `ReconnectState` struct:
  - Attempt count
  - Next attempt time
  - Backoff calculator
- [ ] Implement exponential backoff:
  - Base delay configurable
  - Max delay configurable
  - Jitter (±10%) to prevent thundering herd
- [ ] Implement reconnection goroutine:
  - Triggered on disconnect
  - Respects blacklist
  - Respects max attempts
  - Cancellable via context
- [ ] Implement `CancelReconnection(peerID) error`
- [ ] Implement cooldown after failed handshake
- [ ] Add comprehensive tests for reconnection scenarios

### 6.5 Timeout Enforcement
- [ ] Implement handshake timeout monitoring
- [ ] On timeout:
  - Close handshake stream
  - Disconnect peer
  - Enter cooldown state
  - Emit `HandshakeTimeout` event
- [ ] Add tests for timeout scenarios

## Phase 7: Encrypted Streams

### 7.1 Encrypted Stream Implementation
- [ ] Create `pkg/streams/encrypted.go`
- [ ] Implement `EncryptedStream` struct:
  - Wrap libp2p network.Stream
  - Shared key for encryption
  - Cramberry reader/writer
  - Write mutex for serialization
- [ ] Implement internal methods:
  - `writeEncrypted(data []byte) error`
  - `readDecrypted() ([]byte, error)`
- [ ] Start read goroutine on creation

### 7.2 Stream Manager
- [ ] Create `pkg/streams/manager.go`
- [ ] Implement `Manager` struct:
  - Map of peer → stream name → EncryptedStream
  - Incoming message channel
  - Crypto module reference
- [ ] Implement `EstablishStreams(peerID, peerPubKey, names) error`:
  - Derive shared key via crypto module
  - For each stream name:
    - Open libp2p stream with protocol ID
    - Create EncryptedStream
    - Start read goroutine
  - Store in manager
- [ ] Implement `Send(peerID, streamName, data) error`:
  - Look up stream
  - Encrypt data
  - Write to stream
- [ ] Implement `CloseStreams(peerID) error`
- [ ] Implement incoming stream handler for remote-initiated streams

### 7.3 Message Routing
- [ ] Implement read goroutine for each stream
- [ ] On message received:
  - Decrypt
  - Create `IncomingMessage` struct
  - Send to incoming channel
- [ ] Handle stream errors/closure
- [ ] Add tests with mock encryption

## Phase 8: Event System

### 8.1 Event Types
- [ ] Create `pkg/connection/events.go`
- [ ] Define `ConnectionEvent` struct
- [ ] Define all event types (state changes)

### 8.2 Event Dispatcher
- [ ] Implement event channel management
- [ ] Emit events from connection manager
- [ ] Emit events from stream manager
- [ ] Handle slow consumers (non-blocking send with buffer)
- [ ] Add tests for event ordering

## Phase 9: Node (Public API)

### 9.1 Node Structure
- [ ] Create `node.go`
- [ ] Implement `Node` struct aggregating all components
- [ ] Implement constructor `New(cfg Config) (*Node, error)`:
  - Validate config
  - Initialize crypto module
  - Initialize address book
  - Create libp2p host (don't start yet)
  - Initialize connection manager
  - Initialize stream manager
  - Create event/message channels

### 9.2 Lifecycle Methods
- [ ] Implement `Start() error`:
  - Start libp2p host
  - Register protocol handlers
  - Start event dispatcher
- [ ] Implement `Stop() error`:
  - Cancel all reconnections
  - Close all connections
  - Stop libp2p host
  - Close channels
- [ ] Add graceful shutdown handling

### 9.3 Address Book Methods
- [ ] Implement all address book wrapper methods
- [ ] Wire blacklist changes to connection gater
- [ ] On blacklist: disconnect peer if connected

### 9.4 Connection Methods
- [ ] Implement `Connect(peerID) (*HandshakeStream, error)`
- [ ] Implement `Disconnect(peerID) error`
- [ ] Implement `CancelReconnection(peerID) error`
- [ ] Implement `ConnectionState(peerID) ConnectionState`

### 9.5 Stream Methods
- [ ] Implement `EstablishEncryptedStreams(peerID, peerPubKey, streamNames) error`:
  - Store public key in address book
  - Delegate to stream manager
  - Update connection state
- [ ] Implement `Send(peerID, streamName, data) error`
- [ ] Implement `Messages() <-chan IncomingMessage`
- [ ] Implement `Events() <-chan ConnectionEvent`

### 9.6 Utility Methods
- [ ] Implement `PeerID() peer.ID`
- [ ] Implement `PublicKey() ed25519.PublicKey`
- [ ] Implement `ListenAddrs() []multiaddr.Multiaddr`

## Phase 10: Incoming Connection Handling

### 10.1 Accept Incoming Connections
- [ ] Implement incoming connection handler
- [ ] Check blacklist via connection gater
- [ ] Create PeerConnection for incoming peer
- [ ] Handle incoming handshake stream requests

### 10.2 Incoming Stream Handling
- [ ] Handle incoming encrypted stream requests
- [ ] Verify peer has completed handshake
- [ ] Verify shared key exists
- [ ] Create EncryptedStream for incoming stream

## Phase 11: Testing

### 11.1 Unit Tests
- [ ] Crypto module tests (all functions)
- [ ] Address book tests (CRUD, persistence, concurrency)
- [ ] Connection state machine tests
- [ ] Reconnection logic tests
- [ ] Stream encryption/decryption tests

### 11.2 Integration Tests
- [ ] Two-node connection test
- [ ] Handshake flow test
- [ ] Encrypted messaging test
- [ ] Reconnection test
- [ ] Blacklist enforcement test
- [ ] Timeout handling test

### 11.3 Stress Tests
- [ ] Multiple concurrent connections
- [ ] High message throughput
- [ ] Rapid connect/disconnect cycles
- [ ] Memory leak detection

## Phase 12: Documentation & Polish

### 12.1 API Documentation
- [ ] Ensure all exported types have godoc comments
- [ ] Add package-level documentation
- [ ] Document thread-safety guarantees
- [ ] Document channel ownership

### 12.2 Examples
- [ ] Create `examples/` directory
- [ ] Add basic connection example
- [ ] Add bidirectional messaging example
- [ ] Add reconnection handling example

### 12.3 Error Messages
- [ ] Review all error messages for clarity
- [ ] Add context to wrapped errors
- [ ] Ensure no sensitive data in errors

---

## Dependency Graph

```
Phase 1 (Setup)
    │
    ▼
Phase 2 (Crypto) ──────────────────────────────┐
    │                                          │
    ▼                                          │
Phase 3 (Address Book)                         │
    │                                          │
    ▼                                          │
Phase 4 (libp2p Integration)                   │
    │                                          │
    ├───────────────┐                          │
    ▼               ▼                          │
Phase 5         Phase 6                        │
(Handshake)     (Connection Mgr)               │
    │               │                          │
    └───────┬───────┘                          │
            │                                  │
            ▼                                  │
        Phase 7 (Encrypted Streams) <──────────┘
            │
            ▼
        Phase 8 (Events)
            │
            ▼
        Phase 9 (Node API)
            │
            ▼
        Phase 10 (Incoming Connections)
            │
            ▼
        Phase 11 (Testing)
            │
            ▼
        Phase 12 (Documentation)
```

## Notes

- Each phase should be completed with tests before moving to the next
- Integration tests in Phase 11 may reveal issues requiring backtracking
- Consider adding benchmarks alongside tests for performance-critical code
- The cramberry integration requires the `../cramberry` module to be available
