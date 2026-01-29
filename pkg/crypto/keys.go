// Package crypto provides cryptographic operations for Glueberry including
// key conversion, ECDH key exchange, and symmetric encryption.
package crypto

import (
	"crypto/ed25519"
	"crypto/sha512"
	"fmt"

	"filippo.io/edwards25519"
)

const (
	// X25519KeySize is the size of X25519 public and private keys in bytes.
	X25519KeySize = 32
)

// Ed25519PrivateToX25519 converts an Ed25519 private key to an X25519 private key.
// This follows RFC 8032 and the standard Ed25519-to-X25519 conversion process:
// 1. Hash the Ed25519 seed (first 32 bytes of private key) with SHA-512
// 2. Take the first 32 bytes and apply X25519 clamping
func Ed25519PrivateToX25519(edPriv ed25519.PrivateKey) ([]byte, error) {
	if len(edPriv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid Ed25519 private key size: expected %d, got %d",
			ed25519.PrivateKeySize, len(edPriv))
	}

	// Ed25519 private key is seed || public key
	// The seed is the first 32 bytes
	seed := edPriv[:ed25519.SeedSize]

	// Hash the seed with SHA-512
	h := sha512.Sum512(seed)

	// Zero the hash after extracting the key material
	defer SecureZero(h[:])

	// Take first 32 bytes and apply clamping for X25519
	x25519Priv := make([]byte, X25519KeySize)
	copy(x25519Priv, h[:32])
	clampX25519(x25519Priv)

	return x25519Priv, nil
}

// Ed25519PublicToX25519 converts an Ed25519 public key to an X25519 public key.
// This converts the Edwards curve point to the equivalent Montgomery curve point.
func Ed25519PublicToX25519(edPub ed25519.PublicKey) ([]byte, error) {
	if len(edPub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid Ed25519 public key size: expected %d, got %d",
			ed25519.PublicKeySize, len(edPub))
	}

	// Parse the Ed25519 public key as an Edwards point
	edPoint, err := new(edwards25519.Point).SetBytes(edPub)
	if err != nil {
		return nil, fmt.Errorf("invalid Ed25519 public key: %w", err)
	}

	// Convert Edwards point to Montgomery u-coordinate (X25519 public key)
	// Using the birational map from Edwards to Montgomery:
	// u = (1 + y) / (1 - y)
	x25519Pub := edPoint.BytesMontgomery()

	return x25519Pub, nil
}

// clampX25519 applies the standard X25519 clamping to a 32-byte private key.
// This ensures the private key is a valid scalar for Curve25519:
// - Clear the lowest 3 bits (multiple of 8 for cofactor)
// - Clear the highest bit (ensure value < 2^255)
// - Set the second-highest bit (ensure fixed-time operation)
func clampX25519(k []byte) {
	k[0] &= 248  // Clear lowest 3 bits
	k[31] &= 127 // Clear highest bit
	k[31] |= 64  // Set second-highest bit
}

// ValidateEd25519PrivateKey checks if the provided key is a valid Ed25519 private key.
func ValidateEd25519PrivateKey(key ed25519.PrivateKey) error {
	if key == nil {
		return fmt.Errorf("private key is nil")
	}
	if len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid Ed25519 private key size: expected %d, got %d",
			ed25519.PrivateKeySize, len(key))
	}
	return nil
}

// ValidateEd25519PublicKey checks if the provided key is a valid Ed25519 public key.
func ValidateEd25519PublicKey(key ed25519.PublicKey) error {
	if key == nil {
		return fmt.Errorf("public key is nil")
	}
	if len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid Ed25519 public key size: expected %d, got %d",
			ed25519.PublicKeySize, len(key))
	}
	// Verify it's a valid point on the curve
	_, err := new(edwards25519.Point).SetBytes(key)
	if err != nil {
		return fmt.Errorf("invalid Ed25519 public key: not a valid curve point")
	}
	return nil
}
