package streams

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/blockberries/cramberry/pkg/cramberry"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// HandshakeStreamName is the name of the built-in unencrypted handshake stream.
const HandshakeStreamName = "handshake"

// UnencryptedStream provides a message-oriented interface for unencrypted
// communication over a libp2p stream. This is used for the built-in "handshake"
// stream where encryption is not yet established.
//
// Reads are handled by a background goroutine that forwards messages
// to the incoming channel. Writes are serialized via a mutex.
type UnencryptedStream struct {
	name   string
	peerID peer.ID
	stream network.Stream

	reader  *cramberry.MessageIterator
	writer  *cramberry.StreamWriter
	writeMu sync.Mutex

	incoming         chan<- IncomingMessage
	onMessageDropped MessageDroppedCallback
	callbackMu       sync.RWMutex
	ctx              context.Context
	cancel           context.CancelFunc

	closed   bool
	closeMu  sync.RWMutex
	closeErr error
}

// NewUnencryptedStream creates a new unencrypted stream.
// It starts a read goroutine to handle incoming messages.
func NewUnencryptedStream(
	ctx context.Context,
	name string,
	peerID peer.ID,
	stream network.Stream,
	incoming chan<- IncomingMessage,
) *UnencryptedStream {
	streamCtx, cancel := context.WithCancel(ctx)

	us := &UnencryptedStream{
		name:     name,
		peerID:   peerID,
		stream:   stream,
		reader:   cramberry.NewMessageIterator(stream),
		writer:   cramberry.NewStreamWriter(stream),
		incoming: incoming,
		ctx:      streamCtx,
		cancel:   cancel,
	}

	// Start read goroutine
	go us.readLoop()

	return us
}

// SetOnMessageDropped sets the callback to be called when a message is dropped
// due to the incoming channel being full. This method is thread-safe.
func (us *UnencryptedStream) SetOnMessageDropped(callback MessageDroppedCallback) {
	us.callbackMu.Lock()
	us.onMessageDropped = callback
	us.callbackMu.Unlock()
}

// Send sends data over the stream without encryption.
// This method is thread-safe and can be called concurrently.
func (us *UnencryptedStream) Send(data []byte) error {
	return us.SendCtx(context.Background(), data)
}

// SendCtx sends data over the stream without encryption with context support for cancellation.
// The provided context can be used to cancel the send operation or set a timeout.
// This method is thread-safe and can be called concurrently.
func (us *UnencryptedStream) SendCtx(ctx context.Context, data []byte) error {
	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	us.closeMu.RLock()
	if us.closed {
		us.closeMu.RUnlock()
		return fmt.Errorf("stream is closed")
	}
	us.closeMu.RUnlock()

	// Check context again after checking closed state
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Write with mutex to serialize writes
	us.writeMu.Lock()
	defer us.writeMu.Unlock()

	// Check context before write (we may have waited for the lock)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Set deadline on stream if context has one
	if deadline, ok := ctx.Deadline(); ok {
		_ = us.stream.SetWriteDeadline(deadline)
		defer func() { _ = us.stream.SetWriteDeadline(time.Time{}) }()
	}

	// Write the message as a delimited Cramberry message
	if err := us.writer.WriteDelimited(&data); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	// Flush to ensure delivery
	if err := us.writer.Flush(); err != nil {
		return fmt.Errorf("flush failed: %w", err)
	}

	return nil
}

// readLoop continuously reads messages from the stream.
func (us *UnencryptedStream) readLoop() {
	defer us.markClosed(nil)

	for {
		select {
		case <-us.ctx.Done():
			// Stream closed
			return
		default:
		}

		// Read next message
		var data []byte
		if !us.reader.Next(&data) {
			// Check for error or EOF
			if err := us.reader.Err(); err != nil {
				if err == io.EOF {
					// Normal stream closure
					return
				}
				// Read error
				us.markClosed(fmt.Errorf("read error: %w", err))
				return
			}
			// No more messages
			return
		}

		// Send to incoming channel (non-blocking)
		msg := IncomingMessage{
			PeerID:     us.peerID,
			StreamName: us.name,
			Data:       data,
			Timestamp:  time.Now(),
		}

		select {
		case us.incoming <- msg:
			// Message delivered
		case <-us.ctx.Done():
			// Stream closed while sending
			return
		default:
			// Channel full - drop message to prevent blocking
			us.callbackMu.RLock()
			callback := us.onMessageDropped
			us.callbackMu.RUnlock()
			if callback != nil {
				callback(us.peerID, us.name)
			}
		}
	}
}

// Close closes the unencrypted stream and stops the read goroutine.
// This method is thread-safe and idempotent - calling it multiple times
// from concurrent goroutines is safe.
func (us *UnencryptedStream) Close() error {
	us.closeMu.Lock()
	if us.closed {
		// Already closed, return the stored error
		err := us.closeErr
		us.closeMu.Unlock()
		return err
	}
	// Mark as closed atomically with the check to prevent race where
	// multiple goroutines pass the closed check before any sets it.
	us.closed = true
	us.closeMu.Unlock()

	// Cancel context to stop read loop
	us.cancel()

	// Close the underlying stream
	err := us.stream.Close()

	// Record the close error (we already set closed=true above)
	us.closeMu.Lock()
	us.closeErr = err
	us.closeMu.Unlock()

	return err
}

// markClosed marks the stream as closed with the given error.
func (us *UnencryptedStream) markClosed(err error) {
	us.closeMu.Lock()
	defer us.closeMu.Unlock()

	if !us.closed {
		us.closed = true
		us.closeErr = err
	}
}

// IsClosed returns true if the stream is closed.
func (us *UnencryptedStream) IsClosed() bool {
	us.closeMu.RLock()
	defer us.closeMu.RUnlock()
	return us.closed
}

// Name returns the stream name.
func (us *UnencryptedStream) Name() string {
	return us.name
}

// PeerID returns the peer ID.
func (us *UnencryptedStream) PeerID() peer.ID {
	return us.peerID
}

// Stream returns the underlying libp2p stream.
func (us *UnencryptedStream) Stream() network.Stream {
	return us.stream
}
