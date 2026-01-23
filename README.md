# Glueberry

**Glueberry** is a Go library for secure P2P communications built on [libp2p](https://libp2p.io/). It provides encrypted, multiplexed streams between peers with application-controlled handshaking and automatic reconnection.

## Features

- End-to-End Encryption - ChaCha20-Poly1305 authenticated encryption with X25519 ECDH key exchange
- Symmetric Event-Driven API - Same code handles both initiator and responder
- App-Controlled Handshaking - Flexible handshake protocol defined by your application
- Automatic Reconnection - Exponential backoff with jitter and configurable retry limits
- Stream Multiplexing - Multiple named encrypted streams per peer connection
- Bidirectional - Both peers can initiate connections and streams
- Blacklist Support - Peer blacklisting with connection-level enforcement
- Event System - Non-blocking connection state change notifications
- Thread-Safe - All public APIs safe for concurrent use
- NAT Traversal - Built-in hole punching via libp2p
- Persistent Address Book - JSON-based peer storage

## Installation

```bash
go get github.com/blockberries/glueberry
```

**Requirements:**
- Go 1.25.6 or later
- [Cramberry](https://github.com/blockberries/cramberry) for message serialization

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

    // Optional (with defaults)
    HandshakeTimeout        time.Duration // Default: 30s
    ReconnectBaseDelay      time.Duration // Default: 1s
    ReconnectMaxDelay       time.Duration // Default: 5m
    ReconnectMaxAttempts    int           // Default: 10 (0 = unlimited)
    FailedHandshakeCooldown time.Duration // Default: 1m
    EventBufferSize         int           // Default: 100
    MessageBufferSize       int           // Default: 1000
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
)
```

## Architecture

```
+-------------------------------------------------------------------+
|                        Application                                 |
+-------------------------------------------------------------------+
|                      Glueberry Node                                |
|  +-------------+  +-------------+  +---------------------------+   |
|  | AddressBook |  |  Connection |  |    Stream Manager         |   |
|  |  (JSON)     |  |   Manager   |  | (Encrypted/Unencrypted)   |   |
|  +-------------+  +-------------+  +---------------------------+   |
|  +-------------+  +-------------+  +---------------------------+   |
|  |   Crypto    |  |  Protocol   |  |    Event Dispatcher       |   |
|  |   Module    |  |   Handlers  |  |                           |   |
|  +-------------+  +-------------+  +---------------------------+   |
+-------------------------------------------------------------------+
|                         libp2p                                     |
+-------------------------------------------------------------------+
```

### Connection Flow

1. **App adds peer** - Address book stores multiaddrs, auto-connects if started
2. **StateConnected event** - libp2p connected, handshake stream ready, timeout starts
3. **App sends Hello** - React to StateConnected by sending Hello message
4. **Receive Hello → Send PubKey** - On receiving Hello, respond with public key
5. **Receive PubKey → PrepareStreams() + Send Complete** - Prepare streams, signal readiness
6. **Receive Complete → FinalizeHandshake()** - When peer confirms ready, finalize
7. **StateEstablished** - Encrypted streams active, timeout cancelled
8. **App sends/receives** - Messages encrypted transparently

### Encryption

- **Identity:** Ed25519 (app-provided)
- **Key Exchange:** X25519 ECDH
- **Symmetric:** ChaCha20-Poly1305 AEAD
- **Message Framing:** Cramberry binary format

## API Reference

### Node Methods

**Lifecycle:**
- `New(cfg) (*Node, error)` - Create node
- `Start() error` - Start listening and auto-connect to peers
- `Stop() error` - Graceful shutdown

**Information:**
- `PeerID() peer.ID` - Local peer ID
- `PublicKey() ed25519.PublicKey` - Local public key
- `Addrs() []multiaddr.Multiaddr` - Listen addresses

**Address Book:**
- `AddPeer(peerID, addrs, metadata)` - Add/update peer (auto-connects if started)
- `RemovePeer(peerID)` - Remove peer
- `GetPeer(peerID)` - Get peer info
- `ListPeers()` - List non-blacklisted peers
- `BlacklistPeer(peerID)` - Blacklist and disconnect
- `UnblacklistPeer(peerID)` - Remove from blacklist

**Connections:**
- `Connect(peerID) error` - Explicitly connect to peer
- `Disconnect(peerID)` - Close connection
- `ConnectionState(peerID)` - Query state
- `CancelReconnection(peerID)` - Stop reconnection

**Handshake (Two-Phase):**
- `PrepareStreams(peerID, pubKey, streamNames)` - Derive shared key, register stream handlers (call on receiving PubKey)
- `FinalizeHandshake(peerID)` - Cancel timeout, transition to StateEstablished (call on receiving Complete)
- `CompleteHandshake(peerID, pubKey, streamNames)` - Convenience: calls PrepareStreams + FinalizeHandshake

**Messaging:**
- `Send(peerID, streamName, data)` - Send message (handshake or encrypted)
- `Messages() <-chan IncomingMessage` - Receive all messages
- `Events() <-chan ConnectionEvent` - Connection events

## Connection States

| State | Description |
|-------|-------------|
| `StateDisconnected` | No connection |
| `StateConnecting` | Connection in progress |
| `StateConnected` | libp2p connected, handshake stream ready |
| `StateEstablished` | Encrypted streams active |
| `StateReconnecting` | Attempting reconnection |
| `StateCooldown` | Waiting after failed handshake |

## Security Considerations

- **Private Keys:** Never logged or exposed in errors
- **Shared Secrets:** Derived via ECDH, stored securely in crypto module
- **Message Authentication:** Poly1305 MAC prevents tampering
- **Handshake Timeout:** Prevents resource exhaustion attacks
- **Blacklisting:** Multi-layer enforcement (address book, connection gater, protocol handlers)
- **Input Validation:** All network data treated as untrusted

## Testing

```bash
# Run all tests
make test

# Run tests with coverage
make coverage

# Run linter
make lint

# Build
make build
```

## Documentation

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - Detailed architecture and design
- **[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)** - Implementation roadmap
- **[PROGRESS_REPORT.md](PROGRESS_REPORT.md)** - Development progress and status

## License

This project is part of the Blockberries ecosystem.

## Contributing

Glueberry is actively developed. Contributions are welcome!

## Acknowledgments

Built with:
- [libp2p](https://libp2p.io/) - Modular P2P networking
- [Cramberry](https://github.com/blockberries/cramberry) - High-performance binary serialization
- [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) - Cryptographic primitives
