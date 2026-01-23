/*
Package glueberry provides a secure P2P communication library built on libp2p.

Glueberry handles connection management, key exchange, and encrypted stream
multiplexing while delegating peer discovery and handshake protocol logic to
the consuming application.

# Features

  - End-to-end encryption using ChaCha20-Poly1305 AEAD
  - X25519 ECDH key exchange from Ed25519 identity keys
  - Symmetric, event-driven handshake API
  - Multiple named encrypted streams per peer
  - Automatic reconnection with exponential backoff
  - Peer blacklisting with connection-level enforcement
  - Non-blocking connection state event notifications
  - Thread-safe concurrent operations
  - NAT traversal via libp2p hole punching
  - JSON-persisted address book

# Quick Start

Create a node:

	privateKey, _ := ed25519.GenerateKey(rand.Reader)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9000")

	cfg := glueberry.NewConfig(privateKey, "./addressbook.json",
		[]multiaddr.Multiaddr{listenAddr})

	node, err := glueberry.New(cfg)
	if err != nil {
		// Handle error
	}

	node.Start()
	defer node.Stop()

Connect to a peer:

	// Add peer to address book (auto-connects when node is started)
	node.AddPeer(peerID, peerAddrs, nil)

	// Or explicitly connect
	node.Connect(peerID)

Handle connection events (start handshake when connected):

	for event := range node.Events() {
		if event.State == glueberry.StateConnected {
			// Send Hello to initiate handshake
			node.Send(event.PeerID, streams.HandshakeStreamName, helloMsg)
		}
	}

Handle handshake messages with two-phase completion:

	// Receive Hello → Send PubKey
	// Receive PubKey → PrepareStreams() + Send Complete
	// Receive Complete → FinalizeHandshake()

	var streamsPrepared, gotComplete bool

	for msg := range node.Messages() {
		if msg.StreamName == streams.HandshakeStreamName {
			switch parseMessageType(msg.Data) {
			case MsgHello:
				node.Send(msg.PeerID, streams.HandshakeStreamName, pubKeyMsg)
			case MsgPubKey:
				peerPubKey = extractPubKey(msg.Data)
				// Prepare streams first (ready to receive encrypted messages)
				node.PrepareStreams(msg.PeerID, peerPubKey, []string{"messages"})
				streamsPrepared = true
				// Signal readiness to peer
				node.Send(msg.PeerID, streams.HandshakeStreamName, completeMsg)
			case MsgComplete:
				gotComplete = true
			}
			// Finalize when both sides ready
			if streamsPrepared && gotComplete {
				node.FinalizeHandshake(msg.PeerID)
			}
		}
	}

Send and receive encrypted messages:

	// Send encrypted message (after handshake complete)
	node.Send(peerID, "messages", msgData)

	// Receive messages
	for msg := range node.Messages() {
		if msg.StreamName == "messages" {
			fmt.Printf("From %s: %s\n", msg.PeerID, msg.Data)
		}
	}

Monitor connection events:

	for event := range node.Events() {
		switch event.State {
		case glueberry.StateConnected:
			fmt.Printf("Connected to %s (handshake stream ready)\n", event.PeerID)
		case glueberry.StateEstablished:
			fmt.Printf("Secure connection with %s\n", event.PeerID)
		case glueberry.StateDisconnected:
			fmt.Printf("Disconnected from %s\n", event.PeerID)
		}
	}

# Architecture

Glueberry separates concerns clearly:

Application Responsibilities:
  - Peer discovery (finding peers)
  - Handshake protocol (authentication, versioning)
  - Message handling (application logic)

Glueberry Responsibilities:
  - Connection lifecycle management
  - Encrypted stream multiplexing
  - Automatic reconnection
  - Event notifications

# Connection Flow

 1. AddPeer() or Connect() initiates connection
 2. On success, StateConnected event fires, handshake stream ready, timeout starts
 3. Application reacts to StateConnected by sending Hello
 4. Receive Hello → Send PubKey
 5. Receive PubKey → PrepareStreams() + Send Complete (ready for encrypted messages)
 6. Receive Complete → FinalizeHandshake() (both sides ready)
 7. State transitions to StateEstablished, timeout cancelled
 8. Encrypted streams active

# Security

  - Ed25519 signatures for identity
  - X25519 ECDH for key agreement
  - ChaCha20-Poly1305 for symmetric encryption
  - HKDF-SHA256 for key derivation
  - Nonce uniqueness guaranteed (random 96-bit nonces)
  - Message authentication via Poly1305 MAC
  - Input validation on all network data

Private keys and shared secrets never leave the crypto module and are never
logged or exposed in error messages.

# Thread Safety

All public Node methods are thread-safe and can be called concurrently.
Channels (Messages, Events) are safe for concurrent reads from a single consumer.

# Dependencies

  - github.com/libp2p/go-libp2p - P2P networking
  - github.com/blockberries/cramberry - Binary serialization
  - golang.org/x/crypto - Cryptographic primitives

# See Also

  - ARCHITECTURE.md - Detailed architecture and component descriptions
  - IMPLEMENTATION_PLAN.md - Development roadmap
  - examples/simple-chat - Working example application
*/
package glueberry
