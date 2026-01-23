package protocol

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
)

func generateTestKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return priv
}

func TestNewHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	priv := generateTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := HostConfig{
		PrivateKey:       priv,
		ListenAddrs:      []multiaddr.Multiaddr{listenAddr},
		ConnMgrLowWater:  10,
		ConnMgrHighWater: 20,
	}

	host, err := NewHost(ctx, cfg)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}
	defer host.Close()

	// Verify host was created
	if host.ID() == "" {
		t.Error("host should have a peer ID")
	}

	// Verify it's listening on addresses
	addrs := host.Addrs()
	if len(addrs) == 0 {
		t.Error("host should have listen addresses")
	}
}

func TestNewHost_WithGater(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	priv := generateTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	checker := newMockChecker()
	gater := NewConnectionGater(checker)

	cfg := HostConfig{
		PrivateKey:       priv,
		ListenAddrs:      []multiaddr.Multiaddr{listenAddr},
		Gater:            gater,
		ConnMgrLowWater:  10,
		ConnMgrHighWater: 20,
	}

	host, err := NewHost(ctx, cfg)
	if err != nil {
		t.Fatalf("NewHost with gater failed: %v", err)
	}
	defer host.Close()

	if host.ID() == "" {
		t.Error("host should have a peer ID")
	}
}

func TestHost_AddrInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	priv := generateTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := HostConfig{
		PrivateKey:       priv,
		ListenAddrs:      []multiaddr.Multiaddr{listenAddr},
		ConnMgrLowWater:  10,
		ConnMgrHighWater: 20,
	}

	host, err := NewHost(ctx, cfg)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}
	defer host.Close()

	info := host.AddrInfo()
	if info.ID != host.ID() {
		t.Error("AddrInfo.ID should match host.ID()")
	}
	if len(info.Addrs) == 0 {
		t.Error("AddrInfo.Addrs should not be empty")
	}
}

func TestHost_ConnectDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create two hosts
	priv1 := generateTestKey(t)
	priv2 := generateTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg1 := HostConfig{
		PrivateKey:       priv1,
		ListenAddrs:      []multiaddr.Multiaddr{listenAddr},
		ConnMgrLowWater:  10,
		ConnMgrHighWater: 20,
	}

	cfg2 := HostConfig{
		PrivateKey:       priv2,
		ListenAddrs:      []multiaddr.Multiaddr{listenAddr},
		ConnMgrLowWater:  10,
		ConnMgrHighWater: 20,
	}

	host1, err := NewHost(ctx, cfg1)
	if err != nil {
		t.Fatalf("NewHost 1 failed: %v", err)
	}
	defer host1.Close()

	host2, err := NewHost(ctx, cfg2)
	if err != nil {
		t.Fatalf("NewHost 2 failed: %v", err)
	}
	defer host2.Close()

	// Initially not connected
	if host1.IsConnected(host2.ID()) {
		t.Error("hosts should not be connected initially")
	}

	// Connect host1 to host2
	err = host1.Connect(ctx, host2.AddrInfo())
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Give some time for connection to establish
	time.Sleep(100 * time.Millisecond)

	// Should now be connected
	if !host1.IsConnected(host2.ID()) {
		t.Error("host1 should be connected to host2")
	}
	if !host2.IsConnected(host1.ID()) {
		t.Error("host2 should be connected to host1")
	}

	// Disconnect
	err = host1.Disconnect(host2.ID())
	if err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	// Give some time for disconnection
	time.Sleep(100 * time.Millisecond)

	// Should no longer be connected
	if host1.IsConnected(host2.ID()) {
		t.Error("host1 should not be connected to host2 after disconnect")
	}
}

func TestHost_Connectedness(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	priv := generateTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := HostConfig{
		PrivateKey:       priv,
		ListenAddrs:      []multiaddr.Multiaddr{listenAddr},
		ConnMgrLowWater:  10,
		ConnMgrHighWater: 20,
	}

	host, err := NewHost(ctx, cfg)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}
	defer host.Close()

	// Check connectedness with unknown peer
	unknownPeer, _ := mustParsePeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"), error(nil)
	connectedness := host.Connectedness(unknownPeer)
	if connectedness.String() != "NotConnected" {
		t.Errorf("Connectedness with unknown peer should be NotConnected, got %v", connectedness)
	}
}

func TestDefaultHostConfig(t *testing.T) {
	cfg := DefaultHostConfig()

	if cfg.ConnMgrLowWater != 100 {
		t.Errorf("ConnMgrLowWater = %d, want 100", cfg.ConnMgrLowWater)
	}
	if cfg.ConnMgrHighWater != 400 {
		t.Errorf("ConnMgrHighWater = %d, want 400", cfg.ConnMgrHighWater)
	}
}

func TestHost_Close(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	priv := generateTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := HostConfig{
		PrivateKey:       priv,
		ListenAddrs:      []multiaddr.Multiaddr{listenAddr},
		ConnMgrLowWater:  10,
		ConnMgrHighWater: 20,
	}

	host, err := NewHost(ctx, cfg)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}

	// Close should not error
	err = host.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestHost_LibP2PHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	priv := generateTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := HostConfig{
		PrivateKey:       priv,
		ListenAddrs:      []multiaddr.Multiaddr{listenAddr},
		ConnMgrLowWater:  10,
		ConnMgrHighWater: 20,
	}

	host, err := NewHost(ctx, cfg)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}
	defer host.Close()

	// Should return the underlying host
	libp2pHost := host.LibP2PHost()
	if libp2pHost == nil {
		t.Error("LibP2PHost should not be nil")
	}
	if libp2pHost.ID() != host.ID() {
		t.Error("LibP2PHost ID should match")
	}
}

func TestHost_Network(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	priv := generateTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := HostConfig{
		PrivateKey:       priv,
		ListenAddrs:      []multiaddr.Multiaddr{listenAddr},
		ConnMgrLowWater:  10,
		ConnMgrHighWater: 20,
	}

	host, err := NewHost(ctx, cfg)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}
	defer host.Close()

	network := host.Network()
	if network == nil {
		t.Error("Network should not be nil")
	}
}

func TestHost_Peerstore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	priv := generateTestKey(t)
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

	cfg := HostConfig{
		PrivateKey:       priv,
		ListenAddrs:      []multiaddr.Multiaddr{listenAddr},
		ConnMgrLowWater:  10,
		ConnMgrHighWater: 20,
	}

	host, err := NewHost(ctx, cfg)
	if err != nil {
		t.Fatalf("NewHost failed: %v", err)
	}
	defer host.Close()

	peerstore := host.Peerstore()
	if peerstore == nil {
		t.Error("Peerstore should not be nil")
	}
}
