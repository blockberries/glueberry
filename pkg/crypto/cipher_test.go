package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func generateRandomKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate random key: %v", err)
	}
	return key
}

func TestNewCipher(t *testing.T) {
	key := generateRandomKey(t)

	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}
	if cipher == nil {
		t.Error("NewCipher returned nil")
	}
}

func TestNewCipher_InvalidKeySize(t *testing.T) {
	tests := []struct {
		name    string
		keySize int
	}{
		{"nil key", 0},
		{"empty key", 0},
		{"16 bytes", 16},
		{"24 bytes", 24},
		{"31 bytes", 31},
		{"33 bytes", 33},
		{"64 bytes", 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var key []byte
			if tt.keySize > 0 {
				key = make([]byte, tt.keySize)
			}
			_, err := NewCipher(key)
			if err == nil {
				t.Error("expected error for invalid key size")
			}
		})
	}
}

func TestCipher_EncryptDecrypt_RoundTrip(t *testing.T) {
	key := generateRandomKey(t)
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}

	testCases := []struct {
		name      string
		plaintext []byte
	}{
		{"empty", []byte{}},
		{"single byte", []byte{0x42}},
		{"short message", []byte("hello")},
		{"medium message", bytes.Repeat([]byte("test"), 100)},
		{"large message", bytes.Repeat([]byte("x"), 1024*1024)}, // 1MB
		{"binary data", []byte{0x00, 0xFF, 0x01, 0xFE, 0x02, 0xFD}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ciphertext, err := cipher.Encrypt(tc.plaintext, nil)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			// Ciphertext should be larger than plaintext by nonce + tag size
			expectedLen := len(tc.plaintext) + NonceSize + TagSize
			if len(ciphertext) != expectedLen {
				t.Errorf("ciphertext length = %d, want %d", len(ciphertext), expectedLen)
			}

			decrypted, err := cipher.Decrypt(ciphertext, nil)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			if !bytes.Equal(decrypted, tc.plaintext) {
				t.Error("decrypted plaintext doesn't match original")
			}
		})
	}
}

func TestCipher_EncryptDecrypt_WithAdditionalData(t *testing.T) {
	key := generateRandomKey(t)
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}

	plaintext := []byte("secret message")
	additionalData := []byte("associated data that is authenticated but not encrypted")

	ciphertext, err := cipher.Encrypt(plaintext, additionalData)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Decrypt with correct additional data
	decrypted, err := cipher.Decrypt(ciphertext, additionalData)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Error("decrypted plaintext doesn't match original")
	}
}

func TestCipher_Decrypt_WrongAdditionalData(t *testing.T) {
	key := generateRandomKey(t)
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}

	plaintext := []byte("secret message")
	additionalData := []byte("correct additional data")

	ciphertext, err := cipher.Encrypt(plaintext, additionalData)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Try to decrypt with wrong additional data
	wrongAD := []byte("wrong additional data")
	_, err = cipher.Decrypt(ciphertext, wrongAD)
	if err == nil {
		t.Error("expected error when decrypting with wrong additional data")
	}

	// Try to decrypt with no additional data
	_, err = cipher.Decrypt(ciphertext, nil)
	if err == nil {
		t.Error("expected error when decrypting with missing additional data")
	}
}

func TestCipher_Decrypt_TamperedCiphertext(t *testing.T) {
	key := generateRandomKey(t)
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}

	plaintext := []byte("secret message")
	ciphertext, err := cipher.Encrypt(plaintext, nil)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Tamper with various parts of the ciphertext
	testCases := []struct {
		name   string
		modify func([]byte)
	}{
		{
			name: "flip bit in nonce",
			modify: func(ct []byte) {
				ct[0] ^= 0x01
			},
		},
		{
			name: "flip bit in ciphertext",
			modify: func(ct []byte) {
				ct[NonceSize+1] ^= 0x01
			},
		},
		{
			name: "flip bit in tag",
			modify: func(ct []byte) {
				ct[len(ct)-1] ^= 0x01
			},
		},
		{
			name: "truncate",
			modify: func(ct []byte) {
				// This will be handled by the length check
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tampered := make([]byte, len(ciphertext))
			copy(tampered, ciphertext)
			tc.modify(tampered)

			if tc.name == "truncate" {
				tampered = tampered[:len(tampered)-5]
			}

			_, err := cipher.Decrypt(tampered, nil)
			if err == nil {
				t.Error("expected error when decrypting tampered ciphertext")
			}
		})
	}
}

func TestCipher_Decrypt_TooShort(t *testing.T) {
	key := generateRandomKey(t)
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}

	testCases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"just nonce", make([]byte, NonceSize)},
		{"nonce + partial tag", make([]byte, NonceSize+TagSize-1)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cipher.Decrypt(tc.data, nil)
			if err == nil {
				t.Error("expected error for short ciphertext")
			}
		})
	}
}

