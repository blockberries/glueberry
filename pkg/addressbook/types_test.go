package addressbook

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

func TestPeerEntry_MarshalUnmarshal(t *testing.T) {
	peerID, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	addr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/9000")
	now := time.Now().Truncate(time.Millisecond)

	original := &PeerEntry{
		PeerID:      peerID,
		Multiaddrs:  []multiaddr.Multiaddr{addr},
		PublicKey:   []byte("test-key"),
		Metadata:    map[string]string{"key": "value"},
		LastSeen:    now,
		Blacklisted: true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	var decoded PeerEntry
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify fields
	if decoded.PeerID != original.PeerID {
		t.Errorf("PeerID = %v, want %v", decoded.PeerID, original.PeerID)
	}
	if len(decoded.Multiaddrs) != 1 {
		t.Errorf("Multiaddrs length = %d, want 1", len(decoded.Multiaddrs))
	}
	if decoded.Multiaddrs[0].String() != addr.String() {
		t.Errorf("Multiaddr = %v, want %v", decoded.Multiaddrs[0], addr)
	}
	if string(decoded.PublicKey) != "test-key" {
		t.Errorf("PublicKey = %s, want test-key", decoded.PublicKey)
	}
	if decoded.Metadata["key"] != "value" {
		t.Errorf("Metadata[key] = %s, want value", decoded.Metadata["key"])
	}
	if !decoded.Blacklisted {
		t.Error("Blacklisted should be true")
	}
}

func TestPeerEntry_MarshalJSON_MultipleAddrs(t *testing.T) {
	peerID, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	addr1, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/9000")
	addr2, _ := multiaddr.NewMultiaddr("/ip4/192.168.1.1/tcp/9000")
	addr3, _ := multiaddr.NewMultiaddr("/ip6/::1/tcp/9000")

	entry := &PeerEntry{
		PeerID:     peerID,
		Multiaddrs: []multiaddr.Multiaddr{addr1, addr2, addr3},
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded PeerEntry
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Multiaddrs) != 3 {
		t.Errorf("Multiaddrs length = %d, want 3", len(decoded.Multiaddrs))
	}
}

func TestPeerEntry_UnmarshalJSON_InvalidMultiaddr(t *testing.T) {
	// JSON with invalid multiaddr - should skip it but not fail
	jsonData := `{
		"peer_id": "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
		"multiaddrs": ["/ip4/127.0.0.1/tcp/9000", "invalid-addr", "/ip4/192.168.1.1/tcp/9001"]
	}`

	var entry PeerEntry
	err := json.Unmarshal([]byte(jsonData), &entry)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Should have 2 valid addrs (invalid one skipped)
	if len(entry.Multiaddrs) != 2 {
		t.Errorf("Multiaddrs length = %d, want 2 (invalid skipped)", len(entry.Multiaddrs))
	}
}

func TestPeerEntry_Clone(t *testing.T) {
	peerID, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	addr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/9000")
	now := time.Now()

	original := &PeerEntry{
		PeerID:        peerID,
		Multiaddrs:    []multiaddr.Multiaddr{addr},
		RawMultiaddrs: []string{addr.String()},
		PublicKey:     []byte("test-key"),
		Metadata:      map[string]string{"key": "value"},
		LastSeen:      now,
		Blacklisted:   true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	clone := original.Clone()

	// Verify all fields are equal
	if clone.PeerID != original.PeerID {
		t.Error("PeerID should be equal")
	}
	if len(clone.Multiaddrs) != len(original.Multiaddrs) {
		t.Error("Multiaddrs length should be equal")
	}
	if string(clone.PublicKey) != string(original.PublicKey) {
		t.Error("PublicKey should be equal")
	}
	if clone.Metadata["key"] != original.Metadata["key"] {
		t.Error("Metadata should be equal")
	}
	if clone.Blacklisted != original.Blacklisted {
		t.Error("Blacklisted should be equal")
	}

	// Verify deep copy - modifying clone shouldn't affect original
	clone.Metadata["key"] = "modified"
	if original.Metadata["key"] == "modified" {
		t.Error("modifying clone metadata shouldn't affect original")
	}

	clone.PublicKey[0] = 'X'
	if original.PublicKey[0] == 'X' {
		t.Error("modifying clone public key shouldn't affect original")
	}
}

func TestPeerEntry_Clone_Nil(t *testing.T) {
	var entry *PeerEntry
	clone := entry.Clone()
	if clone != nil {
		t.Error("Clone of nil should return nil")
	}
}

func TestPeerEntry_Clone_EmptyFields(t *testing.T) {
	peerID, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")

	original := &PeerEntry{
		PeerID: peerID,
		// All other fields empty/nil
	}

	clone := original.Clone()

	if clone.PeerID != original.PeerID {
		t.Error("PeerID should be cloned")
	}
	if clone.Multiaddrs != nil && len(clone.Multiaddrs) != 0 {
		t.Error("empty Multiaddrs should result in nil or empty slice")
	}
	if clone.Metadata != nil && len(clone.Metadata) != 0 {
		t.Error("empty Metadata should result in nil or empty map")
	}
}
