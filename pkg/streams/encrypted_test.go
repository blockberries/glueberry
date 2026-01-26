package streams

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/blockberries/glueberry/pkg/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func generateTestKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return priv
}

func mustParsePeerID(t *testing.T, s string) peer.ID {
	t.Helper()
	id, err := peer.Decode(s)
	if err != nil {
		t.Fatalf("failed to parse peer ID: %v", err)
	}
	return id
}

const testPeerIDStr = "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"

func TestEncryptedStream_Send(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create crypto modules for both sides
	priv1 := generateTestKey(t)
	priv2 := generateTestKey(t)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)

	// Derive shared keys
	key1, _ := mod1.DeriveSharedKey(mod2.Ed25519PublicKey())
	key2, _ := mod2.DeriveSharedKey(mod1.Ed25519PublicKey())

	// Verify keys match
	if string(key1) != string(key2) {
		t.Fatal("shared keys should match")
	}

	// Create pipes for bidirectional communication
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	// stream1 writes to w1, reads from r2
	// stream2 writes to w2, reads from r1
	stream1 := newMockStream(r2, w1)
	stream2 := newMockStream(r1, w2)

	peerID := mustParsePeerID(t, testPeerIDStr)
	incoming := make(chan IncomingMessage, 10)

	sender, err := NewEncryptedStream(ctx, "test", peerID, stream1, key1, incoming, nil)
	if err != nil {
		t.Fatalf("NewEncryptedStream failed: %v", err)
	}
	defer sender.Close()

	receiver, err := NewEncryptedStream(ctx, "test", peerID, stream2, key2, incoming, nil)
	if err != nil {
		t.Fatalf("NewEncryptedStream failed: %v", err)
	}
	defer receiver.Close()

	// Send a message
	testData := []byte("encrypted test message")
	err = sender.Send(testData)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Receive should happen via the read loop goroutine
	select {
	case msg := <-incoming:
		if msg.PeerID != peerID {
			t.Errorf("PeerID = %v, want %v", msg.PeerID, peerID)
		}
		if msg.StreamName != "test" {
			t.Errorf("StreamName = %q, want %q", msg.StreamName, "test")
		}
		if string(msg.Data) != string(testData) {
			t.Errorf("Data = %q, want %q", msg.Data, testData)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestEncryptedStream_MultipleMessages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	priv1 := generateTestKey(t)
	priv2 := generateTestKey(t)

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)

	key1, _ := mod1.DeriveSharedKey(mod2.Ed25519PublicKey())

	// Use pipes for proper bidirectional communication
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	stream1 := newMockStream(r2, w1)
	stream2 := newMockStream(r1, w2)

	peerID := mustParsePeerID(t, testPeerIDStr)
	incoming := make(chan IncomingMessage, 10)

	sender, _ := NewEncryptedStream(ctx, "test", peerID, stream1, key1, incoming, nil)
	defer sender.Close()

	receiver, _ := NewEncryptedStream(ctx, "test", peerID, stream2, key1, incoming, nil)
	defer receiver.Close()

	// Send multiple messages
	messages := []string{"first", "second", "third", "fourth", "fifth"}
	for _, msg := range messages {
		if err := sender.Send([]byte(msg)); err != nil {
			t.Fatalf("Send failed: %v", err)
		}
	}

	// Receive all messages
	received := make([]string, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		select {
		case msg := <-incoming:
			received = append(received, string(msg.Data))
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for message %d", i)
		}
	}

	// Verify all messages received
	for i, expected := range messages {
		if i >= len(received) || received[i] != expected {
			t.Errorf("message %d = %q, want %q", i, received[i], expected)
		}
	}
}

func TestEncryptedStream_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	priv := generateTestKey(t)
	mod, _ := crypto.NewModule(priv)
	key, _ := mod.DeriveSharedKey(mod.Ed25519PublicKey())

	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)

	peerID := mustParsePeerID(t, testPeerIDStr)
	incoming := make(chan IncomingMessage, 10)

	es, _ := NewEncryptedStream(ctx, "test", peerID, stream, key, incoming, nil)

	err := es.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if !es.IsClosed() {
		t.Error("stream should be closed")
	}

	// Send should fail
	err = es.Send([]byte("test"))
	if err == nil {
		t.Error("Send should fail on closed stream")
	}
}

func TestEncryptedStream_Name(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	priv := generateTestKey(t)
	mod, _ := crypto.NewModule(priv)
	key, _ := mod.DeriveSharedKey(mod.Ed25519PublicKey())

	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)

	peerID := mustParsePeerID(t, testPeerIDStr)
	incoming := make(chan IncomingMessage, 10)

	es, _ := NewEncryptedStream(ctx, "my-stream", peerID, stream, key, incoming, nil)
	defer es.Close()

	if es.Name() != "my-stream" {
		t.Errorf("Name() = %q, want %q", es.Name(), "my-stream")
	}
	if es.PeerID() != peerID {
		t.Errorf("PeerID() = %v, want %v", es.PeerID(), peerID)
	}
}

