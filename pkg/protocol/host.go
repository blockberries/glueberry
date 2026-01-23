package protocol

import (
	"context"
	"crypto/ed25519"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	"github.com/multiformats/go-multiaddr"
)

// HostConfig contains configuration for creating a libp2p host.
type HostConfig struct {
	// PrivateKey is the Ed25519 private key for the host identity.
	PrivateKey ed25519.PrivateKey

	// ListenAddrs are the multiaddresses to listen on.
	ListenAddrs []multiaddr.Multiaddr

	// ConnectionGater for enforcing blacklist.
	Gater *ConnectionGater

	// ConnMgrLowWater is the low watermark for the connection manager.
	// Connections will be trimmed when above high watermark.
	ConnMgrLowWater int

	// ConnMgrHighWater is the high watermark for the connection manager.
	ConnMgrHighWater int
}

// DefaultHostConfig returns a HostConfig with sensible defaults.
func DefaultHostConfig() HostConfig {
	return HostConfig{
		ConnMgrLowWater:  100,
		ConnMgrHighWater: 400,
	}
}

// Host wraps a libp2p host and provides Glueberry-specific functionality.
type Host struct {
	host   host.Host
	config HostConfig
}

// NewHost creates a new libp2p host with the given configuration.
func NewHost(ctx context.Context, cfg HostConfig) (*Host, error) {
	// Convert Ed25519 key to libp2p format
	libp2pPriv, err := crypto.UnmarshalEd25519PrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to convert private key: %w", err)
	}

	// Convert multiaddrs to strings for libp2p options
	listenAddrs := make([]string, len(cfg.ListenAddrs))
	for i, ma := range cfg.ListenAddrs {
		listenAddrs[i] = ma.String()
	}

	// Create connection manager
	connMgr, err := connmgr.NewConnManager(
		cfg.ConnMgrLowWater,
		cfg.ConnMgrHighWater,
		connmgr.WithGracePeriod(0),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection manager: %w", err)
	}

	// Build libp2p options
	opts := []libp2p.Option{
		libp2p.Identity(libp2pPriv),
		libp2p.ListenAddrStrings(listenAddrs...),
		libp2p.ConnectionManager(connMgr),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelay(),
		libp2p.NATPortMap(),
	}

	// Add connection gater if provided
	if cfg.Gater != nil {
		opts = append(opts, libp2p.ConnectionGater(cfg.Gater))
	}

	// Create the host
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create libp2p host: %w", err)
	}

	return &Host{
		host:   h,
		config: cfg,
	}, nil
}

// ID returns the peer ID of this host.
func (h *Host) ID() peer.ID {
	return h.host.ID()
}

// Addrs returns the addresses this host is listening on.
func (h *Host) Addrs() []multiaddr.Multiaddr {
	return h.host.Addrs()
}

// AddrInfo returns the peer.AddrInfo for this host.
func (h *Host) AddrInfo() peer.AddrInfo {
	return peer.AddrInfo{
		ID:    h.host.ID(),
		Addrs: h.host.Addrs(),
	}
}

// Connect establishes a connection to a peer.
func (h *Host) Connect(ctx context.Context, pi peer.AddrInfo) error {
	// Add addresses to peerstore
	h.host.Peerstore().AddAddrs(pi.ID, pi.Addrs, peerstore.PermanentAddrTTL)

	// Connect
	if err := h.host.Connect(ctx, pi); err != nil {
		return fmt.Errorf("failed to connect to peer %s: %w", pi.ID, err)
	}

	return nil
}

// Disconnect closes the connection to a peer.
func (h *Host) Disconnect(peerID peer.ID) error {
	return h.host.Network().ClosePeer(peerID)
}

// IsConnected checks if there is an active connection to a peer.
func (h *Host) IsConnected(peerID peer.ID) bool {
	return h.host.Network().Connectedness(peerID) == network.Connected
}

// Connectedness returns the connection status with a peer.
func (h *Host) Connectedness(peerID peer.ID) network.Connectedness {
	return h.host.Network().Connectedness(peerID)
}

// SetStreamHandler registers a handler for a protocol.
func (h *Host) SetStreamHandler(protoID string, handler network.StreamHandler) {
	h.host.SetStreamHandler(HandshakeProtocolID, handler)
}

// SetStreamHandlerForProtocol registers a handler for a specific protocol ID.
func (h *Host) SetStreamHandlerForProtocol(proto string, handler network.StreamHandler) {
	h.host.SetStreamHandler(StreamProtocolID(proto), handler)
}

// NewStream opens a new stream to a peer for the given protocol.
func (h *Host) NewStream(ctx context.Context, peerID peer.ID, protoID string) (network.Stream, error) {
	return h.host.NewStream(ctx, peerID, StreamProtocolID(protoID))
}

// NewHandshakeStream opens a new handshake stream to a peer.
func (h *Host) NewHandshakeStream(ctx context.Context, peerID peer.ID) (network.Stream, error) {
	return h.host.NewStream(ctx, peerID, HandshakeProtocolID)
}

// RemoveStreamHandler removes the handler for a protocol.
func (h *Host) RemoveStreamHandler(protoID string) {
	h.host.RemoveStreamHandler(StreamProtocolID(protoID))
}

// Network returns the underlying network.
func (h *Host) Network() network.Network {
	return h.host.Network()
}

// Peerstore returns the peerstore.
func (h *Host) Peerstore() peerstore.Peerstore {
	return h.host.Peerstore()
}

// LibP2PHost returns the underlying libp2p host.
// Use with caution - prefer the wrapped methods when possible.
func (h *Host) LibP2PHost() host.Host {
	return h.host
}

// Close shuts down the host.
func (h *Host) Close() error {
	return h.host.Close()
}
