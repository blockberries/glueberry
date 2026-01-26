package glueberry

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync"
	"time"

	"github.com/blockberries/glueberry/internal/eventdispatch"
	"github.com/blockberries/glueberry/internal/flow"
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

	// Channels
	internalEvents <-chan eventdispatch.ConnectionEvent // from dispatcher
	events         chan ConnectionEvent                 // external channel for application
	internalMsgs   chan streams.IncomingMessage         // internal channel from stream manager
	messages       chan streams.IncomingMessage         // external channel for application

	// Event subscribers for filtered events
	eventSubs   []chan ConnectionEvent
	eventSubsMu sync.RWMutex

	// Stats tracking
	peerStats   map[peer.ID]*PeerStatsTracker
	peerStatsMu sync.RWMutex

	// Flow control
	flowControllers   map[string]*flow.Controller
	flowControllersMu sync.RWMutex

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

	// Create event channels - internal from dispatcher, external for application
	externalEventsChan := make(chan ConnectionEvent, cfg.EventBufferSize)

	// Create message channels - internal for stream manager, external for application
	internalMsgsChan := make(chan streams.IncomingMessage, cfg.MessageBufferSize)
	externalMsgsChan := make(chan streams.IncomingMessage, cfg.MessageBufferSize)

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

	// Create stream manager - uses internal messages channel
	streamMgr := streams.NewManager(ctx, libp2pHost, cryptoModule, internalMsgsChan)

	// Wire up decryption error callback for logging and metrics
	streamMgr.SetDecryptionErrorCallback(func(peerID peer.ID, err error) {
		if cfg.Logger != nil {
			cfg.Logger.Warn("decryption failed", "peer_id", peerID.String(), "error", err.Error())
		}
		if cfg.Metrics != nil {
			cfg.Metrics.DecryptionError()
		}
	})

	// Create connection manager
	connMgrConfig := connection.ManagerConfig{
		HandshakeTimeout:        cfg.HandshakeTimeout,
		ReconnectBaseDelay:      cfg.ReconnectBaseDelay,
		ReconnectMaxDelay:       cfg.ReconnectMaxDelay,
		ReconnectMaxAttempts:    cfg.ReconnectMaxAttempts,
		FailedHandshakeCooldown: cfg.FailedHandshakeCooldown,
	}

	connMgr := connection.NewManager(ctx, libp2pHost, addrBook, connMgrConfig, dispatcher)

	// Wire up connection deduplication: gater checks if we're already connecting
	gater.SetStateChecker(connMgr)

	node := &Node{
		config:          cfg,
		logger:          cfg.Logger,
		metrics:         cfg.Metrics,
		crypto:          cryptoModule,
		addressBook:     addrBook,
		host:            libp2pHost,
		connections:     connMgr,
		streamManager:   streamMgr,
		eventDispatch:   dispatcher,
		internalEvents:  dispatcher.Events(),
		events:          externalEventsChan,
		internalMsgs:    internalMsgsChan,
		messages:        externalMsgsChan,
		peerStats:       make(map[peer.ID]*PeerStatsTracker),
		flowControllers: make(map[string]*flow.Controller),
		ctx:             ctx,
		cancel:          cancel,
		started:         false,
	}

	// Start forwarding goroutines
	go node.forwardMessagesWithStats()
	go node.forwardEvents()

	return node, nil
}

// Start starts the node and begins listening for connections.
// This must be called before the node can connect to peers or receive connections.
// After starting, the node will automatically connect to all peers in the address book.
func (n *Node) Start() error {
	n.startMu.Lock()
	defer n.startMu.Unlock()

	if n.started {
		return ErrNodeAlreadyStarted
	}

	// Register handshake stream protocol handler for incoming connections
	n.registerHandshakeStreamHandler()

	// Note: Encrypted stream handlers are registered dynamically when streams are established
	// because we need the shared key which is only available after handshake

	n.started = true

	// Log node started
	addrs := make([]string, len(n.host.Addrs()))
	for i, addr := range n.host.Addrs() {
		addrs[i] = addr.String()
	}
	n.logger.Info("node started", "peer_id", n.host.ID().String(), "listen_addrs", addrs)

	// Auto-connect to all peers in address book
	n.autoConnectToPeers()

	return nil
}

