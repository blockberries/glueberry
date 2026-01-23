package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"golang.org/x/crypto/curve25519"
)

func generateX25519KeyPair(t *testing.T) (privateKey, publicKey []byte) {
	t.Helper()
	privateKey = make([]byte, X25519KeySize)
	if _, err := rand.Read(privateKey); err != nil {
		t.Fatalf("failed to generate random bytes: %v", err)
	}
	clampX25519(privateKey)

	var err error
	publicKey, err = curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		t.Fatalf("failed to compute X25519 public key: %v", err)
	}
	return
}

func TestComputeX25519SharedSecret(t *testing.T) {
	priv1, pub1 := generateX25519KeyPair(t)
	priv2, pub2 := generateX25519KeyPair(t)

	// Compute shared secret both ways
	secret1, err := ComputeX25519SharedSecret(priv1, pub2)
	if err != nil {
		t.Fatalf("ComputeX25519SharedSecret(priv1, pub2) failed: %v", err)
	}

	secret2, err := ComputeX25519SharedSecret(priv2, pub1)
	if err != nil {
		t.Fatalf("ComputeX25519SharedSecret(priv2, pub1) failed: %v", err)
	}

	// Both should produce the same secret
	if !bytes.Equal(secret1, secret2) {
		t.Error("shared secrets should match")
	}

	// Secret should be 32 bytes
	if len(secret1) != X25519KeySize {
		t.Errorf("shared secret size = %d, want %d", len(secret1), X25519KeySize)
	}
}

func TestComputeX25519SharedSecret_InvalidInputs(t *testing.T) {
	priv, pub := generateX25519KeyPair(t)

	tests := []struct {
		name       string
		privateKey []byte
		publicKey  []byte
	}{
		{
			name:       "nil private key",
			privateKey: nil,
			publicKey:  pub,
		},
		{
			name:       "nil public key",
			privateKey: priv,
			publicKey:  nil,
		},
		{
			name:       "short private key",
			privateKey: make([]byte, 16),
			publicKey:  pub,
		},
		{
			name:       "short public key",
			privateKey: priv,
			publicKey:  make([]byte, 16),
		},
		{
			name:       "long private key",
			privateKey: make([]byte, 64),
			publicKey:  pub,
		},
		{
			name:       "long public key",
			privateKey: priv,
			publicKey:  make([]byte, 64),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ComputeX25519SharedSecret(tt.privateKey, tt.publicKey)
			if err == nil {
				t.Error("expected error for invalid input")
			}
		})
	}
}

func TestComputeX25519SharedSecret_LowOrderPoint(t *testing.T) {
	priv, _ := generateX25519KeyPair(t)

	// All-zeros public key is a low-order point
	lowOrderPub := make([]byte, X25519KeySize)

	_, err := ComputeX25519SharedSecret(priv, lowOrderPub)
	if err == nil {
		t.Error("expected error for low-order point")
	}
}

func TestDeriveSharedKey(t *testing.T) {
	priv1, pub1 := generateX25519KeyPair(t)
	priv2, pub2 := generateX25519KeyPair(t)

	// Derive shared key both ways
	key1, err := DeriveSharedKey(priv1, pub2, nil)
	if err != nil {
		t.Fatalf("DeriveSharedKey(priv1, pub2) failed: %v", err)
	}

	key2, err := DeriveSharedKey(priv2, pub1, nil)
	if err != nil {
		t.Fatalf("DeriveSharedKey(priv2, pub1) failed: %v", err)
	}

	// Both should produce the same key
	if !bytes.Equal(key1, key2) {
		t.Error("derived keys should match")
	}

	// Key should be SharedKeySize bytes
	if len(key1) != SharedKeySize {
		t.Errorf("derived key size = %d, want %d", len(key1), SharedKeySize)
	}
}

func TestDeriveSharedKey_WithSalt(t *testing.T) {
	priv1, pub1 := generateX25519KeyPair(t)
	priv2, pub2 := generateX25519KeyPair(t)

	salt := []byte("test-salt-12345")

	// Derive with salt both ways
	key1, err := DeriveSharedKey(priv1, pub2, salt)
	if err != nil {
		t.Fatalf("DeriveSharedKey with salt failed: %v", err)
	}

	key2, err := DeriveSharedKey(priv2, pub1, salt)
	if err != nil {
		t.Fatalf("DeriveSharedKey with salt failed: %v", err)
	}

	// Both should produce the same key
	if !bytes.Equal(key1, key2) {
		t.Error("derived keys with same salt should match")
	}

	// Derive without salt
	keyNoSalt, err := DeriveSharedKey(priv1, pub2, nil)
	if err != nil {
		t.Fatalf("DeriveSharedKey without salt failed: %v", err)
	}

	// Keys with and without salt should differ
	if bytes.Equal(key1, keyNoSalt) {
		t.Error("derived keys with different salts should differ")
	}
}

