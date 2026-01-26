package streams

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"sync"

	"github.com/blockberries/glueberry/pkg/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// StreamOpener defines the interface for opening streams to peers.
type StreamOpener interface {
	NewStream(ctx context.Context, peerID peer.ID, streamName string) (network.Stream, error)
}

// Manager manages encrypted and unencrypted streams for all connected peers.
// All public methods are thread-safe.
type Manager struct {
	host    StreamOpener
	crypto  *crypto.Module
	streams map[peer.ID]map[string]*EncryptedStream

	// unencryptedStreams tracks unencrypted streams (e.g., "handshake" stream)
	unencryptedStreams map[peer.ID]map[string]*UnencryptedStream

	// allowedStreams tracks which stream names are allowed for each peer
	allowedStreams map[peer.ID]map[string]bool

	// sharedKeys stores the encryption key for each peer
	sharedKeys map[peer.ID][]byte

	mu sync.RWMutex

	incoming chan IncomingMessage
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewManager creates a new stream manager.
func NewManager(
	ctx context.Context,
	host StreamOpener,
	cryptoModule *crypto.Module,
	incomingChan chan IncomingMessage,
) *Manager {
	managerCtx, cancel := context.WithCancel(ctx)

	return &Manager{
		host:               host,
		crypto:             cryptoModule,
		streams:            make(map[peer.ID]map[string]*EncryptedStream),
		unencryptedStreams: make(map[peer.ID]map[string]*UnencryptedStream),
		allowedStreams:     make(map[peer.ID]map[string]bool),
		sharedKeys:         make(map[peer.ID][]byte),
		incoming:           incomingChan,
		ctx:                managerCtx,
		cancel:             cancel,
	}
}

// EstablishStreams registers the shared key and allowed stream names for a peer.
// Streams are opened lazily on first Send() or when received from the remote peer.
// This makes the API symmetric - both peers can call this in any order.
func (m *Manager) EstablishStreams(
	peerID peer.ID,
	peerPubKey ed25519.PublicKey,
	streamNames []string,
) error {
	if len(streamNames) == 0 {
		return fmt.Errorf("no stream names provided")
	}

	// Derive shared key
	sharedKey, err := m.crypto.DeriveSharedKey(peerPubKey)
	if err != nil {
		return fmt.Errorf("failed to derive shared key: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Store shared key
	m.sharedKeys[peerID] = sharedKey

	// Initialize peer streams map if needed
	if _, ok := m.streams[peerID]; !ok {
		m.streams[peerID] = make(map[string]*EncryptedStream)
	}

	// Store allowed stream names
	if _, ok := m.allowedStreams[peerID]; !ok {
		m.allowedStreams[peerID] = make(map[string]bool)
	}
	for _, name := range streamNames {
		m.allowedStreams[peerID][name] = true
	}

	return nil
}

// Send sends data over a stream to a peer.
// If the stream doesn't exist, it is opened lazily.
// The "handshake" stream is unencrypted; all other streams are encrypted.
func (m *Manager) Send(peerID peer.ID, streamName string, data []byte) error {
	// Route handshake stream to unencrypted path
	if streamName == HandshakeStreamName {
		return m.sendUnencrypted(peerID, streamName, data)
	}

	// First try to get existing encrypted stream
	m.mu.RLock()
	peerStreams, hasPeerStreams := m.streams[peerID]
	var stream *EncryptedStream
	if hasPeerStreams {
		stream = peerStreams[streamName]
	}
	m.mu.RUnlock()

	// If stream exists and is not closed, use it
	if stream != nil && !stream.IsClosed() {
		return stream.Send(data)
	}

	// Stream doesn't exist or is closed - open it lazily
	stream, err := m.openStreamLazily(peerID, streamName)
	if err != nil {
		return err
	}

	return stream.Send(data)
}

// sendUnencrypted sends data over an unencrypted stream.
func (m *Manager) sendUnencrypted(peerID peer.ID, streamName string, data []byte) error {
	// First try to get existing stream
	m.mu.RLock()
	peerStreams, hasPeerStreams := m.unencryptedStreams[peerID]
	var stream *UnencryptedStream
	if hasPeerStreams {
		stream = peerStreams[streamName]
	}
	m.mu.RUnlock()

	// If stream exists and is not closed, use it
	if stream != nil && !stream.IsClosed() {
		return stream.Send(data)
	}

	// Stream doesn't exist or is closed - open it lazily
	stream, err := m.openUnencryptedStreamLazily(peerID, streamName)
	if err != nil {
		return err
	}

	return stream.Send(data)
}

// openUnencryptedStreamLazily opens an unencrypted stream on-demand.
func (m *Manager) openUnencryptedStreamLazily(peerID peer.ID, streamName string) (*UnencryptedStream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check stream doesn't exist
	if peerStreams, ok := m.unencryptedStreams[peerID]; ok {
		if stream, exists := peerStreams[streamName]; exists && !stream.IsClosed() {
			return stream, nil
		}
	}

	// Open libp2p stream
	rawStream, err := m.host.NewStream(m.ctx, peerID, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream %q: %w", streamName, err)
	}

	// Create unencrypted stream
	unencStream := NewUnencryptedStream(
		m.ctx,
		streamName,
		peerID,
		rawStream,
		m.incoming,
	)

	// Store the stream
	if _, ok := m.unencryptedStreams[peerID]; !ok {
		m.unencryptedStreams[peerID] = make(map[string]*UnencryptedStream)
	}
	m.unencryptedStreams[peerID][streamName] = unencStream

	return unencStream, nil
}

// openStreamLazily opens a stream on-demand.
func (m *Manager) openStreamLazily(peerID peer.ID, streamName string) (*EncryptedStream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check stream doesn't exist (may have been created while waiting for lock)
	if peerStreams, ok := m.streams[peerID]; ok {
		if stream, exists := peerStreams[streamName]; exists && !stream.IsClosed() {
			return stream, nil
		}
	}

	// Check if this stream name is allowed
	if allowedStreams, ok := m.allowedStreams[peerID]; !ok || !allowedStreams[streamName] {
		return nil, fmt.Errorf("stream %q not allowed for peer %s (call EstablishStreams first)", streamName, peerID)
	}

	// Get shared key
	sharedKey, ok := m.sharedKeys[peerID]
	if !ok {
		return nil, fmt.Errorf("no shared key for peer %s (call EstablishStreams first)", peerID)
	}

	// Open libp2p stream
	rawStream, err := m.host.NewStream(m.ctx, peerID, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream %q: %w", streamName, err)
	}

	// Create encrypted stream
	encStream, err := NewEncryptedStream(
		m.ctx,
		streamName,
		peerID,
		rawStream,
		sharedKey,
		m.incoming,
	)
	if err != nil {
		rawStream.Close()
		return nil, fmt.Errorf("failed to create encrypted stream %q: %w", streamName, err)
	}

	// Store the stream
	if _, ok := m.streams[peerID]; !ok {
		m.streams[peerID] = make(map[string]*EncryptedStream)
	}
	m.streams[peerID][streamName] = encStream

	return encStream, nil
}

// OpenHandshakeStream opens the built-in unencrypted handshake stream for a peer.
// This is called when a connection reaches StateConnected.
// The stream is opened lazily on first Send, but this method ensures the peer
// is registered for the handshake stream.
func (m *Manager) OpenHandshakeStream(peerID peer.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize peer streams map if needed
	if _, ok := m.unencryptedStreams[peerID]; !ok {
		m.unencryptedStreams[peerID] = make(map[string]*UnencryptedStream)
	}

	return nil
}

// CloseHandshakeStream closes the handshake stream for a peer.
// This is called when CompleteHandshake is invoked.
func (m *Manager) CloseHandshakeStream(peerID peer.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	peerStreams, ok := m.unencryptedStreams[peerID]
	if !ok {
		return nil // No handshake stream exists
	}

	stream, exists := peerStreams[HandshakeStreamName]
	if !exists || stream == nil {
		return nil
	}

	err := stream.Close()
	delete(peerStreams, HandshakeStreamName)

	return err
}

// HandleIncomingHandshakeStream handles an incoming handshake stream from a remote peer.
func (m *Manager) HandleIncomingHandshakeStream(
	peerID peer.ID,
	stream network.Stream,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get or create peer streams map
	peerStreams, ok := m.unencryptedStreams[peerID]
	if !ok {
		peerStreams = make(map[string]*UnencryptedStream)
		m.unencryptedStreams[peerID] = peerStreams
	}

	// Check if stream already exists
	if existing, exists := peerStreams[HandshakeStreamName]; exists {
		if !existing.IsClosed() {
			return fmt.Errorf("handshake stream already exists for peer %s", peerID)
		}
		// Stream exists but is closed - replace it
	}

	// Create unencrypted stream
	unencStream := NewUnencryptedStream(
		m.ctx,
		HandshakeStreamName,
		peerID,
		stream,
		m.incoming,
	)

	peerStreams[HandshakeStreamName] = unencStream
	return nil
}

// CloseStreams closes all streams (encrypted and unencrypted) for a peer.
func (m *Manager) CloseStreams(peerID peer.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error

	// Close encrypted streams
	if peerStreams, ok := m.streams[peerID]; ok {
		for _, stream := range peerStreams {
			if err := stream.Close(); err != nil {
				lastErr = err
			}
		}
		delete(m.streams, peerID)
	}

	// Close unencrypted streams
	if peerStreams, ok := m.unencryptedStreams[peerID]; ok {
		for _, stream := range peerStreams {
			if err := stream.Close(); err != nil {
				lastErr = err
			}
		}
		delete(m.unencryptedStreams, peerID)
	}

	// Remove from other maps
	delete(m.allowedStreams, peerID)

	// Securely zero shared key before removing
	if key, ok := m.sharedKeys[peerID]; ok {
		for i := range key {
			key[i] = 0
		}
	}
	delete(m.sharedKeys, peerID)

	return lastErr
}

// GetStream returns the encrypted stream for a peer and stream name.
// Returns nil if not found.
func (m *Manager) GetStream(peerID peer.ID, streamName string) *EncryptedStream {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peerStreams, ok := m.streams[peerID]
	if !ok {
		return nil
	}

	return peerStreams[streamName]
}

// HasStreams returns true if there are any streams for the peer.
func (m *Manager) HasStreams(peerID peer.ID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peerStreams, ok := m.streams[peerID]
	return ok && len(peerStreams) > 0
}

// ListStreams returns the names of all streams for a peer.
func (m *Manager) ListStreams(peerID peer.ID) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peerStreams, ok := m.streams[peerID]
	if !ok {
		return nil
	}

	names := make([]string, 0, len(peerStreams))
	for name := range peerStreams {
		names = append(names, name)
	}
	return names
}

// HandleIncomingStream handles an incoming stream request from a remote peer.
// This is called by the protocol handler when a remote peer opens a stream.
// The shared key must have been previously established via EstablishStreams.
func (m *Manager) HandleIncomingStream(
	peerID peer.ID,
	streamName string,
	stream network.Stream,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if this stream name is allowed
	if allowedStreams, ok := m.allowedStreams[peerID]; !ok || !allowedStreams[streamName] {
		return fmt.Errorf("stream %q not allowed for peer %s (call EstablishStreams first)", streamName, peerID)
	}

	// Get shared key
	sharedKey, ok := m.sharedKeys[peerID]
	if !ok {
		return fmt.Errorf("no shared key for peer %s (call EstablishStreams first)", peerID)
	}

	// Get or create peer streams map
	peerStreams, ok := m.streams[peerID]
	if !ok {
		peerStreams = make(map[string]*EncryptedStream)
		m.streams[peerID] = peerStreams
	}

	// Check if stream already exists
	if existing, exists := peerStreams[streamName]; exists {
		if !existing.IsClosed() {
			return fmt.Errorf("stream %q already exists for peer %s", streamName, peerID)
		}
		// Stream exists but is closed - replace it
	}

	// Create encrypted stream
	encStream, err := NewEncryptedStream(
		m.ctx,
		streamName,
		peerID,
		stream,
		sharedKey,
		m.incoming,
	)
	if err != nil {
		return fmt.Errorf("failed to create encrypted stream: %w", err)
	}

	peerStreams[streamName] = encStream
	return nil
}

// Shutdown closes all streams and stops the manager.
// All key material is securely zeroed before release.
func (m *Manager) Shutdown() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all encrypted streams
	for _, peerStreams := range m.streams {
		for _, stream := range peerStreams {
			stream.Close()
		}
	}

	// Close all unencrypted streams
	for _, peerStreams := range m.unencryptedStreams {
		for _, stream := range peerStreams {
			stream.Close()
		}
	}

	// Securely zero all shared keys before clearing the map
	for _, key := range m.sharedKeys {
		for i := range key {
			key[i] = 0
		}
	}

	m.streams = make(map[peer.ID]map[string]*EncryptedStream)
	m.unencryptedStreams = make(map[peer.ID]map[string]*UnencryptedStream)
	m.allowedStreams = make(map[peer.ID]map[string]bool)
	m.sharedKeys = make(map[peer.ID][]byte)
}
