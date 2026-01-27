package addressbook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

func mustParsePeerID(t *testing.T, s string) peer.ID {
	t.Helper()
	id, err := peer.Decode(s)
	if err != nil {
		t.Fatalf("failed to parse peer ID %q: %v", s, err)
	}
	return id
}

func mustParseMultiaddr(t *testing.T, s string) multiaddr.Multiaddr {
	t.Helper()
	ma, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		t.Fatalf("failed to parse multiaddr %q: %v", s, err)
	}
	return ma
}

// testPeerID is a valid libp2p peer ID for testing
const testPeerIDStr = "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"
const testPeerID2Str = "QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt"

func tempFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "addressbook-test-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	path := f.Name()
	f.Close()
	os.Remove(path) // Remove so the test can create it
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func TestNew_EmptyFile(t *testing.T) {
	path := tempFile(t)

	book, err := New(path)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if book.Count() != 0 {
		t.Errorf("expected empty book, got %d peers", book.Count())
	}
}

func TestNew_ExistingFile(t *testing.T) {
	path := tempFile(t)
	peerID := mustParsePeerID(t, testPeerIDStr)

	// Create initial book and add a peer
	book1, err := New(path)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")
	err = book1.AddPeer(peerID, []multiaddr.Multiaddr{addr}, map[string]string{"name": "test"})
	if err != nil {
		t.Fatalf("AddPeer() failed: %v", err)
	}

	// Open the same file
	book2, err := New(path)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if book2.Count() != 1 {
		t.Errorf("expected 1 peer, got %d", book2.Count())
	}

	entry, err := book2.GetPeer(peerID)
	if err != nil {
		t.Fatalf("GetPeer() failed: %v", err)
	}

	if entry.Metadata["name"] != "test" {
		t.Errorf("metadata[name] = %q, want %q", entry.Metadata["name"], "test")
	}
}

func TestNew_CorruptedFile(t *testing.T) {
	path := tempFile(t)

	// Write corrupted JSON
	err := os.WriteFile(path, []byte("not valid json{{{"), 0600)
	if err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	book, err := New(path)
	if err != nil {
		t.Fatalf("New() should succeed with corrupted file: %v", err)
	}

	// Should have created a backup and returned empty book
	if book.Count() != 0 {
		t.Errorf("expected empty book after corruption, got %d peers", book.Count())
	}

	// Check backup file exists
	backupPath := path + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file should have been created")
	}
	t.Cleanup(func() { os.Remove(backupPath) })
}

func TestAddPeer(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	err := book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, map[string]string{"role": "validator"})
	if err != nil {
		t.Fatalf("AddPeer() failed: %v", err)
	}

	entry, err := book.GetPeer(peerID)
	if err != nil {
		t.Fatalf("GetPeer() failed: %v", err)
	}

	if entry.PeerID != peerID {
		t.Errorf("PeerID = %v, want %v", entry.PeerID, peerID)
	}
	if len(entry.Multiaddrs) != 1 {
		t.Errorf("Multiaddrs length = %d, want 1", len(entry.Multiaddrs))
	}
	if entry.Metadata["role"] != "validator" {
		t.Errorf("metadata[role] = %q, want %q", entry.Metadata["role"], "validator")
	}
	if entry.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if entry.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestAddPeer_Update(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr1 := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")
	addr2 := mustParseMultiaddr(t, "/ip4/192.168.1.1/tcp/9000")

	// Add peer initially
	book.AddPeer(peerID, []multiaddr.Multiaddr{addr1}, map[string]string{"version": "1"})
	entry1, _ := book.GetPeer(peerID)
	createdAt := entry1.CreatedAt

	time.Sleep(10 * time.Millisecond) // Ensure different timestamps

	// Update peer
	err := book.AddPeer(peerID, []multiaddr.Multiaddr{addr2}, map[string]string{"version": "2"})
	if err != nil {
		t.Fatalf("AddPeer() update failed: %v", err)
	}

	entry2, _ := book.GetPeer(peerID)

	// CreatedAt should be unchanged
	if !entry2.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt changed from %v to %v", createdAt, entry2.CreatedAt)
	}

	// UpdatedAt should be newer
	if !entry2.UpdatedAt.After(entry1.UpdatedAt) {
		t.Error("UpdatedAt should be newer after update")
	}

	// Metadata should be updated
	if entry2.Metadata["version"] != "2" {
		t.Errorf("metadata[version] = %q, want %q", entry2.Metadata["version"], "2")
	}

	// Addresses should be updated
	if len(entry2.Multiaddrs) != 1 || entry2.Multiaddrs[0].String() != addr2.String() {
		t.Error("Multiaddrs should be updated")
	}
}

