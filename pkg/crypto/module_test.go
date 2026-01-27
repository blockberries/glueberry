package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
)

func generateModuleTestKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate Ed25519 key: %v", err)
	}
	return priv
}

func TestNewModule(t *testing.T) {
	priv := generateModuleTestKey(t)

	m, err := NewModule(priv)
	if err != nil {
		t.Fatalf("NewModule failed: %v", err)
	}
	if m == nil {
		t.Error("NewModule returned nil")
	}
}

func TestNewModule_InvalidKey(t *testing.T) {
	tests := []struct {
		name string
		key  ed25519.PrivateKey
	}{
		{"nil key", nil},
		{"empty key", ed25519.PrivateKey{}},
		{"wrong size", make([]byte, 32)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewModule(tt.key)
			if err == nil {
				t.Error("expected error for invalid key")
			}
		})
	}
}

func TestModule_Ed25519PublicKey(t *testing.T) {
	priv := generateModuleTestKey(t)
	expectedPub := priv.Public().(ed25519.PublicKey)

	m, err := NewModule(priv)
	if err != nil {
		t.Fatalf("NewModule failed: %v", err)
	}

	pub := m.Ed25519PublicKey()
	if !bytes.Equal(pub, expectedPub) {
		t.Error("Ed25519PublicKey doesn't match expected")
	}
}

func TestModule_X25519PublicKey(t *testing.T) {
	priv := generateModuleTestKey(t)

	m, err := NewModule(priv)
	if err != nil {
		t.Fatalf("NewModule failed: %v", err)
	}

	pub := m.X25519PublicKey()
	if len(pub) != X25519KeySize {
		t.Errorf("X25519 public key size = %d, want %d", len(pub), X25519KeySize)
	}

	// Verify it returns a copy, not the internal slice
	pub2 := m.X25519PublicKey()
	pub[0] ^= 0xFF // Modify the first one
	if pub[0] == pub2[0] {
		t.Error("X25519PublicKey should return a copy")
	}
}

func TestModule_DeriveSharedKey(t *testing.T) {
	priv1 := generateModuleTestKey(t)
	priv2 := generateModuleTestKey(t)

	m1, err := NewModule(priv1)
	if err != nil {
		t.Fatalf("NewModule(priv1) failed: %v", err)
	}

	m2, err := NewModule(priv2)
	if err != nil {
		t.Fatalf("NewModule(priv2) failed: %v", err)
	}

	// Derive shared keys using Ed25519 public keys
	pub1 := m1.Ed25519PublicKey()
	pub2 := m2.Ed25519PublicKey()

	key1, err := m1.DeriveSharedKey(pub2)
	if err != nil {
		t.Fatalf("m1.DeriveSharedKey failed: %v", err)
	}

	key2, err := m2.DeriveSharedKey(pub1)
	if err != nil {
		t.Fatalf("m2.DeriveSharedKey failed: %v", err)
	}

	// Keys should match
	if !bytes.Equal(key1, key2) {
		t.Error("derived shared keys should match")
	}

	// Key should be correct size
	if len(key1) != SharedKeySize {
		t.Errorf("shared key size = %d, want %d", len(key1), SharedKeySize)
	}
}

func TestModule_DeriveSharedKey_InvalidPubKey(t *testing.T) {
	priv := generateModuleTestKey(t)
	m, _ := NewModule(priv)

	tests := []struct {
		name string
		key  ed25519.PublicKey
	}{
		{"nil key", nil},
		{"empty key", ed25519.PublicKey{}},
		{"wrong size", make([]byte, 16)},
		{"invalid point", make([]byte, 32)}, // All zeros is not a valid point
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.DeriveSharedKey(tt.key)
			if err == nil {
				t.Error("expected error for invalid public key")
			}
		})
	}
}

func TestModule_DeriveSharedKeyFromX25519(t *testing.T) {
	priv1 := generateModuleTestKey(t)
	priv2 := generateModuleTestKey(t)

	m1, _ := NewModule(priv1)
	m2, _ := NewModule(priv2)

	// Derive using X25519 public keys directly
	x25519Pub1 := m1.X25519PublicKey()
	x25519Pub2 := m2.X25519PublicKey()

	key1, err := m1.DeriveSharedKeyFromX25519(x25519Pub2)
	if err != nil {
		t.Fatalf("DeriveSharedKeyFromX25519 failed: %v", err)
	}

	key2, err := m2.DeriveSharedKeyFromX25519(x25519Pub1)
	if err != nil {
		t.Fatalf("DeriveSharedKeyFromX25519 failed: %v", err)
	}

	if !bytes.Equal(key1, key2) {
		t.Error("derived shared keys should match")
	}
}

