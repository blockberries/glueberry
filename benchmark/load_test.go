package benchmark

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/blockberries/glueberry"
	"github.com/blockberries/glueberry/pkg/addressbook"
	"github.com/blockberries/glueberry/pkg/crypto"
)

// Load Testing
// These tests measure performance under load and help identify bottlenecks.

// generateLoadTestKey generates an ed25519 private key for load testing.
func generateLoadTestKey(b *testing.B) ed25519.PrivateKey {
	b.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		b.Fatalf("failed to generate key: %v", err)
	}
	return priv
}

// loadToCryptoKey converts ed25519 public key to libp2p crypto key.
func loadToCryptoKey(pub ed25519.PublicKey) libp2pcrypto.PubKey {
	key, _ := libp2pcrypto.UnmarshalEd25519PublicKey(pub)
	return key
}

// generateLoadPeerID creates a peer ID from an ed25519 private key.
func generateLoadPeerID(b *testing.B) peer.ID {
	b.Helper()
	priv := generateLoadTestKey(b)
	pubKey := priv.Public().(ed25519.PublicKey)
	peerID, err := peer.IDFromPublicKey(loadToCryptoKey(pubKey))
	if err != nil {
		b.Fatalf("failed to create peer ID: %v", err)
	}
	return peerID
}

// BenchmarkNodeCreation measures node creation performance.
func BenchmarkNodeCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		priv := generateLoadTestKey(b)
		listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
		cfg := glueberry.NewConfig(priv, b.TempDir()+"/addresses.json", []multiaddr.Multiaddr{listenAddr})

		node, err := glueberry.New(cfg)
		if err != nil {
			b.Fatalf("New failed: %v", err)
		}
		_ = node.Stop()
	}
}

// BenchmarkNodeStartStop measures node start/stop cycle performance.
func BenchmarkNodeStartStop(b *testing.B) {
	priv := generateLoadTestKey(b)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	cfg := glueberry.NewConfig(priv, b.TempDir()+"/addresses.json", []multiaddr.Multiaddr{listenAddr})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		node, err := glueberry.New(cfg)
		if err != nil {
			b.Fatalf("New failed: %v", err)
		}
		if err := node.Start(); err != nil {
			b.Fatalf("Start failed: %v", err)
		}
		_ = node.Stop()
	}
}

// BenchmarkAddPeer measures peer addition performance.
func BenchmarkAddPeer(b *testing.B) {
	priv := generateLoadTestKey(b)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	cfg := glueberry.NewConfig(priv, b.TempDir()+"/addresses.json", []multiaddr.Multiaddr{listenAddr})

	node, err := glueberry.New(cfg)
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		b.Fatalf("Start failed: %v", err)
	}

	testAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/12345")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		peerID := peer.ID(fmt.Sprintf("bench-peer-%d", i))
		_ = node.AddPeer(peerID, []multiaddr.Multiaddr{testAddr}, nil)
	}
}

// BenchmarkAddRemovePeer measures peer add/remove cycle performance.
func BenchmarkAddRemovePeer(b *testing.B) {
	priv := generateLoadTestKey(b)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	cfg := glueberry.NewConfig(priv, b.TempDir()+"/addresses.json", []multiaddr.Multiaddr{listenAddr})

	node, err := glueberry.New(cfg)
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		b.Fatalf("Start failed: %v", err)
	}

	testAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/12345")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		peerID := peer.ID(fmt.Sprintf("bench-peer-%d", i))
		_ = node.AddPeer(peerID, []multiaddr.Multiaddr{testAddr}, nil)
		_ = node.RemovePeer(peerID)
	}
}

// BenchmarkListPeers measures peer listing performance with various peer counts.
func BenchmarkListPeers_10(b *testing.B)   { benchmarkListPeers(b, 10) }
func BenchmarkListPeers_100(b *testing.B)  { benchmarkListPeers(b, 100) }
func BenchmarkListPeers_500(b *testing.B)  { benchmarkListPeers(b, 500) }
func BenchmarkListPeers_1000(b *testing.B) { benchmarkListPeers(b, 1000) }