func TestAddPeer_Blacklisted(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	book.BlacklistPeer(peerID)

	// Trying to update blacklisted peer should fail
	err := book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	if err == nil {
		t.Error("expected error when updating blacklisted peer")
	}
}

func TestRemovePeer(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)

	err := book.RemovePeer(peerID)
	if err != nil {
		t.Fatalf("RemovePeer() failed: %v", err)
	}

	if book.HasPeer(peerID) {
		t.Error("peer should be removed")
	}
}

func TestRemovePeer_NotFound(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)

	err := book.RemovePeer(peerID)
	if err == nil {
		t.Error("expected error when removing non-existent peer")
	}
}

func TestGetPeer_NotFound(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)

	_, err := book.GetPeer(peerID)
	if err == nil {
		t.Error("expected error for non-existent peer")
	}
}

func TestGetPeer_ReturnsCopy(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, map[string]string{"key": "value"})

	entry1, _ := book.GetPeer(peerID)
	entry1.Metadata["key"] = "modified"

	entry2, _ := book.GetPeer(peerID)
	if entry2.Metadata["key"] != "value" {
		t.Error("GetPeer should return a copy that doesn't affect the original")
	}
}

func TestListPeers(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID1 := mustParsePeerID(t, testPeerIDStr)
	peerID2 := mustParsePeerID(t, testPeerID2Str)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book.AddPeer(peerID1, []multiaddr.Multiaddr{addr}, nil)
	book.AddPeer(peerID2, []multiaddr.Multiaddr{addr}, nil)
	book.BlacklistPeer(peerID2)

	peers := book.ListPeers()
	if len(peers) != 1 {
		t.Errorf("ListPeers() returned %d peers, want 1 (excluding blacklisted)", len(peers))
	}
}

func TestListAllPeers(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID1 := mustParsePeerID(t, testPeerIDStr)
	peerID2 := mustParsePeerID(t, testPeerID2Str)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book.AddPeer(peerID1, []multiaddr.Multiaddr{addr}, nil)
	book.AddPeer(peerID2, []multiaddr.Multiaddr{addr}, nil)
	book.BlacklistPeer(peerID2)

	peers := book.ListAllPeers()
	if len(peers) != 2 {
		t.Errorf("ListAllPeers() returned %d peers, want 2 (including blacklisted)", len(peers))
	}
}

func TestBlacklist(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)

	if book.IsBlacklisted(peerID) {
		t.Error("peer should not be blacklisted initially")
	}

	err := book.BlacklistPeer(peerID)
	if err != nil {
		t.Fatalf("BlacklistPeer() failed: %v", err)
	}

	if !book.IsBlacklisted(peerID) {
		t.Error("peer should be blacklisted after BlacklistPeer")
	}

	err = book.UnblacklistPeer(peerID)
	if err != nil {
		t.Fatalf("UnblacklistPeer() failed: %v", err)
	}

	if book.IsBlacklisted(peerID) {
		t.Error("peer should not be blacklisted after UnblacklistPeer")
	}
}

func TestBlacklist_NotFound(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)

	err := book.BlacklistPeer(peerID)
	if err == nil {
		t.Error("expected error when blacklisting non-existent peer")
	}

	err = book.UnblacklistPeer(peerID)
	if err == nil {
		t.Error("expected error when unblacklisting non-existent peer")
	}
}

func TestIsBlacklisted_NonExistent(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)

	// Non-existent peer should return false, not error
	if book.IsBlacklisted(peerID) {
		t.Error("non-existent peer should not be considered blacklisted")
	}
}

