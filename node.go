package glueberry

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync"

	"github.com/blockberries/glueberry/internal/eventdispatch"
	"github.com/blockberries/glueberry/pkg/addressbook"
	"github.com/blockberries/glueberry/pkg/connection"
	"github.com/blockberries/glueberry/pkg/crypto"
	"github.com/blockberries/glueberry/pkg/protocol"
	"github.com/blockberries/glueberry/pkg/streams"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// Node is the main entry point for Glueberry P2P communications.
// It aggregates all components and provides a unified public API.
//
// All public methods are thread-safe.
type Node struct {
	config *Config

	// Core components
	crypto        *crypto.Module
	addressBook   *addressbook.Book
	host          *protocol.Host
	connections   *connection.Manager
	streamManager *streams.Manager
	eventDispatch *eventdispatch.Dispatcher

	// Channels
	events             <-chan eventdispatch.ConnectionEvent
	messages           chan streams.IncomingMessage
	incomingHandshakes chan protocol.IncomingHandshake

	// Lifecycle
	ctx     context.Context
	cancel  context.CancelFunc
	started bool
	startMu sync.Mutex
}

// New creates a new Glueberry node with the given configuration.
// The node is not started until Start() is called.
func New(cfg *Config) (*Node, error) {
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Apply defaults
	cfg.applyDefaults()

	ctx, cancel := context.WithCancel(context.Background())

	// Create crypto module
	cryptoModule, err := crypto.NewModule(cfg.PrivateKey)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create crypto module: %w", err)
	}

	// Create address book
	addrBook, err := addressbook.New(cfg.AddressBookPath)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create address book: %w", err)
	}

	// Create event dispatcher
	dispatcher := eventdispatch.NewDispatcher(cfg.EventBufferSize)

	// Create message channel
	messagesChan := make(chan streams.IncomingMessage, cfg.MessageBufferSize)

	// Create incoming handshakes channel
	incomingHandshakesChan := make(chan protocol.IncomingHandshake, cfg.EventBufferSize)

	// Create connection gater
	gater := protocol.NewConnectionGater(addrBook)

	// Create libp2p host
	hostConfig := protocol.HostConfig{
		PrivateKey:       cfg.PrivateKey,
		ListenAddrs:      cfg.ListenAddrs,
		Gater:            gater,
		ConnMgrLowWater:  100,
		ConnMgrHighWater: 400,
	}

	libp2pHost, err := protocol.NewHost(ctx, hostConfig)
	if err != nil {
		cancel()
		dispatcher.Close()
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	// Create stream manager
	streamMgr := streams.NewManager(ctx, libp2pHost, cryptoModule, messagesChan)

	// Create connection manager
	connMgrConfig := connection.ManagerConfig{
		HandshakeTimeout:        cfg.HandshakeTimeout,
		ReconnectBaseDelay:      cfg.ReconnectBaseDelay,
		ReconnectMaxDelay:       cfg.ReconnectMaxDelay,
		ReconnectMaxAttempts:    cfg.ReconnectMaxAttempts,
		FailedHandshakeCooldown: cfg.FailedHandshakeCooldown,
	}

	connMgr := connection.NewManager(ctx, libp2pHost, addrBook, connMgrConfig, dispatcher)

	return &Node{
		config:             cfg,
		crypto:             cryptoModule,
		addressBook:        addrBook,
		host:               libp2pHost,
		connections:        connMgr,
		streamManager:      streamMgr,
		eventDispatch:      dispatcher,
		events:             dispatcher.Events(),
		messages:           messagesChan,
		incomingHandshakes: incomingHandshakesChan,
		ctx:                ctx,
		cancel:             cancel,
		started:            false,
	}, nil
}

// Start starts the node and begins listening for connections.
// This must be called before the node can connect to peers or receive connections.
func (n *Node) Start() error {
	n.startMu.Lock()
	defer n.startMu.Unlock()

	if n.started {
		return ErrNodeAlreadyStarted
	}

	// Register handshake protocol handler
	handshakeHandler := protocol.NewHandshakeHandler(n.config.HandshakeTimeout, n.incomingHandshakes)
	n.host.LibP2PHost().SetStreamHandler(protocol.HandshakeProtocolID, handshakeHandler.HandleStream)

	// Note: Encrypted stream handlers are registered dynamically when streams are established
	// because we need the shared key which is only available after handshake

	n.started = true

	return nil
}

