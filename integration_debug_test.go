package glueberry

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blockberries/glueberry/pkg/protocol"
	"github.com/multiformats/go-multiaddr"
)

// Debug test to understand the connection flow

func TestDebug_HandlerRegistration(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)

	dir := filepath.Join(os.TempDir(), "glueberry-debug-"+time.Now().Format("20060102150405"))
	defer os.RemoveAll(dir)

	abPath := filepath.Join(dir, "ab.json")
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})

	node, _ := New(cfg)

	// Check handler before Start
	protocols := node.host.LibP2PHost().Mux().Protocols()
	t.Logf("Protocols before Start: %v", protocols)

	// Start the node
	node.Start()

	// Check handler after Start
	protocols = node.host.LibP2PHost().Mux().Protocols()
	t.Logf("Protocols after Start: %v", protocols)

	// Check if handshake protocol is registered
	hasHandshake := false
	for _, p := range protocols {
		if p == protocol.HandshakeProtocolID {
			hasHandshake = true
			break
		}
	}

	if !hasHandshake {
		t.Error("Handshake protocol not registered")
	} else {
		t.Log("✅ Handshake protocol registered successfully")
	}

	node.Stop()
}
