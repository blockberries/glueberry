package connection

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/blockberries/glueberry/pkg/addressbook"
	"github.com/blockberries/glueberry/pkg/streams"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// HostConnector defines the interface for connecting to peers via libp2p.
type HostConnector interface {
	ID() peer.ID
	Connect(ctx context.Context, pi peer.AddrInfo) error
	Disconnect(peerID peer.ID) error
	IsConnected(peerID peer.ID) bool
	NewHandshakeStream(ctx context.Context, peerID peer.ID) (network.Stream, error)
}

// EventEmitter defines the interface for emitting connection events.
type EventEmitter interface {
	EmitEvent(event Event)
}

// Event represents a connection state change event.
type Event struct {
	PeerID    peer.ID
	State     ConnectionState
	Error     error
	Timestamp time.Time
}

// ManagerConfig contains configuration for the Connection Manager.
type ManagerConfig struct {
	HandshakeTimeout        time.Duration
	ReconnectBaseDelay      time.Duration
	ReconnectMaxDelay       time.Duration
	ReconnectMaxAttempts    int
	FailedHandshakeCooldown time.Duration
}

// Manager manages peer connections, reconnection, and handshake timeouts.
// All public methods are thread-safe.
type Manager struct {
	host        HostConnector
	addressBook *addressbook.Book
	config      ManagerConfig
	events      EventEmitter

	connections  map[peer.ID]*PeerConnection
	reconnecting map[peer.ID]*ReconnectState
	mu           sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// NewManager creates a new connection manager.
func NewManager(
	ctx context.Context,
	host HostConnector,
	addressBook *addressbook.Book,
	config ManagerConfig,
	events EventEmitter,
) *Manager {
	managerCtx, cancel := context.WithCancel(ctx)

	return &Manager{
		host:         host,
		addressBook:  addressBook,
		config:       config,
		events:       events,
		connections:  make(map[peer.ID]*PeerConnection),
		reconnecting: make(map[peer.ID]*ReconnectState),
		ctx:          managerCtx,
		cancel:       cancel,
	}
}

// Connect establishes a connection to a peer and opens a handshake stream.
// Returns the handshake stream for the application to perform handshaking.
//
// The connection must complete the handshake (via EstablishEncryptedStreams)
// within the configured HandshakeTimeout, or the connection will be dropped
// and the peer will enter cooldown.
func (m *Manager) Connect(peerID peer.ID) (*streams.HandshakeStream, error) {
	m.mu.Lock()

	// Check if peer is blacklisted
	if m.addressBook.IsBlacklisted(peerID) {
		m.mu.Unlock()
		return nil, fmt.Errorf("peer %s is blacklisted", peerID)
	}

	// Check if already connected
	if conn, exists := m.connections[peerID]; exists {
		state := conn.GetState()
		if state.IsActive() {
			m.mu.Unlock()
			return nil, fmt.Errorf("already connected to peer %s (state: %s)", peerID, state)
		}

		// If in cooldown, check if cooldown expired
		if state == StateCooldown {
			if conn.IsInCooldown() {
				remaining := conn.CooldownRemaining()
				m.mu.Unlock()
				return nil, fmt.Errorf("peer %s is in cooldown for %v", peerID, remaining)
			}
			// Cooldown expired, allow connection
		}
	}

	// Get peer info from address book
	peerEntry, err := m.addressBook.GetPeer(peerID)
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("peer %s not in address book: %w", peerID, err)
	}

	// Create or get peer connection
	conn, exists := m.connections[peerID]
	if !exists {
		conn = NewPeerConnection(peerID)
		m.connections[peerID] = conn
	}

	// Transition to connecting
	if err := conn.TransitionTo(StateConnecting); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("cannot transition to connecting: %w", err)
	}

	m.mu.Unlock()

	// Emit connecting event
	m.emitEvent(peerID, StateConnecting, nil)

	// Build peer.AddrInfo
	addrInfo := peer.AddrInfo{
		ID:    peerID,
		Addrs: peerEntry.Multiaddrs,
	}

	// Establish libp2p connection
	connectCtx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	if err := m.host.Connect(connectCtx, addrInfo); err != nil {
		m.handleConnectFailure(peerID, err)
		return nil, fmt.Errorf("failed to connect to peer %s: %w", peerID, err)
	}

	// Update state to connected
	m.mu.Lock()
	if err := conn.TransitionTo(StateConnected); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("cannot transition to connected: %w", err)
	}
	m.mu.Unlock()

	m.emitEvent(peerID, StateConnected, nil)

	// Update last seen
	m.addressBook.UpdateLastSeen(peerID)

	// Open handshake stream
	streamCtx, streamCancel := context.WithTimeout(m.ctx, 10*time.Second)
	defer streamCancel()

	rawStream, err := m.host.NewHandshakeStream(streamCtx, peerID)
	if err != nil {
		m.handleConnectFailure(peerID, err)
		return nil, fmt.Errorf("failed to open handshake stream: %w", err)
	}

	// Wrap in HandshakeStream
	handshakeStream := streams.NewHandshakeStream(rawStream, m.config.HandshakeTimeout)

	// Set up handshake timeout enforcement
	m.setupHandshakeTimeout(conn, handshakeStream)

	// Set the handshake stream
	m.mu.Lock()
	if err := conn.SetHandshakeStream(handshakeStream); err != nil {
		m.mu.Unlock()
		handshakeStream.Close()
		return nil, fmt.Errorf("cannot set handshake stream: %w", err)
	}
	m.mu.Unlock()

	m.emitEvent(peerID, StateHandshaking, nil)

	return handshakeStream, nil
}