// Stop shuts down the node and releases all resources.
// It closes all connections, stops all goroutines, and cleans up state.
func (n *Node) Stop() error {
	n.startMu.Lock()
	defer n.startMu.Unlock()

	if !n.started {
		return ErrNodeNotStarted
	}

	// Cancel context to stop all goroutines
	n.cancel()

	// Shutdown components in reverse order of initialization
	n.connections.Shutdown()
	n.streamManager.Shutdown()
	n.eventDispatch.Close()

	// Close libp2p host
	if err := n.host.Close(); err != nil {
		return fmt.Errorf("failed to close host: %w", err)
	}

	// Close messages channel
	close(n.messages)

	// Close incoming handshakes channel
	close(n.incomingHandshakes)

	n.started = false

	return nil
}

// PeerID returns the local peer ID.
func (n *Node) PeerID() peer.ID {
	return n.host.ID()
}

// PublicKey returns the local Ed25519 public key.
func (n *Node) PublicKey() ed25519.PublicKey {
	return n.crypto.Ed25519PublicKey()
}

// Addrs returns the multiaddresses the node is listening on.
func (n *Node) Addrs() []multiaddr.Multiaddr {
	return n.host.Addrs()
}

// AddPeer adds a peer to the address book.
func (n *Node) AddPeer(peerID peer.ID, addrs []multiaddr.Multiaddr, metadata map[string]string) error {
	return n.addressBook.AddPeer(peerID, addrs, metadata)
}

// RemovePeer removes a peer from the address book.
// This does not disconnect an active connection - use Disconnect() for that.
func (n *Node) RemovePeer(peerID peer.ID) error {
	return n.addressBook.RemovePeer(peerID)
}

// BlacklistPeer blacklists a peer.
// If the peer is currently connected, the connection is closed.
func (n *Node) BlacklistPeer(peerID peer.ID) error {
	// Blacklist in address book
	if err := n.addressBook.BlacklistPeer(peerID); err != nil {
		return err
	}

	// Disconnect if currently connected
	if n.connections.GetState(peerID).IsActive() {
		_ = n.connections.Disconnect(peerID) // Ignore error - best effort disconnect
	}

	return nil
}

// UnblacklistPeer removes a peer from the blacklist.
func (n *Node) UnblacklistPeer(peerID peer.ID) error {
	return n.addressBook.UnblacklistPeer(peerID)
}

// GetPeer retrieves peer information from the address book.
func (n *Node) GetPeer(peerID peer.ID) (*addressbook.PeerEntry, error) {
	return n.addressBook.GetPeer(peerID)
}

// ListPeers returns all non-blacklisted peers.
func (n *Node) ListPeers() []*addressbook.PeerEntry {
	return n.addressBook.ListPeers()
}

// Connect establishes a connection to a peer and returns a handshake stream.
// The application must complete the handshake within the configured timeout.
func (n *Node) Connect(peerID peer.ID) (*streams.HandshakeStream, error) {
	n.startMu.Lock()
	if !n.started {
		n.startMu.Unlock()
		return nil, ErrNodeNotStarted
	}
	n.startMu.Unlock()

	return n.connections.Connect(peerID)
}

// Disconnect closes the connection to a peer.
func (n *Node) Disconnect(peerID peer.ID) error {
	return n.connections.Disconnect(peerID)
}

// ConnectionState returns the current connection state for a peer.
func (n *Node) ConnectionState(peerID peer.ID) ConnectionState {
	return ConnectionState(n.connections.GetState(peerID))
}

// CancelReconnection cancels any ongoing reconnection attempts for a peer.
func (n *Node) CancelReconnection(peerID peer.ID) error {
	return n.connections.CancelReconnection(peerID)
}