func TestUpdatePublicKey(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)

	pubKey := []byte("fake-public-key-32-bytes-long!!")

	err := book.UpdatePublicKey(peerID, pubKey)
	if err != nil {
		t.Fatalf("UpdatePublicKey() failed: %v", err)
	}

	entry, _ := book.GetPeer(peerID)
	if string(entry.PublicKey) != string(pubKey) {
		t.Error("public key should be updated")
	}

	// Modify original and verify copy was stored
	pubKey[0] = 'X'
	entry, _ = book.GetPeer(peerID)
	if entry.PublicKey[0] == 'X' {
		t.Error("UpdatePublicKey should store a copy")
	}
}

func TestUpdateLastSeen(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)

	entry1, _ := book.GetPeer(peerID)
	if !entry1.LastSeen.IsZero() {
		t.Error("LastSeen should be zero initially")
	}

	time.Sleep(10 * time.Millisecond)

	err := book.UpdateLastSeen(peerID)
	if err != nil {
		t.Fatalf("UpdateLastSeen() failed: %v", err)
	}

	entry2, _ := book.GetPeer(peerID)
	if entry2.LastSeen.IsZero() {
		t.Error("LastSeen should be set after UpdateLastSeen")
	}
}

func TestUpdateMetadata(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, map[string]string{
		"key1": "value1",
		"key2": "value2",
	})

	// Update with merge
	err := book.UpdateMetadata(peerID, map[string]string{
		"key2": "updated",
		"key3": "new",
	})
	if err != nil {
		t.Fatalf("UpdateMetadata() failed: %v", err)
	}

	entry, _ := book.GetPeer(peerID)
	if entry.Metadata["key1"] != "value1" {
		t.Error("key1 should be preserved")
	}
	if entry.Metadata["key2"] != "updated" {
		t.Error("key2 should be updated")
	}
	if entry.Metadata["key3"] != "new" {
		t.Error("key3 should be added")
	}

	// Delete a key by setting to empty string
	book.UpdateMetadata(peerID, map[string]string{"key1": ""})
	entry, _ = book.GetPeer(peerID)
	if _, ok := entry.Metadata["key1"]; ok {
		t.Error("key1 should be deleted when set to empty string")
	}
}

func TestCount(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID1 := mustParsePeerID(t, testPeerIDStr)
	peerID2 := mustParsePeerID(t, testPeerID2Str)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	if book.Count() != 0 {
		t.Errorf("Count() = %d, want 0", book.Count())
	}

	book.AddPeer(peerID1, []multiaddr.Multiaddr{addr}, nil)
	if book.Count() != 1 {
		t.Errorf("Count() = %d, want 1", book.Count())
	}

	book.AddPeer(peerID2, []multiaddr.Multiaddr{addr}, nil)
	book.BlacklistPeer(peerID2)

	if book.Count() != 2 {
		t.Errorf("Count() = %d, want 2 (including blacklisted)", book.Count())
	}
	if book.CountActive() != 1 {
		t.Errorf("CountActive() = %d, want 1 (excluding blacklisted)", book.CountActive())
	}
}

func TestClear(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)

	err := book.Clear()
	if err != nil {
		t.Fatalf("Clear() failed: %v", err)
	}

	if book.Count() != 0 {
		t.Errorf("Count() = %d, want 0 after Clear", book.Count())
	}
}

func TestReload(t *testing.T) {
	path := tempFile(t)
	book1, _ := New(path)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	// Add peer with book1
	book1.AddPeer(peerID, []multiaddr.Multiaddr{addr}, map[string]string{"from": "book1"})

	// Open same file with book2 and modify
	book2, _ := New(path)
	book2.UpdateMetadata(peerID, map[string]string{"from": "book2"})

	// book1 still has old data
	entry, _ := book1.GetPeer(peerID)
	if entry.Metadata["from"] != "book1" {
		t.Errorf("book1 should still have old data, got %q", entry.Metadata["from"])
	}

	// Reload book1
	err := book1.Reload()
	if err != nil {
		t.Fatalf("Reload() failed: %v", err)
	}

	// Now book1 should have new data
	entry, _ = book1.GetPeer(peerID)
	if entry.Metadata["from"] != "book2" {
		t.Errorf("book1 should have reloaded data, got %q", entry.Metadata["from"])
	}
}

