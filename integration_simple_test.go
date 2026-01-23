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

// Simple integration test to debug connection issues

func TestSimple_CreateAndStart(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)

	dir := filepath.Join(os.TempDir(), "glueberry-simple-test")
	defer os.RemoveAll(dir)

	abPath := filepath.Join(dir, "ab.json")
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if err := node.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	t.Logf("Node started with ID: %s", node.PeerID())
	t.Logf("Listening on: %v", node.Addrs())

	// Check channels are available
	if node.Messages() == nil {
		t.Error("Messages channel is nil")
	}
	if node.Events() == nil {
		t.Error("Events channel is nil")
	}
	if node.IncomingHandshakes() == nil {
		t.Error("IncomingHandshakes channel is nil")
	}

	// Stop
	if err := node.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	t.Log("✅ Basic node creation and lifecycle works")
}

func TestSimple_AddPeerAndConnect(t *testing.T) {
	// Create two nodes
	_, priv1, _ := ed25519.GenerateKey(rand.Reader)
	_, priv2, _ := ed25519.GenerateKey(rand.Reader)

	dir1 := filepath.Join(os.TempDir(), "glueberry-node1-"+time.Now().Format("20060102150405"))
	dir2 := filepath.Join(os.TempDir(), "glueberry-node2-"+time.Now().Format("20060102150405"))
	defer os.RemoveAll(dir1)
	defer os.RemoveAll(dir2)

	abPath1 := filepath.Join(dir1, "ab.json")
	abPath2 := filepath.Join(dir2, "ab.json")

	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg1 := NewConfig(priv1, abPath1, []multiaddr.Multiaddr{listenAddr})
	cfg2 := NewConfig(priv2, abPath2, []multiaddr.Multiaddr{listenAddr})

	node1, _ := New(cfg1)
	node2, _ := New(cfg2)

	node1.Start()
	node2.Start()

	defer node1.Stop()
	defer node2.Stop()

	t.Logf("Node1 ID: %s, Addrs: %v", node1.PeerID(), node1.Addrs())
	t.Logf("Node2 ID: %s, Addrs: %v", node2.PeerID(), node2.Addrs())

	// Add node2 to node1's address book
	if err := node1.AddPeer(node2.PeerID(), node2.Addrs(), nil); err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	// Node1 connects to node2
	t.Log("Node1 attempting to connect to node2...")
	hs, err := node1.Connect(node2.PeerID())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if hs == nil {
		t.Fatal("Connect returned nil HandshakeStream")
	}

	t.Logf("✅ Node1 successfully connected and got handshake stream")
	t.Logf("   Handshake stream time remaining: %v", hs.TimeRemaining())

	// Now check if node2 receives incoming handshake
	t.Log("Waiting for node2 to receive incoming handshake...")

	select {
	case incoming := <-node2.IncomingHandshakes():
		t.Logf("✅ Node2 received incoming handshake from peer: %s", incoming.PeerID)
		if incoming.PeerID != node1.PeerID() {
			t.Errorf("Wrong peer ID: got %s, want %s", incoming.PeerID, node1.PeerID())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("❌ Timeout waiting for incoming handshake on node2")
	}
}