func TestCipher_encryptWithNonce(t *testing.T) {
	key := generateRandomKey(t)
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}

	nonce := make([]byte, NonceSize)
	for i := range nonce {
		nonce[i] = byte(i)
	}
	plaintext := []byte("test message")

	// Encrypt twice with same nonce should produce same ciphertext
	ct1, err := cipher.encryptWithNonce(nonce, plaintext, nil)
	if err != nil {
		t.Fatalf("encryptWithNonce failed: %v", err)
	}

	ct2, err := cipher.encryptWithNonce(nonce, plaintext, nil)
	if err != nil {
		t.Fatalf("encryptWithNonce failed: %v", err)
	}

	if !bytes.Equal(ct1, ct2) {
		t.Error("same nonce should produce same ciphertext")
	}

	// Verify the nonce is in the output
	if !bytes.Equal(ct1[:NonceSize], nonce) {
		t.Error("nonce should be prepended to ciphertext")
	}
}

func TestCipher_encryptWithNonce_InvalidNonceSize(t *testing.T) {
	key := generateRandomKey(t)
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}

	plaintext := []byte("test")

	testCases := []struct {
		name      string
		nonceSize int
	}{
		{"empty", 0},
		{"too short", NonceSize - 1},
		{"too long", NonceSize + 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nonce := make([]byte, tc.nonceSize)
			_, err := cipher.encryptWithNonce(nonce, plaintext, nil)
			if err == nil {
				t.Error("expected error for invalid nonce size")
			}
		})
	}
}

func TestCipher_Encrypt_UniqueNonces(t *testing.T) {
	key := generateRandomKey(t)
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}

	plaintext := []byte("same message")
	nonces := make(map[string]bool)

	// Encrypt the same message multiple times and verify nonces are unique
	for i := 0; i < 1000; i++ {
		ciphertext, err := cipher.Encrypt(plaintext, nil)
		if err != nil {
			t.Fatalf("Encrypt failed on iteration %d: %v", i, err)
		}

		nonceStr := string(ciphertext[:NonceSize])
		if nonces[nonceStr] {
			t.Fatalf("duplicate nonce detected on iteration %d", i)
		}
		nonces[nonceStr] = true
	}
}

func TestCipher_Decrypt_WrongKey(t *testing.T) {
	key1 := generateRandomKey(t)
	key2 := generateRandomKey(t)

	cipher1, _ := NewCipher(key1)
	cipher2, _ := NewCipher(key2)

	plaintext := []byte("secret message")
	ciphertext, err := cipher1.Encrypt(plaintext, nil)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Try to decrypt with wrong key
	_, err = cipher2.Decrypt(ciphertext, nil)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestEncryptDecrypt_ConvenienceFunctions(t *testing.T) {
	key := generateRandomKey(t)
	plaintext := []byte("test message")

	ciphertext, err := Encrypt(key, plaintext, nil)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := Decrypt(key, ciphertext, nil)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Error("decrypted plaintext doesn't match original")
	}
}

func TestEncryptDecrypt_ConvenienceFunctions_InvalidKey(t *testing.T) {
	invalidKey := make([]byte, 16) // Wrong size
	plaintext := []byte("test")

	_, err := Encrypt(invalidKey, plaintext, nil)
	if err == nil {
		t.Error("expected error for invalid key")
	}

	validKey := generateRandomKey(t)
	ciphertext, _ := Encrypt(validKey, plaintext, nil)

	_, err = Decrypt(invalidKey, ciphertext, nil)
	if err == nil {
		t.Error("expected error for invalid key")
	}
}

func TestCipher_Close(t *testing.T) {
	key := generateRandomKey(t)
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}

	// Verify cipher works before close
	plaintext := []byte("test message")
	_, err = cipher.Encrypt(plaintext, nil)
	if err != nil {
		t.Fatalf("Encrypt before Close failed: %v", err)
	}

	// Check IsClosed before close
	if cipher.IsClosed() {
		t.Error("cipher should not be closed yet")
	}

	// Close should zero the key
	cipher.Close()

	// Verify IsClosed
	if !cipher.IsClosed() {
		t.Error("cipher should be closed")
	}

	// Verify key is zeroed
	if cipher.key != nil {
		t.Error("key should be nil after Close")
	}

	// Close should be idempotent
	cipher.Close() // Should not panic
}

func TestCipher_Close_ZerosKey(t *testing.T) {
	key := generateRandomKey(t)
	cipher, err := NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher failed: %v", err)
	}

	// Get a reference to the internal key before close
	keyRef := cipher.key

	// Verify key is non-zero
	allZero := true
	for _, b := range keyRef {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("key should not be all zeros before Close")
	}

	// Close the cipher
	cipher.Close()

	// Verify the original slice is zeroed (defense in depth)
	for i, b := range keyRef {
		if b != 0 {
			t.Errorf("key byte %d is not zero after Close: got %d", i, b)
		}
	}
}

func BenchmarkCipher_Encrypt(b *testing.B) {
	key := make([]byte, KeySize)
	rand.Read(key)
	cipher, _ := NewCipher(key)
	plaintext := bytes.Repeat([]byte("x"), 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cipher.Encrypt(plaintext, nil)
	}
}

func BenchmarkCipher_Decrypt(b *testing.B) {
	key := make([]byte, KeySize)
	rand.Read(key)
	cipher, _ := NewCipher(key)
	plaintext := bytes.Repeat([]byte("x"), 1024)
	ciphertext, _ := cipher.Encrypt(plaintext, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cipher.Decrypt(ciphertext, nil)
	}
}
