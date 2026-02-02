# Glueberry

<p align="center">
  <strong>Secure P2P Communication Library for Go</strong>
</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/blockberries/glueberry"><img src="https://pkg.go.dev/badge/github.com/blockberries/glueberry.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/blockberries/glueberry"><img src="https://goreportcard.com/badge/github.com/blockberries/glueberry" alt="Go Report Card"></a>
  <a href="https://github.com/blockberries/glueberry/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg" alt="License"></a>
  <a href="https://github.com/blockberries/glueberry/actions"><img src="https://github.com/blockberries/glueberry/workflows/CI/badge.svg" alt="Build Status"></a>
</p>

---

## Overview

**Glueberry** is a production-ready Go library for peer-to-peer communications built on [libp2p](https://libp2p.io/). It provides encrypted, multiplexed streams between peers with app-controlled handshaking and automatic reconnection.

### Key Features

✨ **End-to-End Encryption** – ChaCha20-Poly1305 AEAD with X25519 ECDH key exchange
🔄 **Automatic Reconnection** – Exponential backoff with configurable retry policies
🎯 **Multiple Named Streams** – Per-peer stream multiplexing (e.g., "messages", "consensus")
⚡ **Event-Driven API** – Non-blocking connection state notifications via channels
🔒 **Peer Blacklisting** – Connection-level enforcement via libp2p connection gater
🌐 **NAT Traversal** – Built-in hole punching and relay support via libp2p
📊 **Observability** – Prometheus metrics and OpenTelemetry tracing support
🧵 **Thread-Safe** – All public APIs safe for concurrent use

---

## Table of Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Usage](#usage)
  - [Creating a Node](#creating-a-node)
  - [Connection Management](#connection-management)
  - [Two-Phase Handshake](#two-phase-handshake)
  - [Sending and Receiving Messages](#sending-and-receiving-messages)
  - [Event Handling](#event-handling)
- [Documentation](#documentation)
- [Examples](#examples)
- [API Reference](#api-reference)
- [Configuration](#configuration)
- [Security](#security)
- [Performance](#performance)
- [Testing](#testing)
- [Contributing](#contributing)
- [License](#license)

---

## Installation

### Prerequisites

- **Go 1.25+** (recommended: 1.25.6)
- **libp2p dependencies** (automatically managed by `go mod`)

### Install via `go get`

```bash
go get github.com/blockberries/glueberry@latest
```

### Verify Installation

```go
package main

import (
    "fmt"
    "github.com/blockberries/glueberry"
)

func main() {
    fmt.Println("Glueberry version:", glueberry.CurrentVersion())
}
```

---

## Quick Start

Here's a minimal example to get you started:

```go
package main

import (
    "crypto/ed25519"
    "crypto/rand"
    "fmt"
    "log"

    "github.com/blockberries/glueberry"
    "github.com/blockberries/glueberry/pkg/streams"
    "github.com/multiformats/go-multiaddr"
)

func main() {
    // Generate identity key
    _, privateKey, _ := ed25519.GenerateKey(rand.Reader)

    // Configure listen address
    listenAddr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9000")

    // Create node configuration
    cfg := glueberry.NewConfig(
        privateKey,
        "./addressbook.json",
        []multiaddr.Multiaddr{listenAddr},
    )

    // Create and start node
    node, err := glueberry.New(cfg)
    if err != nil {
        log.Fatalf("Failed to create node: %v", err)
    }

    if err := node.Start(); err != nil {
        log.Fatalf("Failed to start node: %v", err)
    }
    defer node.Stop()

    fmt.Printf("Node started!\n")
    fmt.Printf("  Peer ID: %s\n", node.PeerID())
    fmt.Printf("  Listening on: %v\n", node.Addrs())

    // Handle connection events
    go func() {
        for event := range node.Events() {
            fmt.Printf("Event: %s -> %s\n", event.PeerID.String()[:8], event.State)
        }
    }()

    // Handle incoming messages
    go func() {
        for msg := range node.Messages() {
            fmt.Printf("Message from %s: %s\n", msg.PeerID.String()[:8], msg.Data)
        }
    }()

    // Keep running
    select {}
}
```

---

## Usage

### Creating a Node

#### Basic Configuration

```go
import (
    "crypto/ed25519"
    "crypto/rand"
    "github.com/blockberries/glueberry"
    "github.com/multiformats/go-multiaddr"
)

// Generate identity key
_, privateKey, _ := ed25519.GenerateKey(rand.Reader)

// Configure listen addresses
listenAddr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9000")

// Create configuration
cfg := glueberry.NewConfig(
    privateKey,
    "./addressbook.json",
    []multiaddr.Multiaddr{listenAddr},
)

// Create node
node, err := glueberry.New(cfg)
if err != nil {
    // Handle error
}

// Start listening
if err := node.Start(); err != nil {
    // Handle error
}
defer node.Stop()
```

#### Advanced Configuration

```go
import (
    "log/slog"
    "time"
)

cfg := glueberry.NewConfig(
    privateKey,
    "./addressbook.json",
    []multiaddr.Multiaddr{listenAddr},
    // Optional configuration
    glueberry.WithLogger(slog.Default()),
    glueberry.WithHandshakeTimeout(120*time.Second),
    glueberry.WithMaxMessageSize(50*1024*1024), // 50MB
    glueberry.WithReconnectMaxAttempts(20),
)
```

### Connection Management

#### Adding a Peer

```go
import (
    "github.com/libp2p/go-libp2p/core/peer"
    "github.com/multiformats/go-multiaddr"
)

// Parse peer address
peerAddr, _ := multiaddr.NewMultiaddr("/ip4/192.168.1.100/tcp/9001/p2p/12D3KooW...")
peerInfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)

// Add to address book (auto-connects when node is started)
node.AddPeer(peerInfo.ID, peerInfo.Addrs, nil)
```

#### Explicit Connection

```go
// Connect to peer
if err := node.Connect(peerID); err != nil {
    log.Printf("Connect failed: %v", err)
}

// Check connection state
state := node.ConnectionState(peerID)
fmt.Printf("Connection state: %v\n", state)
```

#### Disconnection

```go
// Disconnect from peer
if err := node.Disconnect(peerID); err != nil {
    log.Printf("Disconnect failed: %v", err)
}

// Cancel automatic reconnection attempts
if err := node.CancelReconnection(peerID); err != nil {
    log.Printf("Cancel reconnection failed: %v", err)
}
```

#### Blacklisting

```go
// Blacklist a peer (blocks all connections)
if err := node.BlacklistPeer(peerID); err != nil {
    log.Printf("Blacklist failed: %v", err)
}

// Remove from blacklist
if err := node.UnblacklistPeer(peerID); err != nil {
    log.Printf("Unblacklist failed: %v", err)
}
```

### Two-Phase Handshake

Glueberry uses a **symmetric, two-phase handshake** to prevent race conditions where encrypted streams become active before both sides are ready.

#### Handshake Flow

```
1. StateConnected event fires (handshake stream ready)
2. App sends Hello message
3. App receives PubKey from peer
4. App calls PrepareStreams() [Phase 1: derive key, ready to receive]
5. App sends Complete message
6. App receives Complete from peer
7. App calls FinalizeHandshake() [Phase 2: activate encrypted streams]
8. StateEstablished event fires (encrypted streams active)
```

#### Example Implementation

```go
import (
    "crypto/ed25519"
    "github.com/blockberries/glueberry/pkg/streams"
)

// Handshake message types
const (
    msgHello    byte = 1
    msgPubKey   byte = 2
    msgComplete byte = 3
)

publicKey := privateKey.Public().(ed25519.PublicKey)

// Track handshake state per peer
type peerState struct {
    gotHello        bool
    gotPubKey       bool
    sentComplete    bool
    gotComplete     bool
    peerPubKey      ed25519.PublicKey
    streamsPrepared bool
}
peerStates := make(map[string]*peerState)

// Handle connection events
go func() {
    for event := range node.Events() {
        if event.State == glueberry.StateConnected {
            // Start handshake: send Hello
            _ = node.Send(event.PeerID, streams.HandshakeStreamName, []byte{msgHello})
        }
    }
}()

// Handle handshake messages
go func() {
    for msg := range node.Messages() {
        if msg.StreamName != streams.HandshakeStreamName {
            continue // Skip non-handshake messages
        }

        peerID := msg.PeerID.String()
        state, ok := peerStates[peerID]
        if !ok {
            state = &peerState{}
            peerStates[peerID] = state
        }

        switch msg.Data[0] {
        case msgHello:
            state.gotHello = true
            // Send Hello back
            _ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgHello})
            // Send our public key
            pubKeyMsg := append([]byte{msgPubKey}, publicKey...)
            _ = node.Send(msg.PeerID, streams.HandshakeStreamName, pubKeyMsg)

        case msgPubKey:
            state.peerPubKey = ed25519.PublicKey(msg.Data[1:])
            state.gotPubKey = true
            // Phase 1: Prepare streams (derive key, ready to receive)
            if err := node.PrepareStreams(msg.PeerID, state.peerPubKey, []string{"messages"}); err != nil {
                log.Printf("PrepareStreams failed: %v", err)
                continue
            }
            state.streamsPrepared = true
            // Send Complete
            _ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgComplete})
            state.sentComplete = true

        case msgComplete:
            state.gotComplete = true
        }

        // Phase 2: Finalize when both sides are ready
        if state.streamsPrepared && state.sentComplete && state.gotComplete {
            if err := node.FinalizeHandshake(msg.PeerID); err != nil {
                log.Printf("FinalizeHandshake failed: %v", err)
                continue
            }
            fmt.Printf("Handshake complete with %s\n", peerID[:8])
            delete(peerStates, peerID) // Cleanup
        }
    }
}()
```

### Sending and Receiving Messages

#### Sending Messages

```go
// Send encrypted message (after handshake complete)
data := []byte("Hello, peer!")
if err := node.Send(peerID, "messages", data); err != nil {
    log.Printf("Send failed: %v", err)
}

// Send with context (timeout)
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := node.SendCtx(ctx, peerID, "messages", data); err != nil {
    log.Printf("Send failed: %v", err)
}
```

#### Receiving Messages

```go
for msg := range node.Messages() {
    switch msg.StreamName {
    case streams.HandshakeStreamName:
        // Handle handshake messages
        handleHandshake(msg)
    case "messages":
        // Handle application messages
        fmt.Printf("Message from %s: %s\n", msg.PeerID, msg.Data)
    case "consensus":
        // Handle consensus messages
        handleConsensus(msg)
    default:
        log.Printf("Unknown stream: %s", msg.StreamName)
    }
}
```

### Event Handling

#### Basic Event Loop

```go
for event := range node.Events() {
    switch event.State {
    case glueberry.StateConnecting:
        fmt.Printf("Connecting to %s\n", event.PeerID)
    case glueberry.StateConnected:
        fmt.Printf("Connected to %s (handshake stream ready)\n", event.PeerID)
        // Start handshake
    case glueberry.StateEstablished:
        fmt.Printf("Connection established with %s (encrypted streams active)\n", event.PeerID)
    case glueberry.StateDisconnected:
        fmt.Printf("Disconnected from %s\n", event.PeerID)
        if event.IsError() {
            fmt.Printf("  Error: %v\n", event.Error)
        }
    case glueberry.StateReconnecting:
        fmt.Printf("Reconnecting to %s\n", event.PeerID)
    }
}
```

#### Event Subscriptions (Multiple Consumers)

```go
// Create subscription
sub := node.SubscribeEvents()
defer sub.Unsubscribe()

go func() {
    for evt := range sub.Events() {
        fmt.Printf("Subscription received: %v\n", evt)
    }
}()
```

#### Filtered Events

```go
// Only established events
sub := node.EventsForStates(glueberry.StateEstablished)
defer sub.Unsubscribe()

// Only events for specific peer
sub := node.EventsForPeer(peerID)
defer sub.Unsubscribe()

// Custom filter
sub := node.FilteredEvents(glueberry.EventFilter{
    PeerID: &peerID,
    States: []glueberry.ConnectionState{
        glueberry.StateEstablished,
        glueberry.StateDisconnected,
    },
})
defer sub.Unsubscribe()
```

---

## Documentation

### Official Documentation

- **[ARCHITECTURE.md](./ARCHITECTURE.md)** – Comprehensive architecture documentation
- **[CHANGELOG.md](./CHANGELOG.md)** – Version history and release notes
- **[CLAUDE.md](./CLAUDE.md)** – Development guidelines and patterns
- **[Godoc](https://pkg.go.dev/github.com/blockberries/glueberry)** – API reference

### Extended Documentation

- **Package Documentation** – See `doc.go` in each package
- **Examples** – See `examples/` directory for complete applications
- **Tests** – See `*_test.go` files for usage patterns

---

## Examples

Glueberry includes several example applications demonstrating different use cases:

### 1. Basic Example (`examples/basic/`)
Minimal example showing node creation and basic connectivity.

```bash
go run examples/basic/main.go
```

### 2. Simple Chat (`examples/simple-chat/`)
Interactive two-peer chat application with complete handshake flow.

```bash
# Terminal 1
go run examples/simple-chat/main.go -port 9000

# Terminal 2
go run examples/simple-chat/main.go -port 9001 -peer /ip4/127.0.0.1/tcp/9000/p2p/...
```

### 3. Cluster (`examples/cluster/`)
Multi-peer cluster with gossip-style message broadcast.

```bash
go run examples/cluster/main.go
```

### 4. File Transfer (`examples/file-transfer/`)
Large file transfer with progress reporting.

```bash
# Sender
go run examples/file-transfer/main.go -mode send -file document.pdf

# Receiver
go run examples/file-transfer/main.go -mode receive
```

### 5. RPC (`examples/rpc/`)
RPC-style request-response communication.

```bash
# Server
go run examples/rpc/main.go -mode server

# Client
go run examples/rpc/main.go -mode client -method echo -params "Hello"
```

### 6. Blockberry Integration (`examples/blockberry-integration/`)
Integration with Blockberry blockchain system (consensus messages, transactions).

```bash
go run examples/blockberry-integration/main.go
```

---

## API Reference

### Node Methods

#### Lifecycle
- `New(cfg *Config) (*Node, error)` – Create node
- `Start() error` – Start listening
- `Stop() error` – Graceful shutdown

#### Identity
- `PeerID() peer.ID` – Get local peer ID
- `PublicKey() ed25519.PublicKey` – Get local public key
- `Addrs() []multiaddr.Multiaddr` – Get listen addresses
- `Version() ProtocolVersion` – Get protocol version

#### Peer Management
- `AddPeer(peerID, addrs, metadata)` – Add peer
- `RemovePeer(peerID)` – Remove peer
- `BlacklistPeer(peerID)` – Blacklist peer
- `UnblacklistPeer(peerID)` – Remove from blacklist
- `GetPeer(peerID)` – Get peer entry
- `ListPeers()` – List all peers

#### Connection Management
- `Connect(peerID)` – Initiate connection
- `ConnectCtx(ctx, peerID)` – Connect with context
- `Disconnect(peerID)` – Disconnect peer
- `DisconnectCtx(ctx, peerID)` – Disconnect with context
- `ConnectionState(peerID)` – Get connection state
- `IsOutbound(peerID)` – Check if outbound connection
- `CancelReconnection(peerID)` – Cancel reconnect

#### Handshake
- `PrepareStreams(peerID, peerPubKey, streamNames)` – Phase 1: Derive key
- `FinalizeHandshake(peerID)` – Phase 2: Activate streams
- `CompleteHandshake(peerID, peerPubKey, streamNames)` – Combined (backward compat)

#### Messaging
- `Send(peerID, streamName, data)` – Send message
- `SendCtx(ctx, peerID, streamName, data)` – Send with context
- `Messages() <-chan IncomingMessage` – Receive messages

#### Events
- `Events() <-chan ConnectionEvent` – Receive events
- `SubscribeEvents() *EventSubscription` – Create subscription
- `FilteredEvents(filter) *EventSubscription` – Filtered subscription
- `EventsForPeer(peerID) *EventSubscription` – Per-peer events
- `EventsForStates(...states) *EventSubscription` – State-filtered events

#### Statistics
- `PeerStatistics(peerID) *PeerStats` – Get peer stats
- `AllPeerStatistics() map[peer.ID]*PeerStats` – All peer stats
- `MessagesSent(peerID) uint64` – Total messages sent
- `Health() *HealthStatus` – Health check

### Types

#### Connection States
```go
const (
    StateDisconnected ConnectionState = iota
    StateConnecting
    StateConnected      // Handshake stream ready
    StateEstablished    // Encrypted streams active
    StateReconnecting
    StateCooldown
)
```

#### Error Codes
```go
const (
    ErrCodeConnectionFailed
    ErrCodeHandshakeFailed
    ErrCodeHandshakeTimeout
    ErrCodeStreamClosed
    ErrCodeEncryptionFailed
    ErrCodeDecryptionFailed
    // ... and more
)
```

### Interfaces

#### Logger
```go
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Info(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
    Error(msg string, keysAndValues ...any)
}
```

#### Metrics
```go
type Metrics interface {
    ConnectionOpened(direction string)
    ConnectionClosed(direction string)
    MessageSent(stream string, bytes int)
    MessageReceived(stream string, bytes int)
    HandshakeDuration(seconds float64)
    // ... and more
}
```

---

## Configuration

### Configuration Options

```go
type Config struct {
    // Required
    PrivateKey      ed25519.PrivateKey
    AddressBookPath string
    ListenAddrs     []multiaddr.Multiaddr

    // Optional (with defaults)
    Logger                   Logger
    Metrics                  Metrics
    HandshakeTimeout         time.Duration  // Default: 60s
    MaxMessageSize           int            // Default: 10MB
    ReconnectInitialBackoff  time.Duration  // Default: 1s
    ReconnectMaxBackoff      time.Duration  // Default: 5m
    ReconnectMaxAttempts     int            // Default: 10
    EventBufferSize          int            // Default: 100
    MessageBufferSize        int            // Default: 1000
    DisableBackpressure      bool           // Default: false
    HighWatermark            int64          // Default: 1000
    LowWatermark             int64          // Default: 500
    OnDecryptionError        func(peer.ID, error)
}
```

### Functional Options

```go
cfg := glueberry.NewConfig(
    privateKey,
    "./addressbook.json",
    listenAddrs,
    glueberry.WithLogger(myLogger),
    glueberry.WithMetrics(myMetrics),
    glueberry.WithHandshakeTimeout(120*time.Second),
    glueberry.WithMaxMessageSize(50*1024*1024),
    glueberry.WithReconnectMaxAttempts(20),
)
```

### Default Values

| Option | Default | Description |
|--------|---------|-------------|
| `HandshakeTimeout` | 60s | Time to complete handshake |
| `MaxMessageSize` | 10MB | Maximum message size |
| `ReconnectInitialBackoff` | 1s | Initial reconnect delay |
| `ReconnectMaxBackoff` | 5m | Maximum reconnect delay |
| `ReconnectMaxAttempts` | 10 | Max reconnect attempts |
| `EventBufferSize` | 100 | Event channel buffer |
| `MessageBufferSize` | 1000 | Message channel buffer |
| `HighWatermark` | 1000 | Backpressure high mark |
| `LowWatermark` | 500 | Backpressure low mark |

---

## Security

### Cryptographic Primitives

| Operation | Algorithm | Key Size |
|-----------|-----------|----------|
| Identity | Ed25519 | 256-bit |
| Key Exchange | X25519 ECDH | 256-bit |
| Key Derivation | HKDF-SHA256 | 256-bit |
| Symmetric Encryption | ChaCha20-Poly1305 | 256-bit |
| Message Authentication | Poly1305 MAC | 128-bit tag |

### Security Features

- **End-to-end encryption** – All post-handshake messages encrypted
- **Forward secrecy** – Unique keys per peer pair
- **Authenticated encryption** – Poly1305 MAC prevents tampering
- **Secure key management** – Private keys never logged, secure zeroing on close
- **Input validation** – All network inputs validated
- **Handshake timeout** – Prevents resource exhaustion
- **Blacklist enforcement** – Connection-level blocking

### Security Best Practices

1. **Protect private keys** – Never commit keys to version control
2. **Use strong identity keys** – Generate with `crypto/rand`
3. **Validate peer identities** – Verify public keys during handshake
4. **Monitor for anomalies** – Use metrics to detect attacks
5. **Keep dependencies updated** – Regularly update Glueberry and libp2p
6. **Limit message sizes** – Use `MaxMessageSize` to prevent DoS

---

## Performance

### Benchmarks

Run benchmarks:

```bash
make bench              # Quick benchmarks
make bench-all          # All benchmarks
make bench-crypto       # Crypto-only
```

**Typical Results (M1 Mac):**

| Operation | Time | Allocs |
|-----------|------|--------|
| Key Derivation (cached) | ~1 µs | 48 B |
| Key Derivation (uncached) | ~50 µs | 2 KB |
| Encryption (1KB) | ~10 µs | 1.5 KB |
| Decryption (1KB) | ~10 µs | 1.5 KB |
| Message Send (1KB) | ~100 µs | 5 KB |

### Optimization Tips

1. **Reuse connections** – Avoid frequent connect/disconnect cycles
2. **Batch small messages** – Reduce encryption overhead
3. **Use flow control** – Enable backpressure to prevent overload
4. **Monitor metrics** – Track performance with Prometheus
5. **Profile your app** – Use pprof to identify bottlenecks

### Profiling

Enable pprof in your application:

```go
import _ "net/http/pprof"

go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

Access profiles:
- CPU: `http://localhost:6060/debug/pprof/profile?seconds=30`
- Memory: `http://localhost:6060/debug/pprof/heap`
- Goroutines: `http://localhost:6060/debug/pprof/goroutine`

---

## Testing

### Run Tests

```bash
make test              # All tests with race detection
make test-short        # Quick tests (no race detection)
make test-race         # Force race detection
```

### Run Integration Tests

```bash
go test -v -run TestIntegration
```

### Run Benchmarks

```bash
make bench
make bench-all
```

### Run Fuzz Tests

```bash
make fuzz              # 30s per target (default)
FUZZTIME=5m make fuzz  # Custom duration
```

### Coverage Report

```bash
make coverage          # Generate HTML report
open coverage.html     # View in browser
```

---

## Contributing

Contributions are welcome! Please follow these guidelines:

### Code Style

- **gofmt** – Format code with `make fmt`
- **go vet** – Run static analysis with `make vet`
- **golangci-lint** – Run linters with `make lint`

### Testing

- **Write tests** – All new features must have tests
- **Run tests** – `make test` must pass
- **Check coverage** – Aim for >80% coverage

### Pull Request Process

1. **Fork** the repository
2. **Create** a feature branch (`git checkout -b feature/my-feature`)
3. **Write** tests for your changes
4. **Run** `make check` (fmt + vet + lint + test)
5. **Commit** with clear messages
6. **Push** to your fork
7. **Submit** a pull request

### Commit Guidelines

- Use clear, descriptive commit messages
- Reference issues in commits (`Fixes #123`)
- Follow conventional commits format (optional)

---

## License

Glueberry is licensed under the **Apache License 2.0**.

See [LICENSE](./LICENSE) for details.

---

## Links

- **Documentation:** [ARCHITECTURE.md](./ARCHITECTURE.md)
- **Godoc:** [pkg.go.dev/github.com/blockberries/glueberry](https://pkg.go.dev/github.com/blockberries/glueberry)
- **Examples:** [examples/](./examples/)
- **Issues:** [GitHub Issues](https://github.com/blockberries/glueberry/issues)
- **Discussions:** [GitHub Discussions](https://github.com/blockberries/glueberry/discussions)

---

## Acknowledgments

Glueberry is built on top of:

- **[libp2p](https://libp2p.io/)** – Modular P2P networking stack
- **[Cramberry](https://github.com/blockberries/cramberry)** – Binary serialization
- **[golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto)** – Cryptographic primitives

Special thanks to the libp2p and Go communities for their excellent work.

---

<p align="center">
  <strong>Built with ❤️ by the Blockberries team</strong>
</p>
