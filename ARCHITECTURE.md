# Glueberry Architecture

## Overview

Glueberry is a Go library providing encrypted P2P communications over libp2p. It handles connection management, key exchange, and encrypted stream multiplexing while delegating peer discovery and handshake logic to the consuming application.

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
│  │   Crypto    │  │  Handshake  │  │    Event Dispatcher     │  │
│  │   Module    │  │   Handler   │  │                         │  │
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

**Stream Setup (Lazy Opening):**
1. App calls `CompleteHandshake(peerID, peerPubKey, streamNames)`
2. Crypto module derives shared key from ECDH
3. Stream names are registered but streams are not opened yet
4. On first `Send()` to a stream name:
   - Open libp2p stream with protocol ID `/glueberry/stream/<name>/1.0.0`
   - Wrap with encryption layer
   - Start read goroutine for incoming messages
5. Incoming streams are accepted and wrapped on arrival

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
}
```

### Node Methods

```go
// Lifecycle
func New(cfg Config) (*Node, error)
func (n *Node) Start() error
func (n *Node) Stop() error
func (n *Node) PeerID() peer.ID
func (n *Node) PublicKey() ed25519.PublicKey

// Address Book
func (n *Node) AddPeer(peerID peer.ID, addrs []multiaddr.Multiaddr, metadata map[string]string) error
func (n *Node) RemovePeer(peerID peer.ID) error
func (n *Node) BlacklistPeer(peerID peer.ID) error
func (n *Node) UnblacklistPeer(peerID peer.ID) error
func (n *Node) GetPeer(peerID peer.ID) (*PeerInfo, error)
func (n *Node) ListPeers() []PeerInfo

// Connection
func (n *Node) Connect(peerID peer.ID) error
func (n *Node) Disconnect(peerID peer.ID) error
func (n *Node) CancelReconnection(peerID peer.ID) error
func (n *Node) ConnectionState(peerID peer.ID) ConnectionState

// Handshake Completion (call after exchanging handshake messages)
func (n *Node) CompleteHandshake(peerID peer.ID, peerPubKey ed25519.PublicKey, streamNames []string) error

// Messaging
func (n *Node) Send(peerID peer.ID, streamName string, data []byte) error
func (n *Node) Messages() <-chan IncomingMessage
func (n *Node) Events() <-chan ConnectionEvent
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

Errors are returned directly from methods. Common error types:

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
)
```
