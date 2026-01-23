package eventdispatch

import (
	"fmt"
	"testing"
	"time"

	"github.com/blockberries/glueberry/pkg/connection"
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

func TestNewDispatcher(t *testing.T) {
	d := NewDispatcher(10)

	if d == nil {
		t.Fatal("NewDispatcher returned nil")
	}
	if d.events == nil {
		t.Error("events channel should be initialized")
	}
	if d.IsClosed() {
		t.Error("dispatcher should not be closed initially")
	}
}

func TestDispatcher_EmitEvent(t *testing.T) {
	d := NewDispatcher(10)
	defer d.Close()

	peerID := mustParsePeerID(t, testPeerID)

	connEvent := connection.Event{
		PeerID:    peerID,
		State:     connection.StateConnected,
		Error:     nil,
		Timestamp: time.Now(),
	}

	d.EmitEvent(connEvent)

	// Should receive the event
	select {
	case evt := <-d.Events():
		if evt.PeerID != peerID {
			t.Errorf("PeerID = %v, want %v", evt.PeerID, peerID)
		}
		if evt.State != connection.StateConnected {
			t.Errorf("State = %v, want StateConnected", evt.State)
		}
		if evt.Error != nil {
			t.Errorf("Error = %v, want nil", evt.Error)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestDispatcher_EmitEvent_Multiple(t *testing.T) {
	d := NewDispatcher(10)
	defer d.Close()

	peerID := mustParsePeerID(t, testPeerID)

	// Emit multiple events
	numEvents := 5
	for i := 0; i < numEvents; i++ {
		d.EmitEvent(connection.Event{
			PeerID:    peerID,
			State:     connection.ConnectionState(i),
			Timestamp: time.Now(),
		})
	}

	// Receive all events
	for i := 0; i < numEvents; i++ {
		select {
		case <-d.Events():
			// Event received
		case <-time.After(1 * time.Second):
			t.Fatalf("timeout waiting for event %d", i)
		}
	}
}

func TestDispatcher_EmitEvent_FullBuffer(t *testing.T) {
	bufferSize := 5
	d := NewDispatcher(bufferSize)
	defer d.Close()

	peerID := mustParsePeerID(t, testPeerID)

	// Fill the buffer
	for i := 0; i < bufferSize; i++ {
		d.EmitEvent(connection.Event{
			PeerID:    peerID,
			State:     connection.StateConnected,
			Timestamp: time.Now(),
		})
	}

	// Emit one more - should be dropped (non-blocking)
	d.EmitEvent(connection.Event{
		PeerID:    peerID,
		State:     connection.StateDisconnected,
		Timestamp: time.Now(),
	})

	// Drain the buffer
	received := 0
	for received < bufferSize {
		select {
		case <-d.Events():
			received++
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout draining buffer")
		}
	}

	// Should not receive the dropped event
	select {
	case <-d.Events():
		t.Error("should not receive dropped event")
	case <-time.After(50 * time.Millisecond):
		// Expected - no event
	}
}

func TestDispatcher_Close(t *testing.T) {
	d := NewDispatcher(10)

	d.Close()

	if !d.IsClosed() {
		t.Error("dispatcher should be closed after Close()")
	}

	// Events channel should be closed
	select {
	case _, ok := <-d.Events():
		if ok {
			t.Error("events channel should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("should be able to read from closed channel immediately")
	}
}

func TestDispatcher_CloseMultiple(t *testing.T) {
	d := NewDispatcher(10)

	// Multiple Close calls should be safe
	d.Close()
	d.Close()
	d.Close()

	if !d.IsClosed() {
		t.Error("dispatcher should be closed")
	}
}

func TestDispatcher_EmitAfterClose(t *testing.T) {
	d := NewDispatcher(10)

	peerID := mustParsePeerID(t, testPeerID)

	d.Close()

	// Emitting after close should not panic
	d.EmitEvent(connection.Event{
		PeerID:    peerID,
		State:     connection.StateConnected,
		Timestamp: time.Now(),
	})

	// Should not receive any events
	select {
	case evt, ok := <-d.Events():
		if ok {
			t.Errorf("received event after close: %v", evt)
		}
	case <-time.After(50 * time.Millisecond):
		// Expected
	}
}

func TestDispatcher_Concurrent(t *testing.T) {
	d := NewDispatcher(100)
	defer d.Close()

	peerID := mustParsePeerID(t, testPeerID)

	numGoroutines := 10
	eventsPerGoroutine := 10

	done := make(chan bool, numGoroutines)

	// Emit from multiple goroutines
	for g := 0; g < numGoroutines; g++ {
		go func() {
			for i := 0; i < eventsPerGoroutine; i++ {
				d.EmitEvent(connection.Event{
					PeerID:    peerID,
					State:     connection.StateConnected,
					Timestamp: time.Now(),
				})
			}
			done <- true
		}()
	}

	// Wait for all to complete
	for g := 0; g < numGoroutines; g++ {
		<-done
	}

	// All events should complete without race conditions
	// We don't verify count due to potential drops, but no panic is success
}

func TestDispatcher_EventConversion(t *testing.T) {
	d := NewDispatcher(10)
	defer d.Close()

	peerID := mustParsePeerID(t, testPeerID)
	testErr := fmt.Errorf("test error")
	testTime := time.Now()

	connEvent := connection.Event{
		PeerID:    peerID,
		State:     connection.StateDisconnected,
		Error:     testErr,
		Timestamp: testTime,
	}

	d.EmitEvent(connEvent)

	select {
	case evt := <-d.Events():
		if evt.PeerID != peerID {
			t.Errorf("PeerID = %v, want %v", evt.PeerID, peerID)
		}
		if evt.State != connection.StateDisconnected {
			t.Errorf("State = %v, want StateDisconnected", evt.State)
		}
		if evt.Error != testErr {
			t.Errorf("Error = %v, want %v", evt.Error, testErr)
		}
		if !evt.Timestamp.Equal(testTime) {
			t.Errorf("Timestamp = %v, want %v", evt.Timestamp, testTime)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}
}
