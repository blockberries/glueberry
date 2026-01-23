package protocol

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/protocol"
)

func TestStreamProtocolID(t *testing.T) {
	tests := []struct {
		name     string
		expected protocol.ID
	}{
		{"messages", "/glueberry/stream/messages/1.0.0"},
		{"blocks", "/glueberry/stream/blocks/1.0.0"},
		{"transactions", "/glueberry/stream/transactions/1.0.0"},
		{"", "/glueberry/stream//1.0.0"}, // Empty name still works
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StreamProtocolID(tt.name)
			if got != tt.expected {
				t.Errorf("StreamProtocolID(%q) = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}

func TestHandshakeProtocolID(t *testing.T) {
	if HandshakeProtocolID != "/glueberry/handshake/1.0.0" {
		t.Errorf("HandshakeProtocolID = %q, want %q", HandshakeProtocolID, "/glueberry/handshake/1.0.0")
	}
}
