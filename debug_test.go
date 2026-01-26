package glueberry

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
)

func createDebugTestNode(t *testing.T) *Node {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	dir := filepath.Join(os.TempDir(), "glueberry-debug-test-"+time.Now().Format("20060102150405"))
	path := filepath.Join(dir, "addressbook.json")
	t.Cleanup(func() { os.RemoveAll(dir) })

	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := NewConfig(priv, path, []multiaddr.Multiaddr{listenAddr})

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}

	if err := node.Start(); err != nil {
		t.Fatalf("failed to start node: %v", err)
	}

	t.Cleanup(func() { node.Stop() })

	return node
}

func TestDumpState(t *testing.T) {
	node := createDebugTestNode(t)

	state := node.DumpState()

	if state.PeerID == "" {
		t.Error("expected non-empty peer ID")
	}
	if state.PublicKey == "" {
		t.Error("expected non-empty public key")
	}
	if state.CapturedAt.IsZero() {
		t.Error("expected non-zero captured time")
	}
}

func TestDumpStateJSON(t *testing.T) {
	node := createDebugTestNode(t)

	jsonStr, err := node.DumpStateJSON()
	if err != nil {
		t.Fatalf("DumpStateJSON() failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Errorf("invalid JSON: %v", err)
	}

	// Check required fields exist
	if _, ok := parsed["peer_id"]; !ok {
		t.Error("missing peer_id field")
	}
	if _, ok := parsed["version"]; !ok {
		t.Error("missing version field")
	}
	if _, ok := parsed["address_book"]; !ok {
		t.Error("missing address_book field")
	}
}

func TestDumpStateString(t *testing.T) {
	node := createDebugTestNode(t)

	str := node.DumpStateString()

	// Check for expected sections
	if !strings.Contains(str, "Glueberry Node Debug State") {
		t.Error("missing header")
	}
	if !strings.Contains(str, "IDENTITY:") {
		t.Error("missing IDENTITY section")
	}
	if !strings.Contains(str, "LISTEN ADDRESSES:") {
		t.Error("missing LISTEN ADDRESSES section")
	}
	if !strings.Contains(str, "ADDRESS BOOK:") {
		t.Error("missing ADDRESS BOOK section")
	}
	if !strings.Contains(str, "CONFIGURATION:") {
		t.Error("missing CONFIGURATION section")
	}
	if !strings.Contains(str, "STATISTICS:") {
		t.Error("missing STATISTICS section")
	}
}

func TestConnectionSummary(t *testing.T) {
	node := createDebugTestNode(t)

	summary := node.ConnectionSummary()

	// Should have "known" key
	if _, ok := summary["known"]; !ok {
		t.Error("expected 'known' key in summary")
	}
}

func TestListKnownPeers(t *testing.T) {
	node := createDebugTestNode(t)

	peers := node.ListKnownPeers()

	// Should be empty initially
	if len(peers) != 0 {
		t.Errorf("expected no known peers, got %d", len(peers))
	}
}

func TestDebugState_AddressBook(t *testing.T) {
	node := createDebugTestNode(t)

	state := node.DumpState()

	// Should have 0 peers initially
	if state.AddressBook.TotalPeers != 0 {
		t.Errorf("expected 0 total peers, got %d", state.AddressBook.TotalPeers)
	}
	if state.AddressBook.ActivePeers != 0 {
		t.Errorf("expected 0 active peers, got %d", state.AddressBook.ActivePeers)
	}
	if state.AddressBook.BlacklistedPeers != 0 {
		t.Errorf("expected 0 blacklisted peers, got %d", state.AddressBook.BlacklistedPeers)
	}
}

func TestDebugState_Config(t *testing.T) {
	node := createDebugTestNode(t)

	state := node.DumpState()

	// Check config values are populated
	if state.Config.HandshakeTimeout == "" {
		t.Error("expected non-empty handshake timeout")
	}
	if state.Config.MaxReconnectDelay == "" {
		t.Error("expected non-empty max reconnect delay")
	}
	if state.Config.HighWatermark == 0 {
		t.Error("expected non-zero high watermark")
	}
	if state.Config.LowWatermark == 0 {
		t.Error("expected non-zero low watermark")
	}
	if state.Config.MaxMessageSize == 0 {
		t.Error("expected non-zero max message size")
	}
}

func TestDebugState_Version(t *testing.T) {
	node := createDebugTestNode(t)

	state := node.DumpState()

	// Version should be non-empty string
	if state.Version == "" {
		t.Error("expected non-empty version")
	}
}

func TestPeerInfo_NotFound(t *testing.T) {
	node := createDebugTestNode(t)

	// Try to get info for a non-existent peer
	_, err := node.PeerInfo("nonexistent-peer")
	if err == nil {
		t.Error("expected error for non-existent peer")
	}
}
