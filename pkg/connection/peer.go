package connection

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// PeerConnection tracks the connection state and resources for a single peer.
// It is NOT safe for concurrent use - the Connection Manager serializes access.
type PeerConnection struct {
	// PeerID is the libp2p peer identifier.
	PeerID peer.ID

	// State is the current connection state.
	State ConnectionState

	// HandshakeTimer tracks the handshake timeout.
	// Set when connection reaches StateConnected.
	HandshakeTimer *time.Timer

	// HandshakeCancel cancels the handshake timeout goroutine.
	HandshakeCancel context.CancelFunc

	// SharedKey is the ECDH-derived shared secret (raw, not for direct use).
	// This is stored for reference but actual encryption uses the crypto module's cache.
	SharedKey []byte

	// EncryptedStreams maps stream names to their stream handlers.
	// Populated when CompleteHandshake is called.
	EncryptedStreams map[string]any

	// ReconnectState tracks reconnection attempts.
	ReconnectState *ReconnectState

	// CooldownUntil is the time when cooldown period ends.
	// Only valid when State == StateCooldown.
	CooldownUntil time.Time

	// LastError stores the last error that occurred.
	LastError error

	// LastStateChange is when the state last changed.
	LastStateChange time.Time

	// mu protects concurrent access to this connection.
	// Note: The Connection Manager should already serialize access,
	// but this provides additional safety.
	mu sync.RWMutex
}

// NewPeerConnection creates a new peer connection in the Disconnected state.
func NewPeerConnection(peerID peer.ID) *PeerConnection {
	return &PeerConnection{
		PeerID:           peerID,
		State:            StateDisconnected,
		LastStateChange:  time.Now(),
		EncryptedStreams: make(map[string]any),
	}
}

// TransitionTo transitions the connection to a new state.
// Returns an error if the transition is invalid.
func (pc *PeerConnection) TransitionTo(newState ConnectionState) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if err := pc.State.ValidateTransition(newState); err != nil {
		return fmt.Errorf("peer %s: %w", pc.PeerID, err)
	}

	pc.State = newState
	pc.LastStateChange = time.Now()
	return nil
}

// GetState returns the current state (thread-safe).
func (pc *PeerConnection) GetState() ConnectionState {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.State
}

// CancelHandshakeTimeout cancels the handshake timeout timer.
// This is called when CompleteHandshake is invoked.
func (pc *PeerConnection) CancelHandshakeTimeout() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Cancel handshake timer if exists
	if pc.HandshakeTimer != nil {
		pc.HandshakeTimer.Stop()
		pc.HandshakeTimer = nil
	}

	// Cancel handshake context if exists
	if pc.HandshakeCancel != nil {
		pc.HandshakeCancel()
		pc.HandshakeCancel = nil
	}
}

// SetSharedKey stores the shared encryption key.
func (pc *PeerConnection) SetSharedKey(key []byte) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Make a copy to prevent external modification
	pc.SharedKey = make([]byte, len(key))
	copy(pc.SharedKey, key)
}

// GetSharedKey returns a copy of the shared key.
func (pc *PeerConnection) GetSharedKey() []byte {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.SharedKey == nil {
		return nil
	}

	key := make([]byte, len(pc.SharedKey))
	copy(key, pc.SharedKey)
	return key
}

// SetError stores an error associated with this connection.
func (pc *PeerConnection) SetError(err error) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	pc.LastError = err
}

// GetError returns the last error.
func (pc *PeerConnection) GetError() error {
	pc.mu.RLock()
	defer pc.mu.RUnlock()
	return pc.LastError
}

// StartCooldown transitions to cooldown state with the given duration.
func (pc *PeerConnection) StartCooldown(duration time.Duration) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if err := pc.State.ValidateTransition(StateCooldown); err != nil {
		return err
	}

	pc.State = StateCooldown
	pc.CooldownUntil = time.Now().Add(duration)
	pc.LastStateChange = time.Now()
	return nil
}

// IsInCooldown returns true if currently in cooldown period.
func (pc *PeerConnection) IsInCooldown() bool {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.State != StateCooldown {
		return false
	}

	return time.Now().Before(pc.CooldownUntil)
}

// CooldownRemaining returns the time remaining in cooldown.
// Returns 0 if not in cooldown or cooldown expired.
func (pc *PeerConnection) CooldownRemaining() time.Duration {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	if pc.State != StateCooldown {
		return 0
	}

	remaining := time.Until(pc.CooldownUntil)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Cleanup releases all resources associated with this connection.
func (pc *PeerConnection) Cleanup() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Stop handshake timer
	if pc.HandshakeTimer != nil {
		pc.HandshakeTimer.Stop()
		pc.HandshakeTimer = nil
	}

	// Cancel handshake context
	if pc.HandshakeCancel != nil {
		pc.HandshakeCancel()
		pc.HandshakeCancel = nil
	}

	// Clear shared key
	pc.SharedKey = nil

	// Clear encrypted streams (will be properly closed by stream manager)
	pc.EncryptedStreams = make(map[string]any)

	// Clear reconnect state
	pc.ReconnectState = nil
}
