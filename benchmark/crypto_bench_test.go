package benchmark

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/blockberries/glueberry/pkg/crypto"
)

// Benchmark Ed25519 to X25519 key conversion
func BenchmarkEd25519PrivateToX25519(b *testing.B) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = crypto.Ed25519PrivateToX25519(priv)
	}
}

func BenchmarkEd25519PublicToX25519(b *testing.B) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = crypto.Ed25519PublicToX25519(pub)
	}
}

// Benchmark X25519 shared secret computation
func BenchmarkComputeX25519SharedSecret(b *testing.B) {
	_, priv1, _ := ed25519.GenerateKey(rand.Reader)
	pub2, _, _ := ed25519.GenerateKey(rand.Reader)

	x25519Priv, _ := crypto.Ed25519PrivateToX25519(priv1)
	x25519Pub, _ := crypto.Ed25519PublicToX25519(pub2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = crypto.ComputeX25519SharedSecret(x25519Priv, x25519Pub)
	}
}

// Benchmark full key derivation (ECDH + HKDF)
func BenchmarkDeriveSharedKey(b *testing.B) {
	_, priv1, _ := ed25519.GenerateKey(rand.Reader)
	pub2, _, _ := ed25519.GenerateKey(rand.Reader)

	x25519Priv, _ := crypto.Ed25519PrivateToX25519(priv1)
	x25519Pub, _ := crypto.Ed25519PublicToX25519(pub2)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = crypto.DeriveSharedKey(x25519Priv, x25519Pub, nil)
	}
}

// Benchmark encryption at various message sizes
func BenchmarkEncrypt_64B(b *testing.B)  { benchmarkEncrypt(b, 64) }
func BenchmarkEncrypt_256B(b *testing.B) { benchmarkEncrypt(b, 256) }
func BenchmarkEncrypt_1KB(b *testing.B)  { benchmarkEncrypt(b, 1024) }
func BenchmarkEncrypt_4KB(b *testing.B)  { benchmarkEncrypt(b, 4096) }
func BenchmarkEncrypt_16KB(b *testing.B) { benchmarkEncrypt(b, 16384) }
func BenchmarkEncrypt_64KB(b *testing.B) { benchmarkEncrypt(b, 65536) }
func BenchmarkEncrypt_1MB(b *testing.B)  { benchmarkEncrypt(b, 1048576) }

func benchmarkEncrypt(b *testing.B, size int) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		b.Fatal(err)
	}

	cipher, err := crypto.NewCipher(key)
	if err != nil {
		b.Fatal(err)
	}

	plaintext := make([]byte, size)
	if _, err := rand.Read(plaintext); err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(size))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cipher.Encrypt(plaintext, nil)
	}
}

// Benchmark decryption at various message sizes
func BenchmarkDecrypt_64B(b *testing.B)  { benchmarkDecrypt(b, 64) }
func BenchmarkDecrypt_256B(b *testing.B) { benchmarkDecrypt(b, 256) }
func BenchmarkDecrypt_1KB(b *testing.B)  { benchmarkDecrypt(b, 1024) }
func BenchmarkDecrypt_4KB(b *testing.B)  { benchmarkDecrypt(b, 4096) }
func BenchmarkDecrypt_16KB(b *testing.B) { benchmarkDecrypt(b, 16384) }
func BenchmarkDecrypt_64KB(b *testing.B) { benchmarkDecrypt(b, 65536) }
func BenchmarkDecrypt_1MB(b *testing.B)  { benchmarkDecrypt(b, 1048576) }

func benchmarkDecrypt(b *testing.B, size int) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		b.Fatal(err)
	}

	cipher, err := crypto.NewCipher(key)
	if err != nil {
		b.Fatal(err)
	}

	plaintext := make([]byte, size)
	if _, err := rand.Read(plaintext); err != nil {
		b.Fatal(err)
	}

	ciphertext, err := cipher.Encrypt(plaintext, nil)
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(size))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = cipher.Decrypt(ciphertext, nil)
	}
}

// Benchmark Module operations (full crypto stack)
func BenchmarkModule_DeriveSharedKey(b *testing.B) {
	_, priv1, _ := ed25519.GenerateKey(rand.Reader)
	pub2, _, _ := ed25519.GenerateKey(rand.Reader)

	mod, err := crypto.NewModule(priv1)
	if err != nil {
		b.Fatal(err)
	}
	defer mod.Close()

	// First call to derive (cold cache)
	_, err = mod.DeriveSharedKey(pub2)
	if err != nil {
		b.Fatal(err)
	}

	// Benchmark cached derivation
	b.Run("cached", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = mod.DeriveSharedKey(pub2)
		}
	})

	// Benchmark uncached derivation
	b.Run("uncached", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, newPriv, _ := ed25519.GenerateKey(rand.Reader)
			newMod, _ := crypto.NewModule(newPriv)
			_, _ = newMod.DeriveSharedKey(pub2)
			newMod.Close()
		}
	})
}

// Benchmark encrypt/decrypt with Module
func BenchmarkModule_EncryptDecrypt(b *testing.B) {
	_, priv1, _ := ed25519.GenerateKey(rand.Reader)
	pub2, priv2, _ := ed25519.GenerateKey(rand.Reader)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)
	defer mod1.Close()
	defer mod2.Close()

	x25519Pub2 := mod2.X25519PublicKey()
	x25519Pub1 := mod1.X25519PublicKey()

	// Derive keys
	_, _ = mod1.DeriveSharedKey(pub2)
	_, _ = mod2.DeriveSharedKey(mod1.Ed25519PublicKey())

	plaintext := make([]byte, 1024)
	rand.Read(plaintext)

	b.Run("encrypt", func(b *testing.B) {
		b.SetBytes(1024)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = mod1.Encrypt(x25519Pub2, plaintext)
		}
	})

	ciphertext, _ := mod1.Encrypt(x25519Pub2, plaintext)

	b.Run("decrypt", func(b *testing.B) {
		b.SetBytes(1024)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = mod2.Decrypt(x25519Pub1, ciphertext)
		}
	})
}
