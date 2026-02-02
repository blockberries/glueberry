// Package fuzz provides fuzz tests for Glueberry components.
// Run with: go test -fuzz=. -fuzztime=30s ./fuzz/
package fuzz

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/blockberries/glueberry/pkg/crypto"
)

// FuzzDecrypt tests the Cipher.Decrypt function with random/malformed ciphertext.
// This helps find panics, buffer overflows, or other issues when handling
// corrupted or malicious ciphertext data.
func FuzzDecrypt(f *testing.F) {
	// Add seed corpus
	key := make([]byte, 32)
	rand.Read(key)

	// Valid ciphertext (encrypt some data first)
	cipher, _ := crypto.NewCipher(key)
	validCiphertext, _ := cipher.Encrypt([]byte("test message"), nil)
	f.Add(key, validCiphertext, []byte{})

	// Too short ciphertext
	f.Add(key, []byte{1, 2, 3}, []byte{})

	// Empty ciphertext
	f.Add(key, []byte{}, []byte{})

	// Random garbage
	garbage := make([]byte, 100)
	rand.Read(garbage)
	f.Add(key, garbage, []byte{})

	// Ciphertext with modified tag (should fail auth)
	modifiedCiphertext := make([]byte, len(validCiphertext))
	copy(modifiedCiphertext, validCiphertext)
	modifiedCiphertext[len(modifiedCiphertext)-1] ^= 0xFF
	f.Add(key, modifiedCiphertext, []byte{})

	f.Fuzz(func(t *testing.T, key, ciphertext, additionalData []byte) {
		// Skip invalid key sizes (NewCipher will reject them)
		if len(key) != 32 {
			return
		}

		cipher, err := crypto.NewCipher(key)
		if err != nil {
			return
		}

		// This should not panic regardless of input
		_, _ = cipher.Decrypt(ciphertext, additionalData)
	})
}

// FuzzDecryptRoundTrip tests that Encrypt/Decrypt roundtrips work correctly.
// This helps ensure the implementation is consistent.
func FuzzDecryptRoundTrip(f *testing.F) {
	// Add seed corpus with various plaintext sizes
	f.Add([]byte("short"))
	f.Add([]byte("a medium length message for testing"))
	f.Add(make([]byte, 1000))  // 1KB
	f.Add(make([]byte, 65536)) // 64KB

	f.Fuzz(func(t *testing.T, plaintext []byte) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			t.Skip("failed to generate key")
		}

		cipher, err := crypto.NewCipher(key)
		if err != nil {
			t.Fatalf("failed to create cipher: %v", err)
		}

		// Encrypt
		ciphertext, err := cipher.Encrypt(plaintext, nil)
		if err != nil {
			t.Fatalf("encryption failed: %v", err)
		}

		// Decrypt
		decrypted, err := cipher.Decrypt(ciphertext, nil)
		if err != nil {
			t.Fatalf("decryption failed: %v", err)
		}

		// Verify roundtrip
		if !bytes.Equal(plaintext, decrypted) {
			t.Errorf("roundtrip mismatch: got %d bytes, want %d bytes",
				len(decrypted), len(plaintext))
		}
	})
}

// FuzzEd25519PublicToX25519 tests the Ed25519 to X25519 public key conversion
// with random/malformed public keys.
func FuzzEd25519PublicToX25519(f *testing.F) {
	// Add seed corpus

	// Valid Ed25519 public key
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	validPub := priv.Public().(ed25519.PublicKey)
	f.Add([]byte(validPub))

	// Too short
	f.Add([]byte{1, 2, 3, 4})

	// Too long
	longKey := make([]byte, 64)
	rand.Read(longKey)
	f.Add(longKey)

	// Empty
	f.Add([]byte{})

	// All zeros (invalid point)
	f.Add(make([]byte, 32))

	// All ones
	allOnes := make([]byte, 32)
	for i := range allOnes {
		allOnes[i] = 0xFF
	}
	f.Add(allOnes)

	f.Fuzz(func(t *testing.T, pubKey []byte) {
		// This should not panic regardless of input
		_, _ = crypto.Ed25519PublicToX25519(ed25519.PublicKey(pubKey))
	})
}

// FuzzEd25519PrivateToX25519 tests the Ed25519 to X25519 private key conversion
// with random/malformed private keys.
func FuzzEd25519PrivateToX25519(f *testing.F) {
	// Add seed corpus

	// Valid Ed25519 private key
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	f.Add([]byte(priv))

	// Too short
	f.Add([]byte{1, 2, 3, 4})

	// Too long
	longKey := make([]byte, 128)
	rand.Read(longKey)
	f.Add(longKey)

	// Empty
	f.Add([]byte{})

	// Exactly seed size (32 bytes - too short for full private key)
	seed := make([]byte, 32)
	rand.Read(seed)
	f.Add(seed)

	f.Fuzz(func(t *testing.T, privKey []byte) {
		// This should not panic regardless of input
		_, _ = crypto.Ed25519PrivateToX25519(ed25519.PrivateKey(privKey))
	})
}

// FuzzNewCipher tests the NewCipher function with various key sizes.
func FuzzNewCipher(f *testing.F) {
	// Add seed corpus
	validKey := make([]byte, 32)
	rand.Read(validKey)
	f.Add(validKey)

	// Too short
	f.Add(make([]byte, 16))

	// Too long
	f.Add(make([]byte, 64))

	// Empty
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, key []byte) {
		// This should not panic regardless of input
		cipher, err := crypto.NewCipher(key)
		if err != nil {
			return
		}

		// If cipher was created successfully, it should work
		plaintext := []byte("test")
		ciphertext, err := cipher.Encrypt(plaintext, nil)
		if err != nil {
			t.Fatalf("encryption failed with valid cipher: %v", err)
		}

		decrypted, err := cipher.Decrypt(ciphertext, nil)
		if err != nil {
			t.Fatalf("decryption failed with valid cipher: %v", err)
		}

		if !bytes.Equal(plaintext, decrypted) {
			t.Error("roundtrip failed with valid cipher")
		}
	})
}
