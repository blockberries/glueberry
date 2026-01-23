package streams

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/blockberries/glueberry/pkg/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
)

// mockStreamOpener implements StreamOpener for testing
type mockStreamOpener struct {
	streams map[string]*mockStream
}

func newMockStreamOpener() *mockStreamOpener {
	return &mockStreamOpener{
		streams: make(map[string]*mockStream),
	}
}

func (mso *mockStreamOpener) NewStream(ctx context.Context, peerID peer.ID, streamName string) (network.Stream, error) {
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
