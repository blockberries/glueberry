# Simple Chat Example

A simple peer-to-peer chat application demonstrating Glueberry's features.

## Features Demonstrated

- Node creation and lifecycle management
- Outgoing connections with handshaking
- Incoming connection handling
- Encrypted messaging over named streams
- Event monitoring
- Address book management
- Interactive CLI

## Running

### Terminal 1 (Node 1)

```bash
cd examples/simple-chat
go run main.go -port 9000 -name Alice
```

### Terminal 2 (Node 2)

```bash
cd examples/simple-chat
go run main.go -port 9001 -name Bob -peer /ip4/127.0.0.1/tcp/9000/p2p/<PEER_ID_FROM_NODE1>
```

Replace `<PEER_ID_FROM_NODE1>` with the peer ID printed by Node 1.

## Commands

Once running, you can use these commands:

- `connect <multiaddr>` - Connect to a peer
- `send <peer-id> <message>` - Send a chat message
- `list` - List all peers and their connection states
- `quit` - Exit the application

## Example Session

**Node 1:**
```
>
📞 Incoming connection from 12D3KooWXXXX
  Peer name: Bob
  ✅ Connected to Bob

💬 [Bob]: Hello Alice!
```

**Node 2:**
```
> connect /ip4/127.0.0.1/tcp/9000/p2p/12D3KooWYYYY
✅ Handshake complete with: Alice
✅ Secure connection established

> send 12D3KooWYYYY Hello Alice!
Sent to 12D3KooWYYYY
```

## How It Works

1. **Handshake:** Nodes exchange name and Ed25519 public key
2. **Key Derivation:** ECDH derives shared encryption key
3. **Stream Setup:** "chat" stream established with ChaCha20-Poly1305 encryption
4. **Messaging:** All messages automatically encrypted/decrypted

## Code Walkthrough

### Main Components

- `loadOrGenerateKey()` - Persistent identity key management
- `handleIncomingHandshakes()` - Accept incoming connections
- `handleIncomingMessages()` - Display received messages
- `handleEvents()` - Monitor connection state changes
- `connectToPeer()` - Initiate outgoing connections
- `sendMessage()` - Send encrypted messages

### Handshake Flow

The recommended two-phase handshake prevents race conditions where one peer sends
encrypted messages before the other is ready to decrypt:

```go
// After exchanging public keys via handshake stream...

// Phase 1: Prepare streams (derive key, register handlers)
// Now you can RECEIVE encrypted messages
node.PrepareStreams(peerID, peerPubKey, []string{"chat"})

// Send "Complete" message to peer so they know we're ready
handshakeStream.Send([]byte{msgComplete})

// Phase 2: Finalize handshake after peer confirms they're ready
// Wait for peer's "Complete" message...
node.FinalizeHandshake(peerID)
// Now fully established - can send and receive
```

Or use the convenience wrapper if you don't need the two-phase safety:

```go
// Single call that does PrepareStreams + FinalizeHandshake
node.CompleteHandshake(peerID, peerPubKey, []string{"chat"})
```

## Notes

- This is a simple demonstration - production apps would add error handling, proper message serialization with Cramberry, authentication, etc.
- Private keys are stored in `./node.key` and reused on restart
- Address book persisted in `./addressbook.json`
- Messages are sent as raw bytes for simplicity - use Cramberry structs in production
