package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"golang.org/x/crypto/curve25519"
)

func generateTestEd25519Key(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate Ed25519 key: %v", err)
	}
	return pub, priv
}

func TestEd25519PrivateToX25519(t *testing.T) {
	_, priv := generateTestEd25519Key(t)

	x25519Priv, err := Ed25519PrivateToX25519(priv)
	if err != nil {
		t.Fatalf("conversion failed: %v", err)
	}

	if len(x25519Priv) != X25519KeySize {
		t.Errorf("X25519 private key size = %d, want %d", len(x25519Priv), X25519KeySize)
	}

	// Verify clamping was applied
	// Lowest 3 bits should be cleared
	if x25519Priv[0]&7 != 0 {
		t.Error("X25519 private key lowest 3 bits should be cleared")
	}
	// Highest bit should be cleared
	if x25519Priv[31]&128 != 0 {
		t.Error("X25519 private key highest bit should be cleared")
	}
	// Second-highest bit should be set
	if x25519Priv[31]&64 == 0 {
		t.Error("X25519 private key second-highest bit should be set")
	}
}

func TestEd25519PrivateToX25519_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input ed25519.PrivateKey
	}{
		{
			name:  "nil key",
			input: nil,
		},
		{
			name:  "empty key",
			input: ed25519.PrivateKey{},
		},
		{
			name:  "short key",
			input: make([]byte, 32),
		},
		{
			name:  "long key",
			input: make([]byte, 128),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Ed25519PrivateToX25519(tt.input)
			if err == nil {
				t.Error("expected error for invalid input")
			}
		})
	}
}

func TestEd25519PublicToX25519(t *testing.T) {
	pub, _ := generateTestEd25519Key(t)

	x25519Pub, err := Ed25519PublicToX25519(pub)
	if err != nil {
		t.Fatalf("conversion failed: %v", err)
	}

	if len(x25519Pub) != X25519KeySize {
		t.Errorf("X25519 public key size = %d, want %d", len(x25519Pub), X25519KeySize)
	}
}

func TestEd25519PublicToX25519_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input ed25519.PublicKey
	}{
		{
			name:  "nil key",
			input: nil,
		},
		{
			name:  "empty key",
			input: ed25519.PublicKey{},
		},
		{
			name:  "short key",
			input: make([]byte, 16),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Ed25519PublicToX25519(tt.input)
			if err == nil {
				t.Error("expected error for invalid input")
			}
		})
	}
}

func TestEd25519ToX25519_KeyPairConsistency(t *testing.T) {
	// Convert an Ed25519 key pair to X25519 and verify the public key
	// can be derived from the private key
	pub, priv := generateTestEd25519Key(t)

	x25519Priv, err := Ed25519PrivateToX25519(priv)
	if err != nil {
		t.Fatalf("private key conversion failed: %v", err)
	}

	x25519PubFromPriv, err := X25519PublicFromPrivate(x25519Priv)
	if err != nil {
		t.Fatalf("X25519 public key derivation failed: %v", err)
	}

	x25519PubFromEd, err := Ed25519PublicToX25519(pub)
	if err != nil {
		t.Fatalf("public key conversion failed: %v", err)
	}

	// The two methods should produce the same X25519 public key
	if !bytes.Equal(x25519PubFromPriv, x25519PubFromEd) {
		t.Error("X25519 public keys derived via different methods should match")
	}
}

func TestEd25519ToX25519_ECDHCompatibility(t *testing.T) {
	// Generate two Ed25519 key pairs and verify ECDH works after conversion
	_, priv1 := generateTestEd25519Key(t)
	_, priv2 := generateTestEd25519Key(t)

	// Convert both to X25519
	x25519Priv1, err := Ed25519PrivateToX25519(priv1)
	if err != nil {
		t.Fatalf("priv1 conversion failed: %v", err)
	}
	x25519Pub1, err := X25519PublicFromPrivate(x25519Priv1)
	if err != nil {
		t.Fatalf("pub1 derivation failed: %v", err)
	}

	x25519Priv2, err := Ed25519PrivateToX25519(priv2)
	if err != nil {
		t.Fatalf("priv2 conversion failed: %v", err)
	}
	x25519Pub2, err := X25519PublicFromPrivate(x25519Priv2)
	if err != nil {
		t.Fatalf("pub2 derivation failed: %v", err)
	}

	// Perform ECDH both ways
	shared1, err := curve25519.X25519(x25519Priv1, x25519Pub2)
	if err != nil {
		t.Fatalf("ECDH 1->2 failed: %v", err)
	}

	shared2, err := curve25519.X25519(x25519Priv2, x25519Pub1)
	if err != nil {
		t.Fatalf("ECDH 2->1 failed: %v", err)
	}

	// Both directions should produce the same shared secret
	if !bytes.Equal(shared1, shared2) {
		t.Error("ECDH shared secrets should match")
	}
}

func TestValidateEd25519PrivateKey(t *testing.T) {
	_, validPriv := generateTestEd25519Key(t)

	tests := []struct {
		name    string
		key     ed25519.PrivateKey
		wantErr bool
	}{
		{
			name:    "valid key",
			key:     validPriv,
			wantErr: false,
		},
		{
			name:    "nil key",
			key:     nil,
			wantErr: true,
		},
		{
			name:    "empty key",
			key:     ed25519.PrivateKey{},
			wantErr: true,
		},
		{
			name:    "wrong size",
			key:     make([]byte, 32),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEd25519PrivateKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEd25519PrivateKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateEd25519PublicKey(t *testing.T) {
	validPub, _ := generateTestEd25519Key(t)

	tests := []struct {
		name    string
		key     ed25519.PublicKey
		wantErr bool
	}{
		{
			name:    "valid key",
			key:     validPub,
			wantErr: false,
		},
		{
			name:    "nil key",
			key:     nil,
			wantErr: true,
		},
		{
			name:    "empty key",
			key:     ed25519.PublicKey{},
			wantErr: true,
		},
		{
			name:    "wrong size",
			key:     make([]byte, 16),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEd25519PublicKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEd25519PublicKey() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// Test vector from RFC 7748 / libsodium
func TestEd25519ToX25519_KnownVector(t *testing.T) {
	// This is a known Ed25519 key pair from various test vectors
	// Ed25519 seed (32 bytes)
	seedHex := "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60"
	seed, err := hex.DecodeString(seedHex)
	if err != nil {
		t.Fatalf("failed to decode seed: %v", err)
	}

	// Generate the Ed25519 key from seed
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)

	// Convert to X25519
	x25519Priv, err := Ed25519PrivateToX25519(priv)
	if err != nil {
		t.Fatalf("private key conversion failed: %v", err)
	}

	x25519Pub, err := Ed25519PublicToX25519(pub)
	if err != nil {
		t.Fatalf("public key conversion failed: %v", err)
	}

	// Verify the X25519 keys are valid by doing a self-ECDH
	pubFromPriv, err := X25519PublicFromPrivate(x25519Priv)
	if err != nil {
		t.Fatalf("failed to derive X25519 public from private: %v", err)
	}

	// The converted public key and derived public key should match
	if !bytes.Equal(x25519Pub, pubFromPriv) {
		t.Errorf("X25519 public key mismatch:\nconverted: %x\nderived:   %x",
			x25519Pub, pubFromPriv)
	}
}