func benchmarkListPeers(b *testing.B, numPeers int) {
	priv := generateLoadTestKey(b)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	cfg := glueberry.NewConfig(priv, b.TempDir()+"/addresses.json", []multiaddr.Multiaddr{listenAddr})

	node, err := glueberry.New(cfg)
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		b.Fatalf("Start failed: %v", err)
	}

	testAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/12345")

	// Add peers
	for i := 0; i < numPeers; i++ {
		peerID := peer.ID(fmt.Sprintf("bench-peer-%d", i))
		_ = node.AddPeer(peerID, []multiaddr.Multiaddr{testAddr}, nil)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		peers := node.ListPeers()
		if len(peers) != numPeers {
			b.Fatalf("expected %d peers, got %d", numPeers, len(peers))
		}
	}
}

// BenchmarkConcurrentAddPeer measures concurrent peer addition.
func BenchmarkConcurrentAddPeer(b *testing.B) {
	priv := generateLoadTestKey(b)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	cfg := glueberry.NewConfig(priv, b.TempDir()+"/addresses.json", []multiaddr.Multiaddr{listenAddr})

	node, err := glueberry.New(cfg)
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		b.Fatalf("Start failed: %v", err)
	}

	testAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/12345")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			peerID := peer.ID(fmt.Sprintf("concurrent-peer-%d-%d", time.Now().UnixNano(), i))
			_ = node.AddPeer(peerID, []multiaddr.Multiaddr{testAddr}, nil)
			i++
		}
	})
}

// BenchmarkHealthCheck measures health check performance.
func BenchmarkHealthCheck(b *testing.B) {
	priv := generateLoadTestKey(b)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	cfg := glueberry.NewConfig(priv, b.TempDir()+"/addresses.json", []multiaddr.Multiaddr{listenAddr})

	node, err := glueberry.New(cfg)
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		b.Fatalf("Start failed: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = node.IsHealthy()
	}
}

// BenchmarkReadinessChecks measures detailed readiness check performance.
func BenchmarkReadinessChecks(b *testing.B) {
	priv := generateLoadTestKey(b)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	cfg := glueberry.NewConfig(priv, b.TempDir()+"/addresses.json", []multiaddr.Multiaddr{listenAddr})

	node, err := glueberry.New(cfg)
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		b.Fatalf("Start failed: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = node.ReadinessChecks()
	}
}

// BenchmarkKeyDerivation measures shared key derivation performance.
func BenchmarkKeyDerivation(b *testing.B) {
	priv1 := generateLoadTestKey(b)
	priv2 := generateLoadTestKey(b)

	mod1, err := crypto.NewModule(priv1)
	if err != nil {
		b.Fatalf("NewModule failed: %v", err)
	}
	defer mod1.Close()

	pub2 := priv2.Public().(ed25519.PublicKey)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := mod1.DeriveSharedKey(pub2)
		if err != nil {
			b.Fatalf("DeriveSharedKey failed: %v", err)
		}
	}
}

// BenchmarkAddressBook measures address book operations.
func BenchmarkAddressBook_Add(b *testing.B) {
	book, err := addressbook.New(b.TempDir() + "/addresses.json")
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}
	defer book.Close()

	testAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/12345")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		peerID := peer.ID(fmt.Sprintf("addr-book-peer-%d", i))
		_ = book.AddPeer(peerID, []multiaddr.Multiaddr{testAddr}, nil)
	}
}

func BenchmarkAddressBook_List(b *testing.B) {
	book, err := addressbook.New(b.TempDir() + "/addresses.json")
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}
	defer book.Close()

	testAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/12345")

	// Pre-populate
	for i := 0; i < 100; i++ {
		peerID := peer.ID(fmt.Sprintf("addr-book-peer-%d", i))
		_ = book.AddPeer(peerID, []multiaddr.Multiaddr{testAddr}, nil)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = book.ListPeers()
	}
}

