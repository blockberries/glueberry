package connection

import (
	"testing"
)

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state    ConnectionState
		expected string
	}{
		{StateDisconnected, "Disconnected"},
		{StateConnecting, "Connecting"},
		{StateConnected, "Connected"},
		{StateHandshaking, "Handshaking"},
		{StateEstablished, "Established"},
		{StateReconnecting, "Reconnecting"},
		{StateCooldown, "Cooldown"},
		{ConnectionState(999), "Unknown(999)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestConnectionState_IsTerminal(t *testing.T) {
	tests := []struct {
		state      ConnectionState
		isTerminal bool
	}{
		{StateDisconnected, true},
		{StateConnecting, false},
		{StateConnected, false},
		{StateHandshaking, false},
		{StateEstablished, false},
		{StateReconnecting, false},
		{StateCooldown, true},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			got := tt.state.IsTerminal()
			if got != tt.isTerminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.isTerminal)
			}
		})
	}
}

func TestConnectionState_IsActive(t *testing.T) {
	tests := []struct {
		state    ConnectionState
		isActive bool
	}{
		{StateDisconnected, false},
		{StateConnecting, true},
		{StateConnected, true},
		{StateHandshaking, true},
		{StateEstablished, true},
		{StateReconnecting, false},
		{StateCooldown, false},
	}

	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			got := tt.state.IsActive()
			if got != tt.isActive {
				t.Errorf("IsActive() = %v, want %v", got, tt.isActive)
			}
		})
	}
}

func TestConnectionState_CanTransitionTo(t *testing.T) {
	tests := []struct {
		name   string
		from   ConnectionState
		to     ConnectionState
		canTransition bool
	}{
		// From Disconnected
		{"disconnected -> connecting", StateDisconnected, StateConnecting, true},
		{"disconnected -> reconnecting", StateDisconnected, StateReconnecting, true},
		{"disconnected -> connected", StateDisconnected, StateConnected, false},
		{"disconnected -> handshaking", StateDisconnected, StateHandshaking, false},

		// From Connecting
		{"connecting -> connected", StateConnecting, StateConnected, true},
		{"connecting -> disconnected", StateConnecting, StateDisconnected, true},
		{"connecting -> handshaking", StateConnecting, StateHandshaking, false},

		// From Connected
		{"connected -> handshaking", StateConnected, StateHandshaking, true},
		{"connected -> disconnected", StateConnected, StateDisconnected, true},
		{"connected -> established", StateConnected, StateEstablished, false},

		// From Handshaking
		{"handshaking -> established", StateHandshaking, StateEstablished, true},
		{"handshaking -> disconnected", StateHandshaking, StateDisconnected, true},
		{"handshaking -> cooldown", StateHandshaking, StateCooldown, true},
		{"handshaking -> connecting", StateHandshaking, StateConnecting, false},

		// From Established
		{"established -> disconnected", StateEstablished, StateDisconnected, true},
		{"established -> handshaking", StateEstablished, StateHandshaking, false},

		// From Reconnecting
		{"reconnecting -> connecting", StateReconnecting, StateConnecting, true},
		{"reconnecting -> disconnected", StateReconnecting, StateDisconnected, true},
		{"reconnecting -> connected", StateReconnecting, StateConnected, false},

		// From Cooldown
		{"cooldown -> disconnected", StateCooldown, StateDisconnected, true},
		{"cooldown -> reconnecting", StateCooldown, StateReconnecting, true},
		{"cooldown -> connecting", StateCooldown, StateConnecting, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.from.CanTransitionTo(tt.to)
			if got != tt.canTransition {
				t.Errorf("CanTransitionTo() = %v, want %v", got, tt.canTransition)
			}
		})
	}
}

func TestConnectionState_ValidateTransition(t *testing.T) {
	// Test valid transitions
	validTransitions := []struct {
		from ConnectionState
		to   ConnectionState
	}{
		{StateDisconnected, StateConnecting},
		{StateConnecting, StateConnected},
		{StateConnected, StateHandshaking},
		{StateHandshaking, StateEstablished},
		{StateEstablished, StateDisconnected},
	}

	for _, tt := range validTransitions {
		t.Run(tt.from.String()+"->"+tt.to.String(), func(t *testing.T) {
			err := tt.from.ValidateTransition(tt.to)
			if err != nil {
				t.Errorf("ValidateTransition() should succeed, got error: %v", err)
			}
		})
	}

	// Test invalid transitions
	invalidTransitions := []struct {
		from ConnectionState
		to   ConnectionState
	}{
		{StateDisconnected, StateHandshaking},
		{StateConnecting, StateEstablished},
		{StateEstablished, StateConnecting},
		{StateCooldown, StateConnecting},
	}

	for _, tt := range invalidTransitions {
		t.Run(tt.from.String()+"->"+tt.to.String()+"_invalid", func(t *testing.T) {
			err := tt.from.ValidateTransition(tt.to)
			if err == nil {
				t.Error("ValidateTransition() should fail for invalid transition")
			}
		})
	}
}
