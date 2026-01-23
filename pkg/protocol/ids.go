// Package protocol provides libp2p protocol handlers and host management
// for Glueberry P2P communications.
package protocol

import "github.com/libp2p/go-libp2p/core/protocol"

const (
	// HandshakeProtocolID is the protocol identifier for handshake streams.
	HandshakeProtocolID protocol.ID = "/glueberry/handshake/1.0.0"

	// StreamProtocolPrefix is the prefix for encrypted stream protocols.
	// The full protocol ID is: /glueberry/stream/{name}/1.0.0
	StreamProtocolPrefix = "/glueberry/stream/"

	// StreamProtocolSuffix is appended after the stream name.
	StreamProtocolSuffix = "/1.0.0"
)

// StreamProtocolID returns the protocol ID for a named encrypted stream.
func StreamProtocolID(name string) protocol.ID {
	return protocol.ID(StreamProtocolPrefix + name + StreamProtocolSuffix)
}
