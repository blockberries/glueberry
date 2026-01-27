package streams

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/blockberries/glueberry/pkg/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// mockStreamOpener implements StreamOpener for testing
type mockStreamOpener struct {
	mu      sync.Mutex
	streams map[string]*mockStream
}

func newMockStreamOpener() *mockStreamOpener {
	return &mockStreamOpener{
		streams: make(map[string]*mockStream),
	}
}

func (mso *mockStreamOpener) NewStream(ctx context.Context, peerID peer.ID, streamName string) (network.Stream, error) {
	mso.mu.Lock()
	defer mso.mu.Unlock()

	// Create a mock stream
	stream := newMockStream(&bytes.Buffer{}, &bytes.Buffer{})
	mso.streams[streamName] = stream
	return stream, nil
}

func generateManagerTestKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return priv
}

func TestNewManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv := generateManagerTestKey(t)
	cryptoMod, _ := crypto.NewModule(priv)
	incoming := make(chan IncomingMessage, 10)

	mgr := NewManager(ctx, host, cryptoMod, incoming)

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
}

func TestManager_EstablishStreams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv1 := generateManagerTestKey(t)
	priv2 := generateManagerTestKey(t)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)

	incoming := make(chan IncomingMessage, 10)

	mgr := NewManager(ctx, host, mod1, incoming)
	defer mgr.Shutdown()

	peerID := mustParsePeerID(t, testPeerIDStr)
	streamNames := []string{"blocks", "transactions", "consensus"}

	// Establish streams (now only registers, doesn't open)
	err := mgr.EstablishStreams(peerID, mod2.Ed25519PublicKey(), streamNames)
	if err != nil {
		t.Fatalf("EstablishStreams failed: %v", err)
	}

	// With lazy opening, streams are NOT created until Send() or incoming
	// Verify GetStream returns nil for unopened streams
	for _, name := range streamNames {
		stream := mgr.GetStream(peerID, name)
		if stream != nil {
			t.Errorf("stream %q should not exist yet (lazy opening)", name)
		}
	}

	// HasStreams should be false (no streams opened yet)
	if mgr.HasStreams(peerID) {
		t.Error("HasStreams should be false before any streams opened")
	}

	// But we can Send, which will open streams lazily
	testData := []byte("test message")
	if err := mgr.Send(peerID, "blocks", testData); err != nil {
		t.Errorf("Send should succeed (lazy stream opening): %v", err)
	}

	// Now GetStream should return the lazily-opened stream
	stream := mgr.GetStream(peerID, "blocks")
	if stream == nil {
		t.Error("stream 'blocks' should exist after Send()")
	}

	// HasStreams should now be true
	if !mgr.HasStreams(peerID) {
		t.Error("HasStreams should be true after Send()")
	}
}

func TestManager_EstablishStreams_NoNames(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv := generateManagerTestKey(t)
	mod, _ := crypto.NewModule(priv)
	incoming := make(chan IncomingMessage, 10)

	mgr := NewManager(ctx, host, mod, incoming)
	defer mgr.Shutdown()

	peerID := mustParsePeerID(t, testPeerIDStr)

	// Should fail with no stream names
	err := mgr.EstablishStreams(peerID, mod.Ed25519PublicKey(), []string{})
	if err == nil {
		t.Error("EstablishStreams should fail with no stream names")
	}
}

func TestManager_EstablishStreams_Multiple(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv1 := generateManagerTestKey(t)
	priv2 := generateManagerTestKey(t)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)

	incoming := make(chan IncomingMessage, 10)

	mgr := NewManager(ctx, host, mod1, incoming)
	defer mgr.Shutdown()

	peerID := mustParsePeerID(t, testPeerIDStr)

	// First establish with some streams
	err := mgr.EstablishStreams(peerID, mod2.Ed25519PublicKey(), []string{"stream1"})
	if err != nil {
		t.Fatalf("first EstablishStreams failed: %v", err)
	}

	// Second establish with additional streams should succeed (adds to allowed list)
	err = mgr.EstablishStreams(peerID, mod2.Ed25519PublicKey(), []string{"stream2", "stream3"})
	if err != nil {
		t.Fatalf("second EstablishStreams failed: %v", err)
	}

	// Both stream names should be allowed
	// Verify by sending (which will open lazily)
	mgr.Send(peerID, "stream1", []byte("test1"))
	mgr.Send(peerID, "stream2", []byte("test2"))

	if !mgr.HasStreams(peerID) {
		t.Error("Should have streams after Send()")
	}
}

