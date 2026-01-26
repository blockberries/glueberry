package connection

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/blockberries/glueberry/pkg/addressbook"
	"github.com/libp2p/go-libp2p/core/peer"
)

// HostConnector defines the interface for connecting to peers via libp2p.
type HostConnector interface {
	ID() peer.ID
	Connect(ctx context.Context, pi peer.AddrInfo) error
	Disconnect(peerID peer.ID) error
	IsConnected(peerID peer.ID) bool
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

// Connect establishes a connection to a peer.
// The connection enters StateConnected when successful, and the app can then
// send/receive handshake messages via Send()/Messages() on the "handshake" stream.
//
// The connection must complete the handshake (via CompleteHandshake)
// within the configured HandshakeTimeout, or the connection will be dropped
// and the peer will enter cooldown.
func (m *Manager) Connect(peerID peer.ID) error {
	return m.ConnectCtx(context.Background(), peerID)
}

// ConnectCtx establishes a connection to a peer with context support for cancellation.
// The provided context can be used to cancel the connection attempt or set a timeout.
// If the context is cancelled before the connection completes, the operation returns
// the context's error.
func (m *Manager) ConnectCtx(ctx context.Context, peerID peer.ID) error {
	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.mu.Lock()

	// Check if peer is blacklisted
	if m.addressBook.IsBlacklisted(peerID) {
		m.mu.Unlock()
		return fmt.Errorf("peer %s is blacklisted", peerID)
	}

	// Check if already connected
	if conn, exists := m.connections[peerID]; exists {
		state := conn.GetState()
		if state.IsActive() {
			m.mu.Unlock()
			return fmt.Errorf("already connected to peer %s (state: %s)", peerID, state)
		}

		// If in cooldown, check if cooldown expired
		if state == StateCooldown {
			if conn.IsInCooldown() {
				remaining := conn.CooldownRemaining()
				m.mu.Unlock()
				return fmt.Errorf("peer %s is in cooldown for %v", peerID, remaining)
			}
			// Cooldown expired, allow connection
		}
	}

	// Get peer info from address book
	peerEntry, err := m.addressBook.GetPeer(peerID)
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("peer %s not in address book: %w", peerID, err)
	}

	// Create or get peer connection
	conn, exists := m.connections[peerID]
	if !exists {
		conn = NewPeerConnection(peerID)
		m.connections[peerID] = conn
	}

	// Mark as outbound connection (we initiated it)
	conn.SetIsOutbound(true)

	// Transition to connecting
	if err := conn.TransitionTo(StateConnecting); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("cannot transition to connecting: %w", err)
	}

	m.mu.Unlock()

	// Emit connecting event
	m.emitEvent(peerID, StateConnecting, nil)

	// Build peer.AddrInfo
	addrInfo := peer.AddrInfo{
		ID:    peerID,
		Addrs: peerEntry.Multiaddrs,
	}

	// Create a context that's cancelled when either the provided context or manager context is done.
	// Use a 30-second timeout as default if the provided context has no deadline.
	var connectCtx context.Context
	var cancel context.CancelFunc
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		// User provided a context with deadline, use it
		connectCtx, cancel = context.WithCancel(ctx)
	} else {
		// No deadline provided, use default timeout
		connectCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
	}
	defer cancel()

	// Also respect the manager's context
	go func() {
		select {
		case <-m.ctx.Done():
			cancel()
		case <-connectCtx.Done():
		}
	}()

	if err := m.host.Connect(connectCtx, addrInfo); err != nil {
		m.handleConnectFailure(peerID, err)
		return fmt.Errorf("failed to connect to peer %s: %w", peerID, err)
	}

	// Update state to connected
	m.mu.Lock()
	if err := conn.TransitionTo(StateConnected); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("cannot transition to connected: %w", err)
	}

	// Set up handshake timeout enforcement (starts at StateConnected)
	m.setupHandshakeTimeout(conn)
	m.mu.Unlock()

	m.emitEvent(peerID, StateConnected, nil)

	// Update last seen (ignore error - not critical)
	_ = m.addressBook.UpdateLastSeen(peerID)

	return nil
}

// Disconnect closes the connection to a peer and cleans up resources.
func (m *Manager) Disconnect(peerID peer.ID) error {
	return m.DisconnectCtx(context.Background(), peerID)
}

// DisconnectCtx closes the connection to a peer with context support for cancellation.
// The provided context can be used to cancel the operation if it takes too long.
func (m *Manager) DisconnectCtx(ctx context.Context, peerID peer.ID) error {
	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

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

	// Check context again before cleanup
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Cleanup connection resources
	conn.Cleanup()

	// Close libp2p connection (ignore error - cleanup path)
	_ = m.host.Disconnect(peerID)

	// Update state
	m.mu.Lock()
	_ = conn.TransitionTo(StateDisconnected) // Ignore error - we're in cleanup path
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

// IsConnecting returns true if we're currently attempting to connect to the peer.
// This is used by the connection gater for connection deduplication.
func (m *Manager) IsConnecting(peerID peer.ID) bool {
	return m.GetState(peerID) == StateConnecting
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
		_ = conn.TransitionTo(StateDisconnected) // Ignore error - reconnection cancelled
	}

	return nil
}

