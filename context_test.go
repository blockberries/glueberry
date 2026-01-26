package glueberry

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
)

// createTestConfigForContext creates a valid test configuration for context tests.
func createTestConfigForContext(t *testing.T) *Config {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	abPath := filepath.Join(t.TempDir(), "addressbook.json")
	listenAddr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	if err != nil {
		t.Fatalf("failed to create multiaddr: %v", err)
	}

	return NewConfig(priv, abPath, []multiaddr.Multiaddr{listenAddr})
}

// TestConnectCtx_ContextCancelled tests that ConnectCtx returns context.Canceled
// when the context is cancelled before the operation starts.
func TestConnectCtx_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cfg := createTestConfigForContext(t)
	node, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		t.Fatalf("failed to start node: %v", err)
	}

	// Try to connect with cancelled context
	err = node.ConnectCtx(ctx, "12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// TestSendCtx_ContextCancelled tests that SendCtx returns context.Canceled
// when the context is cancelled before the operation starts.
func TestSendCtx_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cfg := createTestConfigForContext(t)
	node, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		t.Fatalf("failed to start node: %v", err)
	}

	// Try to send with cancelled context
	err = node.SendCtx(ctx, "somePeer", "stream", []byte("data"))
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// TestDisconnectCtx_ContextCancelled tests that DisconnectCtx returns context.Canceled
// when the context is cancelled before the operation starts.
func TestDisconnectCtx_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	cfg := createTestConfigForContext(t)
	node, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		t.Fatalf("failed to start node: %v", err)
	}

	// Try to disconnect with cancelled context
	err = node.DisconnectCtx(ctx, "12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// TestConnectCtx_Timeout tests that ConnectCtx respects context deadline.
func TestConnectCtx_Timeout(t *testing.T) {
	// Create a very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	cfg := createTestConfigForContext(t)
	node, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	defer node.Stop()

	if err := node.Start(); err != nil {
		t.Fatalf("failed to start node: %v", err)
	}

	// Sleep to let timeout expire
	time.Sleep(5 * time.Millisecond)

	// Try to connect with expired context
	err = node.ConnectCtx(ctx, "12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN")
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got: %v", err)
	}
}

// TestNodeNotStarted_Ctx tests that context methods return ErrNodeNotStarted
// when the node is not started.
func TestNodeNotStarted_Ctx(t *testing.T) {
	cfg := createTestConfigForContext(t)
	node, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	defer node.Stop()

	// Don't start the node

	ctx := context.Background()

	// ConnectCtx should return ErrNodeNotStarted
	err = node.ConnectCtx(ctx, "12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN")
	if err != ErrNodeNotStarted {
		t.Errorf("ConnectCtx: expected ErrNodeNotStarted, got: %v", err)
	}

	// SendCtx should return ErrNodeNotStarted
	err = node.SendCtx(ctx, "somePeer", "stream", []byte("data"))
	if err != ErrNodeNotStarted {
		t.Errorf("SendCtx: expected ErrNodeNotStarted, got: %v", err)
	}
}

// TestConnect_WrapsConnectCtx tests that Connect calls ConnectCtx with background context.
func TestConnect_WrapsConnectCtx(t *testing.T) {
	cfg := createTestConfigForContext(t)
	node, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	defer node.Stop()

	// Don't start the node - this ensures we get a predictable error

	// Connect should return ErrNodeNotStarted (same as ConnectCtx with background context)
	err = node.Connect("12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN")
	if err != ErrNodeNotStarted {
		t.Errorf("Connect: expected ErrNodeNotStarted, got: %v", err)
	}
}

// TestSend_WrapsSendCtx tests that Send calls SendCtx with background context.
func TestSend_WrapsSendCtx(t *testing.T) {
	cfg := createTestConfigForContext(t)
	node, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create node: %v", err)
	}
	defer node.Stop()

	// Don't start the node - this ensures we get a predictable error

	// Send should return ErrNodeNotStarted (same as SendCtx with background context)
	err = node.Send("somePeer", "stream", []byte("data"))
	if err != ErrNodeNotStarted {
		t.Errorf("Send: expected ErrNodeNotStarted, got: %v", err)
	}
}

// Ensure the import is used
var _ = os.Remove
