package crypto

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// NonceSize is the size of the nonce used with ChaCha20-Poly1305.
	NonceSize = chacha20poly1305.NonceSize // 12 bytes

	// TagSize is the size of the authentication tag.
	TagSize = chacha20poly1305.Overhead // 16 bytes

	// KeySize is the required key size for ChaCha20-Poly1305.
	KeySize = chacha20poly1305.KeySize // 32 bytes
)

// Cipher provides ChaCha20-Poly1305 authenticated encryption.
// It is safe for concurrent use.
//
// Call Close() when done to zero key material. Note that the underlying
// chacha20poly1305 library also stores a copy of the key internally which
// cannot be zeroed from outside the package. The Close() method zeros our
// copy for defense in depth.
type Cipher struct {
	aead   cipher
	key    []byte // Our copy for zeroing on Close
	closed bool
}

// cipher is an interface matching chacha20poly1305.AEAD for testing
type cipher interface {
	NonceSize() int
	Overhead() int
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}

// NewCipher creates a new ChaCha20-Poly1305 cipher with the given key.
// The key must be exactly 32 bytes.
// Call Close() when done to zero key material.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("invalid key size: expected %d bytes, got %d", KeySize, len(key))
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Store a copy of the key for zeroing on Close
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	return &Cipher{aead: aead, key: keyCopy}, nil
}

// Encrypt encrypts the plaintext using ChaCha20-Poly1305.
// It generates a random nonce and prepends it to the ciphertext.
// The returned data format is: [12-byte nonce][ciphertext][16-byte tag]
//
// The additionalData parameter provides authenticated but unencrypted data.
// Pass nil if no additional data is needed.
func (c *Cipher) Encrypt(plaintext, additionalData []byte) ([]byte, error) {
	// Generate a random nonce
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	return c.EncryptWithNonce(nonce, plaintext, additionalData)
}

// EncryptWithNonce encrypts the plaintext using the provided nonce.
// This is useful for testing or when deterministic encryption is needed.
// WARNING: Never reuse a nonce with the same key.
//
// The returned data format is: [12-byte nonce][ciphertext][16-byte tag]
func (c *Cipher) EncryptWithNonce(nonce, plaintext, additionalData []byte) ([]byte, error) {
	if len(nonce) != NonceSize {
		return nil, fmt.Errorf("invalid nonce size: expected %d bytes, got %d", NonceSize, len(nonce))
	}

	// Allocate space for nonce + ciphertext + tag
	// ciphertext will be same size as plaintext, plus tag
	result := make([]byte, NonceSize+len(plaintext)+TagSize)

	// Copy nonce to the beginning
	copy(result[:NonceSize], nonce)

	// Encrypt in place after the nonce
	c.aead.Seal(result[NonceSize:NonceSize], nonce, plaintext, additionalData)

	return result, nil
}

// Decrypt decrypts data that was encrypted with Encrypt.
// It extracts the nonce from the beginning of the data and verifies the authentication tag.
// Returns an error if the data is too short, the tag is invalid, or decryption fails.
//
// The additionalData must match what was provided during encryption.
func (c *Cipher) Decrypt(data, additionalData []byte) ([]byte, error) {
	if len(data) < NonceSize+TagSize {
		return nil, fmt.Errorf("ciphertext too short: minimum %d bytes, got %d",
			NonceSize+TagSize, len(data))
	}

	// Extract nonce from the beginning
	nonce := data[:NonceSize]
	ciphertext := data[NonceSize:]

	// Decrypt and verify
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: authentication tag mismatch or corrupted data")
	}

	return plaintext, nil
}

// Close zeros the key material stored in this cipher.
// After Close is called, the cipher should not be used.
//
// Note: The underlying chacha20poly1305 library also stores a copy of the key
// internally which cannot be zeroed from outside the package. This method
// zeros our copy for defense in depth.
func (c *Cipher) Close() {
	if c.closed {
		return
	}
	c.closed = true
	SecureZero(c.key)
	c.key = nil
	c.aead = nil
}

// IsClosed returns true if the cipher has been closed.
func (c *Cipher) IsClosed() bool {
	return c.closed
}

// Encrypt is a convenience function that encrypts plaintext with the given key.
// It creates a temporary cipher, encrypts the data, and returns the result.
// For multiple encryptions with the same key, create a Cipher instance instead.
func Encrypt(key, plaintext, additionalData []byte) ([]byte, error) {
	c, err := NewCipher(key)
	if err != nil {
		return nil, err
	}
	return c.Encrypt(plaintext, additionalData)
}

// Decrypt is a convenience function that decrypts data with the given key.
// It creates a temporary cipher, decrypts the data, and returns the result.
// For multiple decryptions with the same key, create a Cipher instance instead.
func Decrypt(key, data, additionalData []byte) ([]byte, error) {
	c, err := NewCipher(key)
	if err != nil {
		return nil, err
	}
	return c.Decrypt(data, additionalData)
}