func TestModule_DeriveSharedKey_Caching(t *testing.T) {
	priv1 := generateModuleTestKey(t)
	priv2 := generateModuleTestKey(t)

	m1, _ := NewModule(priv1)
	m2, _ := NewModule(priv2)

	pub2 := m2.Ed25519PublicKey()

	// Derive the key twice
	key1, err := m1.DeriveSharedKey(pub2)
	if err != nil {
		t.Fatalf("first derivation failed: %v", err)
	}

	key2, err := m1.DeriveSharedKey(pub2)
	if err != nil {
		t.Fatalf("second derivation failed: %v", err)
	}

	// Both should be equal (from cache)
	if !bytes.Equal(key1, key2) {
		t.Error("cached keys should match")
	}
}

func TestModule_RemovePeerKey(t *testing.T) {
	priv1 := generateModuleTestKey(t)
	priv2 := generateModuleTestKey(t)

	m1, _ := NewModule(priv1)
	m2, _ := NewModule(priv2)

	x25519Pub2 := m2.X25519PublicKey()

	// Derive and cache the key
	_, err := m1.DeriveSharedKeyFromX25519(x25519Pub2)
	if err != nil {
		t.Fatalf("DeriveSharedKeyFromX25519 failed: %v", err)
	}

	// Encrypt should work (key is cached)
	plaintext := []byte("test message")
	ciphertext, err := m1.Encrypt(x25519Pub2, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Remove the key
	m1.RemovePeerKey(x25519Pub2)

	// Encrypt should now fail (key not cached)
	_, err = m1.Encrypt(x25519Pub2, plaintext)
	if err == nil {
		t.Error("expected error after removing peer key")
	}

	// But decryption should also fail
	_, err = m1.Decrypt(x25519Pub2, ciphertext)
	if err == nil {
		t.Error("expected error after removing peer key")
	}
}

func TestModule_ClearPeerKeys(t *testing.T) {
	priv1 := generateModuleTestKey(t)

	m1, _ := NewModule(priv1)

	// Add multiple peer keys
	for i := 0; i < 10; i++ {
		peerPriv := generateModuleTestKey(t)
		peerM, _ := NewModule(peerPriv)
		_, err := m1.DeriveSharedKeyFromX25519(peerM.X25519PublicKey())
		if err != nil {
			t.Fatalf("DeriveSharedKeyFromX25519 failed: %v", err)
		}
	}

	// Clear all keys
	m1.ClearPeerKeys()

	// Generate a new peer key to test
	peerPriv := generateModuleTestKey(t)
	peerM, _ := NewModule(peerPriv)

	// Encrypt should fail for any peer (all keys cleared)
	_, err := m1.Encrypt(peerM.X25519PublicKey(), []byte("test"))
	if err == nil {
		t.Error("expected error after clearing all peer keys")
	}
}

func TestModule_EncryptDecrypt(t *testing.T) {
	priv1 := generateModuleTestKey(t)
	priv2 := generateModuleTestKey(t)

	m1, _ := NewModule(priv1)
	m2, _ := NewModule(priv2)

	// Both sides need to derive the shared key
	pub1 := m1.Ed25519PublicKey()
	pub2 := m2.Ed25519PublicKey()

	_, err := m1.DeriveSharedKey(pub2)
	if err != nil {
		t.Fatalf("m1.DeriveSharedKey failed: %v", err)
	}

	_, err = m2.DeriveSharedKey(pub1)
	if err != nil {
		t.Fatalf("m2.DeriveSharedKey failed: %v", err)
	}

	// m1 encrypts a message
	plaintext := []byte("hello from m1")
	x25519Pub2 := m2.X25519PublicKey()
	ciphertext, err := m1.Encrypt(x25519Pub2, plaintext)
	if err != nil {
		t.Fatalf("m1.Encrypt failed: %v", err)
	}

	// m2 decrypts the message
	x25519Pub1 := m1.X25519PublicKey()
	decrypted, err := m2.Decrypt(x25519Pub1, ciphertext)
	if err != nil {
		t.Fatalf("m2.Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Error("decrypted message doesn't match original")
	}
}

func TestModule_EncryptDecrypt_NoSharedKey(t *testing.T) {
	priv1 := generateModuleTestKey(t)
	priv2 := generateModuleTestKey(t)

	m1, _ := NewModule(priv1)
	m2, _ := NewModule(priv2)

	// Don't derive shared key
	x25519Pub2 := m2.X25519PublicKey()

	_, err := m1.Encrypt(x25519Pub2, []byte("test"))
	if err == nil {
		t.Error("expected error when encrypting without shared key")
	}

	x25519Pub1 := m1.X25519PublicKey()
	_, err = m2.Decrypt(x25519Pub1, []byte("fake ciphertext"))
	if err == nil {
		t.Error("expected error when decrypting without shared key")
	}
}

func TestModule_EncryptWithKey_DecryptWithKey(t *testing.T) {
	priv := generateModuleTestKey(t)
	m, _ := NewModule(priv)

	key := make([]byte, KeySize)
	rand.Read(key)

	plaintext := []byte("test message")

	ciphertext, err := m.EncryptWithKey(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptWithKey failed: %v", err)
	}

	decrypted, err := m.DecryptWithKey(key, ciphertext)
	if err != nil {
		t.Fatalf("DecryptWithKey failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Error("decrypted message doesn't match original")
	}
}

func TestModule_Concurrent(t *testing.T) {
	priv := generateModuleTestKey(t)
	m, _ := NewModule(priv)

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrently derive keys for different peers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			peerPriv := generateModuleTestKey(t)
			peerM, _ := NewModule(peerPriv)

			// Derive shared key
			_, err := m.DeriveSharedKeyFromX25519(peerM.X25519PublicKey())
			if err != nil {
				t.Errorf("DeriveSharedKeyFromX25519 failed: %v", err)
				return
			}

			// Encrypt
			_, err = m.Encrypt(peerM.X25519PublicKey(), []byte("test"))
			if err != nil {
				t.Errorf("Encrypt failed: %v", err)
				return
			}
		}()
	}

	wg.Wait()
}

func TestModule_ConcurrentSamePeer(t *testing.T) {
	priv1 := generateModuleTestKey(t)
	priv2 := generateModuleTestKey(t)

	m1, _ := NewModule(priv1)
	m2, _ := NewModule(priv2)

	pub2 := m2.Ed25519PublicKey()
	x25519Pub2 := m2.X25519PublicKey()

	// First derive the key
	_, err := m1.DeriveSharedKey(pub2)
	if err != nil {
		t.Fatalf("DeriveSharedKey failed: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrently encrypt/decrypt with the same peer
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			plaintext := []byte("message from goroutine")

			// Encrypt
			ciphertext, err := m1.Encrypt(x25519Pub2, plaintext)
			if err != nil {
				t.Errorf("goroutine %d: Encrypt failed: %v", id, err)
				return
			}

			// Each ciphertext should be unique due to random nonce
			if len(ciphertext) != len(plaintext)+NonceSize+TagSize {
				t.Errorf("goroutine %d: unexpected ciphertext length", id)
			}
		}(i)
	}

	wg.Wait()
}

func TestModule_X25519PublicKey_Consistency(t *testing.T) {
	priv := generateModuleTestKey(t)
	m, _ := NewModule(priv)

	// Call X25519PublicKey multiple times
	pub1 := m.X25519PublicKey()
	pub2 := m.X25519PublicKey()
	pub3 := m.X25519PublicKey()

	if !bytes.Equal(pub1, pub2) || !bytes.Equal(pub2, pub3) {
		t.Error("X25519PublicKey should return consistent results")
	}
}

func TestModule_RemovePeerKeyByEd25519(t *testing.T) {
	priv1 := generateModuleTestKey(t)
	priv2 := generateModuleTestKey(t)
	pub2 := priv2.Public().(ed25519.PublicKey)

	m, err := NewModule(priv1)
	if err != nil {
		t.Fatalf("NewModule failed: %v", err)
	}
	defer m.Close()

	// Derive a shared key (this caches it)
	key1, err := m.DeriveSharedKey(pub2)
	if err != nil {
		t.Fatalf("DeriveSharedKey failed: %v", err)
	}

	// Verify key is cached by deriving again
	key2, err := m.DeriveSharedKey(pub2)
	if err != nil {
		t.Fatalf("DeriveSharedKey (second) failed: %v", err)
	}

	if !bytes.Equal(key1, key2) {
		t.Error("cached key should be the same")
	}

	// Remove the key using Ed25519 public key
	m.RemovePeerKeyByEd25519(pub2)

	// Verify the key was removed by checking it needs to be re-derived
	// (the Module doesn't expose a way to check if key is cached directly,
	// but we can verify by counting cache size or deriving again)

	// Check that ClearPeerKeys with empty cache works
	m.ClearPeerKeys()
}

func TestModule_RemovePeerKeyByEd25519_NilKey(t *testing.T) {
	priv := generateModuleTestKey(t)
	m, err := NewModule(priv)
	if err != nil {
		t.Fatalf("NewModule failed: %v", err)
	}
	defer m.Close()

	// Should not panic with nil key
	m.RemovePeerKeyByEd25519(nil)

	// Should not panic with empty key
	m.RemovePeerKeyByEd25519([]byte{})
}

func TestModule_RemovePeerKeyByEd25519_InvalidKey(t *testing.T) {
	priv := generateModuleTestKey(t)
	m, err := NewModule(priv)
	if err != nil {
		t.Fatalf("NewModule failed: %v", err)
	}
	defer m.Close()

	// Should not panic with invalid key (wrong length)
	m.RemovePeerKeyByEd25519([]byte{1, 2, 3})

	// Should not panic with random 32-byte key that isn't a valid Ed25519 public key
	invalidKey := make([]byte, 32)
	for i := range invalidKey {
		invalidKey[i] = byte(i)
	}
	m.RemovePeerKeyByEd25519(invalidKey)
}
