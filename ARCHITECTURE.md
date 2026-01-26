# Glueberry Architecture

## Overview

Glueberry is a Go library providing encrypted P2P communications over libp2p. It handles connection management, key exchange, and encrypted stream multiplexing while delegating peer discovery and handshake logic to the consuming application.

**Version:** 1.0.1
**Protocol Version:** 1.0.0

```
┌─────────────────────────────────────────────────────────────────┐
│                        Application                               │
├─────────────────────────────────────────────────────────────────┤
│                      Glueberry Library                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │ AddressBook │  │  Connection │  │    Stream Manager       │  │
│  │  Manager    │  │   Manager   │  │  (Encrypted Streams)    │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │   Crypto    │  │  Flow       │  │    Event Dispatcher     │  │
│  │   Module    │  │  Controller │  │  (with Filtering)       │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │   Logger    │  │  Metrics    │  │   Peer Statistics       │  │
│  │  Interface  │  │  Interface  │  │   Tracker               │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
├─────────────────────────────────────────────────────────────────┤
│                         libp2p                                   │
└─────────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Node (Main Entry Point)

The `Node` type is the primary interface for applications. It coordinates all other components and exposes the public API.

```go
type Node struct {
    host           host.Host           // libp2p host
    addressBook    *addressbook.Book   // Peer storage
    connections    *connection.Manager // Connection lifecycle
    streams        *streams.Manager    // Encrypted stream handling
    crypto         *crypto.Module      // Key derivation & encryption
    events         chan ConnectionEvent
    messages       chan IncomingMessage
}
```

**Responsibilities:**
- Initialize and manage libp2p host
- Coordinate component interactions
- Expose public API to applications

### 2. Address Book Manager

Persists peer information to JSON file and provides CRUD operations.

```go
type PeerEntry struct {
    PeerID      peer.ID              // libp2p peer ID
    Multiaddrs  []multiaddr.Multiaddr // Network addresses
    PublicKey   []byte               // Ed25519 public key (after handshake)
    Metadata    map[string]string    // App-defined metadata
    LastSeen    time.Time            // Last successful connection
    Blacklisted bool                 // Blacklist flag
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

**Operations:**
- `AddPeer(peerID, multiaddrs, metadata)` - Add or update peer
- `RemovePeer(peerID)` - Remove peer from book
- `BlacklistPeer(peerID)` - Mark peer as blacklisted
- `UnblacklistPeer(peerID)` - Remove blacklist flag
- `GetPeer(peerID)` - Retrieve peer info
- `ListPeers()` - List all non-blacklisted peers
- `UpdatePublicKey(peerID, pubKey)` - Store peer's public key after handshake
- `UpdateLastSeen(peerID)` - Update last seen timestamp

**Persistence:**
- JSON file format for human readability and debugging
- Atomic writes (write to temp file, then rename)
- File locking for concurrent access safety

### 3. Connection Manager

Manages connection lifecycle including establishment, reconnection, and teardown.

```go
type Manager struct {
    host         host.Host
    addressBook  *addressbook.Book
    config       ConnectionConfig
    connections  map[peer.ID]*PeerConnection
    reconnecting map[peer.ID]*ReconnectState
    cancelFuncs  map[peer.ID]context.CancelFunc  // For canceling reconnection
}

type ConnectionConfig struct {
    HandshakeTimeout     time.Duration  // Max time for handshake completion
    ReconnectBaseDelay   time.Duration  // Initial delay before reconnect
    ReconnectMaxDelay    time.Duration  // Maximum backoff delay
    ReconnectMaxAttempts int            // Max reconnection attempts (0 = infinite)
    FailedHandshakeCooldown time.Duration // Delay after failed handshake
}

type PeerConnection struct {
    PeerID          peer.ID
    State           ConnectionState
    HandshakeStream *HandshakeStream  // Available until encrypted streams established
    EncryptedStreams map[string]*EncryptedStream
    SharedKey       []byte            // Derived ECDH shared secret
}
```

**Connection States:**
```go
type ConnectionState int

const (
    StateDisconnected ConnectionState = iota
    StateConnecting
    StateConnected          // libp2p connected, handshake stream ready, timeout started
    StateEstablished        // Encrypted streams active
    StateReconnecting       // Attempting reconnection
    StateCooldown           // Waiting after failed handshake
)
```

**Reconnection Logic:**
1. On disconnect, check if peer is blacklisted → if so, don't reconnect
2. If reconnection enabled and attempts remain:
   - Wait for exponential backoff delay
   - Attempt connection
   - On success, emit `StateConnected` event
   - On failure, increment attempt counter, repeat
3. If max attempts reached, emit final disconnect event
4. After failed handshake (timeout), enter cooldown period before retrying

**Cancellation:**
- `CancelReconnection(peerID)` - Stop ongoing reconnection attempts

### 4. Handshake Stream

The handshake uses an unencrypted stream named "handshake" that is available immediately when `StateConnected` is reached. The app uses `Send()` and `Messages()` to exchange handshake messages, just like with encrypted streams.

```go
// Send handshake message
node.Send(peerID, streams.HandshakeStreamName, helloMsg)

// Receive handshake messages
for msg := range node.Messages() {
    if msg.StreamName == streams.HandshakeStreamName {
        // Process handshake message
    }
}
```

**Timeout Handling:**
- Handshake timeout starts when `StateConnected` is reached
- If app doesn't call `CompleteHandshake()` before timeout:
  - Handshake stream closed
  - Connection dropped
  - Peer enters cooldown state
  - `StateDisconnected` event emitted with timeout error

### 5. Crypto Module

Handles all cryptographic operations.

```go
type Module struct {
    privateKey ed25519.PrivateKey
    publicKey  ed25519.PublicKey
    x25519Priv []byte  // Derived X25519 private key
    x25519Pub  []byte  // Derived X25519 public key
}
```

**Key Derivation:**
```
Ed25519 Private Key
        │
        ▼
   SHA-512 hash
        │
        ▼
   First 32 bytes (clamped)
        │
        ▼
   X25519 Private Key
        │
        ▼
   Scalar multiplication with basepoint
        │
        ▼
   X25519 Public Key
```

**Shared Secret Derivation:**
```
Local X25519 Private Key + Remote X25519 Public Key
                    │
                    ▼
              ECDH (X25519)
                    │
                    ▼
              Raw Shared Secret
                    │
                    ▼
              HKDF-SHA256 (with context)
                    │
                    ▼
              256-bit Symmetric Key
```

**Encryption (ChaCha20-Poly1305):**
- 256-bit key from HKDF
- 96-bit random nonce per message
- 128-bit authentication tag
- Message format: `[12-byte nonce][ciphertext][16-byte tag]`

### 6. Stream Manager

Manages encrypted application streams.

```go
type Manager struct {
    host        host.Host
    crypto      *crypto.Module
    peerStreams map[peer.ID]map[string]*EncryptedStream
    incoming    chan IncomingMessage
}

type EncryptedStream struct {
    name       string
    peerID     peer.ID
    stream     network.Stream
    sharedKey  []byte
    reader     *cramberry.MessageIterator
    writer     *cramberry.StreamWriter
    writeMu    sync.Mutex  // Serialize writes
}

type IncomingMessage struct {
    PeerID     peer.ID
    StreamName string
    Data       []byte      // Decrypted message payload
    Timestamp  time.Time
}
```

**Stream Setup (Two-Phase, Lazy Opening):**
1. App calls `PrepareStreams(peerID, peerPubKey, streamNames)` when receiving peer's PubKey
2. Crypto module derives shared key from ECDH
3. Stream names are registered, handlers ready to accept incoming encrypted messages
4. App calls `FinalizeHandshake(peerID)` when receiving peer's Complete message
5. Connection transitions to StateEstablished
6. On first `Send()` to a stream name:
   - Open libp2p stream with protocol ID `/glueberry/stream/<name>/1.0.0`
   - Wrap with encryption layer
   - Start read goroutine for incoming messages
7. Incoming streams are accepted and wrapped on arrival

**Message Flow:**

*Sending:*
```
App calls Send(peerID, streamName, data)
            │
            ▼
    Serialize with Cramberry
            │
            ▼
    Encrypt with ChaCha20-Poly1305
            │
            ▼
    Write to libp2p stream
```

*Receiving:*
```
    libp2p stream data
            │
            ▼
    Read framed message
            │
            ▼
    Decrypt with ChaCha20-Poly1305
            │
            ▼
    Send to incoming channel
            │
            ▼
    App receives from Messages()
```

### 7. Event Dispatcher

Delivers connection state changes to the application.

```go
type ConnectionEvent struct {
    PeerID    peer.ID
    State     ConnectionState
    Error     error           // Non-nil for error states
    Timestamp time.Time
}
```

**Events Emitted:**
- `StateConnecting` - Connection attempt started
- `StateConnected` - libp2p connection established, handshake stream ready, timeout started
- `StateEstablished` - Encrypted streams active, handshake timeout cancelled
- `StateDisconnected` - Connection lost
- `StateReconnecting` - Reconnection attempt starting
- `StateCooldown` - Entered cooldown after failed handshake

**Event Filtering:**

Applications can subscribe to filtered events for specific peers or states:

```go
// All events
allEvents := node.Events()

// Events for a specific peer
peerEvents := node.EventsForPeer(peerID)

// Events for specific states
connectEvents := node.EventsForStates(StateConnected, StateEstablished)

// Custom filter
filter := EventFilter{
    PeerIDs: []peer.ID{peer1, peer2},
    States:  []ConnectionState{StateEstablished},
}
filtered := node.FilteredEvents(filter)
```

### 8. Observability (Logger & Metrics)

Glueberry provides pluggable interfaces for logging and metrics collection.

**Logger Interface:**

```go
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Info(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
    Error(msg string, keysAndValues ...any)
}
```

Compatible with slog, zap, zerolog. NopLogger provided for when logging is disabled.

**Strategic Log Points:**
| Component | Level | Event | Fields |
|-----------|-------|-------|--------|
| Node | Info | Started/Stopped | peer_id, listen_addrs |
| Connection | Info | Connected | peer_id, addr |
| Connection | Info | Disconnected | peer_id, reason |
| Connection | Debug | Handshake started | peer_id |
| Connection | Warn | Handshake failed | peer_id, error |
| Crypto | Warn | Decryption failed | peer_id, error |
| AddressBook | Warn | Peer blacklisted | peer_id |

**Metrics Interface:**

```go
type Metrics interface {
    // Connection metrics
    ConnectionOpened(direction string)
    ConnectionClosed(direction string)
    ConnectionAttempt(result string)
    HandshakeDuration(seconds float64)
    HandshakeResult(result string)

    // Stream metrics
    MessageSent(stream string, bytes int)
    MessageReceived(stream string, bytes int)
    StreamOpened(stream string)
    StreamClosed(stream string)

    // Crypto metrics
    EncryptionError()
    DecryptionError()
    KeyDerivation(cached bool)

    // Event metrics
    EventEmitted(state string)
    EventDropped()
    MessageDropped()

    // Flow control metrics
    BackpressureEngaged(stream string)
    BackpressureWait(stream string, seconds float64)
    PendingMessages(stream string, count int)
}
```

NopMetrics provided for when metrics are disabled.

### 9. Flow Controller

Per-stream flow control with backpressure to prevent overwhelming peers or buffers.

```go
type FlowController struct {
    highWatermark int  // Block sends above this threshold (default: 1000)
    lowWatermark  int  // Unblock when pending drops to this (default: 100)
    pending       int  // Current pending message count
}
```

**Behavior:**
1. Before each Send(), Acquire() is called
2. If pending >= highWatermark, sender blocks (context-aware)
3. After message is processed, Release() is called
4. When pending <= lowWatermark, blocked senders unblock

**Configuration:**
```go
cfg := glueberry.NewConfig(
    privateKey, addressBookPath, listenAddrs,
    glueberry.WithHighWatermark(500),
    glueberry.WithLowWatermark(50),
    glueberry.WithMaxMessageSize(512*1024),  // 512KB max
    // glueberry.WithBackpressureDisabled(), // Disable if needed
)
```

### 10. Peer Statistics Tracker

Comprehensive per-peer statistics for monitoring connection health and throughput.

```go
type PeerStats struct {
    PeerID           peer.ID
    Connected        bool
    IsOutbound       bool
    ConnectedAt      time.Time
    TotalConnectTime time.Duration

    // Message stats
    MessagesSent     int64
    MessagesReceived int64
    BytesSent        int64
    BytesReceived    int64

    // Per-stream stats
    StreamStats      map[string]*StreamStats

    // Health
    LastMessageAt    time.Time
    ConnectionCount  int  // Total connections including reconnects
    FailureCount     int
}

type StreamStats struct {
    Name             string
    MessagesSent     int64
    MessagesReceived int64
    BytesSent        int64
    BytesReceived    int64
    LastSentAt       time.Time
    LastReceivedAt   time.Time
}
```

**Usage:**
```go
// Get stats for a specific peer
stats := node.PeerStatistics(peerID)

// Get all peer stats
allStats := node.AllPeerStatistics()
```

### 11. Protocol Versioning

Semantic versioning for protocol compatibility checking.

```go
type ProtocolVersion struct {
    Major uint8  // Breaking changes (must match)
    Minor uint8  // New features (peer's cannot exceed ours)
    Patch uint8  // Bug fixes (always compatible)
}
```

**Compatibility Rules:**
- Major versions must match
- Peer's minor version must not exceed ours
- Patch versions are always compatible

```go
version := node.Version()
fmt.Printf("Protocol version: %s\n", version.String())  // "1.0.0"

// In handshake, check compatibility
if !myVersion.Compatible(peerVersion) {
    return ErrVersionMismatch
}
```

### 12. Debug Utilities

Tools for troubleshooting and diagnostics.

```go
// Get complete node state as struct
state := node.DumpState()

// Get state as formatted JSON
jsonStr, _ := node.DumpStateJSON()

// Get human-readable summary
fmt.Println(node.DumpStateString())

// Connection summary
summary := node.ConnectionSummary()

// List known peers
peerIDs := node.ListKnownPeers()

// Get detailed peer info
info, _ := node.PeerInfo(peerID)
```

## Public API

### Configuration

```go
type Config struct {
    // Required
    PrivateKey      ed25519.PrivateKey  // App-provided identity key
    AddressBookPath string               // Path to JSON persistence file
    ListenAddrs     []multiaddr.Multiaddr // Addresses to listen on

    // Timeouts & Reconnection
    HandshakeTimeout        time.Duration // Default: 30s
    ReconnectBaseDelay      time.Duration // Default: 1s
    ReconnectMaxDelay       time.Duration // Default: 5m
    ReconnectMaxAttempts    int           // Default: 10, 0 = infinite
    FailedHandshakeCooldown time.Duration // Default: 1m

    // Channel buffer sizes
    EventBufferSize   int  // Default: 100
    MessageBufferSize int  // Default: 1000

    // Flow Control
    HighWatermark      int  // Default: 1000 - Block sends above this
    LowWatermark       int  // Default: 100 - Unblock when pending drops to this
    MaxMessageSize     int  // Default: 1MB - Maximum message size
    DisableBackpressure bool // Default: false

    // Observability
    Logger  Logger   // Default: NopLogger
    Metrics Metrics  // Default: NopMetrics
}
```

**Functional Options:**
```go
cfg := glueberry.NewConfig(
    privateKey, addressBookPath, listenAddrs,
    glueberry.WithHandshakeTimeout(60*time.Second),
    glueberry.WithReconnectMaxAttempts(5),
    glueberry.WithHighWatermark(500),
    glueberry.WithLowWatermark(50),
    glueberry.WithMaxMessageSize(512*1024),
    glueberry.WithLogger(myLogger),
    glueberry.WithMetrics(myMetrics),
)
```

### Node Methods

```go
// Lifecycle
func New(cfg Config) (*Node, error)
func (n *Node) Start() error
func (n *Node) Stop() error
func (n *Node) PeerID() peer.ID
func (n *Node) PublicKey() ed25519.PublicKey
func (n *Node) Addrs() []multiaddr.Multiaddr
func (n *Node) Version() ProtocolVersion

// Address Book
func (n *Node) AddPeer(peerID peer.ID, addrs []multiaddr.Multiaddr, metadata map[string]string) error
func (n *Node) RemovePeer(peerID peer.ID) error
func (n *Node) BlacklistPeer(peerID peer.ID) error
func (n *Node) UnblacklistPeer(peerID peer.ID) error
func (n *Node) GetPeer(peerID peer.ID) (*PeerInfo, error)
func (n *Node) ListPeers() []PeerInfo
func (n *Node) PeerAddrs(peerID peer.ID) []multiaddr.Multiaddr

// Connection (with context support)
func (n *Node) Connect(peerID peer.ID) error
func (n *Node) ConnectCtx(ctx context.Context, peerID peer.ID) error
func (n *Node) Disconnect(peerID peer.ID) error
func (n *Node) DisconnectCtx(ctx context.Context, peerID peer.ID) error
func (n *Node) CancelReconnection(peerID peer.ID) error
func (n *Node) ConnectionState(peerID peer.ID) ConnectionState
func (n *Node) IsOutbound(peerID peer.ID) (bool, error)

// Handshake Completion (two-phase for race-free establishment)
func (n *Node) PrepareStreams(peerID peer.ID, peerPubKey ed25519.PublicKey, streamNames []string) error
func (n *Node) FinalizeHandshake(peerID peer.ID) error
func (n *Node) CompleteHandshake(peerID peer.ID, peerPubKey ed25519.PublicKey, streamNames []string) error

// Messaging (with context support)
func (n *Node) Send(peerID peer.ID, streamName string, data []byte) error
func (n *Node) SendCtx(ctx context.Context, peerID peer.ID, streamName string, data []byte) error
func (n *Node) Messages() <-chan IncomingMessage

// Events (with filtering)
func (n *Node) Events() <-chan ConnectionEvent
func (n *Node) FilteredEvents(filter EventFilter) <-chan ConnectionEvent
func (n *Node) EventsForPeer(peerID peer.ID) <-chan ConnectionEvent
func (n *Node) EventsForStates(states ...ConnectionState) <-chan ConnectionEvent

// Statistics
func (n *Node) PeerStatistics(peerID peer.ID) *PeerStats
func (n *Node) AllPeerStatistics() map[peer.ID]*PeerStats

// Debug
func (n *Node) DumpState() *DebugState
func (n *Node) DumpStateJSON() (string, error)
func (n *Node) DumpStateString() string
func (n *Node) ConnectionSummary() map[ConnectionState]int
func (n *Node) ListKnownPeers() []peer.ID
func (n *Node) PeerInfo(peerID peer.ID) (*DebugPeerInfo, error)
```

## Protocol IDs

libp2p protocol identifiers used:

| Protocol | ID |
|----------|-----|
| Handshake | `/glueberry/handshake/1.0.0` |
| Encrypted Stream | `/glueberry/stream/<name>/1.0.0` |

## Message Framing

All messages use Cramberry's streaming format with length-delimited messages:

```
┌─────────────────────────────────────────────┐
│  Varint Length  │  Cramberry Payload        │
│   (1-10 bytes)  │  (length bytes)           │
└─────────────────────────────────────────────┘
```

For encrypted streams, the Cramberry payload contains:

```
┌─────────────────────────────────────────────────────────────┐
│  Nonce (12 bytes)  │  Encrypted Data  │  Auth Tag (16 bytes)│
└─────────────────────────────────────────────────────────────┘
```

## Sequence Diagrams

### Initial Connection & Handshake

```
    App                     Glueberry                    Remote Peer
     │                          │                             │
     │ AddPeer(peerID, addrs)   │                             │
     │─────────────────────────>│                             │
     │                          │  libp2p dial                │
     │                          │────────────────────────────>│
     │                          │  connection established     │
     │                          │<────────────────────────────│
     │                          │                             │
     │  <-Events()              │  start handshake timeout    │
     │  (StateConnected)        │  open /glueberry/handshake  │
     │<─────────────────────────│────────────────────────────>│
     │                          │                             │
     │  Send(peer, "handshake", │                             │
     │       helloMsg)          │  forward                    │
     │─────────────────────────>│────────────────────────────>│
     │                          │                             │
     │                          │  hello from remote          │
     │  <-Messages() (Hello)    │<────────────────────────────│
     │<─────────────────────────│                             │
     │                          │                             │
     │  Send(peer, "handshake", │                             │
     │       pubKeyMsg)         │  forward                    │
     │─────────────────────────>│────────────────────────────>│
     │                          │                             │
     │                          │  pubkey from remote         │
     │  <-Messages() (PubKey)   │<────────────────────────────│
     │<─────────────────────────│                             │
     │                          │                             │
     │  Send(peer, "handshake", │                             │
     │       completeMsg)       │  forward                    │
     │─────────────────────────>│────────────────────────────>│
     │                          │                             │
     │                          │  complete from remote       │
     │  <-Messages() (Complete) │<────────────────────────────│
     │<─────────────────────────│                             │
     │                          │                             │
     │ CompleteHandshake(peer,  │                             │
     │   pubKey, streamNames)   │  cancel timeout             │
     │─────────────────────────>│  derive shared key          │
     │                          │  setup encrypted streams    │
     │                          │                             │
     │  <-Events()              │                             │
     │  (StateEstablished)      │                             │
     │<─────────────────────────│                             │
     │                          │                             │
```

### Establishing Encrypted Streams (via CompleteHandshake)

```
    App                     Glueberry                    Remote Peer
     │                          │                             │
     │ CompleteHandshake        │                             │
     │ (peerID, pubKey, names)  │                             │
     │─────────────────────────>│                             │
     │                          │  cancel handshake timeout   │
     │                          │                             │
     │                          │  ECDH key derivation        │
     │                          │  (local priv + remote pub)  │
     │                          │                             │
     │                          │  close handshake stream     │
     │                          │                             │
     │                          │  register encrypted streams │
     │                          │  (lazy - opened on first    │
     │                          │   Send() call)              │
     │                          │                             │
     │                          │  store pubkey in addressbook│
     │                          │                             │
     │                          │  transition to Established  │
     │                          │                             │
     │  success                 │                             │
     │<─────────────────────────│                             │
     │                          │                             │
```

### Sending & Receiving Messages

```
    App                     Glueberry                    Remote Peer
     │                          │                             │
     │ Send(peer, stream, data) │                             │
     │─────────────────────────>│                             │
     │                          │  Cramberry serialize        │
     │                          │  ChaCha20 encrypt           │
     │                          │  write to stream            │
     │                          │────────────────────────────>│
     │  nil (success)           │                             │
     │<─────────────────────────│                             │
     │                          │                             │
     │                          │  incoming encrypted data    │
     │                          │<────────────────────────────│
     │                          │  ChaCha20 decrypt           │
     │                          │  Cramberry deserialize      │
     │  <-Messages()            │                             │
     │<─────────────────────────│                             │
     │                          │                             │
```

### Reconnection Flow

```
    App                     Glueberry                    Remote Peer
     │                          │                             │
     │                          │  connection lost            │
     │                          │<──────────X                 │
     │                          │                             │
     │  <-Events()              │                             │
     │  (Disconnected)          │                             │
     │<─────────────────────────│                             │
     │                          │                             │
     │  <-Events()              │                             │
     │  (Reconnecting)          │                             │
     │<─────────────────────────│                             │
     │                          │  wait backoff               │
     │                          │  dial attempt               │
     │                          │─────────────X (fail)        │
     │                          │                             │
     │                          │  wait backoff * 2           │
     │                          │  dial attempt               │
     │                          │────────────────────────────>│
     │                          │  success                    │
     │                          │<────────────────────────────│
     │                          │                             │
     │  <-Events()              │  start handshake timeout    │
     │  (StateConnected)        │  open handshake stream      │
     │<─────────────────────────│                             │
     │                          │                             │
     │  Send Hello...           │  (app performs handshake    │
     │  receive Hello...        │   same as initial connect)  │
     │  ...                     │                             │
     │                          │                             │
```

## Security Considerations

1. **Key Material**: Private keys and shared secrets never leave the crypto module. They are not logged or exposed in errors.

2. **Nonce Handling**: Random 96-bit nonces for each message. With ChaCha20-Poly1305, this provides ~2^48 messages before nonce collision risk becomes significant.

3. **Authentication**: Poly1305 MAC provides message authentication. Tampered messages are rejected.

4. **Blacklisting**: Blacklisted peers are immediately disconnected and all connection attempts (incoming and outgoing) are refused.

5. **Input Validation**: All data from the network is treated as untrusted. Cramberry's `SecureOptions` used for deserialization with conservative limits.

6. **Handshake Timeout**: Prevents resource exhaustion from peers that connect but never complete handshake.

## Thread Safety

- All public `Node` methods are thread-safe
- Address book operations are serialized internally
- Stream writes are serialized per-stream (concurrent writes to different streams are safe)
- Event and message channels are safe for concurrent reads (single consumer recommended)

## Error Handling

Glueberry provides both sentinel errors for simple cases and rich error types for programmatic handling.

### Sentinel Errors

```go
var (
    ErrPeerNotFound       = errors.New("peer not found in address book")
    ErrPeerBlacklisted    = errors.New("peer is blacklisted")
    ErrNotConnected       = errors.New("not connected to peer")
    ErrAlreadyConnected   = errors.New("already connected to peer")
    ErrHandshakeTimeout   = errors.New("handshake timeout")
    ErrStreamNotFound     = errors.New("stream not found")
    ErrEncryptionFailed   = errors.New("encryption failed")
    ErrDecryptionFailed   = errors.New("decryption failed")
    ErrInvalidPublicKey   = errors.New("invalid public key")
    ErrVersionMismatch    = errors.New("protocol version mismatch")
    ErrMessageTooLarge    = errors.New("message exceeds max size")
    ErrBackpressureTimeout = errors.New("backpressure timeout")
)
```

### Rich Error Types

For programmatic error handling, Glueberry provides rich error types with error codes:

```go
type ErrorCode int

const (
    ErrCodeUnknown ErrorCode = iota
    ErrCodeConnectionFailed
    ErrCodeHandshakeFailed
    ErrCodeHandshakeTimeout
    ErrCodeStreamClosed
    ErrCodeEncryptionFailed
    ErrCodeDecryptionFailed
    ErrCodePeerNotFound
    ErrCodePeerBlacklisted
    ErrCodeBufferFull
    ErrCodeContextCanceled
    ErrCodeInvalidConfig
    ErrCodeNodeNotStarted
    ErrCodeNodeAlreadyStarted
    ErrCodeVersionMismatch
    ErrCodeMessageTooLarge
    ErrCodeBackpressure
)

type Error struct {
    Code      ErrorCode
    Message   string
    PeerID    peer.ID  // Optional: relevant peer
    Stream    string   // Optional: relevant stream
    Cause     error    // Optional: underlying error
    Retriable bool     // Whether operation can be retried
}
```

**Helper Functions:**
```go
// Check if error can be retried
if IsRetriable(err) {
    // Retry the operation
}

// Check if error is permanent
if IsPermanent(err) {
    // Don't retry, remove peer
}

// Extract error details
var gErr *Error
if errors.As(err, &gErr) {
    log.Printf("Error %s on peer %s: %s", gErr.Code, gErr.PeerID, gErr.Message)
}

// Check specific error codes
if errors.Is(err, &Error{Code: ErrCodeConnectionFailed}) {
    // Handle connection failure
}
```

## Security Hardening

### Key Material Protection

1. **Key Zeroing**: All sensitive key material is zeroed after use using `crypto.ZeroBytes()`:
   ```go
   defer crypto.ZeroBytes(sharedSecret)
   ```

2. **Deep Copy on Access**: Keys are deep-copied when accessed to prevent external modification:
   ```go
   key := node.GetSharedKey(peerID)  // Returns a copy
   ```

3. **No Logging of Secrets**: Private keys, shared secrets, and decrypted messages are never logged, even at debug level.

### Decryption Error Handling

Decryption failures (indicating tampering or corrupted data) are automatically:
1. Logged via the configured Logger at WARN level
2. Counted via the Metrics interface (`DecryptionError()`)
3. The message is silently dropped (not delivered to application)

**Note:** Custom decryption error callbacks are planned for v1.1.0 (see [ROADMAP.md](ROADMAP.md)).

### ECDH Security

1. **Low-Order Point Check**: X25519 shared secret is validated to prevent small subgroup attacks:
   ```go
   // Rejects all-zero shared secrets (indicates invalid peer key)
   if subtle.ConstantTimeCompare(shared, allZeros) == 1 {
       return nil, ErrInvalidPublicKey
   }
   ```

2. **Key Derivation**: HKDF-SHA256 with context binding ensures domain separation.

### Input Validation

1. **Message Size Limits**: Configurable maximum message size (default 1MB)
2. **Cramberry SecureOptions**: Conservative deserialization limits
3. **Stream Name Validation**: Only alphanumeric and hyphen characters allowed

## Testing

### Unit Tests

Comprehensive test coverage with race detection:
```bash
make test        # Run all unit tests
make test-race   # Run with race detector
make coverage    # Generate coverage report
```

### Fuzz Testing

Fuzz tests for security-critical parsing:
- `fuzz/crypto_fuzz_test.go` - Cipher operations, key conversion
- `fuzz/cramberry_fuzz_test.go` - Message parsing
- `fuzz/handshake_fuzz_test.go` - Handshake message parsing

```bash
go test -fuzz=FuzzDecrypt -fuzztime=30s ./fuzz/
```

### Stress Tests

Concurrent stress tests for race condition verification:
- `pkg/connection/stress_test.go` - State machine concurrency

```bash
go test -race -run="Test.*Concurrent" ./pkg/connection/...
```

### Integration Tests

Full connection lifecycle tests:
```bash
make test-integration
```

### MockNode for Application Testing

A MockNode implementation is provided for testing applications:

```go
import "github.com/blockberries/glueberry/pkg/testing"

func TestMyApp(t *testing.T) {
    mock := testing.NewMockNode()

    // Simulate events
    mock.EmitEvent(glueberry.ConnectionEvent{
        PeerID: peerID,
        State:  glueberry.StateConnected,
    })

    // Simulate incoming messages
    mock.SimulateMessage(peerID, "stream", []byte("hello"))

    // Verify sends
    sent := mock.SentMessages()
    assert.Len(t, sent, 1)
}
```

## Examples

Example applications are provided in the `examples/` directory:

| Example | Description |
|---------|-------------|
| `basic/` | Minimal peer-to-peer connection |
| `simple-chat/` | Interactive chat between two peers |
| `file-transfer/` | Chunked file transfer with progress |
| `rpc/` | Request-response RPC pattern |
| `cluster/` | Multi-node cluster mesh |
| `blockberry-integration/` | Integration with Blockberry node |

## Performance Considerations

### Buffer Pooling

Internal byte buffers are pooled to reduce GC pressure:
- Message serialization buffers
- Encryption/decryption buffers

### Lazy Stream Opening

Streams are created on first use, not during handshake completion. This:
- Reduces connection overhead
- Allows either peer to initiate communication
- Avoids unused stream creation

### Non-Blocking Events

Event channels use non-blocking sends with metrics for drops:
```go
select {
case eventChan <- event:
    metrics.EventEmitted(event.State.String())
default:
    metrics.EventDropped()
}
```

### Flow Control

High/low watermark backpressure prevents memory exhaustion from fast producers.

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2026-01-26 | Initial release |
| 1.0.1 | 2026-01-26 | Stress tests, cramberry v1.2.0 |
