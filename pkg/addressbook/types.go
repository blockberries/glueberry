// Package addressbook provides peer storage and management for Glueberry nodes.
// It persists peer information to a JSON file and provides CRUD operations
// with blacklist support.
package addressbook

import (
	"encoding/json"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// PeerEntry represents a peer in the address book.
type PeerEntry struct {
	// PeerID is the libp2p peer identifier.
	PeerID peer.ID `json:"peer_id"`

	// Multiaddrs are the network addresses for this peer.
	Multiaddrs []multiaddr.Multiaddr `json:"-"`

	// RawMultiaddrs stores the string representation for JSON serialization.
	RawMultiaddrs []string `json:"multiaddrs"`

	// PublicKey is the Ed25519 public key, set after successful handshake.
	// May be nil if handshake hasn't completed.
	PublicKey []byte `json:"public_key,omitempty"`

	// Metadata holds application-defined key-value pairs.
	Metadata map[string]string `json:"metadata,omitempty"`

	// LastSeen is the timestamp of the last successful connection.
	LastSeen time.Time `json:"last_seen,omitempty"`

	// Blacklisted indicates if this peer is blacklisted.
	Blacklisted bool `json:"blacklisted"`

	// CreatedAt is when this entry was first created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this entry was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// MarshalJSON implements json.Marshaler for PeerEntry.
// It converts Multiaddrs to string format for JSON serialization.
func (p *PeerEntry) MarshalJSON() ([]byte, error) {
	// Convert multiaddrs to strings
	rawAddrs := make([]string, len(p.Multiaddrs))
	for i, ma := range p.Multiaddrs {
		rawAddrs[i] = ma.String()
	}

	// Create an alias to avoid infinite recursion
	type Alias PeerEntry
	return json.Marshal(&struct {
		*Alias
		RawMultiaddrs []string `json:"multiaddrs"`
	}{
		Alias:         (*Alias)(p),
		RawMultiaddrs: rawAddrs,
	})
}

// UnmarshalJSON implements json.Unmarshaler for PeerEntry.
// It converts string multiaddrs back to Multiaddr type.
func (p *PeerEntry) UnmarshalJSON(data []byte) error {
	// Create an alias to avoid infinite recursion
	type Alias PeerEntry
	aux := &struct {
		*Alias
		RawMultiaddrs []string `json:"multiaddrs"`
	}{
		Alias: (*Alias)(p),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Convert strings back to multiaddrs
	p.Multiaddrs = make([]multiaddr.Multiaddr, 0, len(aux.RawMultiaddrs))
	for _, s := range aux.RawMultiaddrs {
		ma, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			// Skip invalid multiaddrs rather than failing completely
			continue
		}
		p.Multiaddrs = append(p.Multiaddrs, ma)
	}
	p.RawMultiaddrs = aux.RawMultiaddrs

	return nil
}

// Clone creates a deep copy of the PeerEntry.
func (p *PeerEntry) Clone() *PeerEntry {
	if p == nil {
		return nil
	}

	clone := &PeerEntry{
		PeerID:      p.PeerID,
		Blacklisted: p.Blacklisted,
		LastSeen:    p.LastSeen,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}

	// Deep copy multiaddrs
	if len(p.Multiaddrs) > 0 {
		clone.Multiaddrs = make([]multiaddr.Multiaddr, len(p.Multiaddrs))
		copy(clone.Multiaddrs, p.Multiaddrs)
	}

	// Deep copy raw multiaddrs
	if len(p.RawMultiaddrs) > 0 {
		clone.RawMultiaddrs = make([]string, len(p.RawMultiaddrs))
		copy(clone.RawMultiaddrs, p.RawMultiaddrs)
	}

	// Deep copy public key
	if len(p.PublicKey) > 0 {
		clone.PublicKey = make([]byte, len(p.PublicKey))
		copy(clone.PublicKey, p.PublicKey)
	}

	// Deep copy metadata
	if len(p.Metadata) > 0 {
		clone.Metadata = make(map[string]string, len(p.Metadata))
		for k, v := range p.Metadata {
			clone.Metadata[k] = v
		}
	}

	return clone
}

// addressBookData is the internal structure for JSON serialization.
type addressBookData struct {
	Version int                   `json:"version"`
	Peers   map[string]*PeerEntry `json:"peers"`
}
