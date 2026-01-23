package glueberry

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blockberries/glueberry/pkg/protocol"
	"github.com/multiformats/go-multiaddr"
)

// Integration tests for end-to-end communication between two Glueberry nodes.

func createTestNode(t *testing.T, name string) (*Node, ed25519.PrivateKey) {
	t.Helper()

	// Generate key
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key for %s: %v", name, err)
	}

	// Create temp directory for address book
	dir := filepath.Join(os.TempDir(), "glueberry-integration-"+name+"-"+time.Now().Format("20060102150405"))
	t.Cleanup(func() { os.RemoveAll(dir) })

	abPath := filepath.Join(dir, "addressbook.json")

	// Create listen address (port 0 for random)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create node %s: %v", name, err)
	}

	if err := node.Start(); err != nil {
		t.Fatalf("failed to start node %s: %v", name, err)
	}

	t.Cleanup(func() { node.Stop() })

	return node, priv
}

func TestIntegration_TwoNodeHandshake(t *testing.T) {
	// Create two nodes
	node1, priv1 := createTestNode(t, "node1")
	node2, priv2 := createTestNode(t, "node2")

	// Get public keys
	pub1 := priv1.Public().(ed25519.PublicKey)
	pub2 := priv2.Public().(ed25519.PublicKey)

	// Add node2 to node1's address book
	node1.AddPeer(node2.PeerID(), node2.Addrs(), map[string]string{"name": "node2"})

	// Add node1 to node2's address book
	node2.AddPeer(node1.PeerID(), node1.Addrs(), map[string]string{"name": "node1"})

	// Node1 connects to node2
	hs, err := node1.Connect(node2.PeerID())
	if err != nil {
		t.Fatalf("node1 Connect failed: %v", err)
	}

	// Node2 should receive incoming handshake
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var incomingHS protocol.IncomingHandshake
	select {
	case incomingHS = <-node2.IncomingHandshakes():
		if incomingHS.PeerID != node1.PeerID() {
			t.Errorf("incoming handshake from wrong peer: got %v, want %v", incomingHS.PeerID, node1.PeerID())
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for incoming handshake")
	}

	// Perform simple handshake
	type HelloMsg struct {
		ID      int64  `cramberry:"1,required"`
		Message string `cramberry:"2"`
	}

	// Node1 sends hello
	if err := hs.Send(&HelloMsg{ID: 1, Message: "hello from node1"}); err != nil {
		t.Fatalf("node1 handshake send failed: %v", err)
	}

	// Node2 receives hello
	var hello HelloMsg
	if err := incomingHS.HandshakeStream.Receive(&hello); err != nil {
		t.Fatalf("node2 handshake receive failed: %v", err)
	}

	if hello.Message != "hello from node1" {
		t.Errorf("received message = %q, want %q", hello.Message, "hello from node1")
	}

	// Node2 sends response
	if err := incomingHS.HandshakeStream.Send(&HelloMsg{ID: 2, Message: "hello from node2"}); err != nil {
		t.Fatalf("node2 handshake send failed: %v", err)
	}

	// Node1 receives response
	var response HelloMsg
	if err := hs.Receive(&response); err != nil {
		t.Fatalf("node1 handshake receive failed: %v", err)
	}

	if response.Message != "hello from node2" {
		t.Errorf("received response = %q, want %q", response.Message, "hello from node2")
	}

	// Handshake complete - both nodes establish encrypted streams
	streamNames := []string{"test-stream"}

	if err := node1.EstablishEncryptedStreams(node2.PeerID(), pub2, streamNames); err != nil {
		t.Fatalf("node1 EstablishEncryptedStreams failed: %v", err)
	}

	if err := node2.EstablishEncryptedStreams(node1.PeerID(), pub1, streamNames); err != nil {
		t.Fatalf("node2 EstablishEncryptedStreams failed: %v", err)
	}

	// Verify connection state is Established
	if node1.ConnectionState(node2.PeerID()) != StateEstablished {
		t.Errorf("node1 connection state = %v, want Established", node1.ConnectionState(node2.PeerID()))
	}
	if node2.ConnectionState(node1.PeerID()) != StateEstablished {
		t.Errorf("node2 connection state = %v, want Established", node2.ConnectionState(node1.PeerID()))
	}

	t.Log("✅ Two-node handshake completed successfully")
}

func TestIntegration_EncryptedMessaging(t *testing.T) {
	// Create and connect two nodes
	node1, priv1 := createTestNode(t, "node1")
	node2, priv2 := createTestNode(t, "node2")

	pub1 := priv1.Public().(ed25519.PublicKey)
	pub2 := priv2.Public().(ed25519.PublicKey)

	// Add peers
	node1.AddPeer(node2.PeerID(), node2.Addrs(), nil)
	node2.AddPeer(node1.PeerID(), node1.Addrs(), nil)

	// Connect and handshake
	hs1, _ := node1.Connect(node2.PeerID())

	var incomingHS protocol.IncomingHandshake
	select {
	case incomingHS = <-node2.IncomingHandshakes():
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for incoming handshake")
	}

	// Simple handshake exchange
	type Hello struct {
		ID int64 `cramberry:"1,required"`
	}
	hs1.Send(&Hello{ID: 1})
	incomingHS.HandshakeStream.Receive(&Hello{})
	incomingHS.HandshakeStream.Send(&Hello{ID: 2})
	hs1.Receive(&Hello{})

	// Establish encrypted streams
	streamNames := []string{"messages", "blocks"}
	node1.EstablishEncryptedStreams(node2.PeerID(), pub2, streamNames)
	node2.EstablishEncryptedStreams(node1.PeerID(), pub1, streamNames)

	// Give streams time to establish
	time.Sleep(200 * time.Millisecond)

	// Node1 sends to node2
	testMessage1 := []byte("encrypted message from node1")
	if err := node1.Send(node2.PeerID(), "messages", testMessage1); err != nil {
		t.Fatalf("node1 Send failed: %v", err)
	}

	// Node2 should receive the message
	select {
	case msg := <-node2.Messages():
		if msg.PeerID != node1.PeerID() {
			t.Errorf("message from wrong peer: %v", msg.PeerID)
		}
		if msg.StreamName != "messages" {
			t.Errorf("message on wrong stream: %s", msg.StreamName)
		}
		if string(msg.Data) != string(testMessage1) {
			t.Errorf("message data = %q, want %q", msg.Data, testMessage1)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message from node1")
	}

	// Node2 sends to node1
	testMessage2 := []byte("encrypted response from node2")
	if err := node2.Send(node1.PeerID(), "messages", testMessage2); err != nil {
		t.Fatalf("node2 Send failed: %v", err)
	}

	// Node1 should receive the response
	select {
	case msg := <-node1.Messages():
		if msg.PeerID != node2.PeerID() {
			t.Errorf("message from wrong peer: %v", msg.PeerID)
		}
		if string(msg.Data) != string(testMessage2) {
			t.Errorf("message data = %q, want %q", msg.Data, testMessage2)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message from node2")
	}

	// Test multiple streams
	blockData := []byte("block data")
	if err := node1.Send(node2.PeerID(), "blocks", blockData); err != nil {
		t.Fatalf("node1 Send on blocks stream failed: %v", err)
	}

	select {
	case msg := <-node2.Messages():
		if msg.StreamName != "blocks" {
			t.Errorf("message on wrong stream: %s", msg.StreamName)
		}
		if string(msg.Data) != string(blockData) {
			t.Error("block data mismatch")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for block message")
	}

	t.Log("✅ Encrypted messaging between two nodes successful")
}

func TestIntegration_MultipleMessages(t *testing.T) {
	// Create and connect two nodes
	node1, priv1 := createTestNode(t, "node1")
	node2, priv2 := createTestNode(t, "node2")

	pub1 := priv1.Public().(ed25519.PublicKey)
	pub2 := priv2.Public().(ed25519.PublicKey)

	// Setup connection
	node1.AddPeer(node2.PeerID(), node2.Addrs(), nil)
	node2.AddPeer(node1.PeerID(), node1.Addrs(), nil)

	// Connect and handshake
	hs1, _ := node1.Connect(node2.PeerID())
	var incomingHS protocol.IncomingHandshake
	select {
	case incomingHS = <-node2.IncomingHandshakes():
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	// Quick handshake
	type Hello struct {
		ID int64 `cramberry:"1,required"`
	}
	hs1.Send(&Hello{ID: 1})
	incomingHS.HandshakeStream.Receive(&Hello{})
	incomingHS.HandshakeStream.Send(&Hello{ID: 2})
	hs1.Receive(&Hello{})

	// Establish streams
	node1.EstablishEncryptedStreams(node2.PeerID(), pub2, []string{"data"})
	node2.EstablishEncryptedStreams(node1.PeerID(), pub1, []string{"data"})

	time.Sleep(200 * time.Millisecond)

	// Send multiple messages from node1 to node2
	numMessages := 10
	for i := 0; i < numMessages; i++ {
		msg := []byte(time.Now().String())
		if err := node1.Send(node2.PeerID(), "data", msg); err != nil {
			t.Fatalf("Send message %d failed: %v", i, err)
		}
	}

	// Receive all messages
	received := 0
	timeout := time.After(5 * time.Second)
	for received < numMessages {
		select {
		case <-node2.Messages():
			received++
		case <-timeout:
			t.Fatalf("timeout: received %d/%d messages", received, numMessages)
		}
	}

	t.Logf("✅ Successfully sent and received %d messages", numMessages)
}

func TestIntegration_ConnectionEvents(t *testing.T) {
	node1, _ := createTestNode(t, "node1")
	node2, _ := createTestNode(t, "node2")

	// Listen for events from node1
	events := node1.Events()
	eventLog := []ConnectionState{}
	done := make(chan bool)

	go func() {
		for evt := range events {
			if evt.PeerID == node2.PeerID() {
				eventLog = append(eventLog, evt.State)
				if evt.State == StateEstablished {
					done <- true
					return
				}
			}
		}
	}()

	// Setup connection
	node1.AddPeer(node2.PeerID(), node2.Addrs(), nil)
	node2.AddPeer(node1.PeerID(), node1.Addrs(), nil)

	// Connect
	hs1, _ := node1.Connect(node2.PeerID())

	// Handle incoming on node2
	var incomingHS protocol.IncomingHandshake
	select {
	case incomingHS = <-node2.IncomingHandshakes():
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	// Handshake
	type Hello struct {
		ID int64 `cramberry:"1,required"`
	}
	hs1.Send(&Hello{ID: 1})
	incomingHS.HandshakeStream.Receive(&Hello{})
	incomingHS.HandshakeStream.Send(&Hello{ID: 2})
	hs1.Receive(&Hello{})

	// Establish streams
	pub2 := node2.PublicKey()
	node1.EstablishEncryptedStreams(node2.PeerID(), pub2, []string{"test"})

	// Wait for Established event
	select {
	case <-done:
		// Event received
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for Established event")
	}

	// Verify we got the expected state transitions
	// Should have at least: Connecting, Connected, Handshaking, Established
	if len(eventLog) < 4 {
		t.Errorf("expected at least 4 events, got %d: %v", len(eventLog), eventLog)
	}

	expectedStates := []ConnectionState{StateConnecting, StateConnected, StateHandshaking, StateEstablished}
	for i, expected := range expectedStates {
		if i >= len(eventLog) {
			t.Errorf("missing event %d: expected %v", i, expected)
			continue
		}
		if eventLog[i] != expected {
			t.Errorf("event %d: got %v, want %v", i, eventLog[i], expected)
		}
	}

	t.Log("✅ Connection events received in correct order")
}

func TestIntegration_Disconnect(t *testing.T) {
	node1, priv1 := createTestNode(t, "node1")
	node2, priv2 := createTestNode(t, "node2")

	pub1 := priv1.Public().(ed25519.PublicKey)
	pub2 := priv2.Public().(ed25519.PublicKey)

	// Setup and connect
	node1.AddPeer(node2.PeerID(), node2.Addrs(), nil)
	node2.AddPeer(node1.PeerID(), node1.Addrs(), nil)

	hs1, _ := node1.Connect(node2.PeerID())
	var incomingHS protocol.IncomingHandshake
	select {
	case incomingHS = <-node2.IncomingHandshakes():
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	// Handshake
	type Hello struct {
		ID int64 `cramberry:"1,required"`
	}
	hs1.Send(&Hello{ID: 1})
	incomingHS.HandshakeStream.Receive(&Hello{})
	incomingHS.HandshakeStream.Send(&Hello{ID: 2})
	hs1.Receive(&Hello{})

	// Establish streams
	node1.EstablishEncryptedStreams(node2.PeerID(), pub2, []string{"test"})
	node2.EstablishEncryptedStreams(node1.PeerID(), pub1, []string{"test"})

	time.Sleep(200 * time.Millisecond)

	// Verify connected
	if node1.ConnectionState(node2.PeerID()) != StateEstablished {
		t.Fatal("node1 should be in Established state")
	}

	// Disconnect
	if err := node1.Disconnect(node2.PeerID()); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	// Verify disconnected
	time.Sleep(100 * time.Millisecond)
	if node1.ConnectionState(node2.PeerID()) != StateDisconnected {
		t.Errorf("node1 should be Disconnected, got %v", node1.ConnectionState(node2.PeerID()))
	}

	// Send should fail
	err := node1.Send(node2.PeerID(), "test", []byte("should fail"))
	if err == nil {
		t.Error("Send should fail after disconnect")
	}

	t.Log("✅ Disconnect test passed")
}

func TestIntegration_Blacklist(t *testing.T) {
	node1, _ := createTestNode(t, "node1")
	node2, priv2 := createTestNode(t, "node2")

	pub2 := priv2.Public().(ed25519.PublicKey)

	// Add node2 to node1
	node1.AddPeer(node2.PeerID(), node2.Addrs(), nil)
	node2.AddPeer(node1.PeerID(), node1.Addrs(), nil)

	// Connect
	hs1, _ := node1.Connect(node2.PeerID())
	var incomingHS protocol.IncomingHandshake
	select {
	case incomingHS = <-node2.IncomingHandshakes():
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}

	// Handshake
	type Hello struct {
		ID int64 `cramberry:"1,required"`
	}
	hs1.Send(&Hello{ID: 1})
	incomingHS.HandshakeStream.Receive(&Hello{})
	incomingHS.HandshakeStream.Send(&Hello{ID: 2})
	hs1.Receive(&Hello{})

	// Establish streams
	node1.EstablishEncryptedStreams(node2.PeerID(), pub2, []string{"test"})

	time.Sleep(200 * time.Millisecond)

	// Blacklist node2
	if err := node1.BlacklistPeer(node2.PeerID()); err != nil {
		t.Fatalf("BlacklistPeer failed: %v", err)
	}

	// Should be disconnected
	time.Sleep(100 * time.Millisecond)
	state := node1.ConnectionState(node2.PeerID())
	if state != StateDisconnected {
		t.Errorf("connection should be Disconnected after blacklist, got %v", state)
	}

	// Try to connect again - should fail (blacklisted)
	_, err := node1.Connect(node2.PeerID())
	if err == nil {
		t.Error("Connect should fail to blacklisted peer")
	}

	t.Log("✅ Blacklist test passed")
}