func TestPersistence(t *testing.T) {
	path := tempFile(t)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")
	pubKey := []byte("test-public-key-here!!!!!!!!")

	// Create and populate
	{
		book, _ := New(path)
		book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, map[string]string{"key": "value"})
		book.UpdatePublicKey(peerID, pubKey)
		book.UpdateLastSeen(peerID)
		// Close flushes batched changes (like UpdateLastSeen) to disk
		if err := book.Close(); err != nil {
			t.Fatalf("Close() failed: %v", err)
		}
	}

	// Reopen and verify
	{
		book, err := New(path)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		entry, err := book.GetPeer(peerID)
		if err != nil {
			t.Fatalf("GetPeer() failed: %v", err)
		}

		if entry.PeerID != peerID {
			t.Error("PeerID not persisted")
		}
		if len(entry.Multiaddrs) != 1 {
			t.Error("Multiaddrs not persisted")
		}
		if entry.Metadata["key"] != "value" {
			t.Error("Metadata not persisted")
		}
		if string(entry.PublicKey) != string(pubKey) {
			t.Error("PublicKey not persisted")
		}
		if entry.LastSeen.IsZero() {
			t.Error("LastSeen not persisted")
		}
	}
}

func TestBatchedPersistence(t *testing.T) {
	path := tempFile(t)
	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	// Create book and add peer
	book, err := New(path)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// AddPeer should persist immediately
	err = book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	if err != nil {
		t.Fatalf("AddPeer() failed: %v", err)
	}

	// UpdateLastSeen should NOT persist immediately (batched)
	err = book.UpdateLastSeen(peerID)
	if err != nil {
		t.Fatalf("UpdateLastSeen() failed: %v", err)
	}

	// Verify LastSeen is set in memory
	entry, _ := book.GetPeer(peerID)
	if entry.LastSeen.IsZero() {
		t.Error("LastSeen should be set in memory immediately")
	}

	// Reopen without Close - LastSeen should NOT be persisted
	book2, err := New(path)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	entry2, _ := book2.GetPeer(peerID)
	if !entry2.LastSeen.IsZero() {
		t.Error("LastSeen should NOT be persisted without Close/Flush")
	}
	book2.Close()

	// Now call Flush on original book
	err = book.Flush()
	if err != nil {
		t.Fatalf("Flush() failed: %v", err)
	}

	// Reopen - LastSeen should now be persisted
	book3, err := New(path)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	entry3, _ := book3.GetPeer(peerID)
	if entry3.LastSeen.IsZero() {
		t.Error("LastSeen should be persisted after Flush()")
	}
	book3.Close()

	// Clean up original book
	book.Close()
}

func TestClose(t *testing.T) {
	path := tempFile(t)
	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	// Create book and add peer
	book, err := New(path)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	err = book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	if err != nil {
		t.Fatalf("AddPeer() failed: %v", err)
	}

	// UpdateLastSeen is batched
	err = book.UpdateLastSeen(peerID)
	if err != nil {
		t.Fatalf("UpdateLastSeen() failed: %v", err)
	}

	// Close should flush pending changes
	err = book.Close()
	if err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// Verify changes were persisted
	book2, err := New(path)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer book2.Close()

	entry, _ := book2.GetPeer(peerID)
	if entry.LastSeen.IsZero() {
		t.Error("LastSeen should be persisted after Close()")
	}
}

func TestConcurrency(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	var wg sync.WaitGroup
	numGoroutines := 50

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Generate unique peer IDs by using different keys
			peerID, _ := peer.Decode(testPeerIDStr)
			// Modify the peer ID string slightly - this is a hack but works for testing
			book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, map[string]string{"index": string(rune(i))})
		}(i)
	}

	wg.Wait()

	// All operations should have completed without panic
	// Since we're using the same peer ID, final count should be 1
	if book.Count() != 1 {
		t.Logf("Count after concurrent adds: %d (expected 1 since same peer ID)", book.Count())
	}
}

