package testing

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

func mustParseMultiaddr(t *testing.T, s string) multiaddr.Multiaddr {
	t.Helper()
	ma, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		t.Fatalf("failed to parse multiaddr %q: %v", s, err)
	}
	return ma
}

func TestNewMockNode(t *testing.T) {
	node := NewMockNode()

	if node.PeerID() == "" {
		t.Error("expected non-empty peer ID")
	}
	if len(node.PublicKey()) == 0 {
		t.Error("expected non-empty public key")
	}
}

func TestMockNode_StartStop(t *testing.T) {
	node := NewMockNode()

	if err := node.Start(); err != nil {
		t.Errorf("Start() failed: %v", err)
	}

	if err := node.Stop(); err != nil {
		t.Errorf("Stop() failed: %v", err)
	}
}

func TestMockNode_AddPeer(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	err := node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("AddPeer() failed: %v", err)
	}

	entry, err := node.GetPeer(peerID)
	if err != nil {
		t.Fatalf("GetPeer() failed: %v", err)
	}

	if entry.PeerID != peerID {
		t.Errorf("peer ID mismatch: got %v, want %v", entry.PeerID, peerID)
	}
	if entry.Metadata["key"] != "value" {
		t.Errorf("metadata mismatch: got %v", entry.Metadata)
	}
}

func TestMockNode_RemovePeer(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)

	if err := node.RemovePeer(peerID); err != nil {
		t.Fatalf("RemovePeer() failed: %v", err)
	}

	_, err := node.GetPeer(peerID)
	if !errors.Is(err, ErrPeerNotFound) {
		t.Errorf("expected ErrPeerNotFound, got %v", err)
	}
}

func TestMockNode_RemovePeer_NotFound(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("nonexistent")

	err := node.RemovePeer(peerID)
	if !errors.Is(err, ErrPeerNotFound) {
		t.Errorf("expected ErrPeerNotFound, got %v", err)
	}
}

func TestMockNode_Blacklist(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	node.BlacklistPeer(peerID)

	// Should not be in ListPeers
	peers := node.ListPeers()
	for _, p := range peers {
		if p.PeerID == peerID {
			t.Error("blacklisted peer should not be in ListPeers")
		}
	}

	// Should fail to add peer again
	err := node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	if !errors.Is(err, ErrPeerBlacklisted) {
		t.Errorf("expected ErrPeerBlacklisted, got %v", err)
	}

	// Unblacklist
	node.UnblacklistPeer(peerID)

	// Now can add again
	err = node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	if err != nil {
		t.Errorf("AddPeer() after unblacklist failed: %v", err)
	}
}

func TestMockNode_Connect(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)

	if err := node.Connect(peerID); err != nil {
		t.Fatalf("Connect() failed: %v", err)
	}

	if !node.IsConnected(peerID) {
		t.Error("expected to be connected")
	}

	isOutbound, err := node.IsOutbound(peerID)
	if err != nil {
		t.Fatalf("IsOutbound() failed: %v", err)
	}
	if !isOutbound {
		t.Error("expected outbound connection")
	}
}

func TestMockNode_ConnectCtx(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := node.ConnectCtx(ctx, peerID); err != nil {
		t.Fatalf("ConnectCtx() failed: %v", err)
	}

	if !node.IsConnected(peerID) {
		t.Error("expected to be connected")
	}
}

func TestMockNode_Connect_Blacklisted(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")

	node.BlacklistPeer(peerID)

	err := node.Connect(peerID)
	if !errors.Is(err, ErrPeerBlacklisted) {
		t.Errorf("expected ErrPeerBlacklisted, got %v", err)
	}
}

func TestMockNode_Disconnect(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	node.Connect(peerID)

	if err := node.Disconnect(peerID); err != nil {
		t.Fatalf("Disconnect() failed: %v", err)
	}

	if node.IsConnected(peerID) {
		t.Error("expected to be disconnected")
	}
}

func TestMockNode_Send(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	node.Connect(peerID)

	data := []byte("hello world")
	if err := node.Send(peerID, "test-stream", data); err != nil {
		t.Fatalf("Send() failed: %v", err)
	}

	msgs := node.SentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(msgs))
	}

	if msgs[0].PeerID != peerID {
		t.Errorf("peer ID mismatch: got %v, want %v", msgs[0].PeerID, peerID)
	}
	if msgs[0].StreamName != "test-stream" {
		t.Errorf("stream name mismatch: got %v, want %v", msgs[0].StreamName, "test-stream")
	}
	if !bytes.Equal(msgs[0].Data, data) {
		t.Errorf("data mismatch: got %v, want %v", msgs[0].Data, data)
	}
}

