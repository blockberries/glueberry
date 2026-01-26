package glueberry

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// DebugState represents the complete state of a Node for debugging purposes.
type DebugState struct {
	// Node identity
	PeerID    string `json:"peer_id"`
	PublicKey string `json:"public_key"`

	// Listen addresses
	ListenAddrs []string `json:"listen_addrs"`

	// Protocol version
	Version string `json:"version"`

	// Address book summary
	AddressBook DebugAddressBook `json:"address_book"`

	// Configuration
	Config DebugConfig `json:"config"`

	// Statistics summary
	PeersWithStats int `json:"peers_with_stats"`

	// Flow control summary
	FlowControllers []DebugFlowControl `json:"flow_controllers,omitempty"`

	// Timestamp when state was captured
	CapturedAt time.Time `json:"captured_at"`
}

// DebugAddressBook represents address book state for debugging.
type DebugAddressBook struct {
	TotalPeers       int      `json:"total_peers"`
	ActivePeers      int      `json:"active_peers"`
	BlacklistedPeers int      `json:"blacklisted_peers"`
	PeerIDs          []string `json:"peer_ids,omitempty"`
}

// DebugConfig represents configuration summary for debugging.
type DebugConfig struct {
	HandshakeTimeout  string `json:"handshake_timeout"`
	MaxReconnectDelay string `json:"max_reconnect_delay"`
	HighWatermark     int    `json:"high_watermark"`
	LowWatermark      int    `json:"low_watermark"`
	MaxMessageSize    int    `json:"max_message_size"`
}

// DebugFlowControl represents flow controller state for debugging.
type DebugFlowControl struct {
	Stream    string `json:"stream"`
	Pending   int    `json:"pending"`
	IsBlocked bool   `json:"is_blocked"`
}

// DumpState captures the current state of the node for debugging.
// This is useful for troubleshooting connection issues.
func (n *Node) DumpState() *DebugState {
	state := &DebugState{
		PeerID:     n.PeerID().String(),
		PublicKey:  fmt.Sprintf("%x", n.PublicKey()[:]),
		Version:    n.Version().String(),
		CapturedAt: time.Now(),
	}

	// Listen addresses
	for _, addr := range n.Addrs() {
		state.ListenAddrs = append(state.ListenAddrs, addr.String())
	}

	// Address book
	state.AddressBook = n.dumpAddressBook()

	// Config
	state.Config = n.dumpConfig()

	// Statistics
	n.peerStatsMu.RLock()
	state.PeersWithStats = len(n.peerStats)
	n.peerStatsMu.RUnlock()

	// Flow controllers
	n.flowControllersMu.RLock()
	for name, fc := range n.flowControllers {
		state.FlowControllers = append(state.FlowControllers, DebugFlowControl{
			Stream:    name,
			Pending:   fc.Pending(),
			IsBlocked: fc.IsBlocked(),
		})
	}
	n.flowControllersMu.RUnlock()

	return state
}

// dumpAddressBook returns address book debug info.
func (n *Node) dumpAddressBook() DebugAddressBook {
	allPeers := n.addressBook.ListAllPeers()
	activePeers := n.addressBook.ListPeers()

	ab := DebugAddressBook{
		TotalPeers:       len(allPeers),
		ActivePeers:      len(activePeers),
		BlacklistedPeers: len(allPeers) - len(activePeers),
	}

	// Include peer IDs (truncated for brevity)
	for _, p := range activePeers {
		ab.PeerIDs = append(ab.PeerIDs, p.PeerID.String())
	}

	return ab
}

// dumpConfig returns configuration debug info.
func (n *Node) dumpConfig() DebugConfig {
	return DebugConfig{
		HandshakeTimeout:  n.config.HandshakeTimeout.String(),
		MaxReconnectDelay: n.config.ReconnectMaxDelay.String(),
		HighWatermark:     n.config.HighWatermark,
		LowWatermark:      n.config.LowWatermark,
		MaxMessageSize:    n.config.MaxMessageSize,
	}
}

// DumpStateJSON returns the node state as formatted JSON.
func (n *Node) DumpStateJSON() (string, error) {
	state := n.DumpState()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal state: %w", err)
	}
	return string(data), nil
}

