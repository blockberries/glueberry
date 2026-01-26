package glueberry

import (
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestPeerStatsTracker_RecordMessageSent(t *testing.T) {
	tracker := NewPeerStatsTracker()

	// Record some messages
	tracker.RecordMessageSent("data", 100)
	tracker.RecordMessageSent("data", 200)
	tracker.RecordMessageSent("control", 50)

	peerID := peer.ID("test-peer")
	stats := tracker.Snapshot(peerID, true, true)

	if stats.MessagesSent != 3 {
		t.Errorf("MessagesSent = %d, want 3", stats.MessagesSent)
	}
	if stats.BytesSent != 350 {
		t.Errorf("BytesSent = %d, want 350", stats.BytesSent)
	}

	// Check stream stats
	dataStats := stats.StreamStats["data"]
	if dataStats == nil {
		t.Fatal("expected stream stats for 'data'")
	}
	if dataStats.MessagesSent != 2 {
		t.Errorf("data.MessagesSent = %d, want 2", dataStats.MessagesSent)
	}
	if dataStats.BytesSent != 300 {
		t.Errorf("data.BytesSent = %d, want 300", dataStats.BytesSent)
	}

	controlStats := stats.StreamStats["control"]
	if controlStats == nil {
		t.Fatal("expected stream stats for 'control'")
	}
	if controlStats.MessagesSent != 1 {
		t.Errorf("control.MessagesSent = %d, want 1", controlStats.MessagesSent)
	}
}

func TestPeerStatsTracker_RecordMessageReceived(t *testing.T) {
	tracker := NewPeerStatsTracker()

	tracker.RecordMessageReceived("data", 500)
	tracker.RecordMessageReceived("data", 300)

	peerID := peer.ID("test-peer")
	stats := tracker.Snapshot(peerID, true, false)

	if stats.MessagesReceived != 2 {
		t.Errorf("MessagesReceived = %d, want 2", stats.MessagesReceived)
	}
	if stats.BytesReceived != 800 {
		t.Errorf("BytesReceived = %d, want 800", stats.BytesReceived)
	}
	if stats.IsOutbound {
		t.Error("IsOutbound should be false")
	}
}

func TestPeerStatsTracker_RecordConnectionStartEnd(t *testing.T) {
	tracker := NewPeerStatsTracker()

	// Start connection
	tracker.RecordConnectionStart()
	time.Sleep(10 * time.Millisecond)

	// End connection
	tracker.RecordConnectionEnd()

	peerID := peer.ID("test-peer")
	stats := tracker.Snapshot(peerID, false, true)

	if stats.ConnectionCount != 1 {
		t.Errorf("ConnectionCount = %d, want 1", stats.ConnectionCount)
	}
	if stats.TotalConnectTime < 10*time.Millisecond {
		t.Errorf("TotalConnectTime = %v, expected at least 10ms", stats.TotalConnectTime)
	}
}

func TestPeerStatsTracker_RecordFailure(t *testing.T) {
	tracker := NewPeerStatsTracker()

	tracker.RecordFailure()
	tracker.RecordFailure()

	peerID := peer.ID("test-peer")
	stats := tracker.Snapshot(peerID, false, true)

	if stats.FailureCount != 2 {
		t.Errorf("FailureCount = %d, want 2", stats.FailureCount)
	}
}

func TestPeerStatsTracker_Concurrent(t *testing.T) {
	tracker := NewPeerStatsTracker()

	var wg sync.WaitGroup
	numGoroutines := 10
	messagesPerGoroutine := 100

	// Concurrent message recording
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				tracker.RecordMessageSent("test", 10)
				tracker.RecordMessageReceived("test", 20)
			}
		}()
	}

	wg.Wait()

	peerID := peer.ID("test-peer")
	stats := tracker.Snapshot(peerID, true, true)

	expectedMessages := int64(numGoroutines * messagesPerGoroutine)
	if stats.MessagesSent != expectedMessages {
		t.Errorf("MessagesSent = %d, want %d", stats.MessagesSent, expectedMessages)
	}
	if stats.MessagesReceived != expectedMessages {
		t.Errorf("MessagesReceived = %d, want %d", stats.MessagesReceived, expectedMessages)
	}
}

