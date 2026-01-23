/*
Package glueberry provides a secure P2P communication library built on libp2p.

Glueberry handles connection management, key exchange, and encrypted stream
multiplexing while delegating peer discovery and handshake protocol logic to
the consuming application.

# Features

  - End-to-end encryption using ChaCha20-Poly1305 AEAD
  - X25519 ECDH key exchange from Ed25519 identity keys
  - Application-controlled handshake protocol
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

	// Add peer to address book
	node.AddPeer(peerID, peerAddrs, nil)

	// Connect and get handshake stream
	hs, err := node.Connect(peerID)

	// Perform your application's handshake
	hs.Send(&MyHandshakeMsg{...})
	hs.Receive(&response)

	// Establish encrypted streams
	streamNames := []string{"blocks", "transactions"}
	node.EstablishEncryptedStreams(peerID, peerPublicKey, streamNames)

Send and receive messages:

	// Send encrypted message
	node.Send(peerID, "blocks", blockData)

	// Receive messages
	for msg := range node.Messages() {
		fmt.Printf("From %s on %s: %s\n",
			msg.PeerID, msg.StreamName, msg.Data)
	}

Handle incoming connections:

	for incoming := range node.IncomingHandshakes() {
		// Perform handshake with incoming peer
		go handleHandshake(node, incoming)
	}

Monitor connection events:

	for event := range node.Events() {
		switch event.State {
		case glueberry.StateEstablished:
			fmt.Printf("Connected to %s\n", event.PeerID)
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
Channels (Messages, Events, IncomingHandshakes) are safe for concurrent reads
from a single consumer.

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
