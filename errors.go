// Package glueberry provides a P2P communication library built on libp2p
// with encrypted, multiplexed streams and app-controlled handshaking.
package glueberry

import (
	"errors"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
)

// ErrorCode identifies the type of error for programmatic handling.
type ErrorCode int

const (
	// ErrCodeUnknown indicates an unknown or unclassified error.
	ErrCodeUnknown ErrorCode = iota

	// ErrCodeConnectionFailed indicates a connection attempt failed.
	ErrCodeConnectionFailed

	// ErrCodeHandshakeFailed indicates the handshake failed.
	ErrCodeHandshakeFailed

	// ErrCodeHandshakeTimeout indicates the handshake did not complete in time.
	ErrCodeHandshakeTimeout

	// ErrCodeStreamClosed indicates the stream has been closed.
	ErrCodeStreamClosed

	// ErrCodeEncryptionFailed indicates message encryption failed.
	ErrCodeEncryptionFailed

	// ErrCodeDecryptionFailed indicates message decryption failed.
	ErrCodeDecryptionFailed

	// ErrCodePeerNotFound indicates the peer was not found.
	ErrCodePeerNotFound

	// ErrCodePeerBlacklisted indicates the peer is blacklisted.
	ErrCodePeerBlacklisted

	// ErrCodeBufferFull indicates a buffer (event or message) is full.
	ErrCodeBufferFull

	// ErrCodeContextCanceled indicates the operation was cancelled via context.
	ErrCodeContextCanceled

	// ErrCodeInvalidConfig indicates the configuration is invalid.
	ErrCodeInvalidConfig

	// ErrCodeNodeNotStarted indicates the node has not been started.
	ErrCodeNodeNotStarted

	// ErrCodeNodeAlreadyStarted indicates the node is already running.
	ErrCodeNodeAlreadyStarted

	// ErrCodeVersionMismatch indicates incompatible protocol versions.
	ErrCodeVersionMismatch
)

// String returns a human-readable name for the error code.
func (c ErrorCode) String() string {
	switch c {
	case ErrCodeUnknown:
		return "Unknown"
	case ErrCodeConnectionFailed:
		return "ConnectionFailed"
	case ErrCodeHandshakeFailed:
		return "HandshakeFailed"
	case ErrCodeHandshakeTimeout:
		return "HandshakeTimeout"
	case ErrCodeStreamClosed:
		return "StreamClosed"
	case ErrCodeEncryptionFailed:
		return "EncryptionFailed"
	case ErrCodeDecryptionFailed:
		return "DecryptionFailed"
	case ErrCodePeerNotFound:
		return "PeerNotFound"
	case ErrCodePeerBlacklisted:
		return "PeerBlacklisted"
	case ErrCodeBufferFull:
		return "BufferFull"
	case ErrCodeContextCanceled:
		return "ContextCanceled"
	case ErrCodeInvalidConfig:
		return "InvalidConfig"
	case ErrCodeNodeNotStarted:
		return "NodeNotStarted"
	case ErrCodeNodeAlreadyStarted:
		return "NodeAlreadyStarted"
	case ErrCodeVersionMismatch:
		return "VersionMismatch"
	default:
		return fmt.Sprintf("ErrorCode(%d)", c)
	}
}

// Error represents a Glueberry error with rich context.
// It provides structured information for programmatic error handling.
type Error struct {
	// Code identifies the type of error.
	Code ErrorCode

	// Message is a human-readable description of the error.
	Message string

	// PeerID is the peer associated with the error, if any.
	PeerID peer.ID

	// Stream is the stream name associated with the error, if any.
	Stream string

	// Cause is the underlying error, if any.
	Cause error

	// Retriable indicates whether the operation can be retried.
	Retriable bool
}

// Error returns a human-readable error message.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("glueberry: %s: %v", e.Message, e.Cause)
	}
	return fmt.Sprintf("glueberry: %s", e.Message)
}

// Unwrap returns the underlying error.
func (e *Error) Unwrap() error {
	return e.Cause
}

// Is reports whether target matches this error.
// Two Glueberry errors are considered equal if they have the same error code.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return t.Code == e.Code
}

// IsRetriable returns true if the error indicates a retriable operation.
// This checks if the error is a Glueberry Error with Retriable set to true.
func IsRetriable(err error) bool {
	var gErr *Error
	if errors.As(err, &gErr) {
		return gErr.Retriable
	}
	return false
}

// IsPermanent returns true if the error indicates a permanent failure.
// Permanent failures should not be retried.
func IsPermanent(err error) bool {
	var gErr *Error
	if errors.As(err, &gErr) {
		switch gErr.Code {
		case ErrCodePeerBlacklisted, ErrCodeInvalidConfig:
			return true
		}
	}
	return false
}

