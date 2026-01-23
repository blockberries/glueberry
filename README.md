# Glueberry

**Glueberry** is a Go library for secure P2P communications built on [libp2p](https://libp2p.io/). It provides encrypted, multiplexed streams between peers with application-controlled handshaking and automatic reconnection.

## Features

- 🔐 **End-to-End Encryption** - ChaCha20-Poly1305 authenticated encryption with X25519 ECDH key exchange
- 🤝 **App-Controlled Handshaking** - Flexible handshake protocol defined by your application
- 🔄 **Automatic Reconnection** - Exponential backoff with jitter and configurable retry limits
- 📡 **Stream Multiplexing** - Multiple named encrypted streams per peer connection
- 🔌 **Bidirectional** - Both peers can initiate connections and streams
- 🚫 **Blacklist Support** - Peer blacklisting with connection-level enforcement
- 📊 **Event System** - Non-blocking connection state change notifications
- 🧵 **Thread-Safe** - All public APIs safe for concurrent use
- 🕳️ **NAT Traversal** - Built-in hole punching via libp2p
- 💾 **Persistent Address Book** - JSON-based peer storage

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
    "github.com/multiformats/go-multiaddr"
)

func main() {
    // Generate identity key
    _, privateKey, _ := ed25519.GenerateKey(rand.Reader)

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

    // Add a peer and connect
    // peerID, peerAddrs := ... // from somewhere
    // node.AddPeer(peerID, peerAddrs, nil)
    // hs, _ := node.Connect(peerID)

    // Perform handshake and establish encrypted streams
    // ... your handshake logic ...
    // node.EstablishEncryptedStreams(peerID, peerPublicKey, []string{"messages"})

    // Send/receive messages
    // node.Send(peerID, "messages", data)
    // msg := <-node.Messages()
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

### 2. Connect to a Peer (Outgoing)

```go
// Add peer to address book
peerAddr, _ := multiaddr.NewMultiaddr("/ip4/192.168.1.100/tcp/9000")
node.AddPeer(peerID, []multiaddr.Multiaddr{peerAddr}, map[string]string{
    "name": "validator-1",
})

// Connect and get handshake stream
hs, err := node.Connect(peerID)
if err != nil {
    // Handle error
}

// Perform your application's handshake protocol
type HandshakeMsg struct {
    NodeID    string `cramberry:"1,required"`
    PublicKey []byte `cramberry:"2,required"`
}

// Send handshake
hs.Send(&HandshakeMsg{NodeID: "my-node", PublicKey: myPubKey})

// Receive response
var response HandshakeMsg
hs.Receive(&response)

// Establish encrypted streams after successful handshake
streamNames := []string{"blocks", "transactions", "consensus"}
node.EstablishEncryptedStreams(peerID, peerPublicKey, streamNames)
```

### 3. Accept Incoming Connections

```go
// Listen for incoming handshakes
go func() {
    for incoming := range node.IncomingHandshakes() {
        go handleIncomingHandshake(node, incoming)
    }
}()

func handleIncomingHandshake(node *glueberry.Node, incoming protocol.IncomingHandshake) {
    // Perform handshake
    var msg HandshakeMsg
    if err := incoming.HandshakeStream.Receive(&msg); err != nil {
        incoming.HandshakeStream.Close()
        return
    }

    // Send response
    incoming.HandshakeStream.Send(&HandshakeMsg{...})

    // Establish encrypted streams
    streamNames := []string{"blocks", "transactions"}
    node.EstablishEncryptedStreams(incoming.PeerID, remotePubKey, streamNames)
}
```

### 4. Send and Receive Messages

```go
// Send encrypted message
data := []byte("hello, peer!")
if err := node.Send(peerID, "messages", data); err != nil {
    // Handle error
}

// Receive messages
go func() {
    for msg := range node.Messages() {
        fmt.Printf("Received from %s on stream %s: %s\n",
            msg.PeerID, msg.StreamName, msg.Data)

        // Process the message
        handleMessage(msg)
    }
}()
```

### 5. Monitor Connection Events

```go
go func() {
    for event := range node.Events() {
        fmt.Printf("Peer %s: %s\n", event.PeerID, event.State)

        if event.IsError() {
            fmt.Printf("  Error: %v\n", event.Error)
        }

        switch event.State {
        case glueberry.StateEstablished:
            fmt.Println("  Connection fully established!")
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
┌─────────────────────────────────────────────────────────────────┐
│                        Application                               │
├─────────────────────────────────────────────────────────────────┤
│                      Glueberry Node                              │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │ AddressBook │  │  Connection │  │    Stream Manager       │  │
│  │  (JSON)     │  │   Manager   │  │  (Encrypted Streams)    │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐  │
│  │   Crypto    │  │  Handshake  │  │    Event Dispatcher     │  │
│  │   Module    │  │   Handler   │  │                         │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘  │
├─────────────────────────────────────────────────────────────────┤
│                         libp2p                                   │
└─────────────────────────────────────────────────────────────────┘
```

### Connection Flow

1. **App adds peer** → Address book stores multiaddrs
2. **App calls Connect(peerID)** → Returns HandshakeStream
3. **App performs handshake** → Exchanges messages via HandshakeStream
4. **App calls EstablishEncryptedStreams()** → Opens encrypted multiplexed streams
5. **App sends/receives** → Messages encrypted transparently

### Encryption

- **Identity:** Ed25519 (app-provided)
- **Key Exchange:** X25519 ECDH
- **Symmetric:** ChaCha20-Poly1305 AEAD
- **Message Framing:** Cramberry binary format

## API Reference

### Node Methods

**Lifecycle:**
- `New(cfg) (*Node, error)` - Create node
- `Start() error` - Start listening
- `Stop() error` - Graceful shutdown

**Information:**
- `PeerID() peer.ID` - Local peer ID
- `PublicKey() ed25519.PublicKey` - Local public key
- `Addrs() []multiaddr.Multiaddr` - Listen addresses

**Address Book:**
- `AddPeer(peerID, addrs, metadata)` - Add/update peer
- `RemovePeer(peerID)` - Remove peer
- `GetPeer(peerID)` - Get peer info
- `ListPeers()` - List non-blacklisted peers
- `BlacklistPeer(peerID)` - Blacklist and disconnect
- `UnblacklistPeer(peerID)` - Remove from blacklist

**Connections:**
- `Connect(peerID) (*HandshakeStream, error)` - Connect and get handshake stream
- `Disconnect(peerID)` - Close connection
- `ConnectionState(peerID)` - Query state
- `CancelReconnection(peerID)` - Stop reconnection

**Messaging:**
- `EstablishEncryptedStreams(peerID, pubKey, streamNames)` - Setup encrypted streams
- `Send(peerID, streamName, data)` - Send encrypted message
- `Messages() <-chan IncomingMessage` - Receive messages
- `Events() <-chan ConnectionEvent` - Connection events
- `IncomingHandshakes() <-chan IncomingHandshake` - Incoming connections

## Connection States

| State | Description |
|-------|-------------|
| `StateDisconnected` | No connection |
| `StateConnecting` | Connection in progress |
| `StateConnected` | libp2p connected, awaiting handshake |
| `StateHandshaking` | Handshake in progress |
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
