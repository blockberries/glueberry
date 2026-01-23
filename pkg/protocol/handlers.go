package protocol

import (
	"time"

	"github.com/blockberries/glueberry/pkg/streams"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// IncomingHandshake represents an incoming handshake stream request.
type IncomingHandshake struct {
	PeerID          peer.ID
	HandshakeStream *streams.HandshakeStream
	Timestamp       time.Time
}

// HandshakeHandler handles incoming handshake stream requests.
type HandshakeHandler struct {
	handshakeTimeout time.Duration
	incoming         chan<- IncomingHandshake
}

// NewHandshakeHandler creates a new handshake handler.
func NewHandshakeHandler(timeout time.Duration, incoming chan<- IncomingHandshake) *HandshakeHandler {
	return &HandshakeHandler{
		handshakeTimeout: timeout,
		incoming:         incoming,
	}
}

// HandleStream handles an incoming handshake stream.
// This is called by libp2p when a remote peer opens a handshake stream.
func (h *HandshakeHandler) HandleStream(stream network.Stream) {
	peerID := stream.Conn().RemotePeer()

	// Wrap in HandshakeStream
	hs := streams.NewHandshakeStream(stream, h.handshakeTimeout)

	// Send to incoming channel (non-blocking)
	incoming := IncomingHandshake{
		PeerID:          peerID,
		HandshakeStream: hs,
		Timestamp:       time.Now(),
	}

	select {
	case h.incoming <- incoming:
		// Handshake stream delivered
	default:
		// Channel full, close stream
		hs.Close()
	}
}

// IncomingStream represents an incoming encrypted stream request.
type IncomingStream struct {
	PeerID     peer.ID
	StreamName string
	Stream     network.Stream
	Timestamp  time.Time
}

// StreamHandler handles incoming encrypted stream requests.
// It notifies the stream manager to accept the stream.
type StreamHandler struct {
	streamManager StreamAcceptor
}

// StreamAcceptor defines the interface for accepting incoming streams.
type StreamAcceptor interface {
	HandleIncomingStream(peerID peer.ID, streamName string, stream network.Stream, sharedKey []byte) error
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
		stream.Reset()
	}
}
