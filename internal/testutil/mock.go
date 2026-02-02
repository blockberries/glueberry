// Package testutil provides testing utilities for applications that use Glueberry.
// It includes mock implementations and helpers for unit testing.
package testutil

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"sync"
	"time"

	"github.com/blockberries/glueberry/pkg/addressbook"
	"github.com/blockberries/glueberry/pkg/streams"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// Sentinel errors for mock operations.
var (
	ErrPeerNotFound    = errors.New("peer not found")
	ErrPeerBlacklisted = errors.New("peer is blacklisted")
	ErrNotConnected    = errors.New("not connected to peer")
)

// SentMessage records a message that was sent via MockNode.
type SentMessage struct {
	PeerID     peer.ID
	StreamName string
	Data       []byte
	Timestamp  time.Time
}

// MockNode provides a mock implementation of Glueberry's Node for testing.
// It tracks all operations and allows simulating incoming messages and events.
type MockNode struct {
	mu sync.RWMutex

	// Identity
	peerID    peer.ID
	publicKey ed25519.PublicKey

	// State
	started bool
	addrs   []multiaddr.Multiaddr

	// Peer tracking
	peers       map[peer.ID]*mockPeerState
	connections map[peer.ID]bool
	blacklisted map[peer.ID]bool

	// Message tracking
	sentMessages []SentMessage

	// Channels for simulating incoming data
	messages chan streams.IncomingMessage
	events   chan ConnectionEvent

	// Error injection for testing error handling
	connectErr    error
	sendErr       error
	disconnectErr error
}

// mockPeerState tracks state for a mock peer.
type mockPeerState struct {
	Addrs      []multiaddr.Multiaddr
	Metadata   map[string]string
	PublicKey  []byte
	IsOutbound bool
	State      string
}

// ConnectionEvent mirrors the main package's ConnectionEvent for testing.
type ConnectionEvent struct {
	PeerID peer.ID
	State  string
	Error  error
}

// NewMockNode creates a new mock node with a randomly generated identity.
func NewMockNode() *MockNode {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)

	// Create a deterministic peer ID from the public key
	// Use a simpler approach that doesn't require crypto.PubKey interface
	peerID := peer.ID(pub[:20]) // Use first 20 bytes as peer ID (simplified)

	return &MockNode{
		peerID:      peerID,
		publicKey:   pub,
		peers:       make(map[peer.ID]*mockPeerState),
		connections: make(map[peer.ID]bool),
		blacklisted: make(map[peer.ID]bool),
		messages:    make(chan streams.IncomingMessage, 100),
		events:      make(chan ConnectionEvent, 100),
	}
}

// NewMockNodeWithID creates a new mock node with a specific peer ID.
func NewMockNodeWithID(peerID peer.ID, publicKey ed25519.PublicKey) *MockNode {
	return &MockNode{
		peerID:      peerID,
		publicKey:   publicKey,
		peers:       make(map[peer.ID]*mockPeerState),
		connections: make(map[peer.ID]bool),
		blacklisted: make(map[peer.ID]bool),
		messages:    make(chan streams.IncomingMessage, 100),
		events:      make(chan ConnectionEvent, 100),
	}
}

// Start simulates starting the node.
func (m *MockNode) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	return nil
}

// Stop simulates stopping the node.
func (m *MockNode) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = false
	close(m.messages)
	close(m.events)
	return nil
}

// PeerID returns the mock node's peer ID.
func (m *MockNode) PeerID() peer.ID {
	return m.peerID
}

// PublicKey returns the mock node's public key.
func (m *MockNode) PublicKey() ed25519.PublicKey {
	return m.publicKey
}

// Addrs returns the mock node's listen addresses.
func (m *MockNode) Addrs() []multiaddr.Multiaddr {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.addrs
}

// SetAddrs sets the mock node's listen addresses.
func (m *MockNode) SetAddrs(addrs []multiaddr.Multiaddr) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addrs = addrs
}