// DumpStateString returns a human-readable string representation of the node state.
func (n *Node) DumpStateString() string {
	state := n.DumpState()
	var sb strings.Builder

	sb.WriteString("=== Glueberry Node Debug State ===\n\n")

	// Identity
	sb.WriteString("IDENTITY:\n")
	sb.WriteString(fmt.Sprintf("  Peer ID:    %s\n", state.PeerID))
	if len(state.PublicKey) >= 16 {
		sb.WriteString(fmt.Sprintf("  Public Key: %s...\n", state.PublicKey[:16]))
	}
	sb.WriteString(fmt.Sprintf("  Version:    %s\n", state.Version))
	sb.WriteString("\n")

	// Listen addresses
	sb.WriteString("LISTEN ADDRESSES:\n")
	if len(state.ListenAddrs) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, addr := range state.ListenAddrs {
			sb.WriteString(fmt.Sprintf("  - %s\n", addr))
		}
	}
	sb.WriteString("\n")

	// Address book
	sb.WriteString("ADDRESS BOOK:\n")
	sb.WriteString(fmt.Sprintf("  Total:       %d peers\n", state.AddressBook.TotalPeers))
	sb.WriteString(fmt.Sprintf("  Active:      %d peers\n", state.AddressBook.ActivePeers))
	sb.WriteString(fmt.Sprintf("  Blacklisted: %d peers\n", state.AddressBook.BlacklistedPeers))
	sb.WriteString("\n")

	// Config
	sb.WriteString("CONFIGURATION:\n")
	sb.WriteString(fmt.Sprintf("  Handshake Timeout:  %s\n", state.Config.HandshakeTimeout))
	sb.WriteString(fmt.Sprintf("  Max Reconnect:      %s\n", state.Config.MaxReconnectDelay))
	sb.WriteString(fmt.Sprintf("  High Watermark:     %d\n", state.Config.HighWatermark))
	sb.WriteString(fmt.Sprintf("  Low Watermark:      %d\n", state.Config.LowWatermark))
	sb.WriteString(fmt.Sprintf("  Max Message Size:   %d bytes\n", state.Config.MaxMessageSize))
	sb.WriteString("\n")

	// Statistics
	sb.WriteString("STATISTICS:\n")
	sb.WriteString(fmt.Sprintf("  Peers tracked: %d\n", state.PeersWithStats))

	// Flow controllers
	if len(state.FlowControllers) > 0 {
		sb.WriteString("  Flow controllers:\n")
		for _, fc := range state.FlowControllers {
			blocked := ""
			if fc.IsBlocked {
				blocked = " (BLOCKED)"
			}
			sb.WriteString(fmt.Sprintf("    %s: %d pending%s\n", fc.Stream, fc.Pending, blocked))
		}
	}
	sb.WriteString("\n")

	sb.WriteString(fmt.Sprintf("Captured at: %s\n", state.CapturedAt.Format(time.RFC3339)))
	sb.WriteString("=================================\n")

	return sb.String()
}

// ConnectionSummary returns a brief summary of connection states.
// Note: This uses the address book since we don't track connections directly.
func (n *Node) ConnectionSummary() map[string]int {
	peers := n.addressBook.ListPeers()
	return map[string]int{
		"known": len(peers),
	}
}

// ListConnectedPeers returns a list of peer IDs from the address book.
// Note: This returns known peers, not necessarily connected ones.
func (n *Node) ListKnownPeers() []peer.ID {
	peers := n.addressBook.ListPeers()
	var peerIDs []peer.ID
	for _, p := range peers {
		peerIDs = append(peerIDs, p.PeerID)
	}
	return peerIDs
}

// PeerInfo returns basic information about a peer.
func (n *Node) PeerInfo(peerID peer.ID) (map[string]any, error) {
	entry, err := n.addressBook.GetPeer(peerID)
	if err != nil {
		return nil, err
	}

	info := map[string]any{
		"peer_id":     entry.PeerID.String(),
		"blacklisted": entry.Blacklisted,
		"created_at":  entry.CreatedAt,
		"updated_at":  entry.UpdatedAt,
	}

	if !entry.LastSeen.IsZero() {
		info["last_seen"] = entry.LastSeen
	}

	if len(entry.Multiaddrs) > 0 {
		var addrs []string
		for _, ma := range entry.Multiaddrs {
			addrs = append(addrs, ma.String())
		}
		info["addresses"] = addrs
	}

	if len(entry.Metadata) > 0 {
		info["metadata"] = entry.Metadata
	}

	// Add stats if available
	stats := n.PeerStatistics(peerID)
	if stats != nil {
		info["stats"] = map[string]any{
			"messages_sent":     stats.MessagesSent,
			"messages_received": stats.MessagesReceived,
			"bytes_sent":        stats.BytesSent,
			"bytes_received":    stats.BytesReceived,
		}
	}

	return info, nil
}
