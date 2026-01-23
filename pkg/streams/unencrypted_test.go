package streams

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnencryptedStream_SendReceive(t *testing.T) {
	// Create pipes for bidirectional communication
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	// stream1 writes to w1, reads from r2
	// stream2 writes to w2, reads from r1
	stream1 := newMockStream(r2, w1)
	stream2 := newMockStream(r1, w2)

	incoming := make(chan IncomingMessage, 10)
	ctx := context.Background()

	peerID := mustParsePeerID(t, testPeerIDStr)

	// Create unencrypted streams
	us1 := NewUnencryptedStream(ctx, "handshake", peerID, stream1, incoming)
	us2 := NewUnencryptedStream(ctx, "handshake", peerID, stream2, incoming)

	defer us1.Close()
	defer us2.Close()

	// Send message from us1
	testMsg := []byte("hello world")
	err := us1.Send(testMsg)
	require.NoError(t, err)

	// Receive on us2's incoming channel
	select {
	case msg := <-incoming:
		assert.Equal(t, peerID, msg.PeerID)
		assert.Equal(t, "handshake", msg.StreamName)
		assert.Equal(t, testMsg, msg.Data)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestUnencryptedStream_MultipleMessages(t *testing.T) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	stream1 := newMockStream(r2, w1)
	stream2 := newMockStream(r1, w2)

	incoming := make(chan IncomingMessage, 10)
	ctx := context.Background()

	peerID := mustParsePeerID(t, testPeerIDStr)

	us1 := NewUnencryptedStream(ctx, "handshake", peerID, stream1, incoming)
	us2 := NewUnencryptedStream(ctx, "handshake", peerID, stream2, incoming)

	defer us1.Close()
	defer us2.Close()

	messages := []string{"hello", "world", "test"}

	// Send multiple messages
	for _, msg := range messages {
		err := us1.Send([]byte(msg))
		require.NoError(t, err)
	}

	// Receive all messages
	for i, expected := range messages {
		select {
		case msg := <-incoming:
			assert.Equal(t, expected, string(msg.Data), "message %d", i)
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for message %d", i)
		}
	}
}

func TestUnencryptedStream_Close(t *testing.T) {
	r1, w1 := io.Pipe()

	stream := newMockStream(r1, w1)

	incoming := make(chan IncomingMessage, 10)
	ctx := context.Background()

	peerID := mustParsePeerID(t, testPeerIDStr)

	us := NewUnencryptedStream(ctx, "handshake", peerID, stream, incoming)

	assert.False(t, us.IsClosed())

	err := us.Close()
	assert.NoError(t, err)
	assert.True(t, us.IsClosed())

	// Send should fail after close
	err = us.Send([]byte("test"))
	assert.Error(t, err)
}

func TestUnencryptedStream_CloseIdempotent(t *testing.T) {
	r1, w1 := io.Pipe()

	stream := newMockStream(r1, w1)

	incoming := make(chan IncomingMessage, 10)
	ctx := context.Background()

	peerID := mustParsePeerID(t, testPeerIDStr)

	us := NewUnencryptedStream(ctx, "handshake", peerID, stream, incoming)

	// Multiple closes should not panic
	us.Close()
	us.Close()
	us.Close()
}

func TestUnencryptedStream_Accessors(t *testing.T) {
	r1, w1 := io.Pipe()

	stream := newMockStream(r1, w1)

	incoming := make(chan IncomingMessage, 10)
	ctx := context.Background()

	peerID := mustParsePeerID(t, testPeerIDStr)

	us := NewUnencryptedStream(ctx, "handshake", peerID, stream, incoming)
	defer us.Close()

	assert.Equal(t, "handshake", us.Name())
	assert.Equal(t, peerID, us.PeerID())
	assert.Equal(t, stream, us.Stream())
}

func TestUnencryptedStream_ConcurrentSend(t *testing.T) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	stream1 := newMockStream(r2, w1)
	stream2 := newMockStream(r1, w2)

	incoming := make(chan IncomingMessage, 1000)
	ctx := context.Background()

	peerID := mustParsePeerID(t, testPeerIDStr)

	us1 := NewUnencryptedStream(ctx, "handshake", peerID, stream1, incoming)
	_ = NewUnencryptedStream(ctx, "handshake", peerID, stream2, incoming)

	defer us1.Close()

	// Concurrent sends should not race
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				us1.Send([]byte("test"))
			}
		}(i)
	}

	wg.Wait()
}

func TestUnencryptedStream_Bidirectional(t *testing.T) {
	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()

	// stream1 writes to w1, reads from r2
	// stream2 writes to w2, reads from r1
	stream1 := newMockStream(r2, w1)
	stream2 := newMockStream(r1, w2)

	incoming := make(chan IncomingMessage, 10)
	ctx := context.Background()

	peer1 := mustParsePeerID(t, testPeerIDStr)
	peer2 := peer.ID("peer2")

	us1 := NewUnencryptedStream(ctx, "handshake", peer1, stream1, incoming)
	us2 := NewUnencryptedStream(ctx, "handshake", peer2, stream2, incoming)

	defer us1.Close()
	defer us2.Close()

	// Send from us1 to us2
	err := us1.Send([]byte("from-1-to-2"))
	require.NoError(t, err)

	// Send from us2 to us1
	err = us2.Send([]byte("from-2-to-1"))
	require.NoError(t, err)

	// Receive both messages (order not guaranteed)
	received := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case msg := <-incoming:
			received[string(msg.Data)] = true
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for message")
		}
	}

	assert.True(t, received["from-1-to-2"], "should receive message from 1 to 2")
	assert.True(t, received["from-2-to-1"], "should receive message from 2 to 1")
}