// EstablishEncryptedStreams derives a shared key and establishes encrypted streams.
// This should be called after successful handshake with the peer's Ed25519 public key.
// It also registers handlers for incoming streams from the remote peer.
func (n *Node) EstablishEncryptedStreams(
	peerID peer.ID,
	peerPubKey ed25519.PublicKey,
	streamNames []string,
) error {
	n.startMu.Lock()
	if !n.started {
		n.startMu.Unlock()
		return ErrNodeNotStarted
	}
	n.startMu.Unlock()

	if len(streamNames) == 0 {
		return ErrNoStreamsRequested
	}

	// Check if this is an incoming connection (peer is in Disconnected or unknown state)
	// If so, register it and transition to Handshaking state
	currentState := n.connections.GetState(peerID)
	if currentState == connection.StateDisconnected {
		// This is an incoming connection - register it
		_ = n.connections.RegisterIncomingConnection(peerID)
	}

	// Derive shared key (this also caches it in crypto module)
	sharedKey, err := n.crypto.DeriveSharedKey(peerPubKey)
	if err != nil {
		return fmt.Errorf("failed to derive shared key: %w", err)
	}

	// Store shared key in connection manager for incoming stream handling
	if err := n.connections.SetSharedKey(peerID, sharedKey); err != nil {
		return fmt.Errorf("failed to store shared key: %w", err)
	}

	// Register handlers for incoming encrypted streams
	// This allows the remote peer to open streams to us
	n.registerIncomingStreamHandlers(streamNames)

	// Establish encrypted streams via stream manager
	if err := n.streamManager.EstablishStreams(peerID, peerPubKey, streamNames); err != nil {
		return err
	}

	// Store public key in address book (ignore error - not critical)
	_ = n.addressBook.UpdatePublicKey(peerID, peerPubKey)

	// Update connection state to Established (ignore error - streams already established)
	_ = n.connections.MarkEstablished(peerID)

	return nil
}

// registerIncomingStreamHandlers registers protocol handlers for incoming encrypted streams.
func (n *Node) registerIncomingStreamHandlers(streamNames []string) {
	for _, streamName := range streamNames {
		protoID := protocol.StreamProtocolID(streamName)

		// Create handler for this stream
		handler := func(stream network.Stream) {
			remotePeerID := stream.Conn().RemotePeer()

			// Accept the incoming stream
			// The stream manager will check for shared key and allowed streams
			if err := n.streamManager.HandleIncomingStream(remotePeerID, streamName, stream); err != nil {
				// Failed to create encrypted stream (no shared key or not allowed)
				_ = stream.Reset() // Ignore error - error path
				return
			}
		}

		n.host.LibP2PHost().SetStreamHandler(protoID, handler)
	}
}

// Send sends data over an encrypted stream to a peer.
func (n *Node) Send(peerID peer.ID, streamName string, data []byte) error {
	n.startMu.Lock()
	if !n.started {
		n.startMu.Unlock()
		return ErrNodeNotStarted
	}
	n.startMu.Unlock()

	return n.streamManager.Send(peerID, streamName, data)
}

// Messages returns the channel for receiving incoming messages.
// The application should read from this channel to receive messages.
func (n *Node) Messages() <-chan streams.IncomingMessage {
	return n.messages
}

// Events returns the channel for receiving connection events.
// The application should read from this channel to receive state change notifications.
func (n *Node) Events() <-chan ConnectionEvent {
	// Create a conversion goroutine to convert internal events to public events
	publicEvents := make(chan ConnectionEvent, cap(n.messages))

	go func() {
		for evt := range n.events {
			publicEvents <- ConnectionEvent{
				PeerID:    evt.PeerID,
				State:     ConnectionState(evt.State),
				Error:     evt.Error,
				Timestamp: evt.Timestamp,
			}
		}
		close(publicEvents)
	}()

	return publicEvents
}

// IncomingHandshakes returns the channel for receiving incoming handshake streams.
// The application should read from this channel to handle incoming connection requests.
func (n *Node) IncomingHandshakes() <-chan protocol.IncomingHandshake {
	return n.incomingHandshakes
}
