package protocol

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// mockBlacklistChecker is a mock implementation of BlacklistChecker for testing.
type mockBlacklistChecker struct {
	blacklisted map[peer.ID]bool
}

func newMockChecker() *mockBlacklistChecker {
	return &mockBlacklistChecker{
		blacklisted: make(map[peer.ID]bool),
	}
}

func (m *mockBlacklistChecker) IsBlacklisted(peerID peer.ID) bool {
	return m.blacklisted[peerID]
}

func (m *mockBlacklistChecker) Blacklist(peerID peer.ID) {
	m.blacklisted[peerID] = true
}

func (m *mockBlacklistChecker) Unblacklist(peerID peer.ID) {
	delete(m.blacklisted, peerID)
}

// mockConnMultiaddrs implements network.ConnMultiaddrs for testing.
type mockConnMultiaddrs struct {
	local  multiaddr.Multiaddr
	remote multiaddr.Multiaddr
}

func (m *mockConnMultiaddrs) LocalMultiaddr() multiaddr.Multiaddr  { return m.local }
func (m *mockConnMultiaddrs) RemoteMultiaddr() multiaddr.Multiaddr { return m.remote }

// mockConn implements network.Conn for testing.
type mockConn struct {
	network.Conn
	remotePeer peer.ID
}

func (m *mockConn) RemotePeer() peer.ID { return m.remotePeer }

func mustParsePeerID(t *testing.T, s string) peer.ID {
	t.Helper()
	id, err := peer.Decode(s)
	if err != nil {
		t.Fatalf("failed to parse peer ID: %v", err)
	}
	return id
}

func mustParseMultiaddr(t *testing.T, s string) multiaddr.Multiaddr {
	t.Helper()
	ma, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		t.Fatalf("failed to parse multiaddr: %v", err)
	}
	return ma
}

func TestConnectionGater_InterceptPeerDial(t *testing.T) {
	checker := newMockChecker()
	gater := NewConnectionGater(checker)

	peerID := mustParsePeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")

	// Non-blacklisted peer should be allowed
	if !gater.InterceptPeerDial(peerID) {
		t.Error("InterceptPeerDial should allow non-blacklisted peer")
	}

	// Blacklist the peer
	checker.Blacklist(peerID)

	// Blacklisted peer should be blocked
	if gater.InterceptPeerDial(peerID) {
		t.Error("InterceptPeerDial should block blacklisted peer")
	}

	// Unblacklist
	checker.Unblacklist(peerID)

	// Should be allowed again
	if !gater.InterceptPeerDial(peerID) {
		t.Error("InterceptPeerDial should allow unblacklisted peer")
	}
}

func TestConnectionGater_InterceptAddrDial(t *testing.T) {
	checker := newMockChecker()
	gater := NewConnectionGater(checker)

	peerID := mustParsePeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	// Non-blacklisted peer should be allowed
	if !gater.InterceptAddrDial(peerID, addr) {
		t.Error("InterceptAddrDial should allow non-blacklisted peer")
	}

	// Blacklist the peer
	checker.Blacklist(peerID)

	// Blacklisted peer should be blocked
	if gater.InterceptAddrDial(peerID, addr) {
		t.Error("InterceptAddrDial should block blacklisted peer")
	}
}

func TestConnectionGater_InterceptAccept(t *testing.T) {
	checker := newMockChecker()
	gater := NewConnectionGater(checker)

	local := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")
	remote := mustParseMultiaddr(t, "/ip4/192.168.1.1/tcp/9001")
	addrs := &mockConnMultiaddrs{local: local, remote: remote}

	// InterceptAccept should always allow (we don't know peer ID yet)
	if !gater.InterceptAccept(addrs) {
		t.Error("InterceptAccept should always allow (peer ID not yet known)")
	}
}

func TestConnectionGater_InterceptSecured(t *testing.T) {
	checker := newMockChecker()
	gater := NewConnectionGater(checker)

	peerID := mustParsePeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	local := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")
	remote := mustParseMultiaddr(t, "/ip4/192.168.1.1/tcp/9001")
	addrs := &mockConnMultiaddrs{local: local, remote: remote}

	// Non-blacklisted peer should be allowed (inbound)
	if !gater.InterceptSecured(network.DirInbound, peerID, addrs) {
		t.Error("InterceptSecured should allow non-blacklisted peer (inbound)")
	}

	// Non-blacklisted peer should be allowed (outbound)
	if !gater.InterceptSecured(network.DirOutbound, peerID, addrs) {
		t.Error("InterceptSecured should allow non-blacklisted peer (outbound)")
	}

	// Blacklist the peer
	checker.Blacklist(peerID)

	// Blacklisted peer should be blocked (inbound)
	if gater.InterceptSecured(network.DirInbound, peerID, addrs) {
		t.Error("InterceptSecured should block blacklisted peer (inbound)")
	}

	// Blacklisted peer should be blocked (outbound)
	if gater.InterceptSecured(network.DirOutbound, peerID, addrs) {
		t.Error("InterceptSecured should block blacklisted peer (outbound)")
	}
}

