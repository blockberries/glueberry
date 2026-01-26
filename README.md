# Glueberry

**Glueberry** is a Go library for secure P2P communications built on [libp2p](https://libp2p.io/). It provides encrypted, multiplexed streams between peers with application-controlled handshaking and automatic reconnection.

**Version:** 1.0.1 | **Protocol Version:** 1.0.0

## Features

### Core
- **End-to-End Encryption** - ChaCha20-Poly1305 authenticated encryption with X25519 ECDH key exchange
- **Symmetric Event-Driven API** - Same code handles both initiator and responder
- **App-Controlled Handshaking** - Flexible handshake protocol defined by your application
- **Two-Phase Handshake** - Race-condition-free stream establishment with `PrepareStreams` + `FinalizeHandshake`
- **Automatic Reconnection** - Exponential backoff with jitter and configurable retry limits
- **Stream Multiplexing** - Multiple named encrypted streams per peer connection
- **Lazy Stream Opening** - Streams created on first use, not during handshake

### Operations
- **Context Support** - `ConnectCtx`, `SendCtx`, `DisconnectCtx` for cancellation and timeouts
- **Flow Control** - High/low watermark backpressure to prevent memory exhaustion
- **Event Filtering** - Subscribe to events for specific peers or states
- **Peer Statistics** - Per-peer message counts, bytes, latency tracking

### Observability
- **Logger Interface** - Pluggable structured logging (slog, zap, zerolog compatible)
- **Metrics Interface** - Prometheus-compatible metrics collection
- **Debug Utilities** - State dumping, connection summaries, peer info

### Security
- **Key Material Protection** - Automatic zeroing of sensitive data
- **Decryption Callbacks** - Observability for tampering detection
- **Input Validation** - Message size limits, Cramberry SecureOptions
- **Blacklist Support** - Multi-layer connection enforcement

### Testing
- **MockNode** - For application testing without network
- **Fuzz Tests** - Security-critical parsing
- **Stress Tests** - Concurrent operation verification

## Installation

```bash
go get github.com/blockberries/glueberry@v1.0.1
```

**Requirements:**
- Go 1.21 or later
- [Cramberry](https://github.com/blockberries/cramberry) v1.2.0+ for message serialization

## Quick Start

```go
package main

import (
    "crypto/ed25519"
    "crypto/rand"
    "fmt"

    "github.com/blockberries/glueberry"
    "github.com/blockberries/glueberry/pkg/streams"
    "github.com/multiformats/go-multiaddr"
)

func main() {
    // Generate identity key
    _, privateKey, _ := ed25519.GenerateKey(rand.Reader)
    publicKey := privateKey.Public().(ed25519.PublicKey)

    // Configure listen address
    listenAddr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9000")

    // Create node
    cfg := glueberry.NewConfig(
        privateKey,
        "./addressbook.json",
        []multiaddr.Multiaddr{listenAddr},
    )

    node, err := glueberry.New(cfg)
    if err != nil {
        panic(err)
    }

    // Start the node
    if err := node.Start(); err != nil {
        panic(err)
    }
    defer node.Stop()

    fmt.Printf("Node started with ID: %s\n", node.PeerID())
    fmt.Printf("Listening on: %v\n", node.Addrs())

    // Handle connection events - send Hello when connected
    go func() {
        for event := range node.Events() {
            if event.State == glueberry.StateConnected {
                // Connection established, send Hello to initiate handshake
                node.Send(event.PeerID, streams.HandshakeStreamName, helloMsg)
            }
        }
    }()

    // Handle handshake messages with event-driven flow
    go func() {
        var peerPubKey ed25519.PublicKey
        var gotPubKey, gotComplete bool

        for msg := range node.Messages() {
            if msg.StreamName == streams.HandshakeStreamName {
                handleHandshakeMessage(node, msg, publicKey, &peerPubKey, &gotPubKey, &gotComplete)
            } else {
                // Handle encrypted message
                fmt.Printf("Received: %s\n", msg.Data)
            }
        }
    }()

    // Connect to a peer (or use AddPeer for auto-connect)
    // node.AddPeer(peerID, peerAddrs, nil)

    select {} // Keep running
}
```

## Usage

### 1. Create and Start a Node

```go
// Generate or load your Ed25519 private key
_, privateKey, _ := ed25519.GenerateKey(rand.Reader)

// Configure the node
cfg := glueberry.NewConfig(
    privateKey,
    "./data/addressbook.json",
    []multiaddr.Multiaddr{listenAddr},
    glueberry.WithHandshakeTimeout(60*time.Second),
    glueberry.WithReconnectMaxAttempts(5),
)

// Create and start
node, _ := glueberry.New(cfg)
node.Start()
defer node.Stop()
```

### 2. Connect to a Peer

```go
// Add peer to address book (auto-connects when node is started)
peerAddr, _ := multiaddr.NewMultiaddr("/ip4/192.168.1.100/tcp/9000")
node.AddPeer(peerID, []multiaddr.Multiaddr{peerAddr}, nil)

// Or explicitly connect if auto-connect isn't desired
node.Connect(peerID)

// When connected, StateConnected event fires and handshake stream is ready
// Your event handler should react by sending the first handshake message
```

### 3. Handle Handshake Messages (Two-Phase API)

The handshake uses a two-phase completion to prevent race conditions:

```go
// StateConnected event → Send Hello
// Receive Hello → Send PubKey
// Receive PubKey → PrepareStreams() + Send Complete
// Receive Complete → FinalizeHandshake()

var streamsPrepared, gotComplete bool

for msg := range node.Messages() {
    if msg.StreamName == streams.HandshakeStreamName {
        switch msg.Data[0] {
        case msgHello:
            // Receive Hello → Send our public key
            node.Send(msg.PeerID, streams.HandshakeStreamName, pubKeyMsg)

        case msgPubKey:
            // Receive PubKey → Prepare streams (ready to receive encrypted messages)
            peerPubKey := ed25519.PublicKey(msg.Data[1:])
            node.PrepareStreams(msg.PeerID, peerPubKey, []string{"messages"})
            streamsPrepared = true
            // Send Complete to signal we're ready
            node.Send(msg.PeerID, streams.HandshakeStreamName, completeMsg)

        case msgComplete:
            // Receive Complete → Peer is ready
            gotComplete = true
        }

        // When both sides are ready, finalize the handshake
        if streamsPrepared && gotComplete {
            node.FinalizeHandshake(msg.PeerID)
            streamsPrepared, gotComplete = false, false // Reset for next peer
        }
    }
}
```

**Why Two Phases?**
- `PrepareStreams()` derives the shared key and registers stream handlers, so the node can receive encrypted messages immediately
- `FinalizeHandshake()` transitions to `StateEstablished` only after the peer confirms it's also ready
- This prevents race conditions where one peer sends encrypted messages before the other has prepared its streams

### 4. Send and Receive Encrypted Messages

```go
// Send encrypted message (after handshake complete)
data := []byte("hello, peer!")
if err := node.Send(peerID, "messages", data); err != nil {
    // Handle error
}

// Receive messages
go func() {
    for msg := range node.Messages() {
        if msg.StreamName == "messages" {
            fmt.Printf("Received from %s: %s\n", msg.PeerID, msg.Data)
        }
    }
}()
```

### 5. Monitor Connection Events

Connection events drive the handshake flow. React to `StateConnected` to initiate:

```go
go func() {
    for event := range node.Events() {
        fmt.Printf("Peer %s: %s\n", event.PeerID, event.State)

        switch event.State {
        case glueberry.StateConnected:
            // Handshake stream ready - send Hello to initiate handshake
            node.Send(event.PeerID, streams.HandshakeStreamName, helloMsg)
        case glueberry.StateEstablished:
            fmt.Println("  Encrypted streams ready")
        case glueberry.StateDisconnected:
            fmt.Println("  Peer disconnected")
        }
    }
}()
```

### 6. Manage Peers

```go
// List all peers
peers := node.ListPeers()
for _, peer := range peers {
    fmt.Printf("Peer: %s, LastSeen: %s\n", peer.PeerID, peer.LastSeen)
}

// Blacklist a misbehaving peer
node.BlacklistPeer(badPeerID) // Also disconnects if connected

// Remove a peer
node.RemovePeer(oldPeerID)
```

## Configuration Options

```go
type Config struct {
    // Required
    PrivateKey      ed25519.PrivateKey   // Your node's identity
    AddressBookPath string                // Path to address book JSON file
    ListenAddrs     []multiaddr.Multiaddr // Addresses to listen on

    // Timeouts & Reconnection
    HandshakeTimeout        time.Duration // Default: 30s
    ReconnectBaseDelay      time.Duration // Default: 1s
    ReconnectMaxDelay       time.Duration // Default: 5m
    ReconnectMaxAttempts    int           // Default: 10 (0 = unlimited)
    FailedHandshakeCooldown time.Duration // Default: 1m

    // Buffers
    EventBufferSize         int           // Default: 100
    MessageBufferSize       int           // Default: 1000

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
    privateKey,
    "./data/addressbook.json",
    listenAddrs,
    glueberry.WithHandshakeTimeout(60*time.Second),
    glueberry.WithReconnectMaxAttempts(5),
    glueberry.WithMessageBufferSize(5000),
    glueberry.WithHighWatermark(500),
    glueberry.WithMaxMessageSize(512*1024),
    glueberry.WithLogger(myLogger),
    glueberry.WithMetrics(myMetrics),
    glueberry.WithDecryptionErrorCallback(func(peerID peer.ID, err error) {
        log.Warn("possible tampering", "peer", peerID)
    }),
)
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          Application Layer                               │
│    ┌─────────────────────────────────────────────────────────────────┐   │
│    │  Peer Discovery    Handshake Protocol    Message Handling        │   │
│    │  (Your Code)       (Your Code)           (Your Code)             │   │
│    └─────────────────────────────────────────────────────────────────┘   │
├─────────────────────────────────────────────────────────────────────────┤
│                         Glueberry Node                                   │
│  ┌───────────────┐  ┌────────────────┐  ┌────────────────────────────┐  │
│  │  AddressBook  │  │   Connection   │  │     Stream Manager         │  │
│  │    (JSON)     │  │    Manager     │  │  (Encrypted/Unencrypted)   │  │
│  │  - CRUD ops   │  │  - State FSM   │  │  - Lazy stream opening     │  │
│  │  - Blacklist  │  │  - Reconnect   │  │  - ChaCha20-Poly1305       │  │
│  │  - Persist    │  │  - Timeout     │  │  - Cramberry framing       │  │
│  └───────────────┘  └────────────────┘  └────────────────────────────┘  │
│  ┌───────────────┐  ┌────────────────┐  ┌────────────────────────────┐  │
│  │    Crypto     │  │   Connection   │  │    Event Dispatcher        │  │
│  │    Module     │  │     Gater      │  │  (Non-blocking events)     │  │
│  │  - Ed25519    │  │  - Blacklist   │  │                            │  │
│  │  - X25519     │  │  - Dedup       │  │                            │  │
│  │  - HKDF       │  │                │  │                            │  │
│  └───────────────┘  └────────────────┘  └────────────────────────────┘  │
├─────────────────────────────────────────────────────────────────────────┤
│                            libp2p                                        │
│         NAT Traversal • Hole Punching • Relay • Multiplexing             │
└─────────────────────────────────────────────────────────────────────────┘
```

### Core Data Types

**IncomingMessage** - Received on `Messages()` channel:
```go
type IncomingMessage struct {
    PeerID     peer.ID   // Sender's peer ID
    StreamName string    // Stream name (e.g., "handshake", "messages")
    Data       []byte    // Message payload (decrypted if encrypted stream)
    Timestamp  time.Time // When message was received
}
```

**ConnectionEvent** - Received on `Events()` channel:
```go
type ConnectionEvent struct {
    PeerID    peer.ID         // Peer this event relates to
    State     ConnectionState // New state
    Error     error           // Non-nil for error conditions
    Timestamp time.Time       // When event occurred
}
```

**PeerEntry** - Peer information from address book:
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

### Connection Flow

```
┌──────────────────────────────────────────────────────────────────────────┐
│                       Connection Lifecycle                                │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  1. AddPeer() or Connect()     ──►  StateConnecting                      │
│                                          │                               │
│  2. libp2p connection               ◄────┘                               │
│     established                      ──►  StateConnected                 │
│                                          │  • Handshake stream ready     │
│                                          │  • Timeout starts             │
│  3. App receives StateConnected event    │                               │
│     ──► Sends Hello message         ◄────┘                               │
│                                                                          │
│  4. Receive Hello ──► Send PubKey                                        │
│                                                                          │
│  5. Receive PubKey ──► PrepareStreams() + Send Complete                  │
│     • Derives shared encryption key (X25519 ECDH + HKDF)                 │
│     • Registers handlers for encrypted streams                           │
│     • Node ready to receive encrypted messages                           │
│                                                                          │
│  6. Receive Complete ──► FinalizeHandshake()                             │
│     • Cancels handshake timeout                                          │
│     • Closes handshake stream                                            │
│                                      ──►  StateEstablished               │
│                                          │  • Encrypted streams active   │
│                                          │  • Streams open lazily        │
│  7. Send()/Messages() on encrypted streams ◄─┘                           │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

**Failure Scenarios:**
- **Handshake timeout** → `StateCooldown` → reconnection with backoff
- **Connection lost** → `StateDisconnected` → automatic reconnection
- **Peer blacklisted** → immediate disconnect, no reconnection

### Design Philosophy

**1. Library, Not Application**
Glueberry is a building block. Peer discovery and handshake protocols are your responsibility. This gives you full control over authentication, versioning, and peer selection.

**2. Symmetric P2P API**
Both peers use identical code paths. There's no "server" or "client" role - both sides react to messages and events the same way.

**3. Two-Phase Handshake**
The `PrepareStreams()` + `FinalizeHandshake()` pattern prevents race conditions where one peer sends encrypted messages before the other is ready to decrypt.

**4. Lazy Stream Opening**
Streams are created on first use (`Send()`), not during `PrepareStreams()`. This reduces resource usage and allows either peer to initiate communication.

**5. Non-Blocking Events**
Event channels use non-blocking sends. Slow consumers don't block connection operations. Events may be dropped if the channel is full.

**6. Security by Default**
All post-handshake communication is encrypted. Keys are never logged. Input validation on all network data.

### Encryption

- **Identity:** Ed25519 (app-provided)
- **Key Exchange:** X25519 ECDH
- **Symmetric:** ChaCha20-Poly1305 AEAD
- **Message Framing:** Cramberry binary format

## API Reference

### Node Methods

**Lifecycle:**
| Method | Signature | Description |
|--------|-----------|-------------|
| `New` | `New(cfg *Config) (*Node, error)` | Create a new node (not started) |
| `Start` | `Start() error` | Start listening and auto-connect to peers |
| `Stop` | `Stop() error` | Graceful shutdown and cleanup |
| `Version` | `Version() ProtocolVersion` | Get protocol version |

**Information:**
| Method | Signature | Description |
|--------|-----------|-------------|
| `PeerID` | `PeerID() peer.ID` | Get local peer ID |
| `PublicKey` | `PublicKey() ed25519.PublicKey` | Get local Ed25519 public key |
| `Addrs` | `Addrs() []multiaddr.Multiaddr` | Get listen addresses |

**Address Book:**
| Method | Signature | Description |
|--------|-----------|-------------|
| `AddPeer` | `AddPeer(peerID, addrs, metadata) error` | Add/update peer (auto-connects if started) |
| `RemovePeer` | `RemovePeer(peerID) error` | Remove peer from address book |
| `GetPeer` | `GetPeer(peerID) (*PeerEntry, error)` | Retrieve peer information |
| `ListPeers` | `ListPeers() []*PeerEntry` | List all non-blacklisted peers |
| `PeerAddrs` | `PeerAddrs(peerID) []multiaddr.Multiaddr` | Get addresses from peerstore |
| `BlacklistPeer` | `BlacklistPeer(peerID) error` | Blacklist peer and disconnect |
| `UnblacklistPeer` | `UnblacklistPeer(peerID) error` | Remove from blacklist |

**Connections (with context support):**
| Method | Signature | Description |
|--------|-----------|-------------|
| `Connect` | `Connect(peerID) error` | Connect to a peer |
| `ConnectCtx` | `ConnectCtx(ctx, peerID) error` | Connect with context (cancellation/timeout) |
| `Disconnect` | `Disconnect(peerID) error` | Close connection to peer |
| `DisconnectCtx` | `DisconnectCtx(ctx, peerID) error` | Disconnect with context |
| `ConnectionState` | `ConnectionState(peerID) ConnectionState` | Query connection state |
| `CancelReconnection` | `CancelReconnection(peerID) error` | Stop reconnection attempts |
| `IsOutbound` | `IsOutbound(peerID) (bool, error)` | Check if we initiated the connection |

**Handshake (Two-Phase API):**
| Method | Signature | Description |
|--------|-----------|-------------|
| `PrepareStreams` | `PrepareStreams(peerID, pubKey, streamNames) error` | Derive shared key, register handlers |
| `FinalizeHandshake` | `FinalizeHandshake(peerID) error` | Complete handshake, transition to Established |
| `CompleteHandshake` | `CompleteHandshake(peerID, pubKey, streamNames) error` | Convenience: PrepareStreams + FinalizeHandshake |

**Messaging (with context support):**
| Method | Signature | Description |
|--------|-----------|-------------|
| `Send` | `Send(peerID, streamName, data) error` | Send data on a stream |
| `SendCtx` | `SendCtx(ctx, peerID, streamName, data) error` | Send with context (respects backpressure) |
| `Messages` | `Messages() <-chan IncomingMessage` | Receive channel for messages |

**Events (with filtering):**
| Method | Signature | Description |
|--------|-----------|-------------|
| `Events` | `Events() <-chan ConnectionEvent` | All connection events |
| `FilteredEvents` | `FilteredEvents(filter) <-chan ConnectionEvent` | Events matching filter |
| `EventsForPeer` | `EventsForPeer(peerID) <-chan ConnectionEvent` | Events for specific peer |
| `EventsForStates` | `EventsForStates(states...) <-chan ConnectionEvent` | Events for specific states |

**Statistics:**
| Method | Signature | Description |
|--------|-----------|-------------|
| `PeerStatistics` | `PeerStatistics(peerID) *PeerStats` | Get stats for a peer |
| `AllPeerStatistics` | `AllPeerStatistics() map[peer.ID]*PeerStats` | Get all peer stats |

**Debug:**
| Method | Signature | Description |
|--------|-----------|-------------|
| `DumpState` | `DumpState() *DebugState` | Get complete node state |
| `DumpStateJSON` | `DumpStateJSON() (string, error)` | State as formatted JSON |
| `DumpStateString` | `DumpStateString() string` | Human-readable summary |
| `ConnectionSummary` | `ConnectionSummary() map[ConnectionState]int` | Counts by state |
| `ListKnownPeers` | `ListKnownPeers() []peer.ID` | List all known peers |
| `PeerInfo` | `PeerInfo(peerID) (*DebugPeerInfo, error)` | Detailed peer info |

## Connection States

| State | Description |
|-------|-------------|
| `StateDisconnected` | No connection |
| `StateConnecting` | Connection in progress |
| `StateConnected` | libp2p connected, handshake stream ready |
| `StateEstablished` | Encrypted streams active |
| `StateReconnecting` | Attempting reconnection |
| `StateCooldown` | Waiting after failed handshake |

## Error Handling

Glueberry uses sentinel errors for programmatic error handling:

**Peer/Address Book Errors:**
- `ErrPeerNotFound` - Peer not in address book
- `ErrPeerBlacklisted` - Peer is blacklisted
- `ErrPeerAlreadyExists` - Peer already exists

**Connection Errors:**
- `ErrNotConnected` - No active connection to peer
- `ErrAlreadyConnected` - Already connected to peer
- `ErrConnectionFailed` - Connection attempt failed
- `ErrHandshakeTimeout` - Handshake didn't complete in time
- `ErrHandshakeNotComplete` - Encrypted streams requested before handshake
- `ErrReconnectionCancelled` - Reconnection cancelled by app
- `ErrInCooldown` - Peer in cooldown after failed handshake

**Stream Errors:**
- `ErrStreamNotFound` - Stream doesn't exist
- `ErrStreamClosed` - Stream has been closed
- `ErrStreamsAlreadyEstablished` - Encrypted streams already set up
- `ErrNoStreamsRequested` - No stream names provided

**Crypto Errors:**
- `ErrInvalidPublicKey` - Invalid public key
- `ErrEncryptionFailed` - Encryption failed
- `ErrDecryptionFailed` - Decryption failed (tampering or wrong key)

**Node Errors:**
- `ErrNodeNotStarted` - Node hasn't been started
- `ErrNodeAlreadyStarted` - Node already running

```go
// Example error handling
if err := node.Connect(peerID); err != nil {
    if errors.Is(err, glueberry.ErrPeerBlacklisted) {
        log.Printf("Cannot connect: peer is blacklisted")
    } else if errors.Is(err, glueberry.ErrInCooldown) {
        log.Printf("Cannot connect: peer in cooldown")
    }
}
```

## Observability

### Logger Interface

Glueberry accepts a pluggable logger interface compatible with slog, zap, and zerolog:

```go
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Info(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
    Error(msg string, keysAndValues ...any)
}

// Use with slog
cfg := glueberry.NewConfig(
    privateKey, addressBookPath, listenAddrs,
    glueberry.WithLogger(slog.Default()),
)
```

### Metrics Interface

Prometheus-compatible metrics collection:

```go
type Metrics interface {
    ConnectionOpened(direction string)
    ConnectionClosed(direction string)
    HandshakeDuration(seconds float64)
    MessageSent(stream string, bytes int)
    MessageReceived(stream string, bytes int)
    EncryptionError()
    DecryptionError()
    EventDropped()
    BackpressureEngaged(stream string)
    // ... more methods
}

// Implement for Prometheus
type PrometheusMetrics struct {
    connectionsTotal *prometheus.CounterVec
    // ...
}
```

## Security Considerations

**Key Security:**
- Private keys never logged or exposed in error messages
- Shared secrets derived via ECDH, stored only in crypto module
- Keys deep-copied when accessed to prevent external modification
- Automatic key material zeroing after use

**Message Security:**
- ChaCha20-Poly1305 AEAD provides authenticated encryption
- Poly1305 MAC prevents message tampering
- Random 96-bit nonces ensure uniqueness (~2^48 messages before collision risk)
- Configurable maximum message size (default 1MB)

**Network Security:**
- Handshake timeout prevents resource exhaustion attacks
- Blacklisting enforced at multiple layers (gater, manager, protocol handlers)
- All network data treated as untrusted and validated
- Low-order point attack protection in ECDH
- Decryption error callback for tampering detection

**Thread Safety:**
- All public `Node` methods are thread-safe
- Internal components use appropriate synchronization (RWMutex, channels)
- Event and message channels safe for concurrent reads (single consumer recommended)
- Verified with race detector and stress tests

## Testing

### Running Tests

```bash
# Run all unit tests
make test

# Run with race detector
make test-race

# Run integration tests
make test-integration

# Run tests with coverage
make coverage

# Run linter
make lint

# Build
make build
```

### Fuzz Testing

Security-critical parsers have fuzz tests:

```bash
# Fuzz the cipher
go test -fuzz=FuzzDecrypt -fuzztime=30s ./fuzz/

# Fuzz message parsing
go test -fuzz=FuzzMessageIterator -fuzztime=30s ./fuzz/
```

### MockNode for Application Testing

A MockNode is provided for testing applications without network:

```go
import "github.com/blockberries/glueberry/pkg/testing"

func TestMyApp(t *testing.T) {
    mock := testing.NewMockNode()

    // Simulate events
    mock.EmitEvent(glueberry.ConnectionEvent{
        PeerID: peerID,
        State:  glueberry.StateConnected,
    })

    // Simulate messages
    mock.SimulateMessage(peerID, "stream", []byte("hello"))

    // Verify sends
    sent := mock.SentMessages()
    assert.Len(t, sent, 1)
}
```

## Documentation

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - Detailed architecture, component design, and API reference
- **[SECURITY_REVIEW.md](SECURITY_REVIEW.md)** - Security audit findings and cryptographic analysis
- **[PRERELEASE_IMPLEMENTATION_PLAN.md](PRERELEASE_IMPLEMENTATION_PLAN.md)** - Implementation roadmap
- **[PRERELEASE_PROGRESS_REPORT.md](PRERELEASE_PROGRESS_REPORT.md)** - Development progress and status

## License

This project is part of the Blockberries ecosystem.

## Contributing

Glueberry is actively developed. Contributions are welcome!

## Acknowledgments

Built with:
- [libp2p](https://libp2p.io/) - Modular P2P networking
- [Cramberry](https://github.com/blockberries/cramberry) - High-performance binary serialization
- [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) - Cryptographic primitives