// Disconnect closes the connection to a peer and cleans up resources.
func (m *Manager) Disconnect(peerID peer.ID) error {
	m.mu.Lock()
	conn, exists := m.connections[peerID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("not connected to peer %s", peerID)
	}

	// Cancel any ongoing reconnection
	if reconnState, ok := m.reconnecting[peerID]; ok {
		if reconnState.Cancel != nil {
			reconnState.Cancel()
		}
		delete(m.reconnecting, peerID)
	}

	m.mu.Unlock()

	// Cleanup connection resources
	conn.Cleanup()

	// Close libp2p connection
	if err := m.host.Disconnect(peerID); err != nil {
		// Log but don't fail
	}

	// Update state
	m.mu.Lock()
	conn.TransitionTo(StateDisconnected)
	m.mu.Unlock()

	m.emitEvent(peerID, StateDisconnected, nil)

	return nil
}

// GetState returns the current state of a peer connection.
// Returns StateDisconnected if no connection exists.
func (m *Manager) GetState(peerID peer.ID) ConnectionState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if conn, exists := m.connections[peerID]; exists {
		return conn.GetState()
	}
	return StateDisconnected
}

// CancelReconnection cancels any ongoing reconnection attempts for a peer.
func (m *Manager) CancelReconnection(peerID peer.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	reconnState, exists := m.reconnecting[peerID]
	if !exists {
		return fmt.Errorf("peer %s is not reconnecting", peerID)
	}

	if reconnState.Cancel != nil {
		reconnState.Cancel()
	}
	delete(m.reconnecting, peerID)

	// Update connection state
	if conn, ok := m.connections[peerID]; ok {
		conn.TransitionTo(StateDisconnected)
	}

	return nil
}

// setupHandshakeTimeout sets up a timer to enforce handshake timeout.
func (m *Manager) setupHandshakeTimeout(conn *PeerConnection, hs *streams.HandshakeStream) {
	timeoutCtx, cancel := context.WithCancel(m.ctx)
	conn.HandshakeCancel = cancel

	timer := time.AfterFunc(m.config.HandshakeTimeout, func() {
		select {
		case <-timeoutCtx.Done():
			// Timeout cancelled (handshake completed or connection closed)
			return
		default:
			// Timeout fired - handle it
			m.handleHandshakeTimeout(conn.PeerID)
		}
	})

	conn.HandshakeTimer = timer
}

// handleHandshakeTimeout handles a handshake timeout.
func (m *Manager) handleHandshakeTimeout(peerID peer.ID) {
	m.mu.Lock()
	conn, exists := m.connections[peerID]
	if !exists {
		m.mu.Unlock()
		return
	}

	// Only handle if still in handshaking state
	if conn.GetState() != StateHandshaking {
		m.mu.Unlock()
		return
	}

	m.mu.Unlock()

	// Close handshake stream
	conn.ClearHandshakeStream()

	// Disconnect the peer
	m.host.Disconnect(peerID)

	// Transition to cooldown
	m.mu.Lock()
	conn.StartCooldown(m.config.FailedHandshakeCooldown)
	m.mu.Unlock()

	m.emitEvent(peerID, StateCooldown, fmt.Errorf("handshake timeout"))

	// Schedule reconnection after cooldown
	m.scheduleReconnectAfterCooldown(peerID, m.config.FailedHandshakeCooldown)
}

