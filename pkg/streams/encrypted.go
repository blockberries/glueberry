package streams

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/blockberries/cramberry/pkg/cramberry"
	"github.com/blockberries/glueberry/pkg/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// IncomingMessage represents a decrypted message received from a peer.
type IncomingMessage struct {
	PeerID     peer.ID
	StreamName string
	Data       []byte
	Timestamp  time.Time
}

// EncryptedStream provides transparent encryption/decryption over a libp2p stream.
// It uses ChaCha20-Poly1305 for authenticated encryption and Cramberry for message framing.
//
// Reads are handled by a background goroutine that forwards decrypted messages
// to the incoming channel. Writes are serialized via a mutex.
type EncryptedStream struct {
	name      string
	peerID    peer.ID
	stream    network.Stream
	sharedKey []byte
	cipher    *crypto.Cipher

	reader  *cramberry.MessageIterator
	writer  *cramberry.StreamWriter
	writeMu sync.Mutex

	incoming chan<- IncomingMessage
	ctx      context.Context
	cancel   context.CancelFunc

	closed   bool
	closeMu  sync.Mutex
	closeErr error
}

// NewEncryptedStream creates a new encrypted stream.
// It starts a read goroutine to handle incoming messages.
func NewEncryptedStream(
	ctx context.Context,
	name string,
	peerID peer.ID,
	stream network.Stream,
	sharedKey []byte,
	incoming chan<- IncomingMessage,
) (*EncryptedStream, error) {
	// Create cipher for this shared key
	cipher, err := crypto.NewCipher(sharedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	streamCtx, cancel := context.WithCancel(ctx)

	es := &EncryptedStream{
		name:      name,
		peerID:    peerID,
		stream:    stream,
		sharedKey: sharedKey,
		cipher:    cipher,
		reader:    cramberry.NewMessageIterator(stream),
		writer:    cramberry.NewStreamWriter(stream),
		incoming:  incoming,
		ctx:       streamCtx,
		cancel:    cancel,
	}

	// Start read goroutine
	go es.readLoop()

	return es, nil
}

// Send encrypts and sends data over the stream.
// This method is thread-safe and can be called concurrently.
func (es *EncryptedStream) Send(data []byte) error {
	es.closeMu.Lock()
	if es.closed {
		es.closeMu.Unlock()
		return fmt.Errorf("stream is closed")
	}
	es.closeMu.Unlock()

	// Encrypt the data
	encrypted, err := es.cipher.Encrypt(data, nil)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	// Write with mutex to serialize writes
	es.writeMu.Lock()
	defer es.writeMu.Unlock()

	// Write the encrypted message as a delimited Cramberry message
	if err := es.writer.WriteDelimited(&encrypted); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	// Flush to ensure delivery
	if err := es.writer.Flush(); err != nil {
		return fmt.Errorf("flush failed: %w", err)
	}

	return nil
}

// readLoop continuously reads and decrypts messages from the stream.
func (es *EncryptedStream) readLoop() {
	defer es.markClosed(nil)

	for {
		select {
		case <-es.ctx.Done():
			// Stream closed
			return
		default:
		}

		// Read next encrypted message
		var encrypted []byte
		if !es.reader.Next(&encrypted) {
			// Check for error or EOF
			if err := es.reader.Err(); err != nil {
				if err == io.EOF {
					// Normal stream closure
					return
				}
				// Read error
				es.markClosed(fmt.Errorf("read error: %w", err))
				return
			}
			// No more messages
			return
		}

		// Decrypt the message
		decrypted, err := es.cipher.Decrypt(encrypted, nil)
		if err != nil {
			// Decryption failure could indicate tampering or wrong key
			// Log and continue rather than closing stream
			continue
		}

		// Send to incoming channel (non-blocking)
		msg := IncomingMessage{
			PeerID:     es.peerID,
			StreamName: es.name,
			Data:       decrypted,
			Timestamp:  time.Now(),
		}

		select {
		case es.incoming <- msg:
			// Message delivered
		case <-es.ctx.Done():
			// Stream closed while sending
			return
		default:
			// Channel full - drop message to prevent blocking
			// In production, might want to log this
		}
	}
}

// Close closes the encrypted stream and stops the read goroutine.
func (es *EncryptedStream) Close() error {
	es.closeMu.Lock()
	if es.closed {
		err := es.closeErr
		es.closeMu.Unlock()
		return err
	}
	es.closeMu.Unlock()

	// Cancel context to stop read loop
	es.cancel()

	// Close the underlying stream
	err := es.stream.Close()

	es.markClosed(err)
	return err
}

// markClosed marks the stream as closed with the given error.
// Note: The shared key is NOT zeroed here because:
// 1. The key is owned by the Manager, which handles zeroing in CloseStreams/Shutdown
// 2. Multiple streams may share the same key reference
// 3. The cipher has its own internal copy of the key
func (es *EncryptedStream) markClosed(err error) {
	es.closeMu.Lock()
	defer es.closeMu.Unlock()

	if !es.closed {
		es.closed = true
		es.closeErr = err
	}
}

// IsClosed returns true if the stream is closed.
func (es *EncryptedStream) IsClosed() bool {
	es.closeMu.Lock()
	defer es.closeMu.Unlock()
	return es.closed
}

// Name returns the stream name.
func (es *EncryptedStream) Name() string {
	return es.name
}

// PeerID returns the peer ID.
func (es *EncryptedStream) PeerID() peer.ID {
	return es.peerID
}

// Stream returns the underlying libp2p stream.
func (es *EncryptedStream) Stream() network.Stream {
	return es.stream
}