func TestConnectionGater_InterceptUpgraded(t *testing.T) {
	checker := newMockChecker()
	gater := NewConnectionGater(checker)

	peerID := mustParsePeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	conn := &mockConn{remotePeer: peerID}

	// Non-blacklisted peer should be allowed
	allow, _ := gater.InterceptUpgraded(conn)
	if !allow {
		t.Error("InterceptUpgraded should allow non-blacklisted peer")
	}

	// Blacklist the peer
	checker.Blacklist(peerID)

	// Blacklisted peer should be blocked
	allow, _ = gater.InterceptUpgraded(conn)
	if allow {
		t.Error("InterceptUpgraded should block blacklisted peer")
	}
}

func TestNewConnectionGater(t *testing.T) {
	checker := newMockChecker()
	gater := NewConnectionGater(checker)

	if gater == nil {
		t.Fatal("NewConnectionGater should return non-nil gater")
	}
	if gater.checker != checker {
		t.Error("NewConnectionGater should set checker")
	}
}

// mockStateChecker is a mock implementation of ConnectionStateChecker for testing.
type mockStateChecker struct {
	connecting map[peer.ID]bool
}

func newMockStateChecker() *mockStateChecker {
	return &mockStateChecker{
		connecting: make(map[peer.ID]bool),
	}
}

func (m *mockStateChecker) IsConnecting(peerID peer.ID) bool {
	return m.connecting[peerID]
}

func (m *mockStateChecker) SetConnecting(peerID peer.ID, connecting bool) {
	m.connecting[peerID] = connecting
}

func TestConnectionGater_SetStateChecker(t *testing.T) {
	checker := newMockChecker()
	gater := NewConnectionGater(checker)

	stateChecker := newMockStateChecker()
	gater.SetStateChecker(stateChecker)

	if gater.stateChecker != stateChecker {
		t.Error("SetStateChecker should set the state checker")
	}
}

func TestConnectionGater_InterceptSecured_Deduplication(t *testing.T) {
	checker := newMockChecker()
	gater := NewConnectionGater(checker)
	stateChecker := newMockStateChecker()
	gater.SetStateChecker(stateChecker)

	peerID := mustParsePeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	local := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")
	remote := mustParseMultiaddr(t, "/ip4/192.168.1.1/tcp/9001")
	addrs := &mockConnMultiaddrs{local: local, remote: remote}

	// Test 1: Inbound allowed when not connecting
	if !gater.InterceptSecured(network.DirInbound, peerID, addrs) {
		t.Error("InterceptSecured should allow inbound when not connecting")
	}

	// Test 2: Outbound always allowed (even if connecting)
	stateChecker.SetConnecting(peerID, true)
	if !gater.InterceptSecured(network.DirOutbound, peerID, addrs) {
		t.Error("InterceptSecured should allow outbound (deduplication doesn't apply)")
	}

	// Test 3: Inbound blocked when already connecting (deduplication)
	if gater.InterceptSecured(network.DirInbound, peerID, addrs) {
		t.Error("InterceptSecured should block inbound when already connecting (deduplication)")
	}

	// Test 4: Inbound allowed again when no longer connecting
	stateChecker.SetConnecting(peerID, false)
	if !gater.InterceptSecured(network.DirInbound, peerID, addrs) {
		t.Error("InterceptSecured should allow inbound when no longer connecting")
	}
}

func TestConnectionGater_InterceptSecured_DeduplicationWithoutStateChecker(t *testing.T) {
	checker := newMockChecker()
	gater := NewConnectionGater(checker)
	// Don't set state checker

	peerID := mustParsePeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	local := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")
	remote := mustParseMultiaddr(t, "/ip4/192.168.1.1/tcp/9001")
	addrs := &mockConnMultiaddrs{local: local, remote: remote}

	// Without state checker, all inbound should be allowed (deduplication disabled)
	if !gater.InterceptSecured(network.DirInbound, peerID, addrs) {
		t.Error("InterceptSecured should allow inbound when no state checker")
	}
}
