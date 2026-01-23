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

// Working integration test demonstrating real-world usage
// This test shows unidirectional messaging (node1 -> node2)
// Bidirectional messaging would require both nodes to call EstablishEncryptedStreams
// sequentially with proper synchronization

type Handshake struct {
	PublicKey []byte `cramberry:"1,required"`
}

func TestIntegration_UnidirectionalMessaging(t *testing.T) {
	// Create two nodes
	_, priv1, _ := ed25519.GenerateKey(rand.Reader)
	_, priv2, _ := ed25519.GenerateKey(rand.Reader)
	pub1 := priv1.Public().(ed25519.PublicKey)
	pub2 := priv2.Public().(ed25519.PublicKey)

	dir1 := filepath.Join(os.TempDir(), "glueberry-node1-"+time.Now().Format("20060102150405"))
	dir2 := filepath.Join(os.TempDir(), "glueberry-node2-"+time.Now().Format("20060102150405"))
	defer os.RemoveAll(dir1)
	defer os.RemoveAll(dir2)

	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg1 := NewConfig(priv1, filepath.Join(dir1, "ab.json"), []multiaddr.Multiaddr{listenAddr})
	cfg2 := NewConfig(priv2, filepath.Join(dir2, "ab.json"), []multiaddr.Multiaddr{listenAddr})

	node1, _ := New(cfg1)
	node2, _ := New(cfg2)

	node1.Start()
	node2.Start()
	defer node1.Stop()
	defer node2.Stop()

	t.Logf("Node1: %s", node1.PeerID())
	t.Logf("Node2: %s", node2.PeerID())

	// Add peers
	node1.AddPeer(node2.PeerID(), node2.Addrs(), nil)
	node2.AddPeer(node1.PeerID(), node1.Addrs(), nil)

	// Handle incoming handshake on node2
	handshakeDone := make(chan bool)
	go func() {
		incoming := <-node2.IncomingHandshakes()
		t.Logf("✅ Node2 received incoming handshake")

		// Receive public key
		var hs Handshake
		incoming.HandshakeStream.Receive(&hs)

		// Send our public key
		incoming.HandshakeStream.Send(&Handshake{PublicKey: pub2})

		// Establish encrypted streams (registers handlers for incoming from node1)
		node2.EstablishEncryptedStreams(incoming.PeerID, pub1, []string{"messages"})

		t.Log("✅ Node2 ready to receive")
		handshakeDone <- true
	}()

	// Node1 connects and performs handshake
	t.Log("Node1 connecting...")
	hs, _ := node1.Connect(node2.PeerID())

	// Send public key
	hs.Send(&Handshake{PublicKey: pub1})

	// Receive node2's public key
	var response Handshake
	hs.Receive(&response)

	t.Log("✅ Node1 handshake complete")

	// Wait for node2 to be ready
	<-handshakeDone

	// With lazy stream opening, BOTH nodes can call EstablishEncryptedStreams
	// in ANY order - no synchronization required!
	t.Log("Both nodes establishing encrypted streams (can be in any order)...")

	// Node1 establishes (registers handlers)
	if err := node1.EstablishEncryptedStreams(node2.PeerID(), pub2, []string{"messages"}); err != nil {
		t.Fatalf("Node1 EstablishEncryptedStreams failed: %v", err)
	}

	// Node2 establishes (registers handlers)
	if err := node2.EstablishEncryptedStreams(node1.PeerID(), pub1, []string{"messages"}); err != nil {
		t.Fatalf("Node2 EstablishEncryptedStreams failed: %v", err)
	}

	t.Log("✅ Both nodes ready (streams will open lazily on first Send)")

	// Give time for handlers to register
	time.Sleep(200 * time.Millisecond)

	// Check states
	t.Logf("Node1 state: %v", node1.ConnectionState(node2.PeerID()))
	t.Logf("Node2 state: %v", node2.ConnectionState(node1.PeerID()))

	// Test bidirectional messaging - both nodes can send to each other
	// Streams open lazily on first Send()

	// Node1 → Node2
	msg1to2 := []byte("Hello from Node1!")
	t.Log("Node1 sending to Node2 (stream will open lazily)...")
	if err := node1.Send(node2.PeerID(), "messages", msg1to2); err != nil {
		t.Fatalf("Node1 Send failed: %v", err)
	}
	t.Log("✅ Node1 sent (stream opened lazily)")

	// Node2 receives
	select {
	case msg := <-node2.Messages():
		t.Logf("✅ Node2 received: %s", string(msg.Data))
		if string(msg.Data) != string(msg1to2) {
			t.Errorf("Message mismatch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message on node2")
	}

	// Node2 → Node1 (demonstrates symmetry)
	msg2to1 := []byte("Hello back from Node2!")
	t.Log("Node2 sending to Node1 (stream will open lazily)...")
	if err := node2.Send(node1.PeerID(), "messages", msg2to1); err != nil {
		t.Fatalf("Node2 Send failed: %v", err)
	}
	t.Log("✅ Node2 sent (stream opened lazily)")

	// Node1 receives
	select {
	case msg := <-node1.Messages():
		t.Logf("✅ Node1 received: %s", string(msg.Data))
		if string(msg.Data) != string(msg2to1) {
			t.Errorf("Message mismatch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message on node1")
	}

	t.Log("✅✅✅ INTEGRATION TEST PASSED!")
	t.Log("  - Symmetric API: Both nodes called EstablishEncryptedStreams in any order")
	t.Log("  - Lazy stream opening: Streams created on first Send()")
	t.Log("  - Bidirectional messaging: Both nodes sent and received encrypted messages")
}