// TestLoadScaling tests how performance scales with peer count.
// This is a test (not a benchmark) that produces a performance report.
func TestLoadScaling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping load scaling test in short mode")
	}

	priv := generateLoadTestKeyT(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	cfg := glueberry.NewConfig(priv, t.TempDir()+"/addresses.json", []multiaddr.Multiaddr{listenAddr})

	node, err := glueberry.New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	testAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/12345")

	peerCounts := []int{10, 50, 100, 250, 500, 1000}

	t.Log("=== Load Scaling Report ===")
	t.Log("")

	for _, numPeers := range peerCounts {
		// Remove all existing peers first
		for _, p := range node.ListPeers() {
			_ = node.RemovePeer(p.PeerID)
		}

		// Time adding N peers
		start := time.Now()
		for i := 0; i < numPeers; i++ {
			peerID := peer.ID(fmt.Sprintf("scale-peer-%d-%d", numPeers, i))
			_ = node.AddPeer(peerID, []multiaddr.Multiaddr{testAddr}, nil)
		}
		addDuration := time.Since(start)

		// Time listing all peers
		start = time.Now()
		const listIterations = 100
		for i := 0; i < listIterations; i++ {
			_ = node.ListPeers()
		}
		listDuration := time.Since(start) / listIterations

		// Time health checks
		start = time.Now()
		const healthIterations = 100
		for i := 0; i < healthIterations; i++ {
			_ = node.IsHealthy()
		}
		healthDuration := time.Since(start) / healthIterations

		t.Logf("Peers: %4d | Add all: %v | List: %v | Health: %v",
			numPeers, addDuration, listDuration, healthDuration)
	}

	t.Log("")
	t.Log("=== Memory Profile ===")

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	t.Logf("Alloc: %d MB | TotalAlloc: %d MB | Sys: %d MB | NumGC: %d",
		m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024, m.NumGC)
}

// TestConcurrentLoadThroughput measures throughput under concurrent load.
// This test measures address book operations without triggering actual connections.
func TestConcurrentLoadThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping throughput test in short mode")
	}

	// Test address book throughput directly (faster, no connection attempts)
	book, err := addressbook.New(t.TempDir() + "/addresses.json")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer book.Close()

	testAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/12345")

	// Test with different concurrency levels
	// Note: ops are kept low due to file-based persistence overhead
	concurrencyLevels := []int{1, 2, 4, 8}
	const opsPerWorker = 50

	t.Log("=== Concurrent Throughput Report (Address Book) ===")
	t.Log("")

	for _, numWorkers := range concurrencyLevels {
		// Clear existing entries
		_ = book.Clear()

		var ops atomic.Int64
		var wg sync.WaitGroup
		wg.Add(numWorkers)

		start := time.Now()

		for w := 0; w < numWorkers; w++ {
			go func(workerID int) {
				defer wg.Done()
				for i := 0; i < opsPerWorker; i++ {
					peerID := peer.ID(fmt.Sprintf("throughput-%d-%d", workerID, i))
					_ = book.AddPeer(peerID, []multiaddr.Multiaddr{testAddr}, nil)
					ops.Add(1)
					_ = book.RemovePeer(peerID)
					ops.Add(1)
				}
			}(w)
		}

		wg.Wait()
		duration := time.Since(start)

		opsPerSec := float64(ops.Load()) / duration.Seconds()
		t.Logf("Workers: %2d | Total ops: %6d | Duration: %v | Throughput: %.0f ops/sec",
			numWorkers, ops.Load(), duration, opsPerSec)
	}
}

// generateLoadTestKeyT generates a key for tests (not benchmarks).
func generateLoadTestKeyT(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return priv
}