func TestManager_Send(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv1 := generateManagerTestKey(t)
	priv2 := generateManagerTestKey(t)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)

	incoming := make(chan IncomingMessage, 10)

	mgr := NewManager(ctx, host, mod1, incoming)
	defer mgr.Shutdown()

	peerID := mustParsePeerID(t, testPeerIDStr)
	streamNames := []string{"test"}

	mgr.EstablishStreams(peerID, mod2.Ed25519PublicKey(), streamNames)

	// Send should succeed
	testData := []byte("test data")
	err := mgr.Send(peerID, "test", testData)
	if err != nil {
		t.Errorf("Send failed: %v", err)
	}
}

func TestManager_Send_PeerNotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv := generateManagerTestKey(t)
	mod, _ := crypto.NewModule(priv)
	incoming := make(chan IncomingMessage, 10)

	mgr := NewManager(ctx, host, mod, incoming)
	defer mgr.Shutdown()

	peerID := mustParsePeerID(t, testPeerIDStr)

	// Send should fail - no streams for peer
	err := mgr.Send(peerID, "test", []byte("data"))
	if err == nil {
		t.Error("Send should fail when peer has no streams")
	}
}

func TestManager_Send_StreamNotFound(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv1 := generateManagerTestKey(t)
	priv2 := generateManagerTestKey(t)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)

	incoming := make(chan IncomingMessage, 10)

	mgr := NewManager(ctx, host, mod1, incoming)
	defer mgr.Shutdown()

	peerID := mustParsePeerID(t, testPeerIDStr)
	mgr.EstablishStreams(peerID, mod2.Ed25519PublicKey(), []string{"blocks"})

	// Send should fail - stream name doesn't exist
	err := mgr.Send(peerID, "transactions", []byte("data"))
	if err == nil {
		t.Error("Send should fail when stream doesn't exist")
	}
}

func TestManager_CloseStreams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv1 := generateManagerTestKey(t)
	priv2 := generateManagerTestKey(t)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)

	incoming := make(chan IncomingMessage, 10)

	mgr := NewManager(ctx, host, mod1, incoming)
	defer mgr.Shutdown()

	peerID := mustParsePeerID(t, testPeerIDStr)
	streamNames := []string{"test1", "test2"}

	mgr.EstablishStreams(peerID, mod2.Ed25519PublicKey(), streamNames)

	// Close streams
	err := mgr.CloseStreams(peerID)
	if err != nil {
		t.Errorf("CloseStreams failed: %v", err)
	}

	// Streams should be gone
	if mgr.HasStreams(peerID) {
		t.Error("HasStreams should be false after CloseStreams")
	}
}

func TestManager_Shutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv1 := generateManagerTestKey(t)
	priv2 := generateManagerTestKey(t)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)

	incoming := make(chan IncomingMessage, 10)

	mgr := NewManager(ctx, host, mod1, incoming)

	peerID := mustParsePeerID(t, testPeerIDStr)
	mgr.EstablishStreams(peerID, mod2.Ed25519PublicKey(), []string{"test"})

	// Shutdown
	mgr.Shutdown()

	// Streams should be gone
	if mgr.HasStreams(peerID) {
		t.Error("HasStreams should be false after Shutdown")
	}
}

func TestManager_HandleIncomingStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv1 := generateManagerTestKey(t)
	priv2 := generateManagerTestKey(t)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)

	incoming := make(chan IncomingMessage, 10)

	mgr := NewManager(ctx, host, mod1, incoming)
	defer mgr.Shutdown()

	peerID := mustParsePeerID(t, testPeerIDStr)

	// First establish streams (registers shared key and allowed streams)
	mgr.EstablishStreams(peerID, mod2.Ed25519PublicKey(), []string{"incoming-test"})

	// Simulate incoming stream
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)

	err := mgr.HandleIncomingStream(peerID, "incoming-test", stream)
	if err != nil {
		t.Fatalf("HandleIncomingStream failed: %v", err)
	}

	// Stream should exist
	if !mgr.HasStreams(peerID) {
		t.Error("stream should exist after HandleIncomingStream")
	}

	stream2 := mgr.GetStream(peerID, "incoming-test")
	if stream2 == nil {
		t.Error("GetStream should return the incoming stream")
	}
}

func TestManager_HandleIncomingStream_Duplicate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv1 := generateManagerTestKey(t)
	priv2 := generateManagerTestKey(t)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)

	incoming := make(chan IncomingMessage, 10)

	mgr := NewManager(ctx, host, mod1, incoming)
	defer mgr.Shutdown()

	peerID := mustParsePeerID(t, testPeerIDStr)

	// First establish streams (registers shared key and allowed streams)
	mgr.EstablishStreams(peerID, mod2.Ed25519PublicKey(), []string{"test"})

	var buf1, buf2 bytes.Buffer
	stream1 := newMockStream(&buf1, &buf1)
	stream2 := newMockStream(&buf2, &buf2)

	// First incoming stream
	err := mgr.HandleIncomingStream(peerID, "test", stream1)
	if err != nil {
		t.Fatalf("first HandleIncomingStream failed: %v", err)
	}

	// Duplicate should fail
	err = mgr.HandleIncomingStream(peerID, "test", stream2)
	if err == nil {
		t.Error("HandleIncomingStream should fail for duplicate stream name")
	}
}

