package glueberry

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blockberries/cramberry/pkg/cramberry"
	"github.com/blockberries/glueberry/pkg/streams"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// Integration test demonstrating the symmetric, event-driven API
// using Cramberry polymorphic messages for the handshake protocol.

// HandshakeMessage is the interface for all handshake message types.
// Cramberry uses TypeIDs to distinguish between concrete implementations.
type HandshakeMessage interface {
	isHandshakeMessage()
}

// HelloMessage is sent to initiate a handshake.
type HelloMessage struct {
	Version uint32 `cramberry:"1"`
}

func (HelloMessage) isHandshakeMessage() {}

// PubKeyMessage contains the sender's Ed25519 public key.
type PubKeyMessage struct {
	PublicKey []byte `cramberry:"1"`
}

func (PubKeyMessage) isHandshakeMessage() {}

// CompleteMessage signals handshake completion.
type CompleteMessage struct {
	Success bool `cramberry:"1"`
}

func (CompleteMessage) isHandshakeMessage() {}

// HandshakeEnvelope wraps a HandshakeMessage for polymorphic serialization.
type HandshakeEnvelope struct {
	Message HandshakeMessage `cramberry:"1"`
}

// Type IDs for handshake messages (in user range: 128+)
const (
	typeIDHelloMessage    cramberry.TypeID = 128
	typeIDPubKeyMessage   cramberry.TypeID = 129
	typeIDCompleteMessage cramberry.TypeID = 130
)

// registerHandshakeTypes registers all handshake message types with Cramberry.
// This must be called before marshaling/unmarshaling handshake messages.
func registerHandshakeTypes() {
	// Clear any previous registrations (useful for tests)
	cramberry.DefaultRegistry.Clear()

	// Register concrete types with specific TypeIDs
	cramberry.MustRegisterWithID[HelloMessage](typeIDHelloMessage)
	cramberry.MustRegisterWithID[PubKeyMessage](typeIDPubKeyMessage)
	cramberry.MustRegisterWithID[CompleteMessage](typeIDCompleteMessage)
}

// marshalHandshakeMsg serializes a handshake message using Cramberry.
func marshalHandshakeMsg(msg HandshakeMessage) ([]byte, error) {
	envelope := HandshakeEnvelope{Message: msg}
	return cramberry.Marshal(envelope)
}

// unmarshalHandshakeMsg deserializes a handshake message using Cramberry.
func unmarshalHandshakeMsg(data []byte) (HandshakeMessage, error) {
	var envelope HandshakeEnvelope
	if err := cramberry.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal handshake message: %w", err)
	}
	if envelope.Message == nil {
		return nil, fmt.Errorf("nil handshake message")
	}
	return envelope.Message, nil
}

