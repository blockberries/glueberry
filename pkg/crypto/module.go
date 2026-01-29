package crypto

import (
	"crypto/ed25519"
	"fmt"
	"sync"
)

// Module provides cryptographic operations for a Glueberry node.
// It manages the node's identity keys and provides methods for key exchange
// and encryption/decryption operations.
//
// Module is safe for concurrent use.
type Module struct {
	ed25519Private ed25519.PrivateKey
	ed25519Public  ed25519.PublicKey
	x25519Private  []byte
	x25519Public   []byte

	// peerKeys caches derived shared keys for connected peers
	// map[string][]byte where key is peer's X25519 public key as string
	peerKeys   map[string][]byte
	peerKeysMu sync.RWMutex

	// closed indicates that Close() has been called and keys have been zeroed
	closed bool
}

// NewModule creates a new crypto module with the given Ed25519 private key.
// It converts the Ed25519 key to X25519 format for use in key exchange.
func NewModule(privateKey ed25519.PrivateKey) (*Module, error) {
	if err := ValidateEd25519PrivateKey(privateKey); err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	// Extract the public key (last 32 bytes of private key)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// Convert Ed25519 keys to X25519
	x25519Priv, err := Ed25519PrivateToX25519(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert private key to X25519: %w", err)
	}

	x25519Pub, err := Ed25519PublicToX25519(publicKey)
	if err != nil {
		// Zero the X25519 private key before returning error
		SecureZero(x25519Priv)
		return nil, fmt.Errorf("failed to convert public key to X25519: %w", err)
	}

	return &Module{
		ed25519Private: privateKey,
		ed25519Public:  publicKey,
		x25519Private:  x25519Priv,
		x25519Public:   x25519Pub,
		peerKeys:       make(map[string][]byte),
	}, nil
}

// Ed25519PublicKey returns the node's Ed25519 public key.
func (m *Module) Ed25519PublicKey() ed25519.PublicKey {
	return m.ed25519Public
}

// X25519PublicKey returns the node's X25519 public key for key exchange.
func (m *Module) X25519PublicKey() []byte {
	result := make([]byte, len(m.x25519Public))
	copy(result, m.x25519Public)
	return result
}

// DeriveSharedKey derives a shared encryption key from the remote peer's Ed25519 public key.
// The key is cached for subsequent calls with the same peer.
func (m *Module) DeriveSharedKey(remotePubKey ed25519.PublicKey) ([]byte, error) {
	if err := ValidateEd25519PublicKey(remotePubKey); err != nil {
		return nil, fmt.Errorf("invalid remote public key: %w", err)
	}

	// Convert remote Ed25519 public key to X25519
	remoteX25519, err := Ed25519PublicToX25519(remotePubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert remote public key: %w", err)
	}

	return m.DeriveSharedKeyFromX25519(remoteX25519)
}

// DeriveSharedKeyFromX25519 derives a shared encryption key from the remote peer's X25519 public key.
// The key is cached for subsequent calls with the same peer.
func (m *Module) DeriveSharedKeyFromX25519(remoteX25519 []byte) ([]byte, error) {
	if len(remoteX25519) != X25519KeySize {
		return nil, fmt.Errorf("invalid X25519 public key size: expected %d, got %d",
			X25519KeySize, len(remoteX25519))
	}

	// Check cache first and copy private key while holding lock
	cacheKey := string(remoteX25519)
	m.peerKeysMu.RLock()
	if m.closed {
		m.peerKeysMu.RUnlock()
		return nil, fmt.Errorf("module is closed")
	}
	if key, ok := m.peerKeys[cacheKey]; ok {
		result := make([]byte, len(key))
		copy(result, key)
		m.peerKeysMu.RUnlock()
		return result, nil
	}
	// Copy x25519Private while holding lock to prevent race with Close()
	x25519PrivCopy := make([]byte, len(m.x25519Private))
	copy(x25519PrivCopy, m.x25519Private)
	m.peerKeysMu.RUnlock()

	// Zero the copy after use
	defer SecureZero(x25519PrivCopy)

	// Derive new shared key using the copy
	sharedKey, err := DeriveSharedKey(x25519PrivCopy, remoteX25519, nil)
	if err != nil {
		return nil, fmt.Errorf("key derivation failed: %w", err)
	}

	// Cache the key
	m.peerKeysMu.Lock()
	if m.closed {
		m.peerKeysMu.Unlock()
		SecureZero(sharedKey)
		return nil, fmt.Errorf("module is closed")
	}
	m.peerKeys[cacheKey] = sharedKey
	m.peerKeysMu.Unlock()

	result := make([]byte, len(sharedKey))
	copy(result, sharedKey)
	return result, nil
}

