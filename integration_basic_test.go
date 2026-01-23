package glueberry

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
)

// Basic integration tests that verify core functionality

func TestIntegration_NodeLifecycle(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	dir := filepath.Join(os.TempDir(), "glueberry-lifecycle-"+time.Now().Format("20060102150405"))
	defer os.RemoveAll(dir)

	abPath := filepath.Join(dir, "ab.json")
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	// Create node
	node, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Verify not started
	_, err = node.Connect(node.PeerID())
	if err != ErrNodeNotStarted {
		t.Error("operations should fail before Start()")
	}

	// Start
	if err := node.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify node info
	if node.PeerID() == "" {
		t.Error("PeerID should not be empty")
	}

	pubKey := node.PublicKey()
	if len(pubKey) != ed25519.PublicKeySize {
		t.Errorf("PublicKey size = %d, want %d", len(pubKey), ed25519.PublicKeySize)
	}

	addrs := node.Addrs()
	if len(addrs) == 0 {
		t.Error("Addrs should not be empty")
	}

	t.Logf("Node ID: %s", node.PeerID())
	t.Logf("Listening on: %v", addrs)

	// Stop
	if err := node.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify stopped
	_, err = node.Connect(node.PeerID())
	if err != ErrNodeNotStarted {
		t.Error("operations should fail after Stop()")
	}

	t.Log("✅ Node lifecycle test passed")
}

func TestIntegration_AddressBook(t *testing.T) {
	_, priv1, _ := ed25519.GenerateKey(rand.Reader)
	_, priv2, _ := ed25519.GenerateKey(rand.Reader)

	dir := filepath.Join(os.TempDir(), "glueberry-addressbook-"+time.Now().Format("20060102150405"))
	defer os.RemoveAll(dir)

	abPath := filepath.Join(dir, "ab.json")
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := NewConfig(priv1, abPath, []multiaddr.Multiaddr{listenAddr})

	node1, _ := New(cfg)
	node1.Start()
	defer node1.Stop()

	// Create a second node to get a peer ID
	cfg2 := NewConfig(priv2, abPath+"2", []multiaddr.Multiaddr{listenAddr})
	node2, _ := New(cfg2)
	node2.Start()
	defer node2.Stop()

	peerID := node2.PeerID()
	peerAddr := node2.Addrs()[0]

	// Add peer
	err := node1.AddPeer(peerID, []multiaddr.Multiaddr{peerAddr}, map[string]string{"name": "test-peer"})
	if err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	// Get peer
	entry, err := node1.GetPeer(peerID)
	if err != nil {
		t.Fatalf("GetPeer failed: %v", err)
	}

	if entry.PeerID != peerID {
		t.Error("PeerID mismatch")
	}
	if entry.Metadata["name"] != "test-peer" {
		t.Error("Metadata not stored")
	}

	// List peers
	peers := node1.ListPeers()
	if len(peers) != 1 {
		t.Errorf("ListPeers returned %d peers, want 1", len(peers))
	}

	// Blacklist
	if err := node1.BlacklistPeer(peerID); err != nil {
		t.Fatalf("BlacklistPeer failed: %v", err)
	}

	// List should be empty
	peers = node1.ListPeers()
	if len(peers) != 0 {
		t.Errorf("ListPeers returned %d peers after blacklist, want 0", len(peers))
	}

	// Unblacklist
	if err := node1.UnblacklistPeer(peerID); err != nil {
		t.Fatalf("UnblacklistPeer failed: %v", err)
	}

	// Remove
	if err := node1.RemovePeer(peerID); err != nil {
		t.Fatalf("RemovePeer failed: %v", err)
	}

	// Should be gone
	_, err = node1.GetPeer(peerID)
	if err == nil {
		t.Error("GetPeer should fail for removed peer")
	}

	t.Log("✅ Address book integration test passed")
}

func TestIntegration_Channels(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)

	dir := filepath.Join(os.TempDir(), "glueberry-channels-"+time.Now().Format("20060102150405"))
	defer os.RemoveAll(dir)

	abPath := filepath.Join(dir, "ab.json")
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, _ := New(cfg)
	node.Start()
	defer node.Stop()

	// Verify channels are available
	messagesChan := node.Messages()
	if messagesChan == nil {
		t.Error("Messages() returned nil")
	}

	eventsChan := node.Events()
	if eventsChan == nil {
		t.Error("Events() returned nil")
	}

	incomingChan := node.IncomingHandshakes()
	if incomingChan == nil {
		t.Error("IncomingHandshakes() returned nil")
	}

	// Channels should be non-blocking for select
	select {
	case <-messagesChan:
		// Got a message (shouldn't happen)
	case <-eventsChan:
		// Got an event (might happen)
	case <-incomingChan:
		// Got an incoming handshake (shouldn't happen)
	default:
		// Expected - no data yet
	}

	t.Log("✅ Channels integration test passed")
}