// setupHandshakeTimeout sets up a timer to enforce handshake timeout.
func (m *Manager) setupHandshakeTimeout(conn *PeerConnection) {
	timeoutCtx, cancel := context.WithCancel(m.ctx)

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

	// Set fields under connection lock to avoid race with CancelHandshakeTimeout
	conn.mu.Lock()
	conn.HandshakeCancel = cancel
	conn.HandshakeTimer = timer
	conn.mu.Unlock()
}

// handleHandshakeTimeout handles a handshake timeout.
func (m *Manager) handleHandshakeTimeout(peerID peer.ID) {
	m.mu.Lock()
	conn, exists := m.connections[peerID]
	if !exists {
		m.mu.Unlock()
		return
	}

	// Only handle if still in connected state (waiting for handshake to complete)
	if conn.GetState() != StateConnected {
		m.mu.Unlock()
		return
	}

	m.mu.Unlock()

	// Cancel handshake timeout resources
	conn.CancelHandshakeTimeout()

	// Disconnect the peer
	_ = m.host.Disconnect(peerID) // Ignore error - best effort disconnect

	// Transition to cooldown
	m.mu.Lock()
	_ = conn.StartCooldown(m.config.FailedHandshakeCooldown) // Ignore error - cleanup path
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
		_ = conn.TransitionTo(StateDisconnected) // Ignore error - in error path
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
	_ = conn.TransitionTo(StateReconnecting) // Ignore error - starting reconnection
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
		_ = conn.TransitionTo(StateConnected) // Ignore error - reconnection success
	}
	m.mu.Unlock()

	m.emitEvent(peerID, StateConnected, nil)

	// Update last seen (ignore error - not critical)
	_ = m.addressBook.UpdateLastSeen(peerID)

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
					_ = conn.TransitionTo(StateDisconnected) // Ignore error - cooldown expired
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
		_ = conn.TransitionTo(StateDisconnected) // Ignore error - reconnection exhausted
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
		_ = conn.TransitionTo(StateDisconnected) // Ignore error - reconnection cancelled
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
	_ = conn.TransitionTo(StateDisconnected) // Ignore error - handling disconnect
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
// Creates a connection entry if one doesn't exist (for incoming connections).
func (m *Manager) SetSharedKey(peerID peer.ID, key []byte) error {
	conn := m.GetOrCreateConnection(peerID)
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

	// Cancel handshake timeout
	conn.CancelHandshakeTimeout()

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
// GetConnection returns the connection for a peer, or nil if not found.
func (m *Manager) GetConnection(peerID peer.ID) *PeerConnection {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connections[peerID]
}

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

// RegisterIncomingConnection registers an incoming connection and transitions to StateConnected.
// This should be called when a new incoming connection is established.
// It also starts the handshake timeout.
func (m *Manager) RegisterIncomingConnection(peerID peer.ID) error {
	conn := m.GetOrCreateConnection(peerID)

	// Mark as inbound connection (they initiated it)
	conn.SetIsOutbound(false)

	// Transition through states to reach Connected
	state := conn.GetState()

	if state == StateDisconnected {
		_ = conn.TransitionTo(StateConnecting)
		_ = conn.TransitionTo(StateConnected)

		// Start handshake timeout for incoming connection
		m.setupHandshakeTimeout(conn)

		m.emitEvent(peerID, StateConnected, nil)
	}

	return nil
}

// AutoConnect attempts to connect to a peer if not already connected.
// This is used for automatic connection on AddPeer and Start.
// It does not return errors - connection failures are handled via events.
func (m *Manager) AutoConnect(peerID peer.ID) {
	state := m.GetState(peerID)
	if state.IsActive() {
		return // Already connected or connecting
	}

	// Attempt connection in background
	go func() {
		_ = m.Connect(peerID) // Errors handled via events
	}()
}

// CancelHandshakeTimeout cancels the handshake timeout for a peer.
// This is called when CompleteHandshake is invoked.
func (m *Manager) CancelHandshakeTimeout(peerID peer.ID) error {
	m.mu.RLock()
	conn, exists := m.connections[peerID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("peer %s not connected", peerID)
	}

	conn.CancelHandshakeTimeout()
	return nil
}

// IsOutbound returns whether the connection to the peer was initiated by us.
// Returns false and an error if no connection exists for the peer.
func (m *Manager) IsOutbound(peerID peer.ID) (bool, error) {
	m.mu.RLock()
	conn, exists := m.connections[peerID]
	m.mu.RUnlock()

	if !exists {
		return false, fmt.Errorf("peer %s not connected", peerID)
	}

	return conn.GetIsOutbound(), nil
}