func TestDeriveSharedKey_DifferentSalts(t *testing.T) {
	priv1, _ := generateX25519KeyPair(t)
	_, pub2 := generateX25519KeyPair(t)

	salt1 := []byte("salt-one")
	salt2 := []byte("salt-two")

	key1, err := DeriveSharedKey(priv1, pub2, salt1)
	if err != nil {
		t.Fatalf("DeriveSharedKey with salt1 failed: %v", err)
	}

	key2, err := DeriveSharedKey(priv1, pub2, salt2)
	if err != nil {
		t.Fatalf("DeriveSharedKey with salt2 failed: %v", err)
	}

	// Different salts should produce different keys
	if bytes.Equal(key1, key2) {
		t.Error("derived keys with different salts should differ")
	}
}

func TestDeriveSharedKey_Deterministic(t *testing.T) {
	priv, pub := generateX25519KeyPair(t)
	_, remotePub := generateX25519KeyPair(t)

	// Derive the same key multiple times
	key1, err := DeriveSharedKey(priv, remotePub, nil)
	if err != nil {
		t.Fatalf("first derivation failed: %v", err)
	}

	key2, err := DeriveSharedKey(priv, remotePub, nil)
	if err != nil {
		t.Fatalf("second derivation failed: %v", err)
	}

	key3, err := DeriveSharedKey(priv, remotePub, nil)
	if err != nil {
		t.Fatalf("third derivation failed: %v", err)
	}

	// All derivations should produce the same key
	if !bytes.Equal(key1, key2) || !bytes.Equal(key2, key3) {
		t.Error("key derivation should be deterministic")
	}

	// Should be different from derivation with our own public key
	keyWithSelf, err := DeriveSharedKey(priv, pub, nil)
	if err != nil {
		t.Fatalf("self derivation failed: %v", err)
	}
	if bytes.Equal(key1, keyWithSelf) {
		t.Error("derivation with different public keys should produce different results")
	}
}

func TestX25519PublicFromPrivate(t *testing.T) {
	priv, expectedPub := generateX25519KeyPair(t)

	pub, err := X25519PublicFromPrivate(priv)
	if err != nil {
		t.Fatalf("X25519PublicFromPrivate failed: %v", err)
	}

	if !bytes.Equal(pub, expectedPub) {
		t.Error("derived public key doesn't match")
	}
}

func TestX25519PublicFromPrivate_InvalidInput(t *testing.T) {
	tests := []struct {
		name       string
		privateKey []byte
	}{
		{
			name:       "nil key",
			privateKey: nil,
		},
		{
			name:       "empty key",
			privateKey: []byte{},
		},
		{
			name:       "short key",
			privateKey: make([]byte, 16),
		},
		{
			name:       "long key",
			privateKey: make([]byte, 64),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := X25519PublicFromPrivate(tt.privateKey)
			if err == nil {
				t.Error("expected error for invalid input")
			}
		})
	}
}

func TestDeriveSharedKey_WithEd25519Keys(t *testing.T) {
	// Test the full flow: Ed25519 -> X25519 -> ECDH -> shared key
	_, ed1 := generateEd25519KeyForECDH(t)
	_, ed2 := generateEd25519KeyForECDH(t)

	// Convert to X25519
	x1Priv, err := Ed25519PrivateToX25519(ed1)
	if err != nil {
		t.Fatalf("Ed25519PrivateToX25519(ed1) failed: %v", err)
	}
	x1Pub, err := X25519PublicFromPrivate(x1Priv)
	if err != nil {
		t.Fatalf("X25519PublicFromPrivate(x1Priv) failed: %v", err)
	}

	x2Priv, err := Ed25519PrivateToX25519(ed2)
	if err != nil {
		t.Fatalf("Ed25519PrivateToX25519(ed2) failed: %v", err)
	}
	x2Pub, err := X25519PublicFromPrivate(x2Priv)
	if err != nil {
		t.Fatalf("X25519PublicFromPrivate(x2Priv) failed: %v", err)
	}

	// Derive shared keys
	key1, err := DeriveSharedKey(x1Priv, x2Pub, nil)
	if err != nil {
		t.Fatalf("DeriveSharedKey(x1, x2) failed: %v", err)
	}

	key2, err := DeriveSharedKey(x2Priv, x1Pub, nil)
	if err != nil {
		t.Fatalf("DeriveSharedKey(x2, x1) failed: %v", err)
	}

	if !bytes.Equal(key1, key2) {
		t.Error("shared keys from Ed25519-derived X25519 keys should match")
	}
}

func generateEd25519KeyForECDH(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate Ed25519 key: %v", err)
	}
	return pub, priv
}