func TestIntegration_SymmetricHandshake(t *testing.T) {
	// Register Cramberry types for polymorphic handshake messages
	registerHandshakeTypes()

	// Create two nodes
	_, priv1, _ := ed25519.GenerateKey(rand.Reader)
	_, priv2, _ := ed25519.GenerateKey(rand.Reader)
	pub1 := priv1.Public().(ed25519.PublicKey)
	pub2 := priv2.Public().(ed25519.PublicKey)

	dir1 := filepath.Join(os.TempDir(), "glueberry-node1-"+randomSuffix())
	dir2 := filepath.Join(os.TempDir(), "glueberry-node2-"+randomSuffix())
	defer os.RemoveAll(dir1)
	defer os.RemoveAll(dir2)

	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg1 := NewConfig(priv1, filepath.Join(dir1, "ab.json"), []multiaddr.Multiaddr{listenAddr})
	cfg2 := NewConfig(priv2, filepath.Join(dir2, "ab.json"), []multiaddr.Multiaddr{listenAddr})

	cfg1.HandshakeTimeout = 30 * time.Second
	cfg2.HandshakeTimeout = 30 * time.Second

	node1, err := New(cfg1)
	if err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}
	node2, err := New(cfg2)
	if err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}

	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}
	defer node1.Stop()
	defer node2.Stop()

	t.Logf("Node1: %s", node1.PeerID())
	t.Logf("Node2: %s", node2.PeerID())

	// Channels to signal when each node reaches StateEstablished
	node1Established := make(chan struct{})
	node2Established := make(chan struct{})
	errChan := make(chan error, 8)

	// sendHello sends a Hello message - called on StateConnected
	sendHello := func(node *Node, peerID peer.ID, name string) error {
		helloMsg := HelloMessage{Version: 1}
		data, err := marshalHandshakeMsg(helloMsg)
		if err != nil {
			return fmt.Errorf("marshal Hello: %w", err)
		}
		if err := node.Send(peerID, streams.HandshakeStreamName, data); err != nil {
			return fmt.Errorf("send Hello: %w", err)
		}
		t.Logf("[%s] Sent HelloMessage", name)
		return nil
	}

	// Node1's event handler:
	// - StateConnected → Send Hello
	// - StateEstablished → Signal completion
	go func() {
		for event := range node1.Events() {
			t.Logf("[node1] Event: %s for peer %s", event.State, event.PeerID.String()[:8])
			switch event.State {
			case StateConnected:
				if event.PeerID == node2.PeerID() {
					if err := sendHello(node1, node2.PeerID(), "node1"); err != nil {
						errChan <- err
						return
					}
				}
			case StateEstablished:
				if event.PeerID == node2.PeerID() {
					t.Log("[node1] StateEstablished!")
					close(node1Established)
					return
				}
			}
		}
	}()

	// Node2's event handler (symmetric to Node1)
	go func() {
		for event := range node2.Events() {
			t.Logf("[node2] Event: %s for peer %s", event.State, event.PeerID.String()[:8])
			switch event.State {
			case StateConnected:
				if event.PeerID == node1.PeerID() {
					if err := sendHello(node2, node1.PeerID(), "node2"); err != nil {
						errChan <- err
						return
					}
				}
			case StateEstablished:
				if event.PeerID == node1.PeerID() {
					t.Log("[node2] StateEstablished!")
					close(node2Established)
					return
				}
			}
		}
	}()

	// Node1's message handler - symmetric handshake protocol (two-phase):
	// Receive Hello → Send PubKey
	// Receive PubKey → PrepareStreams() + Send Complete
	// Receive Complete → FinalizeHandshake()
	go func() {
		var streamsPrepared, gotComplete bool

		for msg := range node1.Messages() {
			if msg.StreamName != streams.HandshakeStreamName {
				continue
			}

			hsMsg, err := unmarshalHandshakeMsg(msg.Data)
			if err != nil {
				t.Logf("[node1] Failed to unmarshal: %v", err)
				continue
			}

			switch m := hsMsg.(type) {
			case *HelloMessage:
				t.Logf("[node1] Received HelloMessage (version=%d)", m.Version)
				// Receive Hello → Send PubKey
				pubKeyMsg := PubKeyMessage{PublicKey: pub1}
				data, _ := marshalHandshakeMsg(pubKeyMsg)
				if err := node1.Send(node2.PeerID(), streams.HandshakeStreamName, data); err != nil {
					errChan <- err
					return
				}
				t.Log("[node1] Sent PubKeyMessage")

			case *PubKeyMessage:
				t.Log("[node1] Received PubKeyMessage")
				peerPubKey := ed25519.PublicKey(m.PublicKey)
				// Receive PubKey → PrepareStreams (ready to receive encrypted messages)
				if err := node1.PrepareStreams(node2.PeerID(), peerPubKey, []string{"messages"}); err != nil {
					errChan <- err
					return
				}
				streamsPrepared = true
				t.Log("[node1] PrepareStreams called")
				// Send Complete to signal we're ready
				completeMsg := CompleteMessage{Success: true}
				data, _ := marshalHandshakeMsg(completeMsg)
				if err := node1.Send(node2.PeerID(), streams.HandshakeStreamName, data); err != nil {
					errChan <- err
					return
				}
				t.Log("[node1] Sent CompleteMessage")

			case *CompleteMessage:
				t.Logf("[node1] Received CompleteMessage (success=%v)", m.Success)
				gotComplete = true
			}

			// Receive Complete (and streams prepared) → FinalizeHandshake
			if streamsPrepared && gotComplete {
				if err := node1.FinalizeHandshake(node2.PeerID()); err != nil {
					errChan <- err
					return
				}
				t.Log("[node1] FinalizeHandshake called")
				return
			}
		}
	}()

	// Node2's message handler - identical to Node1 (true symmetry)
	go func() {
		var streamsPrepared, gotComplete bool

		for msg := range node2.Messages() {
			if msg.StreamName != streams.HandshakeStreamName {
				continue
			}

			hsMsg, err := unmarshalHandshakeMsg(msg.Data)
			if err != nil {
				t.Logf("[node2] Failed to unmarshal: %v", err)
				continue
			}

			switch m := hsMsg.(type) {
			case *HelloMessage:
				t.Logf("[node2] Received HelloMessage (version=%d)", m.Version)
				// Receive Hello → Send PubKey
				pubKeyMsg := PubKeyMessage{PublicKey: pub2}
				data, _ := marshalHandshakeMsg(pubKeyMsg)
				if err := node2.Send(node1.PeerID(), streams.HandshakeStreamName, data); err != nil {
					errChan <- err
					return
				}
				t.Log("[node2] Sent PubKeyMessage")

			case *PubKeyMessage:
				t.Log("[node2] Received PubKeyMessage")
				peerPubKey := ed25519.PublicKey(m.PublicKey)
				// Receive PubKey → PrepareStreams (ready to receive encrypted messages)
				if err := node2.PrepareStreams(node1.PeerID(), peerPubKey, []string{"messages"}); err != nil {
					errChan <- err
					return
				}
				streamsPrepared = true
				t.Log("[node2] PrepareStreams called")
				// Send Complete to signal we're ready
				completeMsg := CompleteMessage{Success: true}
				data, _ := marshalHandshakeMsg(completeMsg)
				if err := node2.Send(node1.PeerID(), streams.HandshakeStreamName, data); err != nil {
					errChan <- err
					return
				}
				t.Log("[node2] Sent CompleteMessage")

			case *CompleteMessage:
				t.Logf("[node2] Received CompleteMessage (success=%v)", m.Success)
				gotComplete = true
			}

			// Receive Complete (and streams prepared) → FinalizeHandshake
			if streamsPrepared && gotComplete {
				if err := node2.FinalizeHandshake(node1.PeerID()); err != nil {
					errChan <- err
					return
				}
				t.Log("[node2] FinalizeHandshake called")
				return
			}
		}
	}()

	// Add node2 to node1's address book and initiate connection
	// Note: AddPeer may trigger auto-connect, so Connect() may return "already connecting"
	node1.AddPeer(node2.PeerID(), node2.Addrs(), nil)

	t.Log("Node1 connecting...")
	if err := node1.Connect(node2.PeerID()); err != nil {
		// Ignore "already connected" errors - auto-connect may have started
		if !strings.Contains(err.Error(), "already connected") {
			t.Fatalf("Connect failed: %v", err)
		}
		t.Logf("Connect returned (already connecting): %v", err)
	}
	t.Log("Node1 connect initiated - waiting for StateEstablished events")

	// Wait for nodes to reach StateEstablished
	timeout := time.After(10 * time.Second)

	select {
	case <-node1Established:
	case err := <-errChan:
		t.Fatalf("Node1 error: %v", err)
	case <-timeout:
		t.Fatal("Timeout waiting for node1 StateEstablished")
	}

	select {
	case <-node2Established:
	case err := <-errChan:
		t.Fatalf("Node2 error: %v", err)
	case <-timeout:
		t.Fatal("Timeout waiting for node2 StateEstablished")
	}

	t.Log("Both nodes reached StateEstablished!")

	// Test encrypted messaging
	t.Log("Testing encrypted messaging...")

	// Node1 sends to Node2
	msg1 := []byte("Hello from Node1!")
	if err := node1.Send(node2.PeerID(), "messages", msg1); err != nil {
		t.Fatalf("Send from node1 failed: %v", err)
	}

	select {
	case msg := <-node2.Messages():
		if msg.StreamName != "messages" {
			t.Errorf("Expected stream 'messages', got %q", msg.StreamName)
		}
		if string(msg.Data) != string(msg1) {
			t.Errorf("Node2 got %q, want %q", msg.Data, msg1)
		}
		t.Logf("Node2 received: %s", msg.Data)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message on node2")
	}

	// Node2 sends to Node1
	msg2 := []byte("Reply from Node2!")
	if err := node2.Send(node1.PeerID(), "messages", msg2); err != nil {
		t.Fatalf("Send from node2 failed: %v", err)
	}

	select {
	case msg := <-node1.Messages():
		if msg.StreamName != "messages" {
			t.Errorf("Expected stream 'messages', got %q", msg.StreamName)
		}
		if string(msg.Data) != string(msg2) {
			t.Errorf("Node1 got %q, want %q", msg.Data, msg2)
		}
		t.Logf("Node1 received: %s", msg.Data)
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for message on node1")
	}

	t.Log("SUCCESS!")
}