// handleConnectFailure handles a connection failure.
func (m *Manager) handleConnectFailure(peerID peer.ID, err error) {
	m.mu.Lock()
	conn, exists := m.connections[peerID]
	if exists {
		conn.SetError(err)
		conn.TransitionTo(StateDisconnected)
	}
	m.mu.Unlock()

	m.emitEvent(peerID, StateDisconnected, err)

	// Start reconnection if enabled
	m.startReconnection(peerID)
}

// emitEvent emits a connection event.
func (m *Manager) emitEvent(peerID peer.ID, state ConnectionState, err error) {
	if m.events != nil {
		m.events.EmitEvent(Event{
			PeerID:    peerID,
			State:     state,
			Error:     err,
			Timestamp: time.Now(),
		})
	}
}

// startReconnection initiates reconnection attempts for a peer.
func (m *Manager) startReconnection(peerID peer.ID) {
	// Check if peer is blacklisted
	if m.addressBook.IsBlacklisted(peerID) {
		return
	}

	m.mu.Lock()
	// Check if already reconnecting
	if _, exists := m.reconnecting[peerID]; exists {
		m.mu.Unlock()
		return
	}

	conn, connExists := m.connections[peerID]
	if !connExists {
		m.mu.Unlock()
		return
	}

	// Start reconnection goroutine
	reconnCtx, reconnCancel := context.WithCancel(m.ctx)
	reconnState := NewReconnectState(reconnCancel)
	m.reconnecting[peerID] = reconnState

	// Update connection state
	conn.ReconnectState = reconnState
	conn.TransitionTo(StateReconnecting)
	m.mu.Unlock()

	m.emitEvent(peerID, StateReconnecting, nil)

	// Start reconnection loop in goroutine
	go m.reconnectionLoop(reconnCtx, peerID, reconnState)
}

// reconnectionLoop attempts to reconnect to a peer with exponential backoff.
func (m *Manager) reconnectionLoop(ctx context.Context, peerID peer.ID, reconnState *ReconnectState) {
	backoff := NewBackoffCalculator(m.config.ReconnectBaseDelay, m.config.ReconnectMaxDelay)

	for {
		// Check if we should continue
		if !ShouldRetry(reconnState.Attempts, m.config.ReconnectMaxAttempts) {
			m.handleReconnectionExhausted(peerID)
			return
		}

		// Calculate next delay
		backoff.ScheduleNext(reconnState)

		// Wait for the backoff delay
		select {
		case <-ctx.Done():
			// Reconnection cancelled
			return
		case <-time.After(reconnState.CurrentDelay):
			// Backoff elapsed, proceed with attempt
		}

		// Check again if we should continue (in case blacklisted during wait)
		if m.addressBook.IsBlacklisted(peerID) {
			m.handleReconnectionCancelled(peerID)
			return
		}

		// Attempt to connect
		err := m.attemptReconnect(peerID)
		if err == nil {
			// Success - reconnection complete
			m.mu.Lock()
			delete(m.reconnecting, peerID)
			m.mu.Unlock()
			return
		}

		// Failed - continue loop for next attempt
	}
}

// attemptReconnect performs a single reconnection attempt.
func (m *Manager) attemptReconnect(peerID peer.ID) error {
	// Get peer info
	peerEntry, err := m.addressBook.GetPeer(peerID)
	if err != nil {
		return fmt.Errorf("peer not in address book: %w", err)
	}

	// Build peer.AddrInfo
	addrInfo := peer.AddrInfo{
		ID:    peerID,
		Addrs: peerEntry.Multiaddrs,
	}

	// Try to connect
	connectCtx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	if err := m.host.Connect(connectCtx, addrInfo); err != nil {
		return err
	}

	// Update state
	m.mu.Lock()
	conn := m.connections[peerID]
	if conn != nil {
		conn.TransitionTo(StateConnected)
	}
	m.mu.Unlock()

	m.emitEvent(peerID, StateConnected, nil)

	// Update last seen
	m.addressBook.UpdateLastSeen(peerID)

	// Reconnection successful - handshake stream will be opened when app calls Connect() again
	return nil
}