// AddPeer adds a peer to the mock address book.
func (m *MockNode) AddPeer(peerID peer.ID, addrs []multiaddr.Multiaddr, metadata map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.blacklisted[peerID] {
		return ErrPeerBlacklisted
	}

	m.peers[peerID] = &mockPeerState{
		Addrs:    addrs,
		Metadata: metadata,
	}
	return nil
}

// RemovePeer removes a peer from the mock address book.
func (m *MockNode) RemovePeer(peerID peer.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.peers[peerID]; !ok {
		return ErrPeerNotFound
	}
	delete(m.peers, peerID)
	delete(m.connections, peerID)
	return nil
}

// BlacklistPeer blacklists a peer.
func (m *MockNode) BlacklistPeer(peerID peer.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blacklisted[peerID] = true
	return nil
}

// UnblacklistPeer removes a peer from the blacklist.
func (m *MockNode) UnblacklistPeer(peerID peer.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.blacklisted, peerID)
	return nil
}

// GetPeer returns a mock peer entry.
func (m *MockNode) GetPeer(peerID peer.ID) (*addressbook.PeerEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.peers[peerID]
	if !ok {
		return nil, ErrPeerNotFound
	}

	return &addressbook.PeerEntry{
		PeerID:      peerID,
		Multiaddrs:  state.Addrs,
		Metadata:    state.Metadata,
		PublicKey:   state.PublicKey,
		Blacklisted: m.blacklisted[peerID],
	}, nil
}

// ListPeers returns all non-blacklisted mock peers.
func (m *MockNode) ListPeers() []*addressbook.PeerEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*addressbook.PeerEntry
	for peerID, state := range m.peers {
		if !m.blacklisted[peerID] {
			result = append(result, &addressbook.PeerEntry{
				PeerID:     peerID,
				Multiaddrs: state.Addrs,
				Metadata:   state.Metadata,
				PublicKey:  state.PublicKey,
			})
		}
	}
	return result
}

// Connect simulates connecting to a peer.
func (m *MockNode) Connect(peerID peer.ID) error {
	return m.ConnectCtx(context.Background(), peerID)
}

// ConnectCtx simulates connecting to a peer with context.
func (m *MockNode) ConnectCtx(ctx context.Context, peerID peer.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.connectErr != nil {
		return m.connectErr
	}

	if m.blacklisted[peerID] {
		return ErrPeerBlacklisted
	}

	m.connections[peerID] = true
	if state, ok := m.peers[peerID]; ok {
		state.IsOutbound = true
	}
	return nil
}

// Disconnect simulates disconnecting from a peer.
func (m *MockNode) Disconnect(peerID peer.ID) error {
	return m.DisconnectCtx(context.Background(), peerID)
}

// DisconnectCtx simulates disconnecting from a peer with context.
func (m *MockNode) DisconnectCtx(ctx context.Context, peerID peer.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.disconnectErr != nil {
		return m.disconnectErr
	}

	delete(m.connections, peerID)
	return nil
}

// IsConnected returns true if connected to the peer.
func (m *MockNode) IsConnected(peerID peer.ID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connections[peerID]
}

// IsOutbound returns true if the connection was initiated by us.
func (m *MockNode) IsOutbound(peerID peer.ID) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.peers[peerID]
	if !ok {
		return false, ErrPeerNotFound
	}
	return state.IsOutbound, nil
}

// Send simulates sending a message to a peer.
func (m *MockNode) Send(peerID peer.ID, streamName string, data []byte) error {
	return m.SendCtx(context.Background(), peerID, streamName, data)
}

// SendCtx simulates sending a message to a peer with context.
func (m *MockNode) SendCtx(ctx context.Context, peerID peer.ID, streamName string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.sendErr != nil {
		return m.sendErr
	}

	if !m.connections[peerID] {
		return ErrNotConnected
	}

	// Make a copy of the data
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	m.sentMessages = append(m.sentMessages, SentMessage{
		PeerID:     peerID,
		StreamName: streamName,
		Data:       dataCopy,
		Timestamp:  time.Now(),
	})
	return nil
}

