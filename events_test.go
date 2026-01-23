package glueberry

import (
	"fmt"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state    ConnectionState
		expected string
	}{
		{StateDisconnected, "Disconnected"},
		{StateConnecting, "Connecting"},
		{StateConnected, "Connected"},
		{StateEstablished, "Established"},
		{StateReconnecting, "Reconnecting"},
		{StateCooldown, "Cooldown"},
		{ConnectionState(999), "Unknown"},
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

func TestConnectionEvent_IsError(t *testing.T) {
	peerID, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")

	// Event without error
	event1 := ConnectionEvent{
		PeerID: peerID,
		State:  StateConnected,
		Error:  nil,
	}

	if event1.IsError() {
		t.Error("IsError should be false when Error is nil")
	}

	// Event with error
	event2 := ConnectionEvent{
		PeerID: peerID,
		State:  StateDisconnected,
		Error:  fmt.Errorf("connection failed"),
	}

	if !event2.IsError() {
		t.Error("IsError should be true when Error is not nil")
	}
}