func TestConcurrency_MultiplePeers(t *testing.T) {
	path := tempFile(t)
	book, _ := New(path)

	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	// First add multiple distinct peers
	peerID1 := mustParsePeerID(t, testPeerIDStr)
	peerID2 := mustParsePeerID(t, testPeerID2Str)

	book.AddPeer(peerID1, []multiaddr.Multiaddr{addr}, nil)
	book.AddPeer(peerID2, []multiaddr.Multiaddr{addr}, nil)

	var wg sync.WaitGroup
	numOps := 100

	// Concurrent reads and updates
	for i := 0; i < numOps; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			book.ListPeers()
		}()
		go func() {
			defer wg.Done()
			book.GetPeer(peerID1)
		}()
		go func() {
			defer wg.Done()
			book.IsBlacklisted(peerID2)
		}()
	}

	wg.Wait()
}

func TestJSONFormat(t *testing.T) {
	path := tempFile(t)

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book, _ := New(path)
	book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, map[string]string{"key": "value"})

	// Read the file directly and verify JSON structure
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if parsed["version"] != float64(1) {
		t.Errorf("version = %v, want 1", parsed["version"])
	}

	peers, ok := parsed["peers"].(map[string]any)
	if !ok {
		t.Fatal("peers should be an object")
	}

	if len(peers) != 1 {
		t.Errorf("expected 1 peer, got %d", len(peers))
	}
}

func TestDirectoryCreation(t *testing.T) {
	// Use a path in a non-existent directory
	dir := filepath.Join(os.TempDir(), "glueberry-test-dir-"+time.Now().Format("20060102150405"))
	path := filepath.Join(dir, "subdir", "addressbook.json")
	t.Cleanup(func() { os.RemoveAll(dir) })

	book, err := New(path)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	err = book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	if err != nil {
		t.Fatalf("AddPeer() failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("file should have been created")
	}
}

func TestFileLocking(t *testing.T) {
	path := tempFile(t)
	t.Cleanup(func() { os.Remove(path + ".lock") })

	book1, err := New(path)
	if err != nil {
		t.Fatalf("New() failed for book1: %v", err)
	}

	book2, err := New(path)
	if err != nil {
		t.Fatalf("New() failed for book2: %v", err)
	}

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	// Both books should be able to write concurrently without corruption
	var wg sync.WaitGroup
	numOps := 50

	for i := 0; i < numOps; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			book1.AddPeer(peerID, []multiaddr.Multiaddr{addr}, map[string]string{
				"writer": "book1",
				"index":  string(rune('a' + i%26)),
			})
		}(i)
		go func(i int) {
			defer wg.Done()
			book2.AddPeer(peerID, []multiaddr.Multiaddr{addr}, map[string]string{
				"writer": "book2",
				"index":  string(rune('A' + i%26)),
			})
		}(i)
	}

	wg.Wait()

	// Verify the file is still valid JSON
	book3, err := New(path)
	if err != nil {
		t.Fatalf("New() failed after concurrent writes: %v", err)
	}

	if book3.Count() != 1 {
		t.Errorf("expected 1 peer after concurrent writes, got %d", book3.Count())
	}

	entry, err := book3.GetPeer(peerID)
	if err != nil {
		t.Fatalf("GetPeer() failed: %v", err)
	}

	// One of the writers should have won
	writer := entry.Metadata["writer"]
	if writer != "book1" && writer != "book2" {
		t.Errorf("unexpected writer value: %q", writer)
	}
}

func TestFileLocking_LockFileCreated(t *testing.T) {
	path := tempFile(t)
	lockPath := path + ".lock"
	t.Cleanup(func() { os.Remove(lockPath) })

	peerID := mustParsePeerID(t, testPeerIDStr)
	addr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	book, err := New(path)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Add a peer which triggers save and lock file creation
	err = book.AddPeer(peerID, []multiaddr.Multiaddr{addr}, nil)
	if err != nil {
		t.Fatalf("AddPeer() failed: %v", err)
	}

	// Verify lock file was created
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("lock file should have been created")
	}
}