func TestIntegration_HandshakeTimeout(t *testing.T) {
	// Register Cramberry types for polymorphic handshake messages
	registerHandshakeTypes()

	// Create two nodes with SHORT handshake timeout
	_, priv1, _ := ed25519.GenerateKey(rand.Reader)
	_, priv2, _ := ed25519.GenerateKey(rand.Reader)
	pub1 := priv1.Public().(ed25519.PublicKey)

	dir1 := filepath.Join(os.TempDir(), "glueberry-timeout-node1-"+randomSuffix())
	dir2 := filepath.Join(os.TempDir(), "glueberry-timeout-node2-"+randomSuffix())
	defer os.RemoveAll(dir1)
	defer os.RemoveAll(dir2)

	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg1 := NewConfig(priv1, filepath.Join(dir1, "ab.json"), []multiaddr.Multiaddr{listenAddr})
	cfg2 := NewConfig(priv2, filepath.Join(dir2, "ab.json"), []multiaddr.Multiaddr{listenAddr})

	// Set SHORT handshake timeout for testing
	cfg1.HandshakeTimeout = 500 * time.Millisecond
	cfg2.HandshakeTimeout = 500 * time.Millisecond

	node1, err := New(cfg1)
	if err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}
	node2, err := New(cfg2)
	if err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}

	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}
	defer node1.Stop()
	defer node2.Stop()

	t.Logf("Node1: %s", node1.PeerID())
	t.Logf("Node2: %s", node2.PeerID())

	// Channel to capture timeout/cooldown event
	timeoutDetected := make(chan struct{})
	errChan := make(chan error, 4)

	// Node1's event handler: StateConnected → Send Hello, watch for Cooldown
	go func() {
		for event := range node1.Events() {
			t.Logf("[node1] Event: %s for peer %s", event.State, event.PeerID.String()[:8])
			if event.State == StateConnected && event.PeerID == node2.PeerID() {
				// Send Hello to start handshake
				helloMsg := HelloMessage{Version: 1}
				data, _ := marshalHandshakeMsg(helloMsg)
				if err := node1.Send(node2.PeerID(), streams.HandshakeStreamName, data); err != nil {
					errChan <- err
					return
				}
				t.Log("[node1] Sent HelloMessage")
			}
			if event.State == StateCooldown && event.PeerID == node2.PeerID() {
				t.Logf("[node1] Detected cooldown (timeout): %v", event.Error)
				close(timeoutDetected)
				return
			}
		}
	}()

	// Node2's event handler: StateConnected → Send Hello, but then STALL (don't complete handshake)
	go func() {
		for event := range node2.Events() {
			t.Logf("[node2] Event: %s for peer %s", event.State, event.PeerID.String()[:8])
			if event.State == StateConnected && event.PeerID == node1.PeerID() {
				// Send Hello to start handshake
				helloMsg := HelloMessage{Version: 1}
				data, _ := marshalHandshakeMsg(helloMsg)
				if err := node2.Send(node1.PeerID(), streams.HandshakeStreamName, data); err != nil {
					errChan <- err
					return
				}
				t.Log("[node2] Sent HelloMessage")
			}
		}
	}()

	// Node1's message handler: normal handshake flow
	go func() {
		for msg := range node1.Messages() {
			if msg.StreamName != streams.HandshakeStreamName {
				continue
			}

			hsMsg, err := unmarshalHandshakeMsg(msg.Data)
			if err != nil {
				continue
			}

			switch m := hsMsg.(type) {
			case *HelloMessage:
				t.Logf("[node1] Received HelloMessage (version=%d)", m.Version)
				// Receive Hello → Send PubKey
				pubKeyMsg := PubKeyMessage{PublicKey: pub1}
				data, _ := marshalHandshakeMsg(pubKeyMsg)
				if err := node1.Send(node2.PeerID(), streams.HandshakeStreamName, data); err != nil {
					return
				}
				t.Log("[node1] Sent PubKeyMessage")

			case *PubKeyMessage:
				t.Log("[node1] Received PubKeyMessage")
				// Receive PubKey → Send Complete
				completeMsg := CompleteMessage{Success: true}
				data, _ := marshalHandshakeMsg(completeMsg)
				if err := node1.Send(node2.PeerID(), streams.HandshakeStreamName, data); err != nil {
					return
				}
				t.Log("[node1] Sent CompleteMessage")

			case *CompleteMessage:
				t.Log("[node1] Received CompleteMessage - would complete handshake")
			}
		}
	}()

	// Node2's message handler: INTENTIONALLY STALLS - never sends Complete
	go func() {
		for msg := range node2.Messages() {
			if msg.StreamName != streams.HandshakeStreamName {
				continue
			}

			hsMsg, err := unmarshalHandshakeMsg(msg.Data)
			if err != nil {
				continue
			}

			switch m := hsMsg.(type) {
			case *HelloMessage:
				t.Logf("[node2] Received HelloMessage (version=%d)", m.Version)
				// DO NOT send PubKey - intentionally stall

			case *PubKeyMessage:
				t.Log("[node2] Received PubKeyMessage - INTENTIONALLY NOT RESPONDING")
				// DO NOT send Complete - intentionally stall to trigger timeout

			case *CompleteMessage:
				t.Log("[node2] Received CompleteMessage - IGNORING")
			}
		}
	}()

	// Add node2 to node1's address book
	node1.AddPeer(node2.PeerID(), node2.Addrs(), nil)

	// Node1 initiates connection
	t.Log("Node1 connecting...")
	if err := node1.Connect(node2.PeerID()); err != nil {
		if !strings.Contains(err.Error(), "already connected") {
			t.Fatalf("Connect failed: %v", err)
		}
	}
	t.Log("Node1 connected - waiting for handshake timeout...")

	// Wait for timeout (should happen within ~500ms + some buffer)
	select {
	case <-timeoutDetected:
		t.Log("SUCCESS: Handshake timeout detected correctly!")
	case err := <-errChan:
		t.Fatalf("Error during test: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Test timeout: handshake timeout was not detected within expected time")
	}

	// Verify node1 is in cooldown state
	state1 := node1.ConnectionState(node2.PeerID())
	if state1 != StateCooldown && state1 != StateReconnecting {
		t.Errorf("Expected node1 state Cooldown or Reconnecting, got %v", state1)
	}
	t.Logf("Node1 final state: %v", state1)
}