// Messages returns the channel for receiving mock messages.
func (m *MockNode) Messages() <-chan streams.IncomingMessage {
	return m.messages
}

// Events returns the channel for receiving mock events.
func (m *MockNode) Events() <-chan ConnectionEvent {
	return m.events
}

// --- Test Helpers ---

// SimulateConnect simulates a peer connecting to this node (inbound connection).
func (m *MockNode) SimulateConnect(peerID peer.ID, addrs []multiaddr.Multiaddr) {
	m.mu.Lock()
	m.peers[peerID] = &mockPeerState{
		Addrs:      addrs,
		IsOutbound: false,
	}
	m.connections[peerID] = true
	m.mu.Unlock()

	// Send event
	select {
	case m.events <- ConnectionEvent{PeerID: peerID, State: "connected"}:
	default:
	}
}

// SimulateDisconnect simulates a peer disconnecting.
func (m *MockNode) SimulateDisconnect(peerID peer.ID) {
	m.mu.Lock()
	delete(m.connections, peerID)
	m.mu.Unlock()

	// Send event
	select {
	case m.events <- ConnectionEvent{PeerID: peerID, State: "disconnected"}:
	default:
	}
}

// SimulateMessage simulates receiving a message from a peer.
func (m *MockNode) SimulateMessage(peerID peer.ID, streamName string, data []byte) {
	// Make a copy of the data
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	select {
	case m.messages <- streams.IncomingMessage{
		PeerID:     peerID,
		StreamName: streamName,
		Data:       dataCopy,
		Timestamp:  time.Now(),
	}:
	default:
		// Channel full
	}
}

// SimulateEvent simulates a connection event.
func (m *MockNode) SimulateEvent(peerID peer.ID, state string, err error) {
	select {
	case m.events <- ConnectionEvent{PeerID: peerID, State: state, Error: err}:
	default:
	}
}

// SetConnectError sets an error to be returned by Connect.
func (m *MockNode) SetConnectError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectErr = err
}

// SetSendError sets an error to be returned by Send.
func (m *MockNode) SetSendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
}

// SetDisconnectError sets an error to be returned by Disconnect.
func (m *MockNode) SetDisconnectError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.disconnectErr = err
}

// SentMessages returns all messages that were sent.
func (m *MockNode) SentMessages() []SentMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]SentMessage, len(m.sentMessages))
	copy(result, m.sentMessages)
	return result
}

// ClearSentMessages clears the sent messages list.
func (m *MockNode) ClearSentMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sentMessages = nil
}

// AssertSent asserts that a message was sent to the specified peer and stream.
// Returns the matching messages.
func (m *MockNode) AssertSent(peerID peer.ID, streamName string) []SentMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matches []SentMessage
	for _, msg := range m.sentMessages {
		if msg.PeerID == peerID && msg.StreamName == streamName {
			matches = append(matches, msg)
		}
	}
	return matches
}

// AssertNotSent asserts that no messages were sent to the specified peer and stream.
// Returns true if no messages were sent.
func (m *MockNode) AssertNotSent(peerID peer.ID, streamName string) bool {
	return len(m.AssertSent(peerID, streamName)) == 0
}

// ConnectedPeers returns a list of connected peer IDs.
func (m *MockNode) ConnectedPeers() []peer.ID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []peer.ID
	for peerID := range m.connections {
		result = append(result, peerID)
	}
	return result
}

// Reset clears all state in the mock node.
func (m *MockNode) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.peers = make(map[peer.ID]*mockPeerState)
	m.connections = make(map[peer.ID]bool)
	m.blacklisted = make(map[peer.ID]bool)
	m.sentMessages = nil
	m.connectErr = nil
	m.sendErr = nil
	m.disconnectErr = nil
}
