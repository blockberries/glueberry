package glueberry

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

func generateNodeTestKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return priv
}

func tempAddressBookPath(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(os.TempDir(), "glueberry-test-"+time.Now().Format("20060102150405"))
	path := filepath.Join(dir, "addressbook.json")
	t.Cleanup(func() { os.RemoveAll(dir) })
	return path
}

func TestNew(t *testing.T) {
	priv := generateNodeTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	abPath := tempAddressBookPath(t)

	cfg := NewConfig(
		priv,
		abPath,
		[]multiaddr.Multiaddr{listenAddr},
	)

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	if node == nil {
		t.Fatal("New returned nil")
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	// Missing private key
	cfg := &Config{
		AddressBookPath: "/tmp/test.json",
	}

	_, err := New(cfg)
	if err == nil {
		t.Error("New should fail with invalid config")
	}
}

func TestNode_StartStop(t *testing.T) {
	priv := generateNodeTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	abPath := tempAddressBookPath(t)

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Start
	err = node.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Double start should fail
	err = node.Start()
	if err != ErrNodeAlreadyStarted {
		t.Error("second Start should return ErrNodeAlreadyStarted")
	}

	// Stop
	err = node.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Stop without start should fail
	err = node.Stop()
	if err != ErrNodeNotStarted {
		t.Error("Stop after Stop should return ErrNodeNotStarted")
	}
}

func TestNode_PeerID(t *testing.T) {
	priv := generateNodeTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	abPath := tempAddressBookPath(t)

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	peerID := node.PeerID()
	if peerID == "" {
		t.Error("PeerID should not be empty")
	}
}

func TestNode_PublicKey(t *testing.T) {
	priv := generateNodeTestKey(t)
	expectedPub := priv.Public().(ed25519.PublicKey)

	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	abPath := tempAddressBookPath(t)

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	pubKey := node.PublicKey()
	if string(pubKey) != string(expectedPub) {
		t.Error("PublicKey doesn't match expected")
	}
}

func TestNode_Addrs(t *testing.T) {
	priv := generateNodeTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	abPath := tempAddressBookPath(t)

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	addrs := node.Addrs()
	if len(addrs) == 0 {
		t.Error("Addrs should not be empty")
	}
}

func TestNode_AddressBookMethods(t *testing.T) {
	priv := generateNodeTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	abPath := tempAddressBookPath(t)

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer node.Stop()

	// Generate a peer ID
	peerPriv := generateNodeTestKey(t)
	peerPubKey := peerPriv.Public().(ed25519.PublicKey)
	peerID, _ := peer.IDFromPublicKey(ed25519ToCryptoKey(peerPubKey))

	// AddPeer
	peerAddr, _ := multiaddr.NewMultiaddr("/ip4/192.168.1.1/tcp/9000")
	err = node.AddPeer(peerID, []multiaddr.Multiaddr{peerAddr}, map[string]string{"role": "validator"})
	if err != nil {
		t.Fatalf("AddPeer failed: %v", err)
	}

	// GetPeer
	entry, err := node.GetPeer(peerID)
	if err != nil {
		t.Fatalf("GetPeer failed: %v", err)
	}
	if entry.Metadata["role"] != "validator" {
		t.Error("metadata not preserved")
	}

	// ListPeers
	peers := node.ListPeers()
	if len(peers) != 1 {
		t.Errorf("ListPeers returned %d peers, want 1", len(peers))
	}

	// BlacklistPeer
	err = node.BlacklistPeer(peerID)
	if err != nil {
		t.Fatalf("BlacklistPeer failed: %v", err)
	}

	// ListPeers should now be empty (blacklisted filtered)
	peers = node.ListPeers()
	if len(peers) != 0 {
		t.Errorf("ListPeers returned %d peers after blacklist, want 0", len(peers))
	}

	// UnblacklistPeer
	err = node.UnblacklistPeer(peerID)
	if err != nil {
		t.Fatalf("UnblacklistPeer failed: %v", err)
	}

	// RemovePeer
	err = node.RemovePeer(peerID)
	if err != nil {
		t.Fatalf("RemovePeer failed: %v", err)
	}
}

func TestNode_ConnectBeforeStart(t *testing.T) {
	priv := generateNodeTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	abPath := tempAddressBookPath(t)

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Generate peer ID
	peerPriv := generateNodeTestKey(t)
	peerPubKey := peerPriv.Public().(ed25519.PublicKey)
	peerID, _ := peer.IDFromPublicKey(ed25519ToCryptoKey(peerPubKey))

	// Connect before Start should fail
	_, err = node.Connect(peerID)
	if err != ErrNodeNotStarted {
		t.Errorf("Connect before Start should return ErrNodeNotStarted, got %v", err)
	}
}

func TestNode_Messages(t *testing.T) {
	priv := generateNodeTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	abPath := tempAddressBookPath(t)

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, _ := New(cfg)
	defer node.Stop()

	msgChan := node.Messages()
	if msgChan == nil {
		t.Error("Messages() should return non-nil channel")
	}
}

func TestNode_Events(t *testing.T) {
	priv := generateNodeTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	abPath := tempAddressBookPath(t)

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, _ := New(cfg)
	defer node.Stop()

	eventChan := node.Events()
	if eventChan == nil {
		t.Error("Events() should return non-nil channel")
	}
}

func TestNode_IncomingHandshakes(t *testing.T) {
	priv := generateNodeTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	abPath := tempAddressBookPath(t)

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, _ := New(cfg)
	defer node.Stop()

	handshakesChan := node.IncomingHandshakes()
	if handshakesChan == nil {
		t.Error("IncomingHandshakes() should return non-nil channel")
	}
}

// Helper to convert Ed25519 public key to libp2p crypto key
func ed25519ToCryptoKey(pub ed25519.PublicKey) libp2pcrypto.PubKey {
	key, _ := libp2pcrypto.UnmarshalEd25519PublicKey(pub)
	return key
}