func TestMockNode_Send_NotConnected(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")

	err := node.Send(peerID, "test-stream", []byte("data"))
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("expected ErrNotConnected, got %v", err)
	}
}

func TestMockNode_SimulateConnect(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.SimulateConnect(peerID, []multiaddr.Multiaddr{addr})

	if !node.IsConnected(peerID) {
		t.Error("expected to be connected")
	}

	isOutbound, err := node.IsOutbound(peerID)
	if err != nil {
		t.Fatalf("IsOutbound() failed: %v", err)
	}
	if isOutbound {
		t.Error("simulated connection should be inbound")
	}

	// Check for event
	select {
	case event := <-node.Events():
		if event.PeerID != peerID {
			t.Errorf("event peer ID mismatch")
		}
		if event.State != "connected" {
			t.Errorf("event state mismatch: got %v, want connected", event.State)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected connection event")
	}
}

func TestMockNode_SimulateDisconnect(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.SimulateConnect(peerID, []multiaddr.Multiaddr{addr})
	// Drain the connect event
	<-node.Events()

	node.SimulateDisconnect(peerID)

	if node.IsConnected(peerID) {
		t.Error("expected to be disconnected")
	}

	// Check for event
	select {
	case event := <-node.Events():
		if event.State != "disconnected" {
			t.Errorf("event state mismatch: got %v, want disconnected", event.State)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected disconnect event")
	}
}

func TestMockNode_SimulateMessage(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	data := []byte("test message")

	node.SimulateMessage(peerID, "test-stream", data)

	select {
	case msg := <-node.Messages():
		if msg.PeerID != peerID {
			t.Errorf("peer ID mismatch")
		}
		if msg.StreamName != "test-stream" {
			t.Errorf("stream name mismatch")
		}
		if !bytes.Equal(msg.Data, data) {
			t.Errorf("data mismatch")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected message")
	}
}

func TestMockNode_SetConnectError(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	expectedErr := errors.New("connection failed")

	node.SetConnectError(expectedErr)

	err := node.Connect(peerID)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestMockNode_SetSendError(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")
	expectedErr := errors.New("send failed")

	node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	node.Connect(peerID)
	node.SetSendError(expectedErr)

	err := node.Send(peerID, "test-stream", []byte("data"))
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestMockNode_AssertSent(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	node.Connect(peerID)

	node.Send(peerID, "stream1", []byte("msg1"))
	node.Send(peerID, "stream2", []byte("msg2"))
	node.Send(peerID, "stream1", []byte("msg3"))

	msgs := node.AssertSent(peerID, "stream1")
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages on stream1, got %d", len(msgs))
	}

	msgs = node.AssertSent(peerID, "stream2")
	if len(msgs) != 1 {
		t.Errorf("expected 1 message on stream2, got %d", len(msgs))
	}

	if !node.AssertNotSent(peerID, "stream3") {
		t.Error("expected no messages on stream3")
	}
}

func TestMockNode_ClearSentMessages(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	node.Connect(peerID)
	node.Send(peerID, "test-stream", []byte("data"))

	node.ClearSentMessages()

	if len(node.SentMessages()) != 0 {
		t.Error("expected empty sent messages after clear")
	}
}

func TestMockNode_ConnectedPeers(t *testing.T) {
	node := NewMockNode()
	peer1 := peer.ID("peer1")
	peer2 := peer.ID("peer2")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.AddPeer(peer1, []multiaddr.Multiaddr{addr}, nil)
	node.AddPeer(peer2, []multiaddr.Multiaddr{addr}, nil)
	node.Connect(peer1)
	node.Connect(peer2)

	peers := node.ConnectedPeers()
	if len(peers) != 2 {
		t.Errorf("expected 2 connected peers, got %d", len(peers))
	}
}

func TestMockNode_Reset(t *testing.T) {
	node := NewMockNode()
	peerID := peer.ID("test-peer")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	node.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	node.Connect(peerID)
	node.Send(peerID, "test-stream", []byte("data"))
	node.SetConnectError(errors.New("error"))

	node.Reset()

	if len(node.ListPeers()) != 0 {
		t.Error("expected no peers after reset")
	}
	if len(node.SentMessages()) != 0 {
		t.Error("expected no sent messages after reset")
	}
	if len(node.ConnectedPeers()) != 0 {
		t.Error("expected no connected peers after reset")
	}
}

func TestNewMockNodeWithID(t *testing.T) {
	peerID := peer.ID("custom-peer-id")
	pubKey := make([]byte, 32)

	node := NewMockNodeWithID(peerID, pubKey)

	if node.PeerID() != peerID {
		t.Errorf("peer ID mismatch: got %v, want %v", node.PeerID(), peerID)
	}
}
