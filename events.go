package glueberry

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ConnectionState represents the state of a peer connection.
// This is re-exported from the connection package for public API.
type ConnectionState int

const (
	// StateDisconnected indicates no connection exists.
	StateDisconnected ConnectionState = iota

	// StateConnecting indicates a connection attempt is in progress.
	StateConnecting

	// StateConnected indicates libp2p connection established and the
	// handshake stream is available. The app performs the handshake
	// protocol via Send()/Messages() on the "handshake" stream.
	StateConnected

	// StateEstablished indicates handshake complete and
	// encrypted streams are active.
	StateEstablished

	// StateReconnecting indicates attempting to reconnect after disconnect.
	StateReconnecting

	// StateCooldown indicates waiting after a failed handshake
	// before allowing reconnection.
	StateCooldown
)

// String returns a human-readable representation of the connection state.
func (s ConnectionState) String() string {
	switch s {
	case StateDisconnected:
		return "Disconnected"
	case StateConnecting:
		return "Connecting"
	case StateConnected:
		return "Connected"
	case StateEstablished:
		return "Established"
	case StateReconnecting:
		return "Reconnecting"
	case StateCooldown:
		return "Cooldown"
	default:
		return "Unknown"
	}
}

// ConnectionEvent represents a connection state change event.
// These events are emitted by the Glueberry node to notify the application
// of connection lifecycle changes.
type ConnectionEvent struct {
	// PeerID is the peer this event relates to.
	PeerID peer.ID

	// State is the new connection state.
	State ConnectionState

	// Error contains error information if this event represents a failure.
	// Nil for successful state changes.
	Error error

	// Timestamp is when this event occurred.
	Timestamp time.Time
}

// IsError returns true if this event represents an error condition.
func (e ConnectionEvent) IsError() bool {
	return e.Error != nil
}