// NewError creates a new Glueberry Error with the given code and message.
func NewError(code ErrorCode, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// NewErrorWithCause creates a new Glueberry Error with the given code, message, and cause.
func NewErrorWithCause(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// NewPeerError creates a new Glueberry Error associated with a specific peer.
func NewPeerError(code ErrorCode, message string, peerID peer.ID) *Error {
	return &Error{
		Code:    code,
		Message: message,
		PeerID:  peerID,
	}
}

// NewStreamError creates a new Glueberry Error associated with a specific stream.
func NewStreamError(code ErrorCode, message string, peerID peer.ID, stream string) *Error {
	return &Error{
		Code:    code,
		Message: message,
		PeerID:  peerID,
		Stream:  stream,
	}
}

// Sentinel errors for peer and address book operations.
var (
	// ErrPeerNotFound indicates the peer is not in the address book.
	ErrPeerNotFound = errors.New("peer not found in address book")

	// ErrPeerBlacklisted indicates the peer is blacklisted.
	ErrPeerBlacklisted = errors.New("peer is blacklisted")

	// ErrPeerAlreadyExists indicates the peer already exists in the address book.
	ErrPeerAlreadyExists = errors.New("peer already exists in address book")
)

// Sentinel errors for connection operations.
var (
	// ErrNotConnected indicates there is no active connection to the peer.
	ErrNotConnected = errors.New("not connected to peer")

	// ErrAlreadyConnected indicates a connection to the peer already exists.
	ErrAlreadyConnected = errors.New("already connected to peer")

	// ErrConnectionFailed indicates the connection attempt failed.
	ErrConnectionFailed = errors.New("connection failed")

	// ErrHandshakeTimeout indicates the handshake did not complete in time.
	ErrHandshakeTimeout = errors.New("handshake timeout")

	// ErrHandshakeNotComplete indicates encrypted streams were requested
	// before the handshake was completed.
	ErrHandshakeNotComplete = errors.New("handshake not complete")

	// ErrReconnectionCancelled indicates the reconnection was cancelled by the app.
	ErrReconnectionCancelled = errors.New("reconnection cancelled")

	// ErrInCooldown indicates the peer is in cooldown after a failed handshake.
	ErrInCooldown = errors.New("peer is in cooldown period")
)

// Sentinel errors for stream operations.
var (
	// ErrStreamNotFound indicates the requested stream does not exist.
	ErrStreamNotFound = errors.New("stream not found")

	// ErrStreamClosed indicates the stream has been closed.
	ErrStreamClosed = errors.New("stream closed")

	// ErrStreamsAlreadyEstablished indicates encrypted streams were already set up.
	ErrStreamsAlreadyEstablished = errors.New("encrypted streams already established")

	// ErrNoStreamsRequested indicates no stream names were provided.
	ErrNoStreamsRequested = errors.New("no stream names requested")
)

// Sentinel errors for cryptographic operations.
var (
	// ErrInvalidPublicKey indicates the provided public key is invalid.
	ErrInvalidPublicKey = errors.New("invalid public key")

	// ErrInvalidPrivateKey indicates the provided private key is invalid.
	ErrInvalidPrivateKey = errors.New("invalid private key")

	// ErrEncryptionFailed indicates message encryption failed.
	ErrEncryptionFailed = errors.New("encryption failed")

	// ErrDecryptionFailed indicates message decryption failed,
	// possibly due to tampering or wrong key.
	ErrDecryptionFailed = errors.New("decryption failed")

	// ErrKeyDerivationFailed indicates the shared key derivation failed.
	ErrKeyDerivationFailed = errors.New("key derivation failed")
)

// Sentinel errors for configuration.
var (
	// ErrInvalidConfig indicates the configuration is invalid.
	ErrInvalidConfig = errors.New("invalid configuration")

	// ErrMissingPrivateKey indicates no private key was provided.
	ErrMissingPrivateKey = errors.New("private key is required")

	// ErrMissingAddressBookPath indicates no address book path was provided.
	ErrMissingAddressBookPath = errors.New("address book path is required")

	// ErrMissingListenAddrs indicates no listen addresses were provided.
	ErrMissingListenAddrs = errors.New("at least one listen address is required")
)

// Sentinel errors for protocol versioning.
var (
	// ErrVersionMismatch indicates incompatible protocol versions.
	ErrVersionMismatch = errors.New("incompatible protocol version")
)

// Sentinel errors for node operations.
var (
	// ErrNodeNotStarted indicates the node has not been started.
	ErrNodeNotStarted = errors.New("node not started")

	// ErrNodeAlreadyStarted indicates the node is already running.
	ErrNodeAlreadyStarted = errors.New("node already started")

	// ErrNodeStopped indicates the node has been stopped.
	ErrNodeStopped = errors.New("node stopped")
)
