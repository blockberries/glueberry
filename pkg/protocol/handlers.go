package protocol

import (
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// IncomingStream represents an incoming encrypted stream request.
type IncomingStream struct {
	PeerID     peer.ID
	StreamName string
	Stream     network.Stream
	Timestamp  time.Time
}

// StreamAcceptor defines the interface for accepting incoming streams.
type StreamAcceptor interface {
	HandleIncomingStream(peerID peer.ID, streamName string, stream network.Stream, sharedKey []byte) error
}

// StreamHandler handles incoming encrypted stream requests.
// It notifies the stream manager to accept the stream.
type StreamHandler struct {
	streamManager StreamAcceptor
}

// NewStreamHandler creates a new stream handler.
func NewStreamHandler(manager StreamAcceptor) *StreamHandler {
	return &StreamHandler{
		streamManager: manager,
	}
}

// HandleStream handles an incoming encrypted stream.
// This is called by libp2p when a remote peer opens an encrypted stream.
//
// Note: This assumes the handshake has already completed and the shared key exists.
// If the shared key doesn't exist, the stream is rejected.
func (sh *StreamHandler) HandleStream(streamName string) network.StreamHandler {
	return func(stream network.Stream) {
		_ = stream.Conn().RemotePeer()

		// This handler is now deprecated - Node handles this directly
		// in registerIncomingStreamHandlers with access to shared keys

		// Placeholder: close the stream
		_ = stream.Reset() // Ignore error - just closing
	}
}