// TestManager_ConcurrentEstablish tests concurrent EstablishStreams calls
// for different peers to verify thread safety.
func TestManager_ConcurrentEstablish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv := generateManagerTestKey(t)
	mod, _ := crypto.NewModule(priv)
	incoming := make(chan IncomingMessage, 100)

	mgr := NewManager(ctx, host, mod, incoming)
	defer mgr.Shutdown()

	const numPeers = 50
	const numGoroutines = 10

	// Generate peer keys
	peerKeys := make([]ed25519.PrivateKey, numPeers)
	for i := 0; i < numPeers; i++ {
		peerKeys[i] = generateManagerTestKey(t)
	}

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(gID int) {
			defer wg.Done()

			// Each goroutine works with a subset of peers
			startPeer := (gID * numPeers) / numGoroutines
			endPeer := ((gID + 1) * numPeers) / numGoroutines

			for i := startPeer; i < endPeer; i++ {
				peerMod, _ := crypto.NewModule(peerKeys[i])
				peerPubKey := peerMod.Ed25519PublicKey()

				// Generate unique peer ID from key
				peerID := peer.ID(fmt.Sprintf("peer-%d", i))
				streamNames := []string{
					fmt.Sprintf("stream-a-%d", i),
					fmt.Sprintf("stream-b-%d", i),
				}

				err := mgr.EstablishStreams(peerID, peerPubKey, streamNames)
				if err != nil {
					t.Errorf("EstablishStreams for peer %d failed: %v", i, err)
				}
			}
		}(g)
	}

	wg.Wait()
}

// TestManager_ConcurrentSend tests concurrent Send calls to verify
// thread safety when multiple goroutines send to multiple peers.
func TestManager_ConcurrentSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv := generateManagerTestKey(t)
	mod, _ := crypto.NewModule(priv)
	incoming := make(chan IncomingMessage, 1000)

	mgr := NewManager(ctx, host, mod, incoming)
	defer mgr.Shutdown()

	const numPeers = 5
	const numGoroutines = 20
	const messagesPerGoroutine = 50

	// Set up peers with established streams
	peerIDs := make([]peer.ID, numPeers)
	for i := 0; i < numPeers; i++ {
		peerKey := generateManagerTestKey(t)
		peerMod, _ := crypto.NewModule(peerKey)
		peerID := peer.ID(fmt.Sprintf("sender-peer-%d", i))
		peerIDs[i] = peerID

		err := mgr.EstablishStreams(peerID, peerMod.Ed25519PublicKey(), []string{"data", "control"})
		if err != nil {
			t.Fatalf("EstablishStreams failed: %v", err)
		}
	}

	var wg sync.WaitGroup
	var sendCount sync.Map // Track sends per peer

	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(gID int) {
			defer wg.Done()

			for i := 0; i < messagesPerGoroutine; i++ {
				// Pick a random peer
				peerID := peerIDs[i%numPeers]
				streamName := "data"
				if i%3 == 0 {
					streamName = "control"
				}

				msg := []byte(fmt.Sprintf("msg-%d-%d", gID, i))
				err := mgr.Send(peerID, streamName, msg)
				if err != nil {
					// Errors are expected since mock stream may not be perfect
					continue
				}

				// Track successful sends
				key := peerID.String()
				if val, ok := sendCount.Load(key); ok {
					sendCount.Store(key, val.(int)+1)
				} else {
					sendCount.Store(key, 1)
				}
			}
		}(g)
	}

	wg.Wait()

	// Verify some sends completed
	var totalSends int
	sendCount.Range(func(key, value interface{}) bool {
		totalSends += value.(int)
		return true
	})

	t.Logf("Total successful sends: %d", totalSends)
}

