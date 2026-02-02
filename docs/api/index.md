# Glueberry API Reference

Complete API reference for Glueberry P2P communication library.

---

## Table of Contents

- [Core Package (glueberry)](#core-package-glueberry)
- [Public Packages (pkg/)](#public-packages-pkg)
  - [addressbook](#addressbook-package)
  - [connection](#connection-package)
  - [crypto](#crypto-package)
  - [protocol](#protocol-package)
  - [streams](#streams-package)
- [Interfaces](#interfaces)
- [Error Types](#error-types)
- [Constants](#constants)

---

## Core Package (glueberry)

The root package provides the main `Node` API that applications interact with.

### Node Type

```go
type Node struct {
    // Contains filtered or unexported fields
}
```

The `Node` type is the primary entry point for all Glueberry operations. All public methods are thread-safe and can be called concurrently.

#### Creating a Node

```go
func New(cfg *Config) (*Node, error)
```

Creates a new Glueberry node with the specified configuration.

**Parameters:**
- `cfg *Config` - Node configuration (required)

**Returns:**
- `*Node` - The created node
- `error` - Error if configuration is invalid or initialization fails

**Example:**
```go
cfg := glueberry.NewConfig(privateKey, "./addressbook.json", listenAddrs)
node, err := glueberry.New(cfg)
if err != nil {
    log.Fatalf("Failed to create node: %v", err)
}
```

---

### Lifecycle Methods

#### Start

```go
func (n *Node) Start() error
```

Starts the node, begins listening on configured addresses, and starts background goroutines.

**Returns:** `error` if start fails (e.g., port already in use)

**Note:** Must be called before connecting to peers or sending messages.

#### Stop

```go
func (n *Node) Stop() error
```

Stops the node gracefully, closing all connections and cleaning up resources.

**Returns:** `error` if stop fails

**Note:** Blocks until all background goroutines have terminated.

---

### Identity Methods

#### PeerID

```go
func (n *Node) PeerID() peer.ID
```

Returns the local peer ID derived from the node's public key.

**Returns:** `peer.ID` - The local peer ID

#### PublicKey

```go
func (n *Node) PublicKey() ed25519.PublicKey
```

Returns the node's Ed25519 public key.

**Returns:** `ed25519.PublicKey` - The public key (32 bytes)

#### Addrs

```go
func (n *Node) Addrs() []multiaddr.Multiaddr
```

Returns the multiaddresses the node is listening on.

**Returns:** `[]multiaddr.Multiaddr` - List of listen addresses

**Example:**
```go
for _, addr := range node.Addrs() {
    fmt.Printf("Listening on: %s\n", addr.String())
}
```

#### Version

```go
func (n *Node) Version() ProtocolVersion
```

Returns the Glueberry protocol version.

**Returns:** `ProtocolVersion` - Protocol version (major.minor.patch)

---

### Peer Management Methods

#### AddPeer

```go
func (n *Node) AddPeer(peerID peer.ID, addrs []multiaddr.Multiaddr, metadata map[string]string) error
```

Adds a peer to the address book. If the node is started, automatically attempts to connect.

**Parameters:**
- `peerID peer.ID` - Peer's unique identifier
- `addrs []multiaddr.Multiaddr` - Peer's multiaddresses
- `metadata map[string]string` - Optional key-value metadata

**Returns:** `error` if peer is blacklisted or addition fails

**Example:**
```go
peerAddr, _ := multiaddr.NewMultiaddr("/ip4/192.168.1.100/tcp/9001/p2p/12D3KooW...")
peerInfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
err := node.AddPeer(peerInfo.ID, peerInfo.Addrs, map[string]string{
    "name": "Peer1",
    "region": "us-east",
})
```

#### RemovePeer

```go
func (n *Node) RemovePeer(peerID peer.ID) error
```

Removes a peer from the address book and disconnects if connected.

**Parameters:**
- `peerID peer.ID` - Peer to remove

**Returns:** `error` if peer not found

#### BlacklistPeer

```go
func (n *Node) BlacklistPeer(peerID peer.ID) error
```

Blacklists a peer, immediately disconnecting and preventing future connections. Enforced at libp2p connection gater level.

**Parameters:**
- `peerID peer.ID` - Peer to blacklist

**Returns:** `error` if peer not found

**Note:** Blacklisted status persists in address book across restarts.

#### UnblacklistPeer

```go
func (n *Node) UnblacklistPeer(peerID peer.ID) error
```

Removes a peer from the blacklist.

**Parameters:**
- `peerID peer.ID` - Peer to unblacklist

**Returns:** `error` if peer not found

#### GetPeer

```go
func (n *Node) GetPeer(peerID peer.ID) (*addressbook.PeerEntry, error)
```

Retrieves a peer entry from the address book.

**Parameters:**
- `peerID peer.ID` - Peer to retrieve

**Returns:**
- `*addressbook.PeerEntry` - Peer entry with addresses and metadata
- `error` - `ErrPeerNotFound` if peer doesn't exist

#### ListPeers

```go
func (n *Node) ListPeers() []*addressbook.PeerEntry
```

Returns all peers in the address book.

**Returns:** `[]*addressbook.PeerEntry` - List of peer entries

---

### Connection Management Methods

#### Connect

```go
func (n *Node) Connect(peerID peer.ID) error
```

Initiates a connection to a peer. Uses `context.Background()` internally.

**Parameters:**
- `peerID peer.ID` - Peer to connect to

**Returns:** `error` if peer not in address book, blacklisted, or connection fails

**Note:** Connection is asynchronous. Monitor `Events()` for `StateConnected`.

#### ConnectCtx

```go
func (n *Node) ConnectCtx(ctx context.Context, peerID peer.ID) error
```

Initiates a connection with a custom context (for timeout/cancellation).

**Parameters:**
- `ctx context.Context` - Context for cancellation
- `peerID peer.ID` - Peer to connect to

**Returns:** `error` if connection fails or context is cancelled

**Example:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
err := node.ConnectCtx(ctx, peerID)
```

#### Disconnect

```go
func (n *Node) Disconnect(peerID peer.ID) error
```

Disconnects from a peer and cancels any reconnection attempts.

**Parameters:**
- `peerID peer.ID` - Peer to disconnect from

**Returns:** `error` if not connected

#### DisconnectCtx

```go
func (n *Node) DisconnectCtx(ctx context.Context, peerID peer.ID) error
```

Disconnects with a custom context.

**Parameters:**
- `ctx context.Context` - Context for cancellation
- `peerID peer.ID` - Peer to disconnect from

**Returns:** `error` if disconnect fails or context is cancelled

#### ConnectionState

```go
func (n *Node) ConnectionState(peerID peer.ID) ConnectionState
```

Returns the current connection state for a peer.

**Parameters:**
- `peerID peer.ID` - Peer to query

**Returns:** `ConnectionState` - Current state (see [Connection States](#connection-states))

**Example:**
```go
state := node.ConnectionState(peerID)
if state == glueberry.StateEstablished {
    // Send encrypted messages
}
```

#### IsOutbound

```go
func (n *Node) IsOutbound(peerID peer.ID) (bool, error)
```

Checks if the connection to a peer is outbound (we initiated) or inbound (peer initiated).

**Parameters:**
- `peerID peer.ID` - Peer to query

**Returns:**
- `bool` - `true` if outbound, `false` if inbound
- `error` - `ErrNotConnected` if not connected

#### CancelReconnection

```go
func (n *Node) CancelReconnection(peerID peer.ID) error
```

Cancels any automatic reconnection attempts to a peer.

**Parameters:**
- `peerID peer.ID` - Peer to cancel reconnection for

**Returns:** `error` if peer not in reconnection state

---

### Handshake Methods

#### PrepareStreams

```go
func (n *Node) PrepareStreams(
    peerID peer.ID,
    peerPubKey ed25519.PublicKey,
    streamNames []string,
) error
```

**Phase 1 of two-phase handshake:** Derives the shared key and prepares to receive encrypted messages, but does NOT activate encrypted streams yet.

**Parameters:**
- `peerID peer.ID` - Peer to prepare streams for
- `peerPubKey ed25519.PublicKey` - Peer's Ed25519 public key (32 bytes)
- `streamNames []string` - Names of encrypted streams to prepare (e.g., `["messages", "consensus"]`)

**Returns:** `error` if key derivation or stream preparation fails

**Usage:** Call after receiving peer's public key, BEFORE calling `FinalizeHandshake()`.

#### FinalizeHandshake

```go
func (n *Node) FinalizeHandshake(peerID peer.ID) error
```

**Phase 2 of two-phase handshake:** Activates encrypted streams after both sides have prepared and confirmed readiness.

**Parameters:**
- `peerID peer.ID` - Peer to finalize handshake with

**Returns:** `error` if streams not prepared or finalization fails

**Usage:** Call after BOTH sides have sent "Complete" message.

**Note:** After successful finalization, connection state transitions to `StateEstablished` and handshake timeout is cancelled.

#### CompleteHandshake

```go
func (n *Node) CompleteHandshake(
    peerID peer.ID,
    peerPubKey ed25519.PublicKey,
    streamNames []string,
) error
```

**Backward compatibility method:** Combines `PrepareStreams()` and `FinalizeHandshake()` into a single call.

**Parameters:** Same as `PrepareStreams()`

**Returns:** `error` if handshake fails

**Note:** Only use if you're certain both sides have exchanged all necessary handshake messages. For safer handshaking, use the two-phase API (`PrepareStreams` + `FinalizeHandshake`).

---

### Messaging Methods

#### Send

```go
func (n *Node) Send(peerID peer.ID, streamName string, data []byte) error
```

Sends a message to a peer on the specified stream.

**Parameters:**
- `peerID peer.ID` - Destination peer
- `streamName string` - Stream name (e.g., `"messages"`, `streams.HandshakeStreamName`)
- `data []byte` - Message data (must be ≤ `MaxMessageSize`)

**Returns:** `error` if peer not connected, stream closed, or send fails

**Notes:**
- For **handshake stream** (`streams.HandshakeStreamName`): Message is sent **unencrypted**
- For **encrypted streams**: Message is automatically encrypted with ChaCha20-Poly1305
- Streams are opened **lazily** on first send
- Flow control (backpressure) is applied if enabled

**Example:**
```go
// Send handshake message
err := node.Send(peerID, streams.HandshakeStreamName, helloMsg)

// Send encrypted message
err := node.Send(peerID, "messages", []byte("Hello!"))
```

#### SendCtx

```go
func (n *Node) SendCtx(ctx context.Context, peerID peer.ID, streamName string, data []byte) error
```

Sends a message with a custom context (for timeout/cancellation).

**Parameters:**
- `ctx context.Context` - Context for cancellation
- `peerID peer.ID` - Destination peer
- `streamName string` - Stream name
- `data []byte` - Message data

**Returns:** `error` if send fails or context is cancelled

**Example:**
```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
err := node.SendCtx(ctx, peerID, "messages", data)
```

#### Messages

```go
func (n *Node) Messages() <-chan streams.IncomingMessage
```

Returns a channel for receiving incoming messages from all peers and streams.

**Returns:** `<-chan streams.IncomingMessage` - Receive-only channel

**Notes:**
- **Single consumer only** - Do not read from multiple goroutines
- **Non-blocking delivery** - Messages are dropped if channel is full
- **Automatic decryption** - Encrypted stream messages are decrypted automatically
- Channel is closed when `Node.Stop()` is called

**Example:**
```go
for msg := range node.Messages() {
    fmt.Printf("From %s on %s: %s\n", msg.PeerID, msg.StreamName, msg.Data)
}
```

---

### Event Methods

#### Events

```go
func (n *Node) Events() <-chan ConnectionEvent
```

Returns a channel for receiving connection state events from all peers.

**Returns:** `<-chan ConnectionEvent` - Receive-only channel

**Notes:**
- **Single consumer only** - Do not read from multiple goroutines
- **Non-blocking delivery** - Events are dropped if channel is full
- Channel is closed when `Node.Stop()` is called

**Example:**
```go
for event := range node.Events() {
    fmt.Printf("Peer %s: %s\n", event.PeerID, event.State)
}
```

#### SubscribeEvents

```go
func (n *Node) SubscribeEvents() *EventSubscription
```

Creates a new event subscription for receiving events. Allows multiple consumers.

**Returns:** `*EventSubscription` - Event subscription

**Notes:**
- Each subscription has its own buffered channel
- Subscriptions must be unsubscribed to avoid resource leaks
- Events are fan-out from the main event channel

**Example:**
```go
sub := node.SubscribeEvents()
defer sub.Unsubscribe()

for evt := range sub.Events() {
    // Handle event
}
```

#### FilteredEvents

```go
func (n *Node) FilteredEvents(filter EventFilter) *EventSubscription
```

Creates a filtered event subscription.

**Parameters:**
- `filter EventFilter` - Event filter criteria

**Returns:** `*EventSubscription` - Filtered event subscription

**Example:**
```go
sub := node.FilteredEvents(glueberry.EventFilter{
    PeerID: &peerID,
    States: []glueberry.ConnectionState{glueberry.StateEstablished},
})
defer sub.Unsubscribe()
```

#### EventsForPeer

```go
func (n *Node) EventsForPeer(peerID peer.ID) *EventSubscription
```

Creates a subscription for events from a specific peer only.

**Parameters:**
- `peerID peer.ID` - Peer to monitor

**Returns:** `*EventSubscription` - Per-peer event subscription

#### EventsForStates

```go
func (n *Node) EventsForStates(states ...ConnectionState) *EventSubscription
```

Creates a subscription for specific connection states only.

**Parameters:**
- `states ...ConnectionState` - States to monitor

**Returns:** `*EventSubscription` - State-filtered subscription

**Example:**
```go
// Only receive established and disconnected events
sub := node.EventsForStates(glueberry.StateEstablished, glueberry.StateDisconnected)
defer sub.Unsubscribe()
```

---

### Statistics Methods

#### PeerStatistics

```go
func (n *Node) PeerStatistics(peerID peer.ID) *PeerStats
```

Returns statistics for a specific peer.

**Parameters:**
- `peerID peer.ID` - Peer to query

**Returns:** `*PeerStats` - Peer statistics or `nil` if peer not found

**Example:**
```go
stats := node.PeerStatistics(peerID)
if stats != nil {
    fmt.Printf("Messages sent: %d, received: %d\n", stats.MessagesSent, stats.MessagesReceived)
}
```

#### AllPeerStatistics

```go
func (n *Node) AllPeerStatistics() map[peer.ID]*PeerStats
```

Returns statistics for all peers.

**Returns:** `map[peer.ID]*PeerStats` - Map of peer ID to statistics

#### MessagesSent

```go
func (n *Node) MessagesSent(peerID peer.ID) uint64
```

Returns the total number of messages sent to a peer.

**Parameters:**
- `peerID peer.ID` - Peer to query

**Returns:** `uint64` - Total messages sent (0 if peer not found)

---

### Health and Debug Methods

#### Health

```go
func (n *Node) Health() *HealthStatus
```

Returns the node's health status.

**Returns:** `*HealthStatus` - Health information

**Example:**
```go
health := node.Health()
fmt.Printf("Healthy: %v, Active connections: %d\n", health.Healthy, health.ActiveConnections)
```

#### DebugState

```go
func (n *Node) DebugState() *DebugState
```

Returns detailed debug information about the node's internal state.

**Returns:** `*DebugState` - Debug state snapshot

**Note:** Intended for debugging and diagnostics only.

---

## Public Packages (pkg/)

### addressbook Package

**Import:** `github.com/blockberries/glueberry/pkg/addressbook`

#### PeerEntry Type

```go
type PeerEntry struct {
    PeerID      peer.ID
    Addrs       []multiaddr.Multiaddr
    Metadata    map[string]string
    Blacklisted bool
    AddedAt     time.Time
    UpdatedAt   time.Time
}
```

Represents a peer entry in the address book.

**Fields:**
- `PeerID` - Unique peer identifier
- `Addrs` - List of multiaddresses for the peer
- `Metadata` - Key-value metadata (e.g., name, region)
- `Blacklisted` - Whether peer is blacklisted
- `AddedAt` - When peer was added
- `UpdatedAt` - Last update timestamp

---

### connection Package

**Import:** `github.com/blockberries/glueberry/pkg/connection`

#### ConnectionState Type

```go
type ConnectionState int

const (
    StateDisconnected ConnectionState = iota
    StateConnecting
    StateConnected      // Handshake stream ready, timeout started
    StateEstablished    // Encrypted streams active, timeout cancelled
    StateReconnecting
    StateCooldown
)
```

Represents the connection state with a peer.

**States:**
- `StateDisconnected` - No connection
- `StateConnecting` - Connection in progress
- `StateConnected` - Handshake stream ready (start handshake now!)
- `StateEstablished` - Handshake complete, encrypted streams active
- `StateReconnecting` - Attempting to reconnect
- `StateCooldown` - Temporary ban after handshake timeout

---

### crypto Package

**Import:** `github.com/blockberries/glueberry/pkg/crypto`

#### Ed25519ToX25519

```go
func Ed25519ToX25519PublicKey(ed25519Pub ed25519.PublicKey) ([]byte, error)
```

Converts an Ed25519 public key to X25519 format for ECDH.

**Parameters:**
- `ed25519Pub ed25519.PublicKey` - Ed25519 public key (32 bytes)

**Returns:**
- `[]byte` - X25519 public key (32 bytes)
- `error` - If conversion fails

#### SecureZero

```go
func SecureZero(b []byte)
```

Securely zeros a byte slice, preventing compiler optimization.

**Parameters:**
- `b []byte` - Slice to zero

**Note:** Used for zeroing key material.

---

### protocol Package

**Import:** `github.com/blockberries/glueberry/pkg/protocol`

Contains libp2p protocol handlers and host management. Not typically used directly by applications.

---

### streams Package

**Import:** `github.com/blockberries/glueberry/pkg/streams`

#### Constants

```go
const HandshakeStreamName = "handshake"
```

The name of the handshake stream. Use this constant when sending handshake messages.

**Example:**
```go
node.Send(peerID, streams.HandshakeStreamName, helloMsg)
```

#### IncomingMessage Type

```go
type IncomingMessage struct {
    PeerID     peer.ID
    StreamName string
    Data       []byte
}
```

Represents an incoming message from a peer.

**Fields:**
- `PeerID` - Sender's peer ID
- `StreamName` - Stream the message arrived on
- `Data` - Message payload (decrypted if from encrypted stream)

---

## Interfaces

### Logger Interface

```go
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Info(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
    Error(msg string, keysAndValues ...any)
}
```

Logger interface for structured logging. Compatible with `log/slog`, `zap`, `zerolog`.

**Key-Value Pairs:**
- Pass in pairs: `logger.Info("message", "key1", value1, "key2", value2)`
- Must have even number of arguments (key, value, key, value, ...)

**Example:**
```go
logger.Info("connection established", "peer_id", peerID.String(), "duration_ms", 150)
```

### Metrics Interface

```go
type Metrics interface {
    ConnectionOpened(direction string)
    ConnectionClosed(direction string)
    MessageSent(stream string, bytes int)
    MessageReceived(stream string, bytes int)
    HandshakeDuration(seconds float64)
    // ... more methods
}
```

Metrics interface for Prometheus-style metrics collection.

**Label Values:**
- `direction`: `"inbound"` or `"outbound"`
- `stream`: Stream name (e.g., `"messages"`, `"handshake"`)
- `result`: `"success"`, `"failure"`, `"timeout"`

**Example:**
```go
metrics.ConnectionOpened("outbound")
metrics.MessageSent("messages", len(data))
```

---

## Error Types

### Error Type

```go
type Error struct {
    Code      ErrorCode
    Message   string
    PeerID    peer.ID
    Stream    string
    Cause     error
    Retriable bool
    Hint      string
}
```

Glueberry's rich error type with structured information.

**Fields:**
- `Code` - Error code (see [Error Codes](#error-codes))
- `Message` - Human-readable message
- `PeerID` - Associated peer (if applicable)
- `Stream` - Associated stream (if applicable)
- `Cause` - Underlying error (if any)
- `Retriable` - Whether operation can be retried
- `Hint` - User-friendly troubleshooting hint

**Usage:**
```go
var gErr *glueberry.Error
if errors.As(err, &gErr) {
    if gErr.Retriable {
        // Retry
    }
    log.Printf("Hint: %s", gErr.Hint)
}
```

### Sentinel Errors

```go
var (
    ErrPeerNotFound      = errors.New("peer not found in address book")
    ErrPeerBlacklisted   = errors.New("peer is blacklisted")
    ErrNotConnected      = errors.New("not connected to peer")
    ErrHandshakeTimeout  = errors.New("handshake timeout")
    ErrStreamClosed      = errors.New("stream is closed")
    ErrMessageTooLarge   = errors.New("message exceeds max size")
    // ... more
)
```

**Usage:**
```go
if errors.Is(err, glueberry.ErrNotConnected) {
    // Handle not connected
}
```

---

## Constants

### Error Codes

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
    ErrCodeInvalidMessage
    ErrCodePeerNotFound
    ErrCodePeerBlacklisted
    ErrCodeInvalidConfig
    ErrCodeNodeNotStarted
    ErrCodeAlreadyConnected
    ErrCodeNotConnected
    ErrCodeBackpressure
    ErrCodeContextCancelled
)
```

### Default Configuration Values

```go
const (
    DefaultHandshakeTimeout        = 60 * time.Second
    DefaultMaxMessageSize          = 10 * 1024 * 1024  // 10MB
    DefaultReconnectInitialBackoff = 1 * time.Second
    DefaultReconnectMaxBackoff     = 5 * time.Minute
    DefaultReconnectMaxAttempts    = 10
    DefaultEventBufferSize         = 100
    DefaultMessageBufferSize       = 1000
    DefaultHighWatermark           = 1000
    DefaultLowWatermark            = 500
)
```

### Protocol Version

```go
const (
    ProtocolVersionMajor = 1
    ProtocolVersionMinor = 2
    ProtocolVersionPatch = 9
)
```

---

## See Also

- [Packages Documentation](./packages.md) - Detailed package documentation
- [Interfaces Documentation](./interfaces.md) - Interface contracts and implementations
- [ARCHITECTURE.md](../../ARCHITECTURE.md) - Architecture overview
- [Godoc](https://pkg.go.dev/github.com/blockberries/glueberry) - Online API reference