func TestPeerStatsTracker_LastMessageAt(t *testing.T) {
	tracker := NewPeerStatsTracker()

	before := time.Now()
	tracker.RecordMessageSent("test", 100)
	after := time.Now()

	peerID := peer.ID("test-peer")
	stats := tracker.Snapshot(peerID, true, true)

	if stats.LastMessageAt.Before(before) || stats.LastMessageAt.After(after) {
		t.Errorf("LastMessageAt = %v, expected between %v and %v", stats.LastMessageAt, before, after)
	}

	// Test stream last sent at
	streamStats := stats.StreamStats["test"]
	if streamStats == nil {
		t.Fatal("expected stream stats")
	}
	if streamStats.LastSentAt.Before(before) || streamStats.LastSentAt.After(after) {
		t.Errorf("LastSentAt = %v, expected between %v and %v", streamStats.LastSentAt, before, after)
	}
}

func TestPeerStatsTracker_TotalConnectTimeDuringConnection(t *testing.T) {
	tracker := NewPeerStatsTracker()

	// Start connection
	tracker.RecordConnectionStart()
	time.Sleep(20 * time.Millisecond)

	// Snapshot while still connected should include current session
	peerID := peer.ID("test-peer")
	stats := tracker.Snapshot(peerID, true, true)

	if stats.TotalConnectTime < 20*time.Millisecond {
		t.Errorf("TotalConnectTime while connected = %v, expected at least 20ms", stats.TotalConnectTime)
	}

	// ConnectedAt should be set
	if stats.ConnectedAt.IsZero() {
		t.Error("ConnectedAt should not be zero while connected")
	}
}

func TestPeerStats_Fields(t *testing.T) {
	peerID := peer.ID("12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN")

	stats := &PeerStats{
		PeerID:           peerID,
		Connected:        true,
		IsOutbound:       true,
		ConnectedAt:      time.Now(),
		TotalConnectTime: 5 * time.Minute,
		MessagesSent:     100,
		MessagesReceived: 200,
		BytesSent:        1024,
		BytesReceived:    2048,
		StreamStats: map[string]*StreamStats{
			"data": {
				Name:             "data",
				MessagesSent:     50,
				MessagesReceived: 100,
				BytesSent:        512,
				BytesReceived:    1024,
			},
		},
		LastMessageAt:   time.Now(),
		ConnectionCount: 3,
		FailureCount:    1,
	}

	if stats.PeerID != peerID {
		t.Errorf("PeerID = %v, want %v", stats.PeerID, peerID)
	}
	if !stats.Connected {
		t.Error("Connected should be true")
	}
	if !stats.IsOutbound {
		t.Error("IsOutbound should be true")
	}
	if stats.ConnectionCount != 3 {
		t.Errorf("ConnectionCount = %d, want 3", stats.ConnectionCount)
	}
	if stats.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", stats.FailureCount)
	}
}

func TestStreamStats_Fields(t *testing.T) {
	now := time.Now()

	stats := &StreamStats{
		Name:             "test-stream",
		MessagesSent:     10,
		MessagesReceived: 20,
		BytesSent:        1000,
		BytesReceived:    2000,
		LastSentAt:       now,
		LastReceivedAt:   now,
	}

	if stats.Name != "test-stream" {
		t.Errorf("Name = %s, want test-stream", stats.Name)
	}
	if stats.MessagesSent != 10 {
		t.Errorf("MessagesSent = %d, want 10", stats.MessagesSent)
	}
	if stats.MessagesReceived != 20 {
		t.Errorf("MessagesReceived = %d, want 20", stats.MessagesReceived)
	}
}
