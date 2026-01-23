package addressbook

import (
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// Book manages the peer address book with persistence and thread-safe operations.
type Book struct {
	storage *storage
	peers   map[string]*PeerEntry
	mu      sync.RWMutex
}

// New creates a new address book with the given file path for persistence.
// If the file exists, it loads the existing data. Otherwise, starts with an empty book.
func New(path string) (*Book, error) {
	s := newStorage(path)

	data, err := s.load()
	if err != nil {
		return nil, fmt.Errorf("failed to load address book: %w", err)
	}

	return &Book{
		storage: s,
		peers:   data.Peers,
	}, nil
}

// AddPeer adds or updates a peer in the address book.
// If the peer already exists, it updates the multiaddrs and metadata.
// Returns an error if the peer is blacklisted.
func (b *Book) AddPeer(peerID peer.ID, addrs []multiaddr.Multiaddr, metadata map[string]string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := peerID.String()
	now := time.Now()

	if existing, ok := b.peers[key]; ok {
		if existing.Blacklisted {
			return fmt.Errorf("cannot update blacklisted peer %s", peerID)
		}
		// Update existing entry
		existing.Multiaddrs = addrs
		if metadata != nil {
			existing.Metadata = metadata
		}
		existing.UpdatedAt = now
	} else {
		// Create new entry
		b.peers[key] = &PeerEntry{
			PeerID:     peerID,
			Multiaddrs: addrs,
			Metadata:   metadata,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	}

	return b.saveLocked()
}

// RemovePeer removes a peer from the address book.
// Returns an error if the peer doesn't exist.
func (b *Book) RemovePeer(peerID peer.ID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := peerID.String()
	if _, ok := b.peers[key]; !ok {
		return fmt.Errorf("peer %s not found", peerID)
	}

	delete(b.peers, key)
	return b.saveLocked()
}

// GetPeer retrieves a peer entry by ID.
// Returns a copy of the entry to prevent external modification.
// Returns an error if the peer doesn't exist.
func (b *Book) GetPeer(peerID peer.ID) (*PeerEntry, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	key := peerID.String()
	entry, ok := b.peers[key]
	if !ok {
		return nil, fmt.Errorf("peer %s not found", peerID)
	}

	return entry.Clone(), nil
}

// ListPeers returns all non-blacklisted peers.
// Returns copies of the entries to prevent external modification.
func (b *Book) ListPeers() []*PeerEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]*PeerEntry, 0, len(b.peers))
	for _, entry := range b.peers {
		if !entry.Blacklisted {
			result = append(result, entry.Clone())
		}
	}
	return result
}

// ListAllPeers returns all peers including blacklisted ones.
// Returns copies of the entries to prevent external modification.
func (b *Book) ListAllPeers() []*PeerEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]*PeerEntry, 0, len(b.peers))
	for _, entry := range b.peers {
		result = append(result, entry.Clone())
	}
	return result
}

// BlacklistPeer marks a peer as blacklisted.
// Returns an error if the peer doesn't exist.
func (b *Book) BlacklistPeer(peerID peer.ID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := peerID.String()
	entry, ok := b.peers[key]
	if !ok {
		return fmt.Errorf("peer %s not found", peerID)
	}

	entry.Blacklisted = true
	entry.UpdatedAt = time.Now()
	return b.saveLocked()
}

// UnblacklistPeer removes the blacklist flag from a peer.
// Returns an error if the peer doesn't exist.
func (b *Book) UnblacklistPeer(peerID peer.ID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := peerID.String()
	entry, ok := b.peers[key]
	if !ok {
		return fmt.Errorf("peer %s not found", peerID)
	}

	entry.Blacklisted = false
	entry.UpdatedAt = time.Now()
	return b.saveLocked()
}

// IsBlacklisted checks if a peer is blacklisted.
// Returns false if the peer doesn't exist.
func (b *Book) IsBlacklisted(peerID peer.ID) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	key := peerID.String()
	entry, ok := b.peers[key]
	if !ok {
		return false
	}
	return entry.Blacklisted
}

// UpdatePublicKey stores the peer's public key after successful handshake.
// Returns an error if the peer doesn't exist.
func (b *Book) UpdatePublicKey(peerID peer.ID, pubKey []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := peerID.String()
	entry, ok := b.peers[key]
	if !ok {
		return fmt.Errorf("peer %s not found", peerID)
	}

	// Make a copy of the public key
	entry.PublicKey = make([]byte, len(pubKey))
	copy(entry.PublicKey, pubKey)
	entry.UpdatedAt = time.Now()
	return b.saveLocked()
}

// UpdateLastSeen updates the last seen timestamp for a peer.
// Returns an error if the peer doesn't exist.
func (b *Book) UpdateLastSeen(peerID peer.ID) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := peerID.String()
	entry, ok := b.peers[key]
	if !ok {
		return fmt.Errorf("peer %s not found", peerID)
	}

	now := time.Now()
	entry.LastSeen = now
	entry.UpdatedAt = now
	return b.saveLocked()
}

// UpdateMetadata updates the metadata for a peer.
// This merges with existing metadata; to remove keys, set them to empty string.
// Returns an error if the peer doesn't exist.
func (b *Book) UpdateMetadata(peerID peer.ID, metadata map[string]string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := peerID.String()
	entry, ok := b.peers[key]
	if !ok {
		return fmt.Errorf("peer %s not found", peerID)
	}

	if entry.Metadata == nil {
		entry.Metadata = make(map[string]string)
	}

	for k, v := range metadata {
		if v == "" {
			delete(entry.Metadata, k)
		} else {
			entry.Metadata[k] = v
		}
	}

	entry.UpdatedAt = time.Now()
	return b.saveLocked()
}

// HasPeer checks if a peer exists in the address book.
func (b *Book) HasPeer(peerID peer.ID) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	key := peerID.String()
	_, ok := b.peers[key]
	return ok
}

// Count returns the total number of peers (including blacklisted).
func (b *Book) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.peers)
}

// CountActive returns the number of non-blacklisted peers.
func (b *Book) CountActive() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	count := 0
	for _, entry := range b.peers {
		if !entry.Blacklisted {
			count++
		}
	}
	return count
}

// Clear removes all peers from the address book.
func (b *Book) Clear() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.peers = make(map[string]*PeerEntry)
	return b.saveLocked()
}

// saveLocked saves the address book to disk.
// Must be called with the write lock held.
func (b *Book) saveLocked() error {
	data := &addressBookData{
		Version: currentVersion,
		Peers:   b.peers,
	}
	return b.storage.save(data)
}

// Reload reloads the address book from disk, discarding in-memory changes.
func (b *Book) Reload() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	data, err := b.storage.load()
	if err != nil {
		return fmt.Errorf("failed to reload address book: %w", err)
	}

	b.peers = data.Peers
	return nil
}
