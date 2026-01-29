package protocol

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// BlacklistChecker is an interface for checking if a peer is blacklisted.
type BlacklistChecker interface {
	IsBlacklisted(peerID peer.ID) bool
}

// ConnectionStateChecker is an interface for checking if we're already
// attempting to connect to a peer. Used for connection deduplication.
type ConnectionStateChecker interface {
	// IsConnecting returns true if we're currently attempting to connect
	// to the peer (StateConnecting).
	IsConnecting(peerID peer.ID) bool
}

// ConnectionGater implements libp2p's ConnectionGater interface to enforce
// blacklisting and connection deduplication at the connection level.
type ConnectionGater struct {
	checker      BlacklistChecker
	stateChecker ConnectionStateChecker
	mu           sync.RWMutex
}

// NewConnectionGater creates a new connection gater with the given blacklist checker.
func NewConnectionGater(checker BlacklistChecker) *ConnectionGater {
	return &ConnectionGater{checker: checker}
}

// SetStateChecker sets the connection state checker for deduplication.
// This is set after construction to avoid circular dependencies.
func (g *ConnectionGater) SetStateChecker(checker ConnectionStateChecker) {
	g.mu.Lock()
	g.stateChecker = checker
	g.mu.Unlock()
}

// InterceptPeerDial is called before dialing a peer.
// Returns false to block the connection if the peer is blacklisted.
func (g *ConnectionGater) InterceptPeerDial(p peer.ID) bool {
	return !g.checker.IsBlacklisted(p)
}

// InterceptAddrDial is called before dialing a specific address.
// We allow all addresses if the peer wasn't blocked by InterceptPeerDial.
func (g *ConnectionGater) InterceptAddrDial(p peer.ID, addr multiaddr.Multiaddr) bool {
	return !g.checker.IsBlacklisted(p)
}

// InterceptAccept is called when accepting an inbound connection.
// At this point we don't know the peer ID yet, so we allow it.
func (g *ConnectionGater) InterceptAccept(addrs network.ConnMultiaddrs) bool {
	return true
}

// InterceptSecured is called after the security handshake completes
// and the peer ID is known. Returns false to reject blacklisted peers
// or to deduplicate connections (reject incoming if we're already connecting outbound).
func (g *ConnectionGater) InterceptSecured(dir network.Direction, p peer.ID, addrs network.ConnMultiaddrs) bool {
	// Always reject blacklisted peers
	if g.checker.IsBlacklisted(p) {
		return false
	}

	// Connection deduplication: if we receive an incoming connection from a peer
	// we're already trying to connect to, reject it. First to complete wins.
	if dir == network.DirInbound {
		g.mu.RLock()
		stateChecker := g.stateChecker
		g.mu.RUnlock()
		if stateChecker != nil && stateChecker.IsConnecting(p) {
			// We're already dialing this peer - reject incoming, our outbound will complete first
			return false
		}
	}

	return true
}

// InterceptUpgraded is called after the connection is fully upgraded.
// We allow all upgraded connections that made it this far.
func (g *ConnectionGater) InterceptUpgraded(conn network.Conn) (bool, control.DisconnectReason) {
	// Final check on the peer ID
	if g.checker.IsBlacklisted(conn.RemotePeer()) {
		return false, control.DisconnectReason(0)
	}
	return true, 0
}

// Ensure ConnectionGater implements the interface
var _ connmgr.ConnectionGater = (*ConnectionGater)(nil)
