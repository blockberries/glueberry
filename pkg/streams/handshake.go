// Package streams provides handshake and encrypted stream management
// for Glueberry P2P communications.
package streams

import (
	"fmt"
	"io"
	"time"

	"github.com/blockberries/cramberry/pkg/cramberry"
	"github.com/libp2p/go-libp2p/core/network"
)

// HandshakeStream provides a message-oriented interface for app-controlled
// handshaking over a libp2p stream. It uses Cramberry for message serialization
// and enforces a timeout for handshake completion.
//
// HandshakeStream is NOT safe for concurrent use. The application should
// coordinate handshake operations from a single goroutine.
type HandshakeStream struct {
	stream   network.Stream
	reader   *cramberry.MessageIterator
	writer   *cramberry.StreamWriter
	deadline time.Time
	closed   bool
}

// NewHandshakeStream creates a new handshake stream with the given timeout.
// The deadline is set to time.Now() + timeout.
func NewHandshakeStream(stream network.Stream, timeout time.Duration) *HandshakeStream {
	deadline := time.Now().Add(timeout)

	// Set deadline on the underlying stream
	// Ignore error as we also check deadline manually
	_ = stream.SetDeadline(deadline)

	return &HandshakeStream{
		stream:   stream,
		reader:   cramberry.NewMessageIterator(stream),
		writer:   cramberry.NewStreamWriter(stream),
		deadline: deadline,
		closed:   false,
	}
}

// Send serializes and sends a message over the handshake stream.
// The message can be any Cramberry-serializable type.
//
// Returns an error if:
// - The stream is closed
// - The deadline has expired
// - Serialization fails
// - The write fails
func (hs *HandshakeStream) Send(msg any) error {
	if hs.closed {
		return fmt.Errorf("handshake stream is closed")
	}

	if time.Now().After(hs.deadline) {
		return fmt.Errorf("handshake deadline exceeded")
	}

	// Write delimited message
	if err := hs.writer.WriteDelimited(msg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	// Flush to ensure message is sent
	if err := hs.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush message: %w", err)
	}

	return nil
}

// Receive reads and deserializes a message from the handshake stream.
// The msg parameter must be a pointer to a Cramberry-deserializable type.
//
// Returns an error if:
// - The stream is closed
// - The deadline has expired
// - The read fails (including EOF)
// - Deserialization fails
func (hs *HandshakeStream) Receive(msg any) error {
	if hs.closed {
		return fmt.Errorf("handshake stream is closed")
	}

	if time.Now().After(hs.deadline) {
		return fmt.Errorf("handshake deadline exceeded")
	}

	// Read next message
	if !hs.reader.Next(msg) {
		// Check for error or EOF
		if err := hs.reader.Err(); err != nil {
			if err == io.EOF {
				return fmt.Errorf("connection closed: %w", err)
			}
			return fmt.Errorf("failed to read message: %w", err)
		}
		return fmt.Errorf("no message available")
	}

	return nil
}

// Close closes the handshake stream and releases resources.
// It is safe to call Close multiple times.
func (hs *HandshakeStream) Close() error {
	if hs.closed {
		return nil
	}

	hs.closed = true

	// Reset the stream (closes both read and write sides)
	return hs.stream.Reset()
}

// CloseWrite closes the write side of the stream, signaling to the remote
// peer that no more data will be sent. The read side remains open.
func (hs *HandshakeStream) CloseWrite() error {
	if hs.closed {
		return fmt.Errorf("handshake stream is closed")
	}

	return hs.stream.CloseWrite()
}

// TimeRemaining returns the time remaining until the handshake deadline.
// Returns 0 if the deadline has already passed.
func (hs *HandshakeStream) TimeRemaining() time.Duration {
	remaining := time.Until(hs.deadline)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Deadline returns the absolute deadline for handshake completion.
func (hs *HandshakeStream) Deadline() time.Time {
	return hs.deadline
}

// Stream returns the underlying libp2p stream.
// Use with caution - prefer the HandshakeStream methods when possible.
func (hs *HandshakeStream) Stream() network.Stream {
	return hs.stream
}

// IsClosed returns true if the stream has been closed.
func (hs *HandshakeStream) IsClosed() bool {
	return hs.closed
}