// RemovePeerKey removes a cached shared key for a peer.
// This should be called when disconnecting from a peer.
// The key is securely zeroed before being removed from the cache.
func (m *Module) RemovePeerKey(remoteX25519 []byte) {
	cacheKey := string(remoteX25519)
	m.peerKeysMu.Lock()
	if key, ok := m.peerKeys[cacheKey]; ok {
		SecureZero(key)
	}
	delete(m.peerKeys, cacheKey)
	m.peerKeysMu.Unlock()
}

// RemovePeerKeyByEd25519 removes a cached shared key for a peer using their Ed25519 public key.
// This is a convenience method that converts the Ed25519 key to X25519 and removes the cached key.
// If the public key is nil or invalid, this is a no-op.
func (m *Module) RemovePeerKeyByEd25519(remotePubKey []byte) {
	if len(remotePubKey) == 0 {
		return
	}

	// Convert Ed25519 public key to X25519
	remoteX25519, err := Ed25519PublicToX25519(remotePubKey)
	if err != nil {
		// Invalid key - nothing to remove
		return
	}

	m.RemovePeerKey(remoteX25519)
}

// ClearPeerKeys removes all cached shared keys.
// All keys are securely zeroed before being removed from the cache.
func (m *Module) ClearPeerKeys() {
	m.peerKeysMu.Lock()
	for _, key := range m.peerKeys {
		SecureZero(key)
	}
	m.peerKeys = make(map[string][]byte)
	m.peerKeysMu.Unlock()
}

// Encrypt encrypts plaintext using the shared key derived for the given peer.
// The peer must have a cached shared key (via DeriveSharedKey).
func (m *Module) Encrypt(remoteX25519, plaintext []byte) ([]byte, error) {
	m.peerKeysMu.RLock()
	if m.closed {
		m.peerKeysMu.RUnlock()
		return nil, fmt.Errorf("module is closed")
	}
	key, ok := m.peerKeys[string(remoteX25519)]
	if !ok {
		m.peerKeysMu.RUnlock()
		return nil, fmt.Errorf("no shared key for peer: call DeriveSharedKey first")
	}
	// Copy key before releasing lock to prevent TOCTOU race with RemovePeerKey
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	m.peerKeysMu.RUnlock()

	// Zero the copy after use
	defer SecureZero(keyCopy)

	return Encrypt(keyCopy, plaintext, nil)
}

// Decrypt decrypts ciphertext using the shared key derived for the given peer.
// The peer must have a cached shared key (via DeriveSharedKey).
func (m *Module) Decrypt(remoteX25519, ciphertext []byte) ([]byte, error) {
	m.peerKeysMu.RLock()
	if m.closed {
		m.peerKeysMu.RUnlock()
		return nil, fmt.Errorf("module is closed")
	}
	key, ok := m.peerKeys[string(remoteX25519)]
	if !ok {
		m.peerKeysMu.RUnlock()
		return nil, fmt.Errorf("no shared key for peer: call DeriveSharedKey first")
	}
	// Copy key before releasing lock to prevent TOCTOU race with RemovePeerKey
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	m.peerKeysMu.RUnlock()

	// Zero the copy after use
	defer SecureZero(keyCopy)

	return Decrypt(keyCopy, ciphertext, nil)
}

// EncryptWithKey encrypts plaintext using a provided key.
// This is useful when the caller manages keys directly.
func (m *Module) EncryptWithKey(key, plaintext []byte) ([]byte, error) {
	return Encrypt(key, plaintext, nil)
}

// DecryptWithKey decrypts ciphertext using a provided key.
// This is useful when the caller manages keys directly.
func (m *Module) DecryptWithKey(key, ciphertext []byte) ([]byte, error) {
	return Decrypt(key, ciphertext, nil)
}

// Close securely zeros all key material and clears the peer key cache.
// This should be called when the Module is no longer needed.
// After Close is called, the Module should not be used.
func (m *Module) Close() {
	m.peerKeysMu.Lock()
	defer m.peerKeysMu.Unlock()

	// Mark as closed to prevent concurrent operations
	m.closed = true

	// Clear all cached peer keys
	for _, key := range m.peerKeys {
		SecureZero(key)
	}
	m.peerKeys = make(map[string][]byte)

	// Zero the private keys while holding lock
	// Note: ed25519.PrivateKey is []byte, so we can zero it directly
	SecureZero(m.ed25519Private)
	SecureZero(m.x25519Private)

	// Note: We don't zero public keys as they are not sensitive
}