func TestIntegration_HandshakeTimeout_SlowComplete(t *testing.T) {
	// This test verifies that if a node takes too long to send Complete,
	// the handshake times out.
	registerHandshakeTypes()

	_, priv1, _ := ed25519.GenerateKey(rand.Reader)
	_, priv2, _ := ed25519.GenerateKey(rand.Reader)
	pub1 := priv1.Public().(ed25519.PublicKey)
	pub2 := priv2.Public().(ed25519.PublicKey)

	dir1 := filepath.Join(os.TempDir(), "glueberry-slow-node1-"+randomSuffix())
	dir2 := filepath.Join(os.TempDir(), "glueberry-slow-node2-"+randomSuffix())
	defer os.RemoveAll(dir1)
	defer os.RemoveAll(dir2)

	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg1 := NewConfig(priv1, filepath.Join(dir1, "ab.json"), []multiaddr.Multiaddr{listenAddr})
	cfg2 := NewConfig(priv2, filepath.Join(dir2, "ab.json"), []multiaddr.Multiaddr{listenAddr})

	// Set SHORT handshake timeout
	cfg1.HandshakeTimeout = 300 * time.Millisecond
	cfg2.HandshakeTimeout = 300 * time.Millisecond

	node1, err := New(cfg1)
	if err != nil {
		t.Fatalf("Failed to create node1: %v", err)
	}
	node2, err := New(cfg2)
	if err != nil {
		t.Fatalf("Failed to create node2: %v", err)
	}

	if err := node1.Start(); err != nil {
		t.Fatalf("Failed to start node1: %v", err)
	}
	if err := node2.Start(); err != nil {
		t.Fatalf("Failed to start node2: %v", err)
	}
	defer node1.Stop()
	defer node2.Stop()

	t.Logf("Node1: %s", node1.PeerID())
	t.Logf("Node2: %s", node2.PeerID())

	timeoutDetected := make(chan struct{})
	errChan := make(chan error, 4)

	// Node1's event handler
	go func() {
		for event := range node1.Events() {
			t.Logf("[node1] Event: %s", event.State)
			if event.State == StateConnected && event.PeerID == node2.PeerID() {
				helloMsg := HelloMessage{Version: 1}
				data, _ := marshalHandshakeMsg(helloMsg)
				_ = node1.Send(node2.PeerID(), streams.HandshakeStreamName, data)
				t.Log("[node1] Sent HelloMessage")
			}
			if event.State == StateCooldown && event.PeerID == node2.PeerID() {
				t.Logf("[node1] Timeout detected: %v", event.Error)
				close(timeoutDetected)
				return
			}
		}
	}()

	// Node2's event handler
	go func() {
		for event := range node2.Events() {
			t.Logf("[node2] Event: %s", event.State)
			if event.State == StateConnected && event.PeerID == node1.PeerID() {
				helloMsg := HelloMessage{Version: 1}
				data, _ := marshalHandshakeMsg(helloMsg)
				_ = node2.Send(node1.PeerID(), streams.HandshakeStreamName, data)
				t.Log("[node2] Sent HelloMessage")
			}
		}
	}()

	// Node1's message handler - normal flow
	go func() {
		for msg := range node1.Messages() {
			if msg.StreamName != streams.HandshakeStreamName {
				continue
			}
			hsMsg, _ := unmarshalHandshakeMsg(msg.Data)
			switch hsMsg.(type) {
			case *HelloMessage:
				t.Log("[node1] Received Hello → Send PubKey")
				pubKeyMsg := PubKeyMessage{PublicKey: pub1}
				data, _ := marshalHandshakeMsg(pubKeyMsg)
				_ = node1.Send(node2.PeerID(), streams.HandshakeStreamName, data)
			case *PubKeyMessage:
				t.Log("[node1] Received PubKey → Send Complete")
				completeMsg := CompleteMessage{Success: true}
				data, _ := marshalHandshakeMsg(completeMsg)
				_ = node1.Send(node2.PeerID(), streams.HandshakeStreamName, data)
			}
		}
	}()

	// testDone signals when the test is complete (to avoid logging after test ends)
	testDone := make(chan struct{})

	// Node2's message handler - DELAYS sending Complete beyond timeout
	go func() {
		for msg := range node2.Messages() {
			if msg.StreamName != streams.HandshakeStreamName {
				continue
			}
			hsMsg, _ := unmarshalHandshakeMsg(msg.Data)
			switch hsMsg.(type) {
			case *HelloMessage:
				t.Log("[node2] Received Hello → Send PubKey")
				pubKeyMsg := PubKeyMessage{PublicKey: pub2}
				data, _ := marshalHandshakeMsg(pubKeyMsg)
				_ = node2.Send(node1.PeerID(), streams.HandshakeStreamName, data)
			case *PubKeyMessage:
				t.Log("[node2] Received PubKey → SLEEPING before sending Complete...")
				// Sleep longer than HandshakeTimeout to trigger timeout
				select {
				case <-time.After(500 * time.Millisecond):
					// Check if test is done before logging
					select {
					case <-testDone:
						return
					default:
						t.Log("[node2] Woke up - trying to send Complete (too late)")
					}
				case <-testDone:
					return
				}
				completeMsg := CompleteMessage{Success: true}
				data, _ := marshalHandshakeMsg(completeMsg)
				_ = node2.Send(node1.PeerID(), streams.HandshakeStreamName, data)
			}
		}
	}()

	node1.AddPeer(node2.PeerID(), node2.Addrs(), nil)

	t.Log("Node1 connecting...")
	if err := node1.Connect(node2.PeerID()); err != nil {
		if !strings.Contains(err.Error(), "already connected") {
			t.Fatalf("Connect failed: %v", err)
		}
	}

	select {
	case <-timeoutDetected:
		t.Log("SUCCESS: Handshake timeout detected when Complete was delayed!")
	case err := <-errChan:
		t.Fatalf("Error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Test timeout: handshake timeout was not detected")
	}

	close(testDone)
}

func randomSuffix() string {
	buf := make([]byte, 8)
	rand.Read(buf)
	return fmt.Sprintf("%x", buf)
}