// scheduleReconnectAfterCooldown schedules reconnection after cooldown expires.
func (m *Manager) scheduleReconnectAfterCooldown(peerID peer.ID, cooldown time.Duration) {
	go func() {
		select {
		case <-m.ctx.Done():
			return
		case <-time.After(cooldown):
			// Cooldown expired, transition to disconnected and start reconnection
			m.mu.Lock()
			if conn, exists := m.connections[peerID]; exists {
				if conn.GetState() == StateCooldown {
					conn.TransitionTo(StateDisconnected)
					m.mu.Unlock()
					m.emitEvent(peerID, StateDisconnected, nil)
					m.startReconnection(peerID)
					return
				}
			}
			m.mu.Unlock()
		}
	}()
}

// handleReconnectionExhausted handles the case when max reconnection attempts are reached.
func (m *Manager) handleReconnectionExhausted(peerID peer.ID) {
	m.mu.Lock()
	delete(m.reconnecting, peerID)
	if conn, exists := m.connections[peerID]; exists {
		conn.TransitionTo(StateDisconnected)
		conn.SetError(fmt.Errorf("reconnection attempts exhausted"))
	}
	m.mu.Unlock()

	m.emitEvent(peerID, StateDisconnected, fmt.Errorf("max reconnection attempts reached"))
}

// handleReconnectionCancelled handles cancelled reconnection.
func (m *Manager) handleReconnectionCancelled(peerID peer.ID) {
	m.mu.Lock()
	delete(m.reconnecting, peerID)
	if conn, exists := m.connections[peerID]; exists {
		conn.TransitionTo(StateDisconnected)
	}
	m.mu.Unlock()

	m.emitEvent(peerID, StateDisconnected, nil)
}

// OnDisconnect should be called when a peer disconnects unexpectedly.
// It initiates reconnection if configured.
func (m *Manager) OnDisconnect(peerID peer.ID, err error) {
	m.mu.Lock()
	conn, exists := m.connections[peerID]
	if !exists {
		m.mu.Unlock()
		return
	}

	// Clean up resources
	conn.Cleanup()

	// Transition to disconnected
	conn.TransitionTo(StateDisconnected)
	conn.SetError(err)
	m.mu.Unlock()

	m.emitEvent(peerID, StateDisconnected, err)

	// Start reconnection
	m.startReconnection(peerID)
}

// Shutdown stops the manager and cancels all ongoing operations.
func (m *Manager) Shutdown() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Clean up all connections
	for _, conn := range m.connections {
		conn.Cleanup()
	}

	// Cancel all reconnections
	for _, reconnState := range m.reconnecting {
		if reconnState.Cancel != nil {
			reconnState.Cancel()
		}
	}

	m.connections = make(map[peer.ID]*PeerConnection)
	m.reconnecting = make(map[peer.ID]*ReconnectState)
}

// GetSharedKey retrieves the shared encryption key for a peer.
// Returns nil if no shared key exists (handshake not completed).
func (m *Manager) GetSharedKey(peerID peer.ID) []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if conn, exists := m.connections[peerID]; exists {
		return conn.GetSharedKey()
	}
	return nil
}

// SetSharedKey stores the shared encryption key for a peer.
// This should be called after successful handshake.
func (m *Manager) SetSharedKey(peerID peer.ID, key []byte) error {
	m.mu.RLock()
	conn, exists := m.connections[peerID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("peer %s not connected", peerID)
	}

	conn.SetSharedKey(key)
	return nil
}

// MarkEstablished transitions a connection to the Established state.
// This should be called after encrypted streams are successfully set up.
func (m *Manager) MarkEstablished(peerID peer.ID) error {
	m.mu.RLock()
	conn, exists := m.connections[peerID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("peer %s not connected", peerID)
	}

	// Clear handshake resources
	conn.ClearHandshakeStream()

	// Transition to established
	m.mu.Lock()
	err := conn.TransitionTo(StateEstablished)
	m.mu.Unlock()

	if err != nil {
		return err
	}

	m.emitEvent(peerID, StateEstablished, nil)
	return nil
}

// GetOrCreateConnection gets an existing connection or creates a new one for incoming connections.
func (m *Manager) GetOrCreateConnection(peerID peer.ID) *PeerConnection {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[peerID]
	if !exists {
		conn = NewPeerConnection(peerID)
		m.connections[peerID] = conn
	}
	return conn
}