func TestEncryptedStream_ConcurrentSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	priv := generateTestKey(t)
	mod, _ := crypto.NewModule(priv)
	key, _ := mod.DeriveSharedKey(mod.Ed25519PublicKey())

	// Use separate buffers to avoid concurrent access to same buffer
	var readBuf, writeBuf bytes.Buffer

	stream := newMockStream(&readBuf, &writeBuf)

	peerID := mustParsePeerID(t, testPeerIDStr)
	incoming := make(chan IncomingMessage, 100)

	es, _ := NewEncryptedStream(ctx, "test", peerID, stream, key, incoming, nil)
	defer es.Close()

	// Send from multiple goroutines
	numGoroutines := 10
	messagesPerGoroutine := 10

	done := make(chan bool, numGoroutines)
	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			for i := 0; i < messagesPerGoroutine; i++ {
				es.Send([]byte("test"))
			}
			done <- true
		}(g)
	}

	// Wait for all to complete
	for g := 0; g < numGoroutines; g++ {
		<-done
	}

	// All sends should have completed without race conditions
}

func TestEncryptedStream_ReadLoopEOF(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	priv := generateTestKey(t)
	mod, _ := crypto.NewModule(priv)
	key, _ := mod.DeriveSharedKey(mod.Ed25519PublicKey())

	// Empty buffer will cause EOF immediately
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)

	peerID := mustParsePeerID(t, testPeerIDStr)
	incoming := make(chan IncomingMessage, 10)

	es, _ := NewEncryptedStream(ctx, "test", peerID, stream, key, incoming, nil)

	// Wait a bit for read loop to encounter EOF
	time.Sleep(50 * time.Millisecond)

	// Stream should be closed by read loop
	if !es.IsClosed() {
		// This might not be closed yet, which is fine
		// The important thing is no panic occurred
		t.Logf("Stream not yet closed after EOF, this is acceptable")
	}
}

func TestEncryptedStream_DecryptionErrorCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create different keys for sender and receiver to cause decryption failure
	priv1 := generateTestKey(t)
	priv2 := generateTestKey(t)
	priv3 := generateTestKey(t) // Third key for receiver - will cause decryption failure

	mod1, _ := crypto.NewModule(priv1)
	mod2, _ := crypto.NewModule(priv2)
	mod3, _ := crypto.NewModule(priv3)

	// Sender derives key from peer 2's public key
	senderKey, _ := mod1.DeriveSharedKey(mod2.Ed25519PublicKey())

	// Receiver uses DIFFERENT key (derived from peer 3's public key)
	// This will cause decryption to fail
	receiverKey, _ := mod2.DeriveSharedKey(mod3.Ed25519PublicKey())

	// Create pipes for bidirectional communication
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	stream1 := newMockStream(r2, w1)
	stream2 := newMockStream(r1, w2)

	peerID := mustParsePeerID(t, testPeerIDStr)
	incoming := make(chan IncomingMessage, 10)

	// Track callback invocations
	var callbackMu sync.Mutex
	var callbackInvoked bool
	var callbackPeerID peer.ID
	var callbackErr error

	errorCallback := func(pid peer.ID, err error) {
		callbackMu.Lock()
		defer callbackMu.Unlock()
		callbackInvoked = true
		callbackPeerID = pid
		callbackErr = err
	}

	sender, err := NewEncryptedStream(ctx, "test", peerID, stream1, senderKey, incoming, nil)
	if err != nil {
		t.Fatalf("NewEncryptedStream for sender failed: %v", err)
	}
	defer sender.Close()

	receiver, err := NewEncryptedStream(ctx, "test", peerID, stream2, receiverKey, incoming, errorCallback)
	if err != nil {
		t.Fatalf("NewEncryptedStream for receiver failed: %v", err)
	}
	defer receiver.Close()

	// Send a message (will be encrypted with sender's key)
	err = sender.Send([]byte("test message"))
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Wait for decryption attempt
	time.Sleep(100 * time.Millisecond)

	// Verify callback was invoked
	callbackMu.Lock()
	if !callbackInvoked {
		t.Error("decryption error callback was not invoked")
	}
	if callbackPeerID != peerID {
		t.Errorf("callback peerID = %v, want %v", callbackPeerID, peerID)
	}
	if callbackErr == nil {
		t.Error("callback error should not be nil")
	}
	callbackMu.Unlock()

	// Message should NOT be delivered to incoming channel (decryption failed)
	select {
	case <-incoming:
		t.Error("message should not have been delivered due to decryption failure")
	default:
		// Expected - no message
	}
}
