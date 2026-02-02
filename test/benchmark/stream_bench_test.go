package benchmark

import (
	"crypto/rand"
	"testing"

	"github.com/blockberries/cramberry/pkg/cramberry"
	"github.com/blockberries/glueberry/pkg/crypto"
)

// Benchmark message serialization + encryption (the hot path for sending)
func BenchmarkMessagePath_64B(b *testing.B)  { benchmarkMessagePath(b, 64) }
func BenchmarkMessagePath_256B(b *testing.B) { benchmarkMessagePath(b, 256) }
func BenchmarkMessagePath_1KB(b *testing.B)  { benchmarkMessagePath(b, 1024) }
func BenchmarkMessagePath_4KB(b *testing.B)  { benchmarkMessagePath(b, 4096) }
func BenchmarkMessagePath_16KB(b *testing.B) { benchmarkMessagePath(b, 16384) }
func BenchmarkMessagePath_64KB(b *testing.B) { benchmarkMessagePath(b, 65536) }

func benchmarkMessagePath(b *testing.B, size int) {
	// Setup encryption
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		b.Fatal(err)
	}

	cipher, err := crypto.NewCipher(key)
	if err != nil {
		b.Fatal(err)
	}

	// Generate test data
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		b.Fatal(err)
	}

	// Simulate message path: serialize with cramberry, then encrypt
	b.SetBytes(int64(size))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Serialize with cramberry
		serialized, err := cramberry.Marshal(data)
		if err != nil {
			b.Fatal(err)
		}

		// Encrypt
		_, err = cipher.Encrypt(serialized, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark message deserialization + decryption (the hot path for receiving)
func BenchmarkReceivePath_64B(b *testing.B)  { benchmarkReceivePath(b, 64) }
func BenchmarkReceivePath_256B(b *testing.B) { benchmarkReceivePath(b, 256) }
func BenchmarkReceivePath_1KB(b *testing.B)  { benchmarkReceivePath(b, 1024) }
func BenchmarkReceivePath_4KB(b *testing.B)  { benchmarkReceivePath(b, 4096) }
func BenchmarkReceivePath_16KB(b *testing.B) { benchmarkReceivePath(b, 16384) }
func BenchmarkReceivePath_64KB(b *testing.B) { benchmarkReceivePath(b, 65536) }

func benchmarkReceivePath(b *testing.B, size int) {
	// Setup encryption
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		b.Fatal(err)
	}

	cipher, err := crypto.NewCipher(key)
	if err != nil {
		b.Fatal(err)
	}

	// Generate test data
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		b.Fatal(err)
	}

	// Pre-create encrypted message
	serialized, _ := cramberry.Marshal(data)
	encrypted, _ := cipher.Encrypt(serialized, nil)

	b.SetBytes(int64(size))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Decrypt
		decrypted, err := cipher.Decrypt(encrypted, nil)
		if err != nil {
			b.Fatal(err)
		}

		// Deserialize
		var result []byte
		if err := cramberry.Unmarshal(decrypted, &result); err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark cramberry serialization alone
func BenchmarkCramberry_Marshal_64B(b *testing.B)  { benchmarkCramberryMarshal(b, 64) }
func BenchmarkCramberry_Marshal_1KB(b *testing.B)  { benchmarkCramberryMarshal(b, 1024) }
func BenchmarkCramberry_Marshal_64KB(b *testing.B) { benchmarkCramberryMarshal(b, 65536) }

func benchmarkCramberryMarshal(b *testing.B, size int) {
	data := make([]byte, size)
	rand.Read(data)

	b.SetBytes(int64(size))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = cramberry.Marshal(data)
	}
}

func BenchmarkCramberry_Unmarshal_64B(b *testing.B)  { benchmarkCramberryUnmarshal(b, 64) }
func BenchmarkCramberry_Unmarshal_1KB(b *testing.B)  { benchmarkCramberryUnmarshal(b, 1024) }
func BenchmarkCramberry_Unmarshal_64KB(b *testing.B) { benchmarkCramberryUnmarshal(b, 65536) }

func benchmarkCramberryUnmarshal(b *testing.B, size int) {
	data := make([]byte, size)
	rand.Read(data)
	serialized, _ := cramberry.Marshal(data)

	b.SetBytes(int64(size))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var result []byte
		_ = cramberry.Unmarshal(serialized, &result)
	}
}