// registerHandshakeStreamHandler registers the protocol handler for incoming handshake streams.
func (n *Node) registerHandshakeStreamHandler() {
	protoID := protocol.StreamProtocolID(streams.HandshakeStreamName)
	handler := func(stream network.Stream) {
		remotePeerID := stream.Conn().RemotePeer()

		// Register the incoming connection
		_ = n.connections.RegisterIncomingConnection(remotePeerID)

		// Accept the incoming handshake stream
		if err := n.streamManager.HandleIncomingHandshakeStream(remotePeerID, stream); err != nil {
			_ = stream.Reset()
			return
		}
	}
	n.host.LibP2PHost().SetStreamHandler(protoID, handler)
}

// autoConnectToPeers initiates connections to all peers in the address book.
func (n *Node) autoConnectToPeers() {
	peers := n.addressBook.ListPeers()
	for _, entry := range peers {
		n.connections.AutoConnect(entry.PeerID)
	}
}

// Stop shuts down the node and releases all resources.
// It closes all connections, stops all goroutines, and cleans up state.
func (n *Node) Stop() error {
	n.startMu.Lock()
	defer n.startMu.Unlock()

	if !n.started {
		return ErrNodeNotStarted
	}

	n.logger.Info("node stopping", "peer_id", n.host.ID().String())

	// Cancel context to stop all goroutines
	n.cancel()

	// Shutdown components in reverse order of initialization
	n.connections.Shutdown()
	n.streamManager.Shutdown()
	n.eventDispatch.Close()

	// Close libp2p host first (before zeroing keys it might still reference)
	if err := n.host.Close(); err != nil {
		n.logger.Error("failed to close host", "error", err)
		return fmt.Errorf("failed to close host: %w", err)
	}

	// Close crypto module to zero key material (after libp2p is fully stopped)
	n.crypto.Close()

	// Note: messages channel is closed by forwardMessagesWithStats goroutine
	// when context is cancelled

	n.started = false

	n.logger.Info("node stopped")

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

// Version returns the Glueberry protocol version.
// Applications should exchange versions during handshake to verify compatibility.
func (n *Node) Version() ProtocolVersion {
	return CurrentVersion()
}

// AddPeer adds a peer to the address book.
// If the node is started, it will automatically attempt to connect to the peer.
func (n *Node) AddPeer(peerID peer.ID, addrs []multiaddr.Multiaddr, metadata map[string]string) error {
	if err := n.addressBook.AddPeer(peerID, addrs, metadata); err != nil {
		return err
	}

	n.logger.Info("peer added", "peer_id", peerID.String())

	// Auto-connect if node is started
	n.startMu.Lock()
	started := n.started
	n.startMu.Unlock()

	if started {
		n.connections.AutoConnect(peerID)
	}

	return nil
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

	n.logger.Warn("peer blacklisted", "peer_id", peerID.String())

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

// PeerAddrs returns the known addresses for a peer from libp2p's peerstore.
// This is useful for getting addresses of peers that connected to us (incoming connections)
// which may not be in our address book.
func (n *Node) PeerAddrs(peerID peer.ID) []multiaddr.Multiaddr {
	return n.host.LibP2PHost().Peerstore().Addrs(peerID)
}

// Connect establishes a connection to a peer.
// On success, the connection enters StateConnected and the "handshake" stream becomes available.
// The application should listen for StateConnected events and perform the handshake
// using Send() and Messages() on the "handshake" stream.
// The handshake must be completed within the configured timeout by calling CompleteHandshake().
func (n *Node) Connect(peerID peer.ID) error {
	return n.ConnectCtx(context.Background(), peerID)
}

// ConnectCtx establishes a connection to a peer with context support for cancellation.
// The provided context can be used to cancel the connection attempt or set a timeout.
// On success, the connection enters StateConnected and the "handshake" stream becomes available.
func (n *Node) ConnectCtx(ctx context.Context, peerID peer.ID) error {
	n.startMu.Lock()
	if !n.started {
		n.startMu.Unlock()
		return ErrNodeNotStarted
	}
	n.startMu.Unlock()

	return n.connections.ConnectCtx(ctx, peerID)
}

// Disconnect closes the connection to a peer.
func (n *Node) Disconnect(peerID peer.ID) error {
	return n.DisconnectCtx(context.Background(), peerID)
}

// DisconnectCtx closes the connection to a peer with context support for cancellation.
// The provided context can be used to cancel the operation if it takes too long.
func (n *Node) DisconnectCtx(ctx context.Context, peerID peer.ID) error {
	return n.connections.DisconnectCtx(ctx, peerID)
}

// PrepareStreams prepares encrypted streams for communication with a peer.
// This should be called when the peer's public key is received during handshake.
//
// The method:
// 1. Derives the shared encryption key from the peer's public key
// 2. Registers handlers for incoming encrypted streams
// 3. Stores the peer's public key in the address book
//
// After calling this method, the node is ready to send/receive encrypted messages,
// but the connection is not yet in StateEstablished. Call FinalizeHandshake() after
// receiving confirmation from the peer to complete the handshake.
//
// This two-phase approach ensures both peers are ready before transitioning to
// StateEstablished, avoiding race conditions where one peer sends encrypted messages
// before the other is ready to receive them.
func (n *Node) PrepareStreams(
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

	n.logger.Debug("preparing streams", "peer_id", peerID.String(), "streams", streamNames)

	// 1. Derive shared key
	sharedKey, err := n.crypto.DeriveSharedKey(peerPubKey)
	if err != nil {
		n.logger.Warn("handshake failed: key derivation error", "peer_id", peerID.String(), "error", err)
		return fmt.Errorf("failed to derive shared key: %w", err)
	}

	// 2. Store shared key in connection manager
	if err := n.connections.SetSharedKey(peerID, sharedKey); err != nil {
		return fmt.Errorf("failed to store shared key: %w", err)
	}

	// 3. Register handlers for incoming encrypted streams
	n.registerIncomingStreamHandlers(streamNames)

	// 4. Establish encrypted streams via stream manager (lazy opening)
	if err := n.streamManager.EstablishStreams(peerID, peerPubKey, streamNames); err != nil {
		return err
	}

	// 5. Store public key in address book
	_ = n.addressBook.UpdatePublicKey(peerID, peerPubKey)

	return nil
}

// FinalizeHandshake completes the handshake and transitions to StateEstablished.
// This should be called after receiving confirmation (e.g., Complete message) from the peer,
// indicating they have also called PrepareStreams and are ready for encrypted communication.
//
// The method:
// 1. Cancels the handshake timeout
// 2. Closes the unencrypted "handshake" stream
// 3. Transitions the connection to StateEstablished
func (n *Node) FinalizeHandshake(peerID peer.ID) error {
	n.startMu.Lock()
	if !n.started {
		n.startMu.Unlock()
		return ErrNodeNotStarted
	}
	n.startMu.Unlock()

	// 1. Cancel handshake timeout
	if err := n.connections.CancelHandshakeTimeout(peerID); err != nil {
		return fmt.Errorf("failed to cancel handshake timeout: %w", err)
	}

	// 2. Close handshake stream (non-fatal - may not exist if we were the receiver)
	_ = n.streamManager.CloseHandshakeStream(peerID)

	// 3. Transition to StateEstablished
	if err := n.connections.MarkEstablished(peerID); err != nil {
		return fmt.Errorf("failed to mark connection as established: %w", err)
	}

	n.logger.Info("handshake complete", "peer_id", peerID.String())

	return nil
}

// CompleteHandshake is a convenience method that calls PrepareStreams followed by FinalizeHandshake.
// Use this for simple cases where you want to complete the handshake in one step.
// For better control over timing (especially in scenarios where peers may complete at different times),
// use PrepareStreams and FinalizeHandshake separately.
func (n *Node) CompleteHandshake(
	peerID peer.ID,
	peerPubKey ed25519.PublicKey,
	streamNames []string,
) error {
	if err := n.PrepareStreams(peerID, peerPubKey, streamNames); err != nil {
		return err
	}
	return n.FinalizeHandshake(peerID)
}

// ConnectionState returns the current connection state for a peer.
func (n *Node) ConnectionState(peerID peer.ID) ConnectionState {
	return ConnectionState(n.connections.GetState(peerID))
}

// IsOutbound returns whether the connection to the peer was initiated by us (outbound)
// or by the remote peer (inbound). Returns false and an error if no connection exists.
//
// This is useful for protocols that need to differentiate behavior based on who
// initiated the connection (e.g., in Blockberry, the initiator sends HelloRequest first).
func (n *Node) IsOutbound(peerID peer.ID) (bool, error) {
	return n.connections.IsOutbound(peerID)
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
	return n.SendCtx(context.Background(), peerID, streamName, data)
}

// SendCtx sends data over an encrypted stream to a peer with context support for cancellation.
// The provided context can be used to cancel the send operation or set a timeout.
func (n *Node) SendCtx(ctx context.Context, peerID peer.ID, streamName string, data []byte) error {
	n.startMu.Lock()
	if !n.started {
		n.startMu.Unlock()
		return ErrNodeNotStarted
	}
	n.startMu.Unlock()

	// Check max message size
	if n.config.MaxMessageSize > 0 && len(data) > n.config.MaxMessageSize {
		return &Error{
			Code:    ErrCodeMessageTooLarge,
			Message: fmt.Sprintf("message size %d exceeds maximum %d", len(data), n.config.MaxMessageSize),
			PeerID:  peerID,
			Stream:  streamName,
		}
	}

	// Apply flow control if enabled (default is enabled, DisableBackpressure = false)
	if !n.config.DisableBackpressure {
		fc := n.getOrCreateFlowController(streamName)

		startWait := time.Now()
		if err := fc.Acquire(ctx); err != nil {
			if err == context.DeadlineExceeded || err == context.Canceled {
				return &Error{
					Code:      ErrCodeBackpressure,
					Message:   "send blocked by backpressure",
					PeerID:    peerID,
					Stream:    streamName,
					Cause:     err,
					Retriable: true,
				}
			}
			return err
		}

		// Record backpressure wait time if we waited
		waitTime := time.Since(startWait)
		if waitTime > time.Millisecond {
			n.metrics.BackpressureWait(streamName, waitTime.Seconds())
		}

		// Release after send (success or failure)
		defer func() {
			fc.Release()
			n.metrics.PendingMessages(streamName, fc.Pending())
		}()
	}

	err := n.streamManager.SendCtx(ctx, peerID, streamName, data)
	if err == nil {
		// Record stats on successful send
		tracker := n.getOrCreatePeerStats(peerID)
		tracker.RecordMessageSent(streamName, len(data))
	}
	return err
}

// Messages returns the channel for receiving incoming messages.
// The application should read from this channel to receive messages.
func (n *Node) Messages() <-chan streams.IncomingMessage {
	return n.messages
}

// Events returns the channel for receiving connection events.
// The application should read from this channel to receive state change notifications.
// This channel is shared - all consumers receive all events.
func (n *Node) Events() <-chan ConnectionEvent {
	return n.events
}

// PeerStatistics returns statistics for a specific peer.
// Returns nil if the peer has no recorded statistics.
func (n *Node) PeerStatistics(peerID peer.ID) *PeerStats {
	n.peerStatsMu.RLock()
	tracker := n.peerStats[peerID]
	n.peerStatsMu.RUnlock()

	if tracker == nil {
		return nil
	}

	// Get connection state
	conn := n.connections.GetConnection(peerID)
	connected := conn != nil && conn.GetState() == connection.StateEstablished
	isOutbound := conn != nil && conn.GetIsOutbound()

	return tracker.Snapshot(peerID, connected, isOutbound)
}

// AllPeerStatistics returns statistics for all peers with recorded stats.
func (n *Node) AllPeerStatistics() map[peer.ID]*PeerStats {
	n.peerStatsMu.RLock()
	defer n.peerStatsMu.RUnlock()

	result := make(map[peer.ID]*PeerStats, len(n.peerStats))
	for peerID, tracker := range n.peerStats {
		// Get connection state
		conn := n.connections.GetConnection(peerID)
		connected := conn != nil && conn.GetState() == connection.StateEstablished
		isOutbound := conn != nil && conn.GetIsOutbound()

		result[peerID] = tracker.Snapshot(peerID, connected, isOutbound)
	}

	return result
}

// getOrCreatePeerStats returns the stats tracker for a peer, creating it if needed.
func (n *Node) getOrCreatePeerStats(peerID peer.ID) *PeerStatsTracker {
	n.peerStatsMu.Lock()
	defer n.peerStatsMu.Unlock()

	tracker := n.peerStats[peerID]
	if tracker == nil {
		tracker = NewPeerStatsTracker()
		n.peerStats[peerID] = tracker
	}
	return tracker
}

// getOrCreateFlowController returns the flow controller for a stream,
// creating one if it doesn't exist.
func (n *Node) getOrCreateFlowController(streamName string) *flow.Controller {
	// Fast path: check if controller exists
	n.flowControllersMu.RLock()
	fc := n.flowControllers[streamName]
	n.flowControllersMu.RUnlock()

	if fc != nil {
		return fc
	}

	// Slow path: create controller under write lock
	n.flowControllersMu.Lock()
	defer n.flowControllersMu.Unlock()

	// Double-check after acquiring write lock
	if fc = n.flowControllers[streamName]; fc != nil {
		return fc
	}

	fc = flow.NewController(n.config.HighWatermark, n.config.LowWatermark)

	// Set up metrics callback for backpressure events
	fc.SetBlockedCallback(func() {
		n.metrics.BackpressureEngaged(streamName)
	})

	n.flowControllers[streamName] = fc
	return fc
}

// forwardMessagesWithStats reads from internal messages channel,
// records stats, and forwards to the external channel.
func (n *Node) forwardMessagesWithStats() {
	for {
		select {
		case <-n.ctx.Done():
			close(n.messages)
			return
		case msg, ok := <-n.internalMsgs:
			if !ok {
				close(n.messages)
				return
			}

			// Record stats for received message
			tracker := n.getOrCreatePeerStats(msg.PeerID)
			tracker.RecordMessageReceived(msg.StreamName, len(msg.Data))

			// Forward to external channel (non-blocking to prevent deadlock)
			select {
			case n.messages <- msg:
				// Message delivered
			default:
				// Channel full - drop message
				// In production, might want to log this
			}
		}
	}
}

// forwardEvents reads from internal events channel and broadcasts
// to the main events channel and all subscribers.
func (n *Node) forwardEvents() {
	defer func() {
		close(n.events)
		// Close all subscriber channels
		n.eventSubsMu.Lock()
		for _, ch := range n.eventSubs {
			close(ch)
		}
		n.eventSubs = nil
		n.eventSubsMu.Unlock()
	}()

	for {
		select {
		case <-n.ctx.Done():
			return
		case evt, ok := <-n.internalEvents:
			if !ok {
				return
			}

			// Convert to public event type
			pubEvt := ConnectionEvent{
				PeerID:    evt.PeerID,
				State:     ConnectionState(evt.State),
				Error:     evt.Error,
				Timestamp: evt.Timestamp,
			}

			// Forward to main events channel (non-blocking)
			select {
			case n.events <- pubEvt:
				// Event delivered
			default:
				// Channel full - drop event
				if n.metrics != nil {
					n.metrics.EventDropped()
				}
			}

			// Forward to all subscribers (non-blocking)
			n.eventSubsMu.RLock()
			for _, ch := range n.eventSubs {
				select {
				case ch <- pubEvt:
					// Event delivered
				default:
					// Subscriber full - drop event for this subscriber
				}
			}
			n.eventSubsMu.RUnlock()
		}
	}
}

// subscribeEvents creates a new event subscriber channel.
func (n *Node) subscribeEvents() chan ConnectionEvent {
	ch := make(chan ConnectionEvent, n.config.EventBufferSize)
	n.eventSubsMu.Lock()
	n.eventSubs = append(n.eventSubs, ch)
	n.eventSubsMu.Unlock()
	return ch
}

// EventFilter specifies which events to receive.
// Nil slices match all values for that field.
type EventFilter struct {
	// PeerIDs filters events to only these peers. Nil means all peers.
	PeerIDs []peer.ID

	// States filters events to only these states. Nil means all states.
	States []ConnectionState
}

// matches returns true if the event matches this filter.
func (f EventFilter) matches(evt ConnectionEvent) bool {
	// Check peer filter
	if len(f.PeerIDs) > 0 {
		found := false
		for _, pid := range f.PeerIDs {
			if pid == evt.PeerID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check state filter
	if len(f.States) > 0 {
		found := false
		for _, s := range f.States {
			if s == evt.State {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// FilteredEvents returns a channel that receives only events matching the filter.
// The returned channel is closed when the node is stopped.
//
// This method creates a new subscriber that receives a copy of all events,
// so it can be used together with Events() without conflict. Each call to
// FilteredEvents creates a new subscription.
func (n *Node) FilteredEvents(filter EventFilter) <-chan ConnectionEvent {
	filtered := make(chan ConnectionEvent, n.config.EventBufferSize)

	// Subscribe to receive all events
	subscription := n.subscribeEvents()

	go func() {
		defer close(filtered)
		for evt := range subscription {
			if filter.matches(evt) {
				select {
				case filtered <- evt:
					// Event delivered
				default:
					// Channel full - drop event
				}
			}
		}
	}()

	return filtered
}

// EventsForPeer returns a channel that receives events only for the specified peer.
// This is a convenience wrapper around FilteredEvents.
func (n *Node) EventsForPeer(peerID peer.ID) <-chan ConnectionEvent {
	return n.FilteredEvents(EventFilter{PeerIDs: []peer.ID{peerID}})
}

// EventsForStates returns a channel that receives events only for the specified states.
// This is a convenience wrapper around FilteredEvents.
func (n *Node) EventsForStates(states ...ConnectionState) <-chan ConnectionEvent {
	return n.FilteredEvents(EventFilter{States: states})
}
