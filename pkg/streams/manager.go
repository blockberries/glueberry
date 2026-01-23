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

// Manager manages encrypted streams for all connected peers.
// All public methods are thread-safe.
type Manager struct {
	host    StreamOpener
	crypto  *crypto.Module
	streams map[peer.ID]map[string]*EncryptedStream
	mu      sync.RWMutex

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
		host:     host,
		crypto:   cryptoModule,
		streams:  make(map[peer.ID]map[string]*EncryptedStream),
		incoming: incomingChan,
		ctx:      managerCtx,
		cancel:   cancel,
	}
}

// EstablishStreams establishes encrypted streams to a peer.
// It derives a shared key from the peer's public key and opens the specified streams.
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

	// Check if streams already exist for this peer
	if existing, ok := m.streams[peerID]; ok && len(existing) > 0 {
		return fmt.Errorf("encrypted streams already established for peer %s", peerID)
	}

	// Create peer streams map
	peerStreams := make(map[string]*EncryptedStream)

	// Open each stream
	for _, name := range streamNames {
		// Open libp2p stream
		rawStream, err := m.host.NewStream(m.ctx, peerID, name)
		if err != nil {
			// Close any streams we've already opened
			for _, s := range peerStreams {
				s.Close()
			}
			return fmt.Errorf("failed to open stream %q: %w", name, err)
		}

		// Create encrypted stream
		encStream, err := NewEncryptedStream(
			m.ctx,
			name,
			peerID,
			rawStream,
			sharedKey,
			m.incoming,
		)
		if err != nil {
			rawStream.Close()
			// Close any streams we've already opened
			for _, s := range peerStreams {
				s.Close()
			}
			return fmt.Errorf("failed to create encrypted stream %q: %w", name, err)
		}

		peerStreams[name] = encStream
	}

	// Store streams
	m.streams[peerID] = peerStreams

	return nil
}

// Send sends data over an encrypted stream to a peer.
func (m *Manager) Send(peerID peer.ID, streamName string, data []byte) error {
	m.mu.RLock()
	peerStreams, ok := m.streams[peerID]
	if !ok {
		m.mu.RUnlock()
		return fmt.Errorf("no streams for peer %s", peerID)
	}

	stream, ok := peerStreams[streamName]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("stream %q not found for peer %s", streamName, peerID)
	}

	return stream.Send(data)
}

// CloseStreams closes all streams for a peer.
func (m *Manager) CloseStreams(peerID peer.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	peerStreams, ok := m.streams[peerID]
	if !ok {
		return fmt.Errorf("no streams for peer %s", peerID)
	}

	// Close all streams for this peer
	var lastErr error
	for _, stream := range peerStreams {
		if err := stream.Close(); err != nil {
			lastErr = err
		}
	}

	// Remove from map
	delete(m.streams, peerID)

	// Clean up crypto module's cached key
	// We need the peer's X25519 public key for this
	// For now, we'll let it be cleaned up naturally
	// TODO: Store X25519 public key in PeerConnection and pass it here

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
func (m *Manager) HandleIncomingStream(
	peerID peer.ID,
	streamName string,
	stream network.Stream,
	sharedKey []byte,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get or create peer streams map
	peerStreams, ok := m.streams[peerID]
	if !ok {
		peerStreams = make(map[string]*EncryptedStream)
		m.streams[peerID] = peerStreams
	}

	// Check if stream already exists
	if _, exists := peerStreams[streamName]; exists {
		return fmt.Errorf("stream %q already exists for peer %s", streamName, peerID)
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
func (m *Manager) Shutdown() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Close all streams
	for _, peerStreams := range m.streams {
		for _, stream := range peerStreams {
			stream.Close()
		}
	}

	m.streams = make(map[peer.ID]map[string]*EncryptedStream)
}
