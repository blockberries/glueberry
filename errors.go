// Package glueberry provides a P2P communication library built on libp2p
// with encrypted, multiplexed streams and app-controlled handshaking.
package glueberry

import "errors"

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

// Sentinel errors for node operations.
var (
	// ErrNodeNotStarted indicates the node has not been started.
	ErrNodeNotStarted = errors.New("node not started")

	// ErrNodeAlreadyStarted indicates the node is already running.
	ErrNodeAlreadyStarted = errors.New("node already started")

	// ErrNodeStopped indicates the node has been stopped.
	ErrNodeStopped = errors.New("node stopped")
)