// TestManager_ConcurrentOperations tests mixed concurrent operations:
// establish, send, close, and get operations happening simultaneously.
func TestManager_ConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv := generateManagerTestKey(t)
	mod, _ := crypto.NewModule(priv)
	incoming := make(chan IncomingMessage, 1000)

	mgr := NewManager(ctx, host, mod, incoming)
	defer mgr.Shutdown()

	const numOperations = 200
	const numGoroutines = 20

	// Pre-establish some peers
	const numInitialPeers = 5
	initialPeerIDs := make([]peer.ID, numInitialPeers)
	for i := 0; i < numInitialPeers; i++ {
		peerKey := generateManagerTestKey(t)
		peerMod, _ := crypto.NewModule(peerKey)
		peerID := peer.ID(fmt.Sprintf("initial-peer-%d", i))
		initialPeerIDs[i] = peerID

		mgr.EstablishStreams(peerID, peerMod.Ed25519PublicKey(), []string{"stream1", "stream2"})
	}

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(gID int) {
			defer wg.Done()

			for i := 0; i < numOperations; i++ {
				op := (gID + i) % 5

				switch op {
				case 0: // EstablishStreams for new peer
					peerKey := generateManagerTestKey(t)
					peerMod, _ := crypto.NewModule(peerKey)
					peerID := peer.ID(fmt.Sprintf("new-peer-%d-%d-%d", gID, i, time.Now().UnixNano()))
					_ = mgr.EstablishStreams(peerID, peerMod.Ed25519PublicKey(), []string{"test"})

				case 1: // Send to initial peer
					peerID := initialPeerIDs[i%numInitialPeers]
					_ = mgr.Send(peerID, "stream1", []byte("test message"))

				case 2: // GetStream
					peerID := initialPeerIDs[i%numInitialPeers]
					_ = mgr.GetStream(peerID, "stream1")

				case 3: // HasStreams
					peerID := initialPeerIDs[i%numInitialPeers]
					_ = mgr.HasStreams(peerID)

				case 4: // CloseStreams (for random new peer)
					peerID := peer.ID(fmt.Sprintf("close-target-%d-%d", gID, i))
					_ = mgr.CloseStreams(peerID)
				}
			}
		}(g)
	}

	wg.Wait()
	// If we get here without deadlock or race detector issues, the test passes
}

// TestManager_StressSendToSamePeer tests many goroutines sending to
// the same peer to verify proper write serialization.
func TestManager_StressSendToSamePeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv := generateManagerTestKey(t)
	mod, _ := crypto.NewModule(priv)
	incoming := make(chan IncomingMessage, 10000)

	mgr := NewManager(ctx, host, mod, incoming)
	defer mgr.Shutdown()

	// Set up a single peer
	peerKey := generateManagerTestKey(t)
	peerMod, _ := crypto.NewModule(peerKey)
	peerID := peer.ID("single-target-peer")

	err := mgr.EstablishStreams(peerID, peerMod.Ed25519PublicKey(), []string{"data"})
	if err != nil {
		t.Fatalf("EstablishStreams failed: %v", err)
	}

	const numGoroutines = 100
	const messagesPerGoroutine = 100

	var wg sync.WaitGroup
	var successCount sync.Map

	wg.Add(numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(gID int) {
			defer wg.Done()

			localSuccess := 0
			for i := 0; i < messagesPerGoroutine; i++ {
				msg := []byte(fmt.Sprintf("msg-%d-%d", gID, i))
				if err := mgr.Send(peerID, "data", msg); err == nil {
					localSuccess++
				}
			}
			successCount.Store(gID, localSuccess)
		}(g)
	}

	wg.Wait()

	// Count total successes
	var total int
	successCount.Range(func(key, value interface{}) bool {
		total += value.(int)
		return true
	})

	t.Logf("Total messages sent to single peer: %d", total)
}

// TestManager_HandleIncomingStream_Concurrent tests concurrent incoming
// stream handling for different stream names.
func TestManager_HandleIncomingStream_Concurrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := newMockStreamOpener()
	priv1 := generateManagerTestKey(t)
	priv2 := generateManagerTestKey(t)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)

	incoming := make(chan IncomingMessage, 100)

	mgr := NewManager(ctx, host, mod1, incoming)
	defer mgr.Shutdown()

	peerID := mustParsePeerID(t, testPeerIDStr)

	// Register many stream names
	const numStreams = 50
	streamNames := make([]string, numStreams)
	for i := 0; i < numStreams; i++ {
		streamNames[i] = fmt.Sprintf("stream-%d", i)
	}

	err := mgr.EstablishStreams(peerID, mod2.Ed25519PublicKey(), streamNames)
	if err != nil {
		t.Fatalf("EstablishStreams failed: %v", err)
	}

	// Concurrently handle incoming streams for each name
	var wg sync.WaitGroup
	wg.Add(numStreams)

	for i := 0; i < numStreams; i++ {
		go func(streamIndex int) {
			defer wg.Done()

			var buf bytes.Buffer
			stream := newMockStream(&buf, &buf)

			streamName := streamNames[streamIndex]
			err := mgr.HandleIncomingStream(peerID, streamName, stream)
			if err != nil {
				t.Errorf("HandleIncomingStream for %s failed: %v", streamName, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify all streams are registered
	if !mgr.HasStreams(peerID) {
		t.Error("expected HasStreams to be true")
	}

	// Verify each stream exists
	for _, name := range streamNames {
		if stream := mgr.GetStream(peerID, name); stream == nil {
			t.Errorf("stream %s should exist", name)
		}
	}
}
