# Glueberry Architecture Documentation

**Version:** 1.2.9
**Module:** `github.com/blockberries/glueberry`
**Go Version:** 1.25.6

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Architecture Principles](#2-architecture-principles)
3. [System Architecture](#3-system-architecture)
4. [Go Package Architecture](#4-go-package-architecture)
5. [Core Components](#5-core-components)
6. [Concurrency Architecture](#6-concurrency-architecture)
7. [API Design](#7-api-design)
8. [Configuration](#8-configuration)
9. [Testing Strategy](#9-testing-strategy)
10. [Performance Considerations](#10-performance-considerations)
11. [Error Handling](#11-error-handling)
12. [Security Architecture](#12-security-architecture)
13. [Deployment Architecture](#13-deployment-architecture)
14. [Dependency Management](#14-dependency-management)
15. [Future Roadmap](#15-future-roadmap)

---

## 1. Project Overview

### 1.1 Purpose and Scope

Glueberry is a **production-grade Go library** for peer-to-peer communications built on libp2p. It provides:

- **Encrypted, multiplexed streams** between peers using ChaCha20-Poly1305 AEAD
- **App-controlled handshaking** with symmetric two-phase completion
- **Automatic reconnection** with exponential backoff
- **Connection lifecycle management** with state machines
- **Event-driven architecture** for connection state notifications
- **Thread-safe concurrent operations** throughout

Glueberry is a **library, not an application**. It exposes APIs for applications to use, delegating peer discovery and handshake protocol logic to the consuming application.

### 1.2 Module Information

- **Module Path:** `github.com/blockberries/glueberry`
- **Protocol Version:** `1.2.9` (semver)
- **Go Version:** `1.25.6`
- **License:** Apache 2.0

### 1.3 Design Philosophy

Glueberry follows several key design principles:

1. **Library, Not Application:** Exposes APIs rather than providing a complete application
2. **Security First:** All post-handshake communication encrypted, no secrets logged
3. **Explicit Concurrency:** Go channels for async events, errors returned from sync operations
4. **Minimal Public API:** Only export what apps need to use
5. **Thread Safety by Default:** All public APIs are thread-safe

### 1.4 Key Features

- **End-to-end Encryption:** ChaCha20-Poly1305 AEAD with X25519 ECDH key exchange
- **Symmetric Handshake API:** Both peers follow the same handshake flow
- **Multiple Named Streams:** Per-peer stream multiplexing (e.g., "messages", "consensus")
- **Automatic Reconnection:** Exponential backoff with configurable max attempts
- **Peer Blacklisting:** Connection-level enforcement via libp2p connection gater
- **NAT Traversal:** Via libp2p hole punching and relay support
- **Flow Control:** Backpressure mechanism with high/low watermarks
- **Observability:** Prometheus metrics and OpenTelemetry tracing support

---

## 2. Architecture Principles

### 2.1 Separation of Concerns

Glueberry maintains clear boundaries between responsibilities:

#### Application Responsibilities
- **Peer Discovery:** Finding peers (DHT, bootstrap nodes, static config, etc.)
- **Handshake Protocol:** Authentication, version negotiation, capability exchange
- **Message Handling:** Application-specific business logic

#### Glueberry Responsibilities
- **Connection Lifecycle:** Establish, maintain, reconnect, disconnect
- **Encrypted Streams:** Setup and management of ChaCha20-Poly1305 encrypted channels
- **Key Exchange:** X25519 ECDH from Ed25519 identity keys
- **Event Notifications:** Connection state changes, non-blocking delivery

### 2.2 Go Idioms and Best Practices

The codebase follows standard Go conventions:

- **Accept interfaces, return concrete types**
- **Composition over inheritance** (no struct embedding)
- **Explicit error handling** with context wrapping
- **Context-aware APIs** for cancellation and timeouts
- **Channel-based concurrency** with select and non-blocking sends
- **Fine-grained locking** to minimize contention
- **Defer for resource cleanup** with secure memory zeroing

### 2.3 SOLID Principles in Go

#### Single Responsibility Principle
Each package has a focused purpose:
- `pkg/crypto` - Cryptographic operations only
- `pkg/connection` - Connection lifecycle only
- `pkg/streams` - Stream management only
- `pkg/addressbook` - Peer address storage only

#### Open/Closed Principle
Extensible via interfaces:
- `Logger` interface - Plug in any logging library
- `Metrics` interface - Plug in any metrics system
- `HostConnector` interface - Mock libp2p for testing

#### Liskov Substitution Principle
`NopLogger` and `NopMetrics` are valid substitutes (null object pattern)

#### Interface Segregation Principle
Small, focused interfaces:
- `Logger` has 4 methods (Debug, Info, Warn, Error)
- `HostConnector` has 4 methods (ID, Connect, Disconnect, IsConnected)

#### Dependency Inversion Principle
High-level `Node` depends on abstractions (`Logger`, `Metrics`, `HostConnector`)

---

## 3. System Architecture

### 3.1 High-Level Architecture

```
┌───────────────────────────────────────────────────────────────┐
│                      Application Layer                        │
│  (Peer Discovery, Handshake Logic, Message Handling)         │
└───────────────────────┬───────────────────────────────────────┘
                        │ Node API (glueberry.Node)
┌───────────────────────┴───────────────────────────────────────┐
│                     Glueberry Library                         │
│                                                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │  Address    │  │ Connection  │  │   Stream    │          │
│  │    Book     │  │   Manager   │  │   Manager   │          │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
│                                                               │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │   Crypto    │  │   Protocol  │  │    Event    │          │
│  │   Module    │  │    Host     │  │  Dispatch   │          │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
│                                                               │
└───────────────────────┬───────────────────────────────────────┘
                        │ libp2p, crypto/*, cramberry
┌───────────────────────┴───────────────────────────────────────┐
│                   External Dependencies                       │
│  libp2p (P2P networking) | golang.org/x/crypto | Cramberry   │
└───────────────────────────────────────────────────────────────┘
```

### 3.2 Component Breakdown

#### **Node (Root Package)**
- **Purpose:** Main entry point, facade for all operations
- **Responsibilities:**
  - Coordinate all components
  - Expose public API
  - Forward messages and events with middleware (stats, flow control)
  - Manage lifecycle (Start/Stop)

#### **Address Book (pkg/addressbook)**
- **Purpose:** Persist peer addresses and metadata
- **Responsibilities:**
  - CRUD operations for peer entries
  - JSON file persistence
  - Blacklist management
  - Thread-safe concurrent access

#### **Crypto Module (pkg/crypto)**
- **Purpose:** Cryptographic operations
- **Responsibilities:**
  - Ed25519 ↔ X25519 key conversion
  - ECDH shared key derivation
  - ChaCha20-Poly1305 encryption/decryption
  - HKDF-SHA256 key derivation
  - Secure memory zeroing

#### **Connection Manager (pkg/connection)**
- **Purpose:** Manage connection lifecycle
- **Responsibilities:**
  - Connection state machine
  - Automatic reconnection with exponential backoff
  - Handshake timeout management
  - Event emission

#### **Stream Manager (pkg/streams)**
- **Purpose:** Manage streams per peer
- **Responsibilities:**
  - Handshake stream (unencrypted)
  - Named encrypted streams
  - Message multiplexing/demultiplexing
  - Lazy stream opening

#### **Protocol Host (pkg/protocol)**
- **Purpose:** libp2p host wrapper
- **Responsibilities:**
  - libp2p host lifecycle
  - Stream handlers registration
  - Connection gating (blacklist enforcement)
  - Peer ID management

### 3.3 Data Flow

#### **Connection Establishment Flow**

```
Application
    │
    ├──> node.Connect(peerID)
    │       │
    │       ├──> connection.Manager.Connect()
    │       │       │
    │       │       ├──> protocol.Host.Connect() [libp2p]
    │       │       │
    │       │       └──> streams.Manager.OpenHandshakeStream()
    │       │
    │       └──> Event: StateConnected (handshake stream ready)
    │
    ├──> node.Send(peerID, "handshake", helloMsg) [App sends Hello]
    │
    ├──> Receive Hello via node.Messages()
    │       │
    │       └──> App sends PubKey back
    │
    ├──> Receive PubKey via node.Messages()
    │       │
    │       ├──> node.PrepareStreams(peerID, peerPubKey, ["messages"])
    │       │       │
    │       │       ├──> crypto.Module.DeriveSharedKey()
    │       │       │
    │       │       └──> streams.Manager.PrepareStreams()
    │       │
    │       └──> App sends Complete
    │
    ├──> Receive Complete via node.Messages()
    │       │
    │       └──> node.FinalizeHandshake(peerID)
    │               │
    │               ├──> connection.Manager.CompleteHandshake()
    │               │
    │               └──> Event: StateEstablished (encrypted streams active)
    │
    └──> node.Send(peerID, "messages", encryptedData)
```

#### **Message Send Flow**

```
Application
    │
    └──> node.Send(peerID, "messages", data)
            │
            ├──> Validate stream name
            │
            ├──> Flow control (acquire token)
            │       │
            │       └──> flow.Controller.Acquire()
            │
            ├──> streams.Manager.Send()
            │       │
            │       ├──> Get or lazily open stream
            │       │
            │       ├──> EncryptedStream.Send()
            │       │       │
            │       │       ├──> crypto.Cipher.Encrypt(data)
            │       │       │
            │       │       └──> cramberry.Writer.WriteDelimited()
            │       │
            │       └──> libp2p network I/O
            │
            ├──> Flow control (release token)
            │
            └──> Record stats (MessageSent)
```

#### **Message Receive Flow**

```
libp2p (incoming stream)
    │
    ├──> protocol.Host.StreamHandler()
    │       │
    │       └──> streams.Manager.HandleIncomingStream()
    │               │
    │               ├──> EncryptedStream.Receive() [background goroutine]
    │               │       │
    │               │       ├──> cramberry.Reader.ReadDelimited()
    │               │       │
    │               │       ├──> crypto.Cipher.Decrypt(data)
    │               │       │
    │               │       └──> Send to internalMsgs channel
    │               │
    │               └──> node.forwardMessagesWithStats() [middleware]
    │                       │
    │                       ├──> Record stats (MessageReceived)
    │                       │
    │                       └──> Forward to external Messages() channel
    │
    └──> Application reads via node.Messages()
```

---

## 4. Go Package Architecture

### 4.1 Package Structure and Organization

Glueberry follows the **Standard Go Project Layout** (golang-standards/project-layout):

```
glueberry/
├── Root Package (github.com/blockberries/glueberry)
│   ├── node.go           - Main Node type (facade)
│   ├── config.go         - Configuration types
│   ├── errors.go         - Error types and codes
│   ├── events.go         - Event types
│   ├── version.go        - Protocol versioning
│   ├── logging.go        - Logger interface
│   ├── metrics.go        - Metrics interface
│   ├── stats.go          - Peer statistics API
│   ├── health.go         - Health monitoring
│   ├── debug.go          - Debug utilities
│   ├── validation.go     - Input validation
│   └── doc.go            - Package documentation
│
├── pkg/ (Public Packages - Importable)
│   ├── addressbook/      - Peer address book
│   ├── connection/       - Connection lifecycle
│   ├── crypto/           - Cryptographic operations
│   ├── protocol/         - libp2p protocol handlers
│   └── streams/          - Stream management
│
├── internal/ (Private Implementation - Not Importable)
│   ├── eventdispatch/    - Event dispatcher
│   ├── flow/             - Flow control
│   ├── pool/             - Buffer pooling
│   ├── handshake/        - Handshake state machine
│   ├── observability/    - Observability
│   │   ├── otel/         - OpenTelemetry tracing
│   │   └── prometheus/   - Prometheus metrics
│   └── testutil/         - Test utilities
│
├── test/                 - Test infrastructure
│   ├── benchmark/        - Performance benchmarks
│   └── fuzz/             - Fuzz tests
│
└── examples/             - Example applications
    ├── basic/
    ├── simple-chat/
    ├── cluster/
    ├── file-transfer/
    ├── rpc/
    └── blockberry-integration/
```

### 4.2 Module Dependency Graph

```
Application
    ↓
glueberry (root package)
    ↓
pkg/* (public utility packages)
    ↓
internal/* (private implementation)
    ↓
External Dependencies (libp2p, crypto/*, cramberry)
```

**Key Design Decision:** The root package (`glueberry`) is the ONLY public entry point. Applications import `github.com/blockberries/glueberry` and interact via `Node`.

### 4.3 Exported vs Internal API Design

#### **Root Package Exports**
- `Node` struct - Main API
- `Config` struct - Configuration
- `ConnectionEvent` type - Event notifications
- `ConnectionState` constants - State enum
- `ErrorCode` constants - Error classification
- `Logger` interface - Logging abstraction
- `Metrics` interface - Metrics abstraction
- `ProtocolVersion` type - Version handling

#### **Public Packages (pkg/)**
Exported for **specialized use** or **testing**:
- `streams.HandshakeStreamName` - Constant for handshake stream
- `streams.IncomingMessage` - Message type (returned by Node.Messages())
- `addressbook.PeerEntry` - Peer entry type (returned by Node.GetPeer())
- `crypto` functions - Key conversion utilities (Ed25519 ↔ X25519)

#### **Internal Packages (internal/)**
**NOT importable** by external code:
- `eventdispatch.Dispatcher` - Event routing implementation
- `flow.Controller` - Backpressure implementation
- `handshake.HandshakeStateMachine` - Protocol state machine
- `pool.BufferPool` - Memory pooling implementation

### 4.4 Interface Definitions and Contracts

#### **Logger Interface** (`logging.go`)

```go
type Logger interface {
    Debug(msg string, keysAndValues ...any)
    Info(msg string, keysAndValues ...any)
    Warn(msg string, keysAndValues ...any)
    Error(msg string, keysAndValues ...any)
}
```

**Contract:**
- Must be thread-safe
- Compatible with slog, zap, zerolog
- Key-value pairs must be even length (key, value, key, value, ...)

#### **Metrics Interface** (`metrics.go`)

```go
type Metrics interface {
    ConnectionOpened(direction string)   // "inbound" | "outbound"
    ConnectionClosed(direction string)
    MessageSent(stream string, bytes int)
    MessageReceived(stream string, bytes int)
    HandshakeDuration(seconds float64)
    // ... 15+ metrics methods
}
```

**Contract:**
- Must be thread-safe
- Labels are strings (compatible with Prometheus)
- Durations in seconds (float64)
- Byte counts in bytes (int)

#### **HostConnector Interface** (`pkg/connection/manager.go`)

```go
type HostConnector interface {
    ID() peer.ID
    Connect(ctx context.Context, pi peer.AddrInfo) error
    Disconnect(peerID peer.ID) error
    IsConnected(peerID peer.ID) bool
}
```

**Contract:**
- Abstracts libp2p host operations
- Used by Connection Manager
- Enables mocking for unit tests

### 4.5 Struct Composition and Embedding Patterns

Glueberry **avoids struct embedding** to maintain explicit relationships:

```go
// USED: Composition (fields)
type Node struct {
    crypto        *crypto.Module       // Explicit field
    addressBook   *addressbook.Book
    connections   *connection.Manager
}

// NOT USED: Embedding
type Node struct {
    *crypto.Module  // Promotes methods, creates implicit relationships
}
```

**Rationale:**
- Makes dependencies explicit
- Prevents unintended method promotion
- Clearer initialization and lifecycle management
- Easier to mock and test

---

## 5. Core Components

### 5.1 Node (Root Package)

**File:** `node.go` (1,174 lines)

#### Responsibilities
- **Facade** for all operations
- **Coordinate** all components (crypto, connections, streams, addressbook)
- **Forward** messages and events with middleware (stats, flow control)
- **Manage** lifecycle (Start/Stop)

#### Key Types

```go
type Node struct {
    config  *Config
    logger  Logger
    metrics Metrics

    // Core components
    crypto        *crypto.Module
    addressBook   *addressbook.Book
    host          *protocol.Host
    connections   *connection.Manager
    streamManager *streams.Manager
    eventDispatch *eventdispatch.Dispatcher

    // Channel pairs (internal/external)
    internalEvents <-chan eventdispatch.ConnectionEvent
    events         chan ConnectionEvent
    internalMsgs   chan streams.IncomingMessage
    messages       chan streams.IncomingMessage

    // State tracking
    peerStats         map[peer.ID]*PeerStatsTracker
    flowControllers   map[string]*flow.Controller
    eventSubs         map[*EventSubscription]struct{}

    // Lifecycle
    ctx    context.Context
    cancel context.CancelFunc
}
```

#### Public API Methods

**Lifecycle:**
- `New(cfg *Config) (*Node, error)` - Create node
- `Start() error` - Start listening and background goroutines
- `Stop() error` - Graceful shutdown

**Identity:**
- `PeerID() peer.ID` - Get local peer ID
- `PublicKey() ed25519.PublicKey` - Get local public key
- `Addrs() []multiaddr.Multiaddr` - Get listen addresses
- `Version() ProtocolVersion` - Get protocol version

**Peer Management:**
- `AddPeer(peerID, addrs, metadata)` - Add peer to address book
- `RemovePeer(peerID)` - Remove peer
- `BlacklistPeer(peerID)` - Blacklist peer (blocks connections)
- `UnblacklistPeer(peerID)` - Remove from blacklist
- `GetPeer(peerID)` - Get peer entry
- `ListPeers()` - List all peers

**Connection Management:**
- `Connect(peerID)` - Initiate connection
- `ConnectCtx(ctx, peerID)` - Connect with context
- `Disconnect(peerID)` - Disconnect peer
- `DisconnectCtx(ctx, peerID)` - Disconnect with context
- `ConnectionState(peerID)` - Get current state
- `IsOutbound(peerID)` - Check if outbound connection
- `CancelReconnection(peerID)` - Cancel reconnect attempts

**Handshake (Two-Phase):**
- `PrepareStreams(peerID, peerPubKey, streamNames)` - Phase 1: Derive key, ready to receive
- `FinalizeHandshake(peerID)` - Phase 2: Activate encrypted streams
- `CompleteHandshake(peerID, peerPubKey, streamNames)` - Combined (backward compat)

**Messaging:**
- `Send(peerID, streamName, data)` - Send message
- `SendCtx(ctx, peerID, streamName, data)` - Send with context
- `Messages() <-chan IncomingMessage` - Receive messages

**Events:**
- `Events() <-chan ConnectionEvent` - Receive all events
- `SubscribeEvents() *EventSubscription` - Create subscription
- `FilteredEvents(filter) *EventSubscription` - Filtered subscription
- `EventsForPeer(peerID) *EventSubscription` - Per-peer events
- `EventsForStates(...states) *EventSubscription` - State-filtered events

**Statistics:**
- `PeerStatistics(peerID) *PeerStats` - Get peer stats
- `AllPeerStatistics() map[peer.ID]*PeerStats` - All peer stats
- `MessagesSent(peerID) uint64` - Total messages sent
- `Health() *HealthStatus` - Health check

**Debug:**
- `DebugState() *DebugState` - Debug information

#### Thread Safety
- **All public methods are thread-safe** via fine-grained locking
- Channels are **single-consumer** (app reads from Messages(), Events())
- Internal state protected by **separate mutexes** per concern (peerStatsMu, flowControllersMu, eventSubsMu)

### 5.2 Crypto Module (pkg/crypto)

**File:** `pkg/crypto/module.go`

#### Responsibilities
- Ed25519 ↔ X25519 key conversion
- ECDH shared key derivation
- ChaCha20-Poly1305 encryption/decryption
- HKDF-SHA256 key derivation
- Secure memory zeroing

#### Key Types

```go
type Module struct {
    ed25519Private ed25519.PrivateKey  // Node's identity key
    x25519Private  []byte               // Derived X25519 key for ECDH
    peerKeys       map[string][]byte    // Cached shared keys
    peerKeysMu     sync.RWMutex
    closed         bool
}

type Cipher struct {
    key    []byte       // 256-bit symmetric key
    keyMu  sync.RWMutex
    closed bool
}
```

#### API Methods

**Module:**
- `NewModule(privateKey ed25519.PrivateKey) (*Module, error)`
- `DeriveSharedKey(remotePubKey ed25519.PublicKey) ([]byte, error)` - ECDH with caching
- `DeriveSharedKeyFromX25519(remoteX25519 []byte) ([]byte, error)`
- `Close()` - Zero all key material

**Cipher:**
- `NewCipher(key []byte) *Cipher`
- `Encrypt(plaintext []byte, aad []byte) ([]byte, error)` - ChaCha20-Poly1305 with random nonce
- `Decrypt(ciphertext []byte, aad []byte) ([]byte, error)`
- `Close()` - Zero key

#### Security Features
- **Constant-time operations** where possible
- **Secure memory zeroing** via `SecureZero()`
- **Random nonces** (96-bit) for encryption uniqueness
- **Key caching** with RWMutex for performance
- **No key logging** - keys never appear in logs or errors

#### Cryptographic Primitives
- **Identity:** Ed25519 signatures (provided by app)
- **Key Exchange:** X25519 ECDH (converted from Ed25519)
- **Key Derivation:** HKDF-SHA256
- **Symmetric Encryption:** ChaCha20-Poly1305 AEAD
- **Nonce:** Random 96-bit (crypto/rand)

### 5.3 Address Book (pkg/addressbook)

**File:** `pkg/addressbook/book.go`

#### Responsibilities
- Persist peer addresses and metadata
- CRUD operations for peers
- Blacklist management
- Thread-safe concurrent access

#### Key Types

```go
type Book struct {
    storage *storage              // File persistence layer
    peers   map[string]*PeerEntry // In-memory cache
    mu      sync.RWMutex
}

type PeerEntry struct {
    PeerID      peer.ID
    Addrs       []multiaddr.Multiaddr
    Metadata    map[string]string
    Blacklisted bool
    AddedAt     time.Time
    UpdatedAt   time.Time
}
```

#### API Methods

**CRUD:**
- `New(path string) (*Book, error)`
- `AddPeer(peerID, addrs, metadata)` - Add or update peer
- `GetPeer(peerID) (*PeerEntry, error)`
- `RemovePeer(peerID) error`
- `ListPeers() []*PeerEntry`
- `UpdateMetadata(peerID, metadata) error`

**Blacklist:**
- `BlacklistPeer(peerID) error`
- `UnblacklistPeer(peerID) error`
- `IsBlacklisted(peerID) bool`

**Persistence:**
- `Flush() error` - Write to disk
- `Reload() error` - Read from disk
- `Close() error` - Final flush

#### Persistence Strategy
- **JSON file** for human-readable storage
- **Periodic flushing** (every 30 seconds by default)
- **File locking** (platform-specific: `storage_unix.go`, `storage_windows.go`)
- **Atomic writes** via temp file + rename

### 5.4 Connection Manager (pkg/connection)

**File:** `pkg/connection/manager.go`

#### Responsibilities
- Connection state machine
- Automatic reconnection with exponential backoff
- Handshake timeout management
- Event emission

#### Key Types

```go
type Manager struct {
    ctx        context.Context
    cancel     context.CancelFunc
    host       HostConnector  // libp2p host abstraction
    config     ManagerConfig

    connections map[peer.ID]*PeerConnection
    mu          sync.RWMutex

    events chan<- Event  // Send events here
}

type PeerConnection struct {
    PeerID           peer.ID
    State            ConnectionState
    Outbound         bool
    LastStateChange  time.Time
    HandshakeTimer   *time.Timer
    HandshakeCancel  context.CancelFunc
    ReconnectState   *ReconnectState
    mu               sync.Mutex
}

type ConnectionState int
const (
    StateDisconnected ConnectionState = iota
    StateConnecting
    StateConnected      // Handshake stream ready
    StateEstablished    // Encrypted streams active
    StateReconnecting
    StateCooldown
)
```

#### State Machine

```
Disconnected
    ├──> Connecting (via Connect())
    │       ├──> Connected (handshake stream ready)
    │       │       ├──> Established (handshake complete)
    │       │       │       └──> Disconnected (via Disconnect())
    │       │       └──> Cooldown (handshake timeout)
    │       │               └──> Reconnecting (after cooldown)
    │       │                       └──> Connecting (retry)
    │       └──> Disconnected (connection failed)
    └──> Reconnecting (auto-reconnect)
            └──> Connecting (retry)
```

#### Reconnection Strategy

```go
type ReconnectState struct {
    Attempt          int
    MaxAttempts      int
    NextAttemptAt    time.Time
    BackoffDuration  time.Duration
}
```

**Exponential Backoff:**
- Initial: 1 second
- Max: 5 minutes
- Multiplier: 2x per attempt
- Jitter: ±25% random

**Formula:** `backoff = min(initialBackoff * 2^attempt, maxBackoff) * (1 + jitter)`

### 5.5 Stream Manager (pkg/streams)

**File:** `pkg/streams/manager.go`

#### Responsibilities
- Manage handshake stream (unencrypted)
- Manage named encrypted streams per peer
- Message multiplexing/demultiplexing
- Lazy stream opening

#### Key Types

```go
type Manager struct {
    ctx    context.Context
    cancel context.CancelFunc
    host   StreamOpener  // libp2p host abstraction

    handshakeStreams map[peer.ID]*HandshakeStream
    streams          map[peer.ID]map[string]*EncryptedStream
    mu               sync.RWMutex

    messages chan<- IncomingMessage  // Send messages here
}

type HandshakeStream struct {
    peerID      peer.ID
    stream      network.Stream  // libp2p stream
    reader      *cramberry.Reader
    writer      *cramberry.Writer
    ctx         context.Context
    cancel      context.CancelFunc
    closeMu     sync.RWMutex
    closed      bool
}

type EncryptedStream struct {
    peerID    peer.ID
    name      string
    stream    network.Stream
    cipher    *crypto.Cipher  // ChaCha20-Poly1305
    reader    *cramberry.Reader
    writer    *cramberry.Writer
    ctx       context.Context
    cancel    context.CancelFunc
    writeMu   sync.Mutex       // Serialize writes
    closeMu   sync.RWMutex
    closed    bool
}
```

#### Stream Types

**Handshake Stream:**
- **Name:** `"handshake"` (constant: `streams.HandshakeStreamName`)
- **Encryption:** None (plaintext)
- **Purpose:** Exchange Hello, PubKey, Complete messages
- **Lifetime:** Opened immediately on connection, closed after handshake

**Encrypted Streams:**
- **Name:** Application-defined (e.g., "messages", "consensus")
- **Encryption:** ChaCha20-Poly1305 with shared key from ECDH
- **Purpose:** Application-level message exchange
- **Lifetime:** Opened lazily on first send, closed on disconnect

#### Message Framing

Uses **Cramberry** (github.com/blockberries/cramberry) for length-delimited framing:

```
[4-byte length][payload]
```

- **Length:** Big-endian uint32
- **Payload:** Encrypted data (ciphertext + 16-byte Poly1305 tag)

### 5.6 Protocol Host (pkg/protocol)

**File:** `pkg/protocol/host.go`

#### Responsibilities
- libp2p host lifecycle management
- Stream handler registration
- Connection gating (blacklist enforcement)
- Peer ID management

#### Key Types

```go
type Host struct {
    host        host.Host      // libp2p host
    ctx         context.Context
    cancel      context.CancelFunc
    privateKey  ed25519.PrivateKey
    mu          sync.Mutex
}

type HostConfig struct {
    PrivateKey       ed25519.PrivateKey
    ListenAddrs      []multiaddr.Multiaddr
    BlacklistChecker func(peer.ID) bool  // Callback for blacklist check
}
```

#### Stream Handler Registration

```go
func (h *Host) SetStreamHandler(protocol string, handler func(network.Stream)) {
    h.host.SetStreamHandler(protocol.ID(protocol), handler)
}
```

**Registered Handlers:**
- `"/glueberry/handshake/1.0.0"` - Handshake stream
- `"/glueberry/stream/<name>/1.0.0"` - Encrypted streams (per stream name)

#### Connection Gater

```go
type ConnectionGater struct {
    checkBlacklist func(peer.ID) bool
}

func (g *ConnectionGater) InterceptPeerDial(p peer.ID) bool {
    return !g.checkBlacklist(p)  // Block if blacklisted
}

func (g *ConnectionGater) InterceptAccept(n network.ConnMultiaddrs) bool {
    // Extract peer ID and check blacklist
}
```

**Enforcement:**
- Blocks **outbound** connections to blacklisted peers
- Rejects **inbound** connections from blacklisted peers
- Enforced at **libp2p connection level** (before stream creation)

### 5.7 Event Dispatcher (internal/eventdispatch)

**File:** `internal/eventdispatch/dispatcher.go`

#### Responsibilities
- Non-blocking event emission
- Buffer events to prevent blocking
- Event fan-out to subscribers

#### Implementation

```go
type Dispatcher struct {
    events chan ConnectionEvent
    mu     sync.Mutex
    closed bool
}

func (d *Dispatcher) EmitEvent(event ConnectionEvent) {
    d.mu.Lock()
    defer d.mu.Unlock()

    if d.closed {
        return
    }

    // Non-blocking send - drop if full
    select {
    case d.events <- event:
        // Event sent
    default:
        // Channel full, drop event
    }
}
```

**Design Decision:** Non-blocking sends prevent connection state changes from being blocked by slow consumers.

### 5.8 Flow Controller (internal/flow)

**File:** `internal/flow/controller.go`

#### Responsibilities
- Implement backpressure mechanism
- Prevent message queue overflows
- High/low watermark algorithm

#### Implementation

```go
type Controller struct {
    mu           sync.Mutex
    pending      int64       // Current pending messages
    highWater    int64       // Block at this level
    lowWater     int64       // Unblock at this level
    blocked      bool
    unblockCh    chan struct{}  // Signal when unblocked
}

func (fc *Controller) Acquire(ctx context.Context) error {
    for {
        fc.mu.Lock()
        if fc.pending < fc.highWater {
            fc.pending++
            fc.mu.Unlock()
            return nil
        }
        // Hit high watermark - block
        fc.blocked = true
        waitCh := fc.unblockCh
        fc.mu.Unlock()

        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-waitCh:
            // Retry
        }
    }
}

func (fc *Controller) Release() {
    fc.mu.Lock()
    defer fc.mu.Unlock()

    fc.pending--
    if fc.blocked && fc.pending <= fc.lowWater {
        fc.blocked = false
        close(fc.unblockCh)
        fc.unblockCh = make(chan struct{})
    }
}
```

**Default Watermarks:**
- **High:** 1000 messages
- **Low:** 500 messages

---

## 6. Concurrency Architecture

### 6.1 Goroutine Usage Patterns

Glueberry uses several goroutine patterns:

#### **Background Workers (Node)**
Started in `node.Start()`:
```go
go node.forwardMessagesWithStats()  // Middleware for messages
go node.forwardEvents()              // Fan-out events to subscribers
go node.cleanupStalePeerStats()     // Periodic cleanup
```

#### **Stream Readers (Per Stream)**
Started when stream is opened:
```go
go encryptedStream.readLoop()  // Continuously read from libp2p stream
```

#### **Event Subscriber Forwarders (Per Subscription)**
Started in `SubscribeEvents()`:
```go
go func() {
    <-ctx.Done()
    sub.closeChannel()
}()
```

#### **Reconnection Timers (Per Peer)**
Started when entering reconnect state:
```go
go func() {
    <-time.After(backoffDuration)
    manager.attemptReconnect(peerID)
}()
```

### 6.2 Channel Patterns

#### **Producer-Consumer with Buffering**

```go
const (
    DefaultEventBufferSize   = 100
    DefaultMessageBufferSize = 1000
)

events := make(chan ConnectionEvent, DefaultEventBufferSize)
messages := make(chan IncomingMessage, DefaultMessageBufferSize)
```

**Buffer sizes chosen to:**
- Tolerate **bursty** traffic (many connections/messages at once)
- Prevent **blocking** in common scenarios
- Allow **non-blocking sends** with drop-on-full semantics

#### **Internal/External Channel Pairs**

```go
type Node struct {
    internalEvents <-chan eventdispatch.ConnectionEvent  // From dispatcher
    events         chan ConnectionEvent                   // To application
    internalMsgs   chan streams.IncomingMessage          // From stream readers
    messages       chan streams.IncomingMessage          // To application
}
```

**Purpose:** Middleware layer for stats recording, filtering, transformation

#### **Non-Blocking Sends**

```go
func (n *Node) forwardEvents() {
    for evt := range n.internalEvents {
        // Forward to main channel (non-blocking)
        select {
        case n.events <- pubEvt:
        default:
            // Drop if full
            n.metrics.EventDropped()
        }

        // Fan-out to subscribers (non-blocking per subscriber)
        n.eventSubsMu.RLock()
        for sub := range n.eventSubs {
            select {
            case sub.ch <- pubEvt:
            default:
                // Slow subscriber - drop for them
            }
        }
        n.eventSubsMu.RUnlock()
    }
}
```

**Rationale:** Never block connection state changes or message processing due to slow consumers

### 6.3 Mutex Strategies

#### **RWMutex for Read-Heavy Workloads**

```go
type Module struct {
    peerKeys   map[string][]byte
    peerKeysMu sync.RWMutex  // Many reads (key lookups), few writes (key derivation)
}

func (m *Module) DeriveSharedKeyFromX25519(remoteX25519 []byte) ([]byte, error) {
    cacheKey := string(remoteX25519)

    // Fast path: read lock for cache hit
    m.peerKeysMu.RLock()
    if key, ok := m.peerKeys[cacheKey]; ok {
        result := make([]byte, len(key))
        copy(result, key)
        m.peerKeysMu.RUnlock()
        return result, nil
    }
    m.peerKeysMu.RUnlock()

    // Slow path: derive key (without lock)
    sharedKey, err := DeriveSharedKey(...)

    // Write lock to cache result
    m.peerKeysMu.Lock()
    m.peerKeys[cacheKey] = sharedKey
    m.peerKeysMu.Unlock()

    return sharedKey, nil
}
```

**Pattern: Lock → Copy → Unlock → Expensive Operation**

#### **Fine-Grained Locking**

```go
type Node struct {
    // Separate mutexes per concern
    eventSubsMu       sync.RWMutex
    peerStatsMu       sync.RWMutex
    flowControllersMu sync.RWMutex
    startMu           sync.Mutex
}
```

**Benefits:**
- **Reduced contention** - operations on different maps don't block each other
- **Better scalability** - parallelism for unrelated operations

#### **Double-Checked Locking (Lazy Initialization)**

```go
func (n *Node) getOrCreateFlowController(streamName string) *flow.Controller {
    // Fast path: read lock
    n.flowControllersMu.RLock()
    fc := n.flowControllers[streamName]
    n.flowControllersMu.RUnlock()

    if fc != nil {
        return fc
    }

    // Slow path: write lock
    n.flowControllersMu.Lock()
    defer n.flowControllersMu.Unlock()

    // Double-check after acquiring write lock
    if fc = n.flowControllers[streamName]; fc != nil {
        return fc
    }

    fc = flow.NewController(...)
    n.flowControllers[streamName] = fc
    return fc
}
```

**Pattern prevents:**
- Multiple goroutines creating redundant controllers
- Unnecessary write lock acquisition on every access

### 6.4 Context Usage

#### **Context Hierarchy**

```go
func New(cfg *Config) (*Node, error) {
    // Root context for node lifetime
    ctx, cancel := context.WithCancel(context.Background())

    // Component contexts derived from root
    connections := connection.NewManager(ctx, ...)
    streamManager := streams.NewManager(ctx, ...)

    return &Node{
        ctx:    ctx,
        cancel: cancel,
    }
}

func (n *Node) Stop() error {
    n.cancel()  // Cascades to all components
}
```

#### **Context in Public APIs**

```go
// Simple API (uses background context)
func (n *Node) Connect(peerID peer.ID) error {
    return n.ConnectCtx(context.Background(), peerID)
}

// Power-user API (custom context)
func (n *Node) ConnectCtx(ctx context.Context, peerID peer.ID) error {
    connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    return n.host.Connect(connectCtx, addrInfo)
}
```

**Pattern:** Dual API - simple version + context version

#### **Context Cancellation in Blocking Operations**

```go
func (fc *Controller) Acquire(ctx context.Context) error {
    for {
        fc.mu.Lock()
        if fc.pending < fc.highWater {
            fc.pending++
            fc.mu.Unlock()
            return nil
        }
        waitCh := fc.unblockCh
        fc.mu.Unlock()

        // Wait for unblock OR context cancellation
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-waitCh:
            continue
        }
    }
}
```

### 6.5 Synchronization Primitives Summary

| Primitive | Use Case | Example |
|-----------|----------|---------|
| `sync.Mutex` | Exclusive access to state | Stream write serialization |
| `sync.RWMutex` | Read-heavy workloads | Crypto key cache, peer stats map |
| `sync.Once` | One-time initialization | Channel closure |
| `chan struct{}` | Signaling | Backpressure unblock |
| `context.Context` | Cancellation | Shutdown, timeouts |
| `time.Timer` | Scheduled actions | Handshake timeout, reconnect |

---

## 7. API Design

### 7.1 Public API Surface

The public API is exposed via the `Node` type in the root package. Applications interact exclusively through `Node` methods.

#### **API Categories**

1. **Lifecycle:** `New`, `Start`, `Stop`
2. **Identity:** `PeerID`, `PublicKey`, `Addrs`, `Version`
3. **Peer Management:** `AddPeer`, `RemovePeer`, `BlacklistPeer`, `GetPeer`, `ListPeers`
4. **Connection:** `Connect`, `Disconnect`, `ConnectionState`, `CancelReconnection`
5. **Handshake:** `PrepareStreams`, `FinalizeHandshake`, `CompleteHandshake`
6. **Messaging:** `Send`, `Messages`
7. **Events:** `Events`, `SubscribeEvents`, `FilteredEvents`
8. **Statistics:** `PeerStatistics`, `AllPeerStatistics`
9. **Health:** `Health`
10. **Debug:** `DebugState`

### 7.2 Two-Phase Handshake API

Glueberry uses a **symmetric, two-phase handshake** to prevent race conditions.

#### **Why Two Phases?**

**Problem:** If streams become active before both sides have derived the shared key, messages can arrive before the receiving side is ready to decrypt them.

**Solution:** Split handshake into two phases:
1. **PrepareStreams()** - Derive key, ready to receive (but don't send encrypted messages yet)
2. **FinalizeHandshake()** - Both sides confirmed ready, activate encrypted streams

#### **Handshake Flow**

```
Peer A                                    Peer B
  │                                         │
  ├──> Send Hello ─────────────────────────>│
  │                                         ├──> Receive Hello
  │                                         ├──> Send Hello back
  │<─────────────────────────────── Hello ──┤
  │                                         │
  ├──> Send PubKey ────────────────────────>│
  │                                         ├──> Receive PubKey
  │<────────────────────────────── PubKey ──┤
  │                                         │
  ├──> PrepareStreams() [derive key]        ├──> PrepareStreams() [derive key]
  │                                         │
  ├──> Send Complete ──────────────────────>│
  │                                         ├──> Receive Complete
  │<──────────────────────────── Complete ──┤
  │                                         │
  ├──> FinalizeHandshake()                  ├──> FinalizeHandshake()
  │                                         │
  │ [Encrypted streams active]              │ [Encrypted streams active]
```

#### **API Methods**

```go
// Phase 1: Derive key, prepare to receive encrypted messages
func (n *Node) PrepareStreams(
    peerID peer.ID,
    peerPubKey ed25519.PublicKey,
    streamNames []string,
) error

// Phase 2: Both sides ready, activate encrypted streams
func (n *Node) FinalizeHandshake(peerID peer.ID) error

// Combined (backward compat): Calls PrepareStreams + FinalizeHandshake
func (n *Node) CompleteHandshake(
    peerID peer.ID,
    peerPubKey ed25519.PublicKey,
    streamNames []string,
) error
```

### 7.3 Event-Driven Architecture

Glueberry uses **channels** for async event delivery, not callbacks.

#### **Event Types**

```go
type ConnectionEvent struct {
    PeerID    peer.ID
    State     ConnectionState
    Error     error
    Outbound  bool
    Timestamp time.Time
}

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

#### **Event Consumption Patterns**

**Basic:**
```go
for event := range node.Events() {
    switch event.State {
    case glueberry.StateConnected:
        // Start handshake
    case glueberry.StateEstablished:
        // Send application messages
    case glueberry.StateDisconnected:
        // Handle disconnect
    }
}
```

**Subscriptions (Multiple Consumers):**
```go
sub1 := node.SubscribeEvents()
sub2 := node.SubscribeEvents()

go func() {
    for evt := range sub1.Events() {
        // Consumer 1
    }
}()

go func() {
    for evt := range sub2.Events() {
        // Consumer 2
    }
}()

// Cleanup
sub1.Unsubscribe()
sub2.Unsubscribe()
```

**Filtered Subscriptions:**
```go
// Only established events
sub := node.EventsForStates(glueberry.StateEstablished)

// Only events for specific peer
sub := node.EventsForPeer(peerID)

// Custom filter
sub := node.FilteredEvents(glueberry.EventFilter{
    PeerID: &peerID,
    States: []glueberry.ConnectionState{glueberry.StateEstablished},
})
```

### 7.4 Message API

#### **Sending Messages**

```go
// Simple API
func (n *Node) Send(peerID peer.ID, streamName string, data []byte) error

// Power-user API with context
func (n *Node) SendCtx(ctx context.Context, peerID peer.ID, streamName string, data []byte) error
```

**Features:**
- **Automatic stream opening** (lazy) on first send
- **Flow control** (backpressure) if enabled
- **Encryption** via ChaCha20-Poly1305
- **Length-delimited framing** via Cramberry
- **Metrics recording** (bytes sent, errors)

**Errors:**
```go
err := node.Send(peerID, "messages", data)
if err != nil {
    if errors.Is(err, glueberry.ErrNotConnected) {
        // Peer not connected
    } else if errors.Is(err, glueberry.ErrStreamClosed) {
        // Stream closed
    }
}
```

#### **Receiving Messages**

```go
type IncomingMessage struct {
    PeerID     peer.ID
    StreamName string
    Data       []byte
}

for msg := range node.Messages() {
    switch msg.StreamName {
    case streams.HandshakeStreamName:
        // Handle handshake messages
    case "messages":
        // Handle application messages
    }
}
```

**Features:**
- **Decryption** automatic (if encrypted stream)
- **Stream multiplexing** - all streams on single channel
- **Non-blocking delivery** - drops if channel full
- **Metrics recording** (bytes received, errors)

### 7.5 Configuration API

```go
type Config struct {
    // Required
    PrivateKey      ed25519.PrivateKey
    AddressBookPath string
    ListenAddrs     []multiaddr.Multiaddr

    // Optional (with sensible defaults)
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

#### **Functional Options Pattern**

```go
func NewConfig(
    privateKey ed25519.PrivateKey,
    addressBookPath string,
    listenAddrs []multiaddr.Multiaddr,
    opts ...ConfigOption,
) *Config

type ConfigOption func(*Config)

func WithLogger(l Logger) ConfigOption
func WithMetrics(m Metrics) ConfigOption
func WithHandshakeTimeout(d time.Duration) ConfigOption
func WithMaxMessageSize(size int) ConfigOption
// ... more options
```

**Usage:**
```go
cfg := glueberry.NewConfig(
    privateKey,
    "./addressbook.json",
    listenAddrs,
    glueberry.WithLogger(myLogger),
    glueberry.WithHandshakeTimeout(120*time.Second),
    glueberry.WithMaxMessageSize(50*1024*1024),
)
```

---

## 8. Configuration

### 8.1 Configuration Structure

Configuration is provided at node creation via the `Config` struct. All configuration is immutable after node creation.

### 8.2 Defaults

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

### 8.3 Validation

Configuration is validated in `cfg.Validate()` called by `New()`:

```go
func (c *Config) Validate() error {
    if c.PrivateKey == nil {
        return fmt.Errorf("private key is required")
    }
    if c.AddressBookPath == "" {
        return fmt.Errorf("address book path is required")
    }
    if len(c.ListenAddrs) == 0 {
        return fmt.Errorf("at least one listen address is required")
    }
    if c.HandshakeTimeout <= 0 {
        return fmt.Errorf("handshake timeout must be positive")
    }
    if c.MaxMessageSize <= 0 {
        return fmt.Errorf("max message size must be positive")
    }
    // ... more validation
    return nil
}
```

### 8.4 Environment Variables

Glueberry does **not** read environment variables directly. Applications should:

```go
timeout := 60 * time.Second
if val := os.Getenv("GLUEBERRY_HANDSHAKE_TIMEOUT"); val != "" {
    if d, err := time.ParseDuration(val); err == nil {
        timeout = d
    }
}

cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithHandshakeTimeout(timeout),
)
```

---

## 9. Testing Strategy

### 9.1 Test Architecture

Glueberry has comprehensive test coverage:

- **Unit tests:** In-package `*_test.go` files (29 files)
- **Integration tests:** `integration_working_test.go`, `chaos_test.go`
- **Benchmark tests:** `test/benchmark/` (3 files)
- **Fuzz tests:** `test/fuzz/` (4 files)

### 9.2 Unit Tests

**Coverage:**
- Root package: `config_test.go`, `errors_test.go`, `events_test.go`, `node_test.go`, etc.
- Public packages: `pkg/addressbook/*_test.go`, `pkg/crypto/*_test.go`, etc.
- Internal packages: `internal/eventdispatch/*_test.go`, `internal/flow/*_test.go`, etc.

**Test Flags:**
- **Race detection:** `-race` (enabled by default via `make test`)
- **Coverage:** `-coverprofile=coverage.out -covermode=atomic`
- **Short mode:** `-short` (via `make test-short`)

**Example:**
```go
func TestNode_Connect(t *testing.T) {
    // Setup
    cfg := testConfig(t)
    node, err := glueberry.New(cfg)
    require.NoError(t, err)
    defer node.Stop()

    // Test
    err = node.Connect(peerID)
    assert.NoError(t, err)

    // Verify state
    state := node.ConnectionState(peerID)
    assert.Equal(t, glueberry.StateConnecting, state)
}
```

### 9.3 Integration Tests

**File:** `integration_working_test.go`

**Tests:**
- Two-node handshake completion
- Message exchange after handshake
- Reconnection after disconnect
- Blacklist enforcement

**Example:**
```go
func TestIntegration_TwoNodeHandshake(t *testing.T) {
    // Create two nodes
    nodeA := createTestNode(t, 9000)
    nodeB := createTestNode(t, 9001)
    defer nodeA.Stop()
    defer nodeB.Stop()

    // Connect A → B
    nodeA.AddPeer(nodeB.PeerID(), nodeB.Addrs(), nil)
    nodeA.Connect(nodeB.PeerID())

    // Perform handshake (both sides)
    // ... handshake message exchange ...

    // Verify established
    assert.Eventually(t, func() bool {
        return nodeA.ConnectionState(nodeB.PeerID()) == glueberry.StateEstablished
    }, 5*time.Second, 100*time.Millisecond)
}
```

### 9.4 Benchmark Tests

**Files:** `test/benchmark/*.go`

**Benchmarks:**
- Crypto operations (key derivation, encryption, decryption)
- Stream operations (send, receive)
- Load testing (concurrent connections, high message rate)

**Run:**
```bash
make bench              # Subset of benchmarks
make bench-crypto       # Crypto-only
make bench-all          # All benchmarks
```

**Example:**
```go
func BenchmarkCrypto_Encrypt(b *testing.B) {
    cipher := crypto.NewCipher(testKey)
    data := make([]byte, 1024)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = cipher.Encrypt(data, nil)
    }
}
```

### 9.5 Fuzz Tests

**Files:** `test/fuzz/*_fuzz_test.go`

**Fuzz Targets:**
- Crypto operations (malformed ciphertexts)
- Address book (malformed JSON)
- Message framing (malformed Cramberry data)
- Handshake protocol (malformed handshake messages)

**Run:**
```bash
make fuzz                # All fuzz tests (30s each)
make fuzz-crypto         # Crypto-only
FUZZTIME=5m make fuzz    # Custom duration
```

**Example:**
```go
func FuzzCrypto_Decrypt(f *testing.F) {
    f.Add([]byte("valid ciphertext"))

    f.Fuzz(func(t *testing.T, data []byte) {
        cipher := crypto.NewCipher(testKey)
        _, _ = cipher.Decrypt(data, nil)  // Should not panic
    })
}
```

### 9.6 Test Utilities

**Package:** `internal/testutil`

**Utilities:**
- `MockNode` - Mock node for testing
- `MockLogger` - Captures log messages for assertions
- `MockMetrics` - Captures metrics for assertions
- `TestConfig()` - Generates test configuration

---

## 10. Performance Considerations

### 10.1 Profiling Integration

Glueberry supports standard Go profiling:

```go
import _ "net/http/pprof"

go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

Access profiles at:
- CPU: `http://localhost:6060/debug/pprof/profile?seconds=30`
- Memory: `http://localhost:6060/debug/pprof/heap`
- Goroutines: `http://localhost:6060/debug/pprof/goroutine`

### 10.2 Memory Allocation Optimization

#### **Buffer Pooling**

**Package:** `internal/pool`

```go
var bufferPool = sync.Pool{
    New: func() interface{} {
        return new(bytes.Buffer)
    },
}

func GetBuffer() *bytes.Buffer {
    return bufferPool.Get().(*bytes.Buffer)
}

func PutBuffer(buf *bytes.Buffer) {
    buf.Reset()
    bufferPool.Put(buf)
}
```

**Used for:**
- Message serialization
- Temporary byte slices

#### **Key Caching**

Crypto module caches shared keys to avoid repeated ECDH:

```go
type Module struct {
    peerKeys map[string][]byte  // Cache: remoteX25519 → sharedKey
}
```

**Benefit:** Reduces CPU-intensive ECDH operations

#### **Lazy Stream Opening**

Streams are opened **on first send**, not preemptively:

```go
func (m *Manager) Send(peerID peer.ID, streamName string, data []byte) error {
    stream := m.getOrOpenStream(peerID, streamName)  // Lazy open
    return stream.Send(data)
}
```

**Benefit:** Avoids creating unused streams

### 10.3 Concurrency Optimizations

#### **RWMutex for Read-Heavy Maps**

```go
type Node struct {
    peerStats   map[peer.ID]*PeerStatsTracker
    peerStatsMu sync.RWMutex  // Many reads (stats queries), few writes (new peers)
}
```

#### **Fine-Grained Locking**

Separate mutexes per concern:
```go
type Node struct {
    eventSubsMu       sync.RWMutex  // Event subscriptions
    peerStatsMu       sync.RWMutex  // Peer statistics
    flowControllersMu sync.RWMutex  // Flow controllers
}
```

#### **Non-Blocking Channel Operations**

```go
select {
case ch <- msg:
default:
    // Drop instead of blocking
    n.metrics.MessageDropped()
}
```

### 10.4 Database Connection Pooling

Not applicable (Glueberry doesn't use databases).

Address book uses **JSON file** with in-memory caching.

### 10.5 Benchmarking Practices

**Run benchmarks:**
```bash
make bench-crypto  # Crypto operations
make bench-all     # All packages
```

**Key metrics:**
- **Crypto operations:** ~10-50 µs per operation
- **Message send:** ~100-500 µs (includes encryption + network)
- **Key derivation (cached):** ~1 µs (map lookup)
- **Key derivation (uncached):** ~50 µs (ECDH + HKDF)

---

## 11. Error Handling

### 11.1 Error Patterns

#### **Typed Errors**

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

func (e *Error) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("glueberry: %s: %v", e.Message, e.Cause)
    }
    return fmt.Sprintf("glueberry: %s", e.Message)
}

func (e *Error) Unwrap() error {
    return e.Cause
}

func (e *Error) Is(target error) bool {
    t, ok := target.(*Error)
    return ok && t.Code == e.Code
}
```

#### **Error Codes**

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

### 11.2 Sentinel Errors

```go
// Peer operations
var (
    ErrPeerNotFound      = errors.New("peer not found in address book")
    ErrPeerBlacklisted   = errors.New("peer is blacklisted")
    ErrPeerAlreadyExists = errors.New("peer already exists in address book")
)

// Connection operations
var (
    ErrNotConnected      = errors.New("not connected to peer")
    ErrHandshakeTimeout  = errors.New("handshake timeout")
    ErrInCooldown        = errors.New("peer is in cooldown period")
)

// Stream operations
var (
    ErrStreamClosed      = errors.New("stream is closed")
    ErrMessageTooLarge   = errors.New("message exceeds max size")
)
```

### 11.3 Error Wrapping

```go
func (n *Node) PrepareStreams(peerID peer.ID, peerPubKey ed25519.PublicKey, streamNames []string) error {
    sharedKey, err := n.crypto.DeriveSharedKey(peerPubKey)
    if err != nil {
        return fmt.Errorf("failed to derive shared key: %w", err)
    }

    if err := n.streamManager.EstablishStreams(peerID, peerPubKey, streamNames); err != nil {
        return fmt.Errorf("failed to establish streams: %w", err)
    }

    return nil
}
```

**Pattern:** Context → Cause

### 11.4 Error Handling Best Practices

#### **Use errors.Is for Sentinel Errors**

```go
if errors.Is(err, glueberry.ErrNotConnected) {
    // Handle not connected
}
```

#### **Use errors.As for Typed Errors**

```go
var gErr *glueberry.Error
if errors.As(err, &gErr) {
    if gErr.Retriable {
        // Retry logic
    }
    log.Printf("Hint: %s", gErr.Hint)
}
```

#### **Never Panic in Library Code**

Glueberry never panics except for programming errors (e.g., nil pointer due to invalid use).

---

## 12. Security Architecture

### 12.1 Cryptographic Primitives

| Operation | Algorithm | Key Size | Notes |
|-----------|-----------|----------|-------|
| Identity | Ed25519 | 256-bit | Provided by app |
| Key Exchange | X25519 ECDH | 256-bit | Converted from Ed25519 |
| Key Derivation | HKDF-SHA256 | 256-bit | From ECDH shared secret |
| Symmetric Encryption | ChaCha20-Poly1305 | 256-bit key | 96-bit random nonce |
| Message Authentication | Poly1305 MAC | 128-bit tag | Included in ChaCha20-Poly1305 |

### 12.2 Key Management

#### **Key Lifecycle**

```
App generates Ed25519 key pair
    ↓
Glueberry converts Ed25519 → X25519 (RFC 7748)
    ↓
On handshake: ECDH(localX25519, remoteX25519) → shared secret
    ↓
HKDF-SHA256(shared secret) → symmetric key (256-bit)
    ↓
ChaCha20-Poly1305 encryption with random nonce
    ↓
On disconnect: Secure zero all key material
```

#### **Key Storage**

- **Private keys:** Stored in `crypto.Module`, never logged or exposed
- **Shared keys:** Cached in memory (map), zeroed on `Module.Close()`
- **Stream keys:** Stored in `crypto.Cipher`, zeroed on `Cipher.Close()`

#### **Secure Zeroing**

```go
func SecureZero(b []byte) {
    for i := range b {
        b[i] = 0
    }
    runtime.KeepAlive(b)  // Prevent compiler optimization
}
```

### 12.3 Input Validation

All network inputs are validated:

```go
// Message size
if len(data) > n.config.MaxMessageSize {
    return ErrMessageTooLarge
}

// Stream name
if len(streamName) > MaxStreamNameLength {
    return fmt.Errorf("stream name too long")
}
if !isValidStreamName(streamName) {
    return fmt.Errorf("invalid stream name")
}

// Peer ID
if peerID == "" {
    return fmt.Errorf("peer ID is required")
}
```

### 12.4 Attack Mitigation

#### **Handshake Timeout**

Prevents resource exhaustion from peers that connect but never complete handshake:

```go
const DefaultHandshakeTimeout = 60 * time.Second
```

After timeout, connection transitions to `StateCooldown`, preventing immediate reconnect.

#### **Blacklist Enforcement**

Enforced at **libp2p connection level** via connection gater:

```go
func (g *ConnectionGater) InterceptPeerDial(p peer.ID) bool {
    return !g.checkBlacklist(p)  // Block if blacklisted
}
```

#### **Max Message Size**

Prevents memory exhaustion from large messages:

```go
const DefaultMaxMessageSize = 10 * 1024 * 1024  // 10MB
```

#### **Flow Control**

Prevents overwhelming receivers with backpressure:

```go
const (
    DefaultHighWatermark = 1000  // Block sending
    DefaultLowWatermark  = 500   // Unblock sending
)
```

### 12.5 Security Best Practices

1. **Never log private keys** - Keys only in crypto module
2. **Validate all inputs** - From network and app
3. **Constant-time comparisons** - For crypto operations (where possible)
4. **Secure memory zeroing** - On close and errors
5. **Random nonces** - For encryption uniqueness
6. **Timeouts** - Prevent resource exhaustion
7. **Blacklist enforcement** - At connection level

---

## 13. Deployment Architecture

### 13.1 Binary Compilation

Glueberry is a **library**, not a standalone application. Applications using Glueberry compile to a single binary:

```bash
go build -o myapp ./cmd/myapp
```

### 13.2 Containerization

**Dockerfile (example for Glueberry-based app):**

```dockerfile
# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o myapp ./cmd/myapp

# Runtime stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/myapp /usr/local/bin/myapp
ENTRYPOINT ["myapp"]
```

### 13.3 Kubernetes Deployment

**Deployment manifest (example):**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: glueberry-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: glueberry-app
  template:
    metadata:
      labels:
        app: glueberry-app
    spec:
      containers:
      - name: app
        image: myapp:latest
        ports:
        - containerPort: 9000  # P2P port
        env:
        - name: LISTEN_ADDR
          value: "/ip4/0.0.0.0/tcp/9000"
        - name: ADDRESSBOOK_PATH
          value: "/data/addressbook.json"
        volumeMounts:
        - name: data
          mountPath: /data
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: glueberry-data
---
apiVersion: v1
kind: Service
metadata:
  name: glueberry-app
spec:
  type: LoadBalancer
  selector:
    app: glueberry-app
  ports:
  - port: 9000
    targetPort: 9000
```

### 13.4 Monitoring

#### **Prometheus Metrics**

**Package:** `internal/observability/prometheus`

```go
import "github.com/blockberries/glueberry/internal/observability/prometheus"

metrics := prometheus.NewMetrics()
cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithMetrics(metrics),
)
```

**Exposed metrics:**
- `glueberry_connections_total{direction="inbound|outbound"}`
- `glueberry_handshake_duration_seconds`
- `glueberry_messages_sent_total{stream="..."}`
- `glueberry_messages_received_total{stream="..."}`
- `glueberry_encryption_errors_total`
- `glueberry_backpressure_engaged_total{stream="..."}`

#### **OpenTelemetry Tracing**

**Package:** `internal/observability/otel`

```go
import "github.com/blockberries/glueberry/internal/observability/otel"

tracer := otel.NewTracer("myapp")
// Use tracer to create spans for Glueberry operations
```

**Traced operations:**
- Connection establishment
- Handshake completion
- Message send/receive
- Stream open/close

### 13.5 Logging

Glueberry uses structured logging via the `Logger` interface:

```go
import "log/slog"

logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithLogger(logger),
)
```

**Log levels:**
- **Debug:** Verbose diagnostics (stream operations, crypto operations)
- **Info:** Significant events (connections, disconnections, handshakes)
- **Warn:** Recoverable issues (handshake failures, decryption errors)
- **Error:** Serious errors (connection failures, crypto errors)

---

## 14. Dependency Management

### 14.1 go.mod Dependencies

**Module:** `github.com/blockberries/glueberry`

**Direct dependencies:**

```
filippo.io/edwards25519 v1.1.0              // Ed25519 curve operations
github.com/blockberries/cramberry v1.5.5    // Binary serialization
github.com/libp2p/go-libp2p v0.46.0         // P2P networking
github.com/multiformats/go-multiaddr v0.16.1 // Multiaddr support
github.com/prometheus/client_golang v1.22.0  // Prometheus metrics
go.opentelemetry.io/otel v1.39.0            // OpenTelemetry tracing
go.opentelemetry.io/otel/sdk v1.39.0        // OTel SDK
go.opentelemetry.io/otel/trace v1.39.0      // OTel trace
golang.org/x/crypto v0.47.0                 // ChaCha20, HKDF
golang.org/x/sys v0.40.0                    // System calls
github.com/stretchr/testify v1.11.1         // Testing (test-only)
```

### 14.2 Module Versioning

Glueberry uses **semantic versioning** (semver):

- **Major version (1.x.x):** Breaking changes to public API
- **Minor version (x.2.x):** New features, backward compatible
- **Patch version (x.x.9):** Bug fixes, backward compatible

**Current version:** `v1.2.9`

### 14.3 Private Module Access

If Glueberry or dependencies are in private repos:

```bash
# Configure GOPRIVATE
export GOPRIVATE=github.com/blockberries/*

# Configure Git credentials
git config --global url."ssh://git@github.com/".insteadOf "https://github.com/"
```

### 14.4 Vendor Directory

Optional vendoring:

```bash
go mod vendor
go build -mod=vendor ./...
```

### 14.5 Dependency Update Policy

- **Security updates:** Applied immediately
- **Minor updates:** Quarterly review
- **Major updates:** Evaluated for breaking changes, migration path provided

---

## 15. Future Roadmap

### 15.1 Planned Features

1. **Stream prioritization** - QoS for different stream types
2. **Message batching** - Reduce overhead for small messages
3. **Compression** - Optional stream compression (zstd, lz4)
4. **Multicast streams** - One-to-many stream support
5. **Connection pooling** - Multiple connections per peer
6. **Dynamic reconnect config** - Per-peer reconnect policies

### 15.2 Performance Improvements

1. **Zero-copy I/O** - Reduce memory allocations
2. **Vectorized encryption** - SIMD optimizations for ChaCha20
3. **Persistent connections** - Connection keepalive and reuse
4. **Stream multiplexing improvements** - Yamux/QUIC integration

### 15.3 Observability Enhancements

1. **Distributed tracing** - Full span propagation
2. **Health check endpoints** - `/health`, `/ready`, `/metrics`
3. **Diagnostic tools** - `glueberry-debug` CLI tool
4. **Performance profiling** - Built-in pprof endpoints

### 15.4 Go Version Upgrades

- **Current:** Go 1.25.6
- **Target:** Follow Go release cycle (upgrade within 6 months of new stable release)

---

## Appendix A: Glossary

- **AEAD:** Authenticated Encryption with Associated Data
- **ECDH:** Elliptic Curve Diffie-Hellman (key exchange)
- **HKDF:** HMAC-based Key Derivation Function
- **libp2p:** Modular P2P networking stack
- **Multiaddr:** Self-describing network address format
- **Peer ID:** Unique identifier for a peer (derived from public key)
- **Stream:** Bidirectional channel for messages (like TCP connection)
- **ChaCha20-Poly1305:** AEAD cipher combining ChaCha20 stream cipher and Poly1305 MAC

## Appendix B: References

- **libp2p:** https://libp2p.io/
- **Multiaddr:** https://github.com/multiformats/multiaddr
- **ChaCha20-Poly1305:** RFC 8439
- **X25519:** RFC 7748
- **Ed25519:** RFC 8032
- **HKDF:** RFC 5869
- **Cramberry:** https://github.com/blockberries/cramberry

## Appendix C: Contributing

See `CONTRIBUTING.md` (if exists) or:

1. Fork the repository
2. Create a feature branch
3. Write tests (unit + integration)
4. Run `make check` (fmt + vet + lint + test)
5. Submit pull request

## Appendix D: License

Glueberry is licensed under the **Apache License 2.0**.

See `LICENSE` file for details.
