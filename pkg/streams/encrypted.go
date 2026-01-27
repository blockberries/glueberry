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

// OversizedMessageCallback is called when a received message exceeds the maximum size.
// It receives the peer ID, stream name, and the actual size of the message.
type OversizedMessageCallback func(peerID peer.ID, streamName string, size int)

// MessageDroppedCallback is called when a message is dropped because the incoming
// channel is full. It receives the peer ID and stream name.
type MessageDroppedCallback func(peerID peer.ID, streamName string)

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

	incoming           chan<- IncomingMessage
	onDecryptionError  DecryptionErrorCallback
	onOversizedMessage OversizedMessageCallback
	onMessageDropped   MessageDroppedCallback
	maxMessageSize     int
	ctx                context.Context
	cancel             context.CancelFunc

	closed   bool
	closeMu  sync.Mutex
	closeErr error
}

// EncryptedStreamConfig holds configuration options for creating an encrypted stream.
type EncryptedStreamConfig struct {
	// OnDecryptionError is called when decryption fails (optional)
	OnDecryptionError DecryptionErrorCallback

	// OnOversizedMessage is called when a message exceeds MaxMessageSize (optional)
	OnOversizedMessage OversizedMessageCallback

	// OnMessageDropped is called when a message is dropped due to the incoming
	// channel being full (optional). This allows tracking message drops.
	OnMessageDropped MessageDroppedCallback

	// MaxMessageSize is the maximum allowed message size in bytes.
	// Messages exceeding this size are dropped and OnOversizedMessage is called.
	// If 0, no size limit is enforced.
	MaxMessageSize int
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
	config EncryptedStreamConfig,
) (*EncryptedStream, error) {
	// Create cipher for this shared key
	cipher, err := crypto.NewCipher(sharedKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	streamCtx, cancel := context.WithCancel(ctx)

	es := &EncryptedStream{
		name:               name,
		peerID:             peerID,
		stream:             stream,
		sharedKey:          sharedKey,
		cipher:             cipher,
		reader:             cramberry.NewMessageIterator(stream),
		writer:             cramberry.NewStreamWriter(stream),
		incoming:           incoming,
		onDecryptionError:  config.OnDecryptionError,
		onOversizedMessage: config.OnOversizedMessage,
		onMessageDropped:   config.OnMessageDropped,
		maxMessageSize:     config.MaxMessageSize,
		ctx:                streamCtx,
		cancel:             cancel,
	}

	// Start read goroutine
	go es.readLoop()

	return es, nil
}

// Send encrypts and sends data over the stream.
// This method is thread-safe and can be called concurrently.
func (es *EncryptedStream) Send(data []byte) error {
	return es.SendCtx(context.Background(), data)
}

// SendCtx encrypts and sends data over the stream with context support for cancellation.
// The provided context can be used to cancel the send operation or set a timeout.
// This method is thread-safe and can be called concurrently.
func (es *EncryptedStream) SendCtx(ctx context.Context, data []byte) error {
	// Check context before starting
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	es.closeMu.Lock()
	if es.closed {
		es.closeMu.Unlock()
		return fmt.Errorf("stream is closed")
	}
	es.closeMu.Unlock()

	// Check context again after checking closed state
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Encrypt the data
	encrypted, err := es.cipher.Encrypt(data, nil)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	// Write with mutex to serialize writes
	es.writeMu.Lock()
	defer es.writeMu.Unlock()

	// Check context before write (we may have waited for the lock)
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Set deadline on stream if context has one
	if deadline, ok := ctx.Deadline(); ok {
		_ = es.stream.SetWriteDeadline(deadline)
		defer func() { _ = es.stream.SetWriteDeadline(time.Time{}) }()
	}

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

		// Check message size before decryption to prevent resource exhaustion.
		// The encrypted message includes nonce (12 bytes) and auth tag (16 bytes),
		// so the plaintext will be smaller. We check the ciphertext size for safety.
		if es.maxMessageSize > 0 && len(encrypted) > es.maxMessageSize {
			if es.onOversizedMessage != nil {
				es.onOversizedMessage(es.peerID, es.name, len(encrypted))
			}
			// Drop oversized message and continue
			continue
		}

		// Decrypt the message
		decrypted, err := es.cipher.Decrypt(encrypted, nil)
		if err != nil {
			// Decryption failure could indicate tampering or wrong key
			// Report the error for logging/metrics if callback is set
			if es.onDecryptionError != nil {
				es.onDecryptionError(es.peerID, err)
			}
			// Continue rather than closing stream
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
			if es.onMessageDropped != nil {
				es.onMessageDropped(es.peerID, es.name)
			}
		}
	}
}

// Close closes the encrypted stream and stops the read goroutine.
// It also zeros the cipher's key material.
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

	// Close the cipher to zero key material
	if es.cipher != nil {
		es.cipher.Close()
	}

	// Close the underlying stream
	err := es.stream.Close()

	es.markClosed(err)
	return err
}

// markClosed marks the stream as closed with the given error.
// Note: The shared key reference (es.sharedKey) is NOT zeroed here because
// it's owned by the Manager, which handles zeroing in CloseStreams/Shutdown.
// The cipher's internal key copy is zeroed in Close().
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
