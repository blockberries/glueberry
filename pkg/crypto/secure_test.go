package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestSecureZero(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{
			name: "empty slice",
			size: 0,
		},
		{
			name: "single byte",
			size: 1,
		},
		{
			name: "multiple bytes",
			size: 5,
		},
		{
			name: "32-byte key",
			size: 32,
		},
		{
			name: "64-byte key (Ed25519 private key size)",
			size: 64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.size)
			// Fill with non-zero values
			for i := range data {
				data[i] = byte(i + 1)
			}

			// Zero the data
			SecureZero(data)

			// Verify all bytes are zero
			for i, b := range data {
				if b != 0 {
					t.Errorf("byte %d is not zero: got %d", i, b)
				}
			}
		})
	}
}

func TestSecureZero_NilSlice(t *testing.T) {
	// Should not panic on nil slice
	var nilSlice []byte
	SecureZero(nilSlice)
}

func TestSecureZeroMultiple(t *testing.T) {
	// Create test slices
	slice1 := []byte{0x01, 0x02, 0x03}
	slice2 := []byte{0x04, 0x05, 0x06, 0x07}
	slice3 := []byte{0x08}

	// Zero all slices
	SecureZeroMultiple(slice1, slice2, slice3)

	// Verify all slices are zeroed
	if !bytes.Equal(slice1, []byte{0, 0, 0}) {
		t.Errorf("slice1 not zeroed: got %v", slice1)
	}
	if !bytes.Equal(slice2, []byte{0, 0, 0, 0}) {
		t.Errorf("slice2 not zeroed: got %v", slice2)
	}
	if !bytes.Equal(slice3, []byte{0}) {
		t.Errorf("slice3 not zeroed: got %v", slice3)
	}
}

func TestSecureZeroMultiple_Empty(t *testing.T) {
	// Should not panic with no arguments
	SecureZeroMultiple()
}

func TestSecureZeroMultiple_WithNil(t *testing.T) {
	// Should not panic with nil slices
	slice1 := []byte{0x01, 0x02}
	var nilSlice []byte
	slice2 := []byte{0x03, 0x04}

	SecureZeroMultiple(slice1, nilSlice, slice2)

	if !bytes.Equal(slice1, []byte{0, 0}) {
		t.Errorf("slice1 not zeroed: got %v", slice1)
	}
	if !bytes.Equal(slice2, []byte{0, 0}) {
		t.Errorf("slice2 not zeroed: got %v", slice2)
	}
}

func generateSecureTestKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate Ed25519 key: %v", err)
	}
	return priv
}

func TestModule_Close_ZerosPrivateKeys(t *testing.T) {
	// Generate a test private key
	privateKey := generateSecureTestKey(t)

	// Create module
	module, err := NewModule(privateKey)
	if err != nil {
		t.Fatalf("NewModule failed: %v", err)
	}

	// Verify x25519 private key is non-zero before close
	allZero := true
	for _, b := range module.x25519Private {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("x25519Private is all zeros before Close")
	}

	// Close the module
	module.Close()

	// Verify x25519 private key is zeroed
	for i, b := range module.x25519Private {
		if b != 0 {
			t.Errorf("x25519Private[%d] is not zero after Close: got %d", i, b)
		}
	}

	// Verify ed25519 private key is zeroed
	for i, b := range module.ed25519Private {
		if b != 0 {
			t.Errorf("ed25519Private[%d] is not zero after Close: got %d", i, b)
		}
	}
}

func TestModule_RemovePeerKey_ZerosKey(t *testing.T) {
	privateKey := generateSecureTestKey(t)
	module, err := NewModule(privateKey)
	if err != nil {
		t.Fatalf("NewModule failed: %v", err)
	}
	defer module.Close()

	// Generate a test peer key
	peerPrivKey := generateSecureTestKey(t)
	peerPubKey := peerPrivKey.Public().(ed25519.PublicKey)
	peerX25519Pub, err := Ed25519PublicToX25519(peerPubKey)
	if err != nil {
		t.Fatalf("Ed25519PublicToX25519 failed: %v", err)
	}

	// Derive shared key (this caches it)
	sharedKey, err := module.DeriveSharedKeyFromX25519(peerX25519Pub)
	if err != nil {
		t.Fatalf("DeriveSharedKeyFromX25519 failed: %v", err)
	}

	// Verify key is non-zero
	allZero := true
	for _, b := range sharedKey {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("shared key is all zeros")
	}

	// Get a reference to the cached key (internal state)
	module.peerKeysMu.RLock()
	cachedKey := module.peerKeys[string(peerX25519Pub)]
	module.peerKeysMu.RUnlock()

	if cachedKey == nil {
		t.Fatal("cached key is nil")
	}

	// Keep a reference to verify zeroing
	cachedKeyRef := cachedKey

	// Remove the peer key
	module.RemovePeerKey(peerX25519Pub)

	// Verify the cached key bytes were zeroed
	for i, b := range cachedKeyRef {
		if b != 0 {
			t.Errorf("cached key byte %d is not zero after RemovePeerKey: got %d", i, b)
		}
	}

	// Verify key is removed from cache
	module.peerKeysMu.RLock()
	_, exists := module.peerKeys[string(peerX25519Pub)]
	module.peerKeysMu.RUnlock()

	if exists {
		t.Error("key still exists in cache after RemovePeerKey")
	}
}

func TestModule_ClearPeerKeys_ZerosAllKeys(t *testing.T) {
	privateKey := generateSecureTestKey(t)
	module, err := NewModule(privateKey)
	if err != nil {
		t.Fatalf("NewModule failed: %v", err)
	}
	defer module.Close()

	// Generate multiple test peer keys and keep references to cached keys
	var cachedKeyRefs [][]byte
	for i := 0; i < 3; i++ {
		peerPrivKey := generateSecureTestKey(t)
		peerPubKey := peerPrivKey.Public().(ed25519.PublicKey)
		peerX25519Pub, err := Ed25519PublicToX25519(peerPubKey)
		if err != nil {
			t.Fatalf("Ed25519PublicToX25519 failed: %v", err)
		}

		_, err = module.DeriveSharedKeyFromX25519(peerX25519Pub)
		if err != nil {
			t.Fatalf("DeriveSharedKeyFromX25519 failed: %v", err)
		}

		// Keep reference to cached key
		module.peerKeysMu.RLock()
		cachedKeyRefs = append(cachedKeyRefs, module.peerKeys[string(peerX25519Pub)])
		module.peerKeysMu.RUnlock()
	}

	// Clear all keys
	module.ClearPeerKeys()

	// Verify all cached keys were zeroed
	for i, cachedKey := range cachedKeyRefs {
		for j, b := range cachedKey {
			if b != 0 {
				t.Errorf("cached key %d byte %d is not zero after ClearPeerKeys: got %d", i, j, b)
			}
		}
	}

	// Verify cache is empty
	module.peerKeysMu.RLock()
	cacheLen := len(module.peerKeys)
	module.peerKeysMu.RUnlock()

	if cacheLen != 0 {
		t.Errorf("cache is not empty after ClearPeerKeys: got %d keys", cacheLen)
	}
}
