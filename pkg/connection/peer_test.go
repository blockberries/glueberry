package connection

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func mustParsePeerID(t *testing.T, s string) peer.ID {
	t.Helper()
	id, err := peer.Decode(s)
	if err != nil {
		t.Fatalf("failed to parse peer ID: %v", err)
	}
	return id
}

const testPeerID = "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N"

func TestNewPeerConnection(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)

	conn := NewPeerConnection(peerID)

	if conn.PeerID != peerID {
		t.Errorf("PeerID = %v, want %v", conn.PeerID, peerID)
	}
	if conn.State != StateDisconnected {
		t.Errorf("State = %v, want StateDisconnected", conn.State)
	}
	if conn.LastStateChange.IsZero() {
		t.Error("LastStateChange should be set")
	}
	if conn.EncryptedStreams == nil {
		t.Error("EncryptedStreams should be initialized")
	}
}

func TestPeerConnection_TransitionTo(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	// Valid transition
	err := conn.TransitionTo(StateConnecting)
	if err != nil {
		t.Errorf("TransitionTo(StateConnecting) failed: %v", err)
	}
	if conn.State != StateConnecting {
		t.Errorf("State = %v, want StateConnecting", conn.State)
	}

	// Invalid transition
	err = conn.TransitionTo(StateEstablished)
	if err == nil {
		t.Error("TransitionTo(StateEstablished) should fail from StateConnecting")
	}
}

func TestPeerConnection_GetState(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	conn.State = StateConnecting
	if conn.GetState() != StateConnecting {
		t.Error("GetState should return current state")
	}
}

func TestPeerConnection_SetSharedKey(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	key := []byte("test-shared-key-32-bytes-long!!!")

	conn.SetSharedKey(key)

	retrieved := conn.GetSharedKey()
	if string(retrieved) != string(key) {
		t.Error("GetSharedKey should return same key")
	}

	// Verify it's a copy
	key[0] = 'X'
	retrieved2 := conn.GetSharedKey()
	if retrieved2[0] == 'X' {
		t.Error("SetSharedKey should store a copy")
	}
}

func TestPeerConnection_GetSharedKey_Nil(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	key := conn.GetSharedKey()
	if key != nil {
		t.Error("GetSharedKey should return nil when no key set")
	}
}

func TestPeerConnection_SetError(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	testErr := fmt.Errorf("test error")
	conn.SetError(testErr)

	retrieved := conn.GetError()
	if retrieved != testErr {
		t.Error("GetError should return set error")
	}
}

func TestPeerConnection_StartCooldown(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	// Must be in a state that can transition to cooldown (StateConnected)
	conn.State = StateConnected

	duration := 1 * time.Minute
	err := conn.StartCooldown(duration)
	if err != nil {
		t.Fatalf("StartCooldown failed: %v", err)
	}

	if conn.State != StateCooldown {
		t.Errorf("State = %v, want StateCooldown", conn.State)
	}

	if !conn.IsInCooldown() {
		t.Error("IsInCooldown should be true")
	}

	remaining := conn.CooldownRemaining()
	if remaining <= 0 || remaining > duration {
		t.Errorf("CooldownRemaining = %v, want between 0 and %v", remaining, duration)
	}
}

func TestPeerConnection_IsInCooldown_Expired(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	conn.State = StateConnected
	conn.StartCooldown(10 * time.Millisecond)

	// Wait for cooldown to expire
	time.Sleep(20 * time.Millisecond)

	if conn.IsInCooldown() {
		t.Error("IsInCooldown should be false after expiration")
	}

	if conn.CooldownRemaining() != 0 {
		t.Errorf("CooldownRemaining should be 0 after expiration, got %v", conn.CooldownRemaining())
	}
}

func TestPeerConnection_IsInCooldown_NotInCooldownState(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	// In Disconnected state
	if conn.IsInCooldown() {
		t.Error("IsInCooldown should be false when not in cooldown state")
	}

	// Even if we set CooldownUntil (shouldn't happen in practice)
	conn.CooldownUntil = time.Now().Add(1 * time.Hour)
	if conn.IsInCooldown() {
		t.Error("IsInCooldown should check state first")
	}
}

func TestPeerConnection_Cleanup(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	// Set up various resources
	conn.SharedKey = []byte("key")
	conn.EncryptedStreams["test"] = struct{}{}
	_, cancel := context.WithCancel(context.Background())
	conn.ReconnectState = NewReconnectState(cancel)

	conn.Cleanup()

	// Verify cleanup
	if conn.SharedKey != nil {
		t.Error("SharedKey should be nil after cleanup")
	}
	if len(conn.EncryptedStreams) != 0 {
		t.Error("EncryptedStreams should be empty after cleanup")
	}
	if conn.ReconnectState != nil {
		t.Error("ReconnectState should be nil after cleanup")
	}
}

func TestPeerConnection_ConcurrentAccess(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	done := make(chan bool)

	// Goroutine 1: GetState
	go func() {
		for i := 0; i < 100; i++ {
			conn.GetState()
		}
		done <- true
	}()

	// Goroutine 2: Set/Get SharedKey
	go func() {
		for i := 0; i < 100; i++ {
			conn.SetSharedKey([]byte("test"))
			conn.GetSharedKey()
		}
		done <- true
	}()

	// Goroutine 3: Set/Get Error
	go func() {
		for i := 0; i < 100; i++ {
			conn.SetError(fmt.Errorf("test"))
			conn.GetError()
		}
		done <- true
	}()

	// Wait for all
	<-done
	<-done
	<-done
}

func TestNewPeerConnection_IsOutbound_DefaultFalse(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	// Default should be false (not outbound)
	if conn.IsOutbound {
		t.Error("new PeerConnection should have IsOutbound = false by default")
	}
	if conn.GetIsOutbound() {
		t.Error("GetIsOutbound should return false for new connection")
	}
}

func TestPeerConnection_SetIsOutbound(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	// Set to true (outbound)
	conn.SetIsOutbound(true)
	if !conn.GetIsOutbound() {
		t.Error("GetIsOutbound should return true after SetIsOutbound(true)")
	}

	// Set to false (inbound)
	conn.SetIsOutbound(false)
	if conn.GetIsOutbound() {
		t.Error("GetIsOutbound should return false after SetIsOutbound(false)")
	}
}

func TestPeerConnection_IsOutbound_ConcurrentAccess(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	done := make(chan bool)

	// Goroutine 1: Set true
	go func() {
		for i := 0; i < 100; i++ {
			conn.SetIsOutbound(true)
		}
		done <- true
	}()

	// Goroutine 2: Set false
	go func() {
		for i := 0; i < 100; i++ {
			conn.SetIsOutbound(false)
		}
		done <- true
	}()

	// Goroutine 3: Get
	go func() {
		for i := 0; i < 100; i++ {
			conn.GetIsOutbound()
		}
		done <- true
	}()

	// Wait for all - test should complete without race detector issues
	<-done
	<-done
	<-done
}
