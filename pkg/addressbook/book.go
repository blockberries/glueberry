package addressbook

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

const (
	// flushInterval is how often the address book flushes dirty changes to disk.
	flushInterval = 5 * time.Second
)

// Book manages the peer address book with persistence and thread-safe operations.
// Changes are batched and periodically flushed to disk to reduce I/O overhead.
// Critical changes (add, remove, blacklist) are saved immediately.
// Non-critical changes (LastSeen updates) are batched and flushed periodically.
type Book struct {
	storage *storage
	peers   map[string]*PeerEntry
	mu      sync.RWMutex

	// dirty indicates there are unsaved changes (from batched operations)
	dirty bool

	// ctx and cancel control the background flush goroutine
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new address book with the given file path for persistence.
// If the file exists, it loads the existing data. Otherwise, starts with an empty book.
// The returned Book must be closed with Close() to ensure all changes are persisted.
func New(path string) (*Book, error) {
	s := newStorage(path)

	data, err := s.load()
	if err != nil {
		return nil, fmt.Errorf("failed to load address book: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	b := &Book{
		storage: s,
		peers:   data.Peers,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Start background flush goroutine
	go b.flushLoop()

	return b, nil
}

// AddPeer adds or updates a peer in the address book.
// If the peer already exists, it updates the multiaddrs and metadata.
// Returns an error if the peer is blacklisted.
// The addrs slice and metadata map are copied to prevent external modification.
func (b *Book) AddPeer(peerID peer.ID, addrs []multiaddr.Multiaddr, metadata map[string]string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	key := peerID.String()
	now := time.Now()

	// Make defensive copies to prevent external modification
	addrsCopy := make([]multiaddr.Multiaddr, len(addrs))
	copy(addrsCopy, addrs)

	var metadataCopy map[string]string
	if metadata != nil {
		metadataCopy = make(map[string]string, len(metadata))
		for k, v := range metadata {
			metadataCopy[k] = v
		}
	}

	if existing, ok := b.peers[key]; ok {
		if existing.Blacklisted {
			return fmt.Errorf("cannot update blacklisted peer %s", peerID)
		}
		// Update existing entry
		existing.Multiaddrs = addrsCopy
		if metadataCopy != nil {
			existing.Metadata = metadataCopy
		}
		existing.UpdatedAt = now
	} else {
		// Create new entry
		b.peers[key] = &PeerEntry{
			PeerID:     peerID,
			Multiaddrs: addrsCopy,
			Metadata:   metadataCopy,
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
// This is a batched operation - changes are persisted periodically, not immediately.
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
	b.dirty = true // Mark dirty for periodic flush, don't save immediately
	return nil
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
	if err := b.storage.save(data); err != nil {
		return err
	}
	b.dirty = false
	return nil
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
	b.dirty = false
	return nil
}

// flushLoop runs in the background and periodically flushes dirty changes to disk.
func (b *Book) flushLoop() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.mu.Lock()
			if b.dirty {
				// Ignore error in background flush - will retry next interval
				_ = b.saveLocked()
			}
			b.mu.Unlock()
		}
	}
}

// Flush explicitly saves any pending changes to disk.
// This is useful when you want to ensure changes are persisted immediately.
func (b *Book) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.dirty {
		return nil
	}
	return b.saveLocked()
}

// Close stops the background flush goroutine and saves any pending changes.
// The Book should not be used after Close is called.
func (b *Book) Close() error {
	// Stop the background goroutine
	b.cancel()

	// Flush any pending changes
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.dirty {
		return b.saveLocked()
	}
	return nil
}
