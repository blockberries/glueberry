// Package connection provides connection lifecycle management including
// state tracking, reconnection logic, and handshake timeout enforcement.
package connection

import "fmt"

// ConnectionState represents the state of a peer connection.
type ConnectionState int

const (
	// StateDisconnected indicates no connection exists.
	StateDisconnected ConnectionState = iota

	// StateConnecting indicates a connection attempt is in progress.
	StateConnecting

	// StateConnected indicates libp2p connection established,
	// awaiting handshake initiation.
	StateConnected

	// StateHandshaking indicates handshake stream is open and
	// handshake is in progress.
	StateHandshaking

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
	case StateHandshaking:
		return "Handshaking"
	case StateEstablished:
		return "Established"
	case StateReconnecting:
		return "Reconnecting"
	case StateCooldown:
		return "Cooldown"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

// IsTerminal returns true if the state is a terminal state
// (no automatic transitions will occur).
func (s ConnectionState) IsTerminal() bool {
	return s == StateDisconnected || s == StateCooldown
}

// IsActive returns true if the connection is in an active state
// (connected or establishing).
func (s ConnectionState) IsActive() bool {
	return s == StateConnecting || s == StateConnected ||
		s == StateHandshaking || s == StateEstablished
}

// CanTransitionTo checks if a transition from the current state to
// the target state is valid.
func (s ConnectionState) CanTransitionTo(target ConnectionState) bool {
	// Define valid state transitions
	validTransitions := map[ConnectionState][]ConnectionState{
		StateDisconnected: {StateConnecting, StateReconnecting},
		StateConnecting:   {StateConnected, StateDisconnected},
		StateConnected:    {StateHandshaking, StateDisconnected},
		StateHandshaking:  {StateEstablished, StateDisconnected, StateCooldown},
		StateEstablished:  {StateDisconnected},
		StateReconnecting: {StateConnecting, StateDisconnected},
		StateCooldown:     {StateDisconnected, StateReconnecting},
	}

	allowed, ok := validTransitions[s]
	if !ok {
		return false
	}

	for _, t := range allowed {
		if t == target {
			return true
		}
	}
	return false
}

// ValidateTransition returns an error if the transition is invalid.
func (s ConnectionState) ValidateTransition(target ConnectionState) error {
	if !s.CanTransitionTo(target) {
		return fmt.Errorf("invalid state transition: %s -> %s", s, target)
	}
	return nil
}
