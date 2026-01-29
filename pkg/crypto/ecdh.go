package crypto

import (
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

const (
	// SharedKeySize is the size of the derived shared key in bytes.
	SharedKeySize = 32

	// hkdfInfo is the context string used in HKDF key derivation.
	hkdfInfo = "glueberry-v1-stream-key"
)

// ComputeX25519SharedSecret performs X25519 ECDH to compute a raw shared secret.
// The result should not be used directly as an encryption key; use DeriveSharedKey instead.
func ComputeX25519SharedSecret(localPrivate, remotePublic []byte) ([]byte, error) {
	if len(localPrivate) != X25519KeySize {
		return nil, fmt.Errorf("invalid X25519 private key size: expected %d, got %d",
			X25519KeySize, len(localPrivate))
	}
	if len(remotePublic) != X25519KeySize {
		return nil, fmt.Errorf("invalid X25519 public key size: expected %d, got %d",
			X25519KeySize, len(remotePublic))
	}

	// Perform X25519 scalar multiplication
	sharedSecret, err := curve25519.X25519(localPrivate, remotePublic)
	if err != nil {
		return nil, fmt.Errorf("X25519 computation failed: %w", err)
	}

	// Check for low-order point (all zeros result)
	// This can happen with malicious public keys
	allZeros := true
	for _, b := range sharedSecret {
		if b != 0 {
			allZeros = false
			break
		}
	}
	if allZeros {
		return nil, fmt.Errorf("X25519 produced all-zero output (low-order point attack)")
	}

	return sharedSecret, nil
}

// DeriveSharedKey derives a symmetric encryption key from X25519 private and public keys.
// It performs ECDH to get a shared secret, then uses HKDF-SHA256 to derive the final key.
// The optional salt can be used for domain separation; if nil, an empty salt is used.
// The raw shared secret is securely zeroed after use to prevent key material from
// persisting in memory.
func DeriveSharedKey(localPrivate, remotePublic []byte, salt []byte) ([]byte, error) {
	// Compute raw shared secret via X25519
	sharedSecret, err := ComputeX25519SharedSecret(localPrivate, remotePublic)
	if err != nil {
		return nil, err
	}
	// Always zero the raw shared secret after deriving the final key.
	// This is critical for security: the raw ECDH output should never persist in memory.
	defer SecureZero(sharedSecret)

	// Derive the final key using HKDF
	return deriveKeyFromSecret(sharedSecret, salt)
}

// deriveKeyFromSecret uses HKDF-SHA256 to derive a key from a shared secret.
func deriveKeyFromSecret(sharedSecret, salt []byte) ([]byte, error) {
	// Use HKDF with SHA-256
	// The info parameter provides domain separation
	hkdfReader := hkdf.New(sha256.New, sharedSecret, salt, []byte(hkdfInfo))

	key := make([]byte, SharedKeySize)
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, fmt.Errorf("HKDF key derivation failed: %w", err)
	}

	return key, nil
}

// X25519PublicFromPrivate computes the X25519 public key from a private key.
func X25519PublicFromPrivate(privateKey []byte) ([]byte, error) {
	if len(privateKey) != X25519KeySize {
		return nil, fmt.Errorf("invalid X25519 private key size: expected %d, got %d",
			X25519KeySize, len(privateKey))
	}

	publicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		return nil, fmt.Errorf("X25519 public key computation failed: %w", err)
	}

	return publicKey, nil
}
