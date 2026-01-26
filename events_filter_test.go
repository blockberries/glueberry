package glueberry

import (
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestEventFilter_Matches_AllEvents(t *testing.T) {
	filter := EventFilter{} // Empty filter matches all

	events := []ConnectionEvent{
		{PeerID: peer.ID("peer1"), State: StateConnecting},
		{PeerID: peer.ID("peer2"), State: StateConnected},
		{PeerID: peer.ID("peer3"), State: StateEstablished},
	}

	for _, evt := range events {
		if !filter.matches(evt) {
			t.Errorf("empty filter should match event %v", evt)
		}
	}
}

func TestEventFilter_Matches_PeerFilter(t *testing.T) {
	peer1 := peer.ID("peer1")
	peer2 := peer.ID("peer2")
	peer3 := peer.ID("peer3")

	filter := EventFilter{
		PeerIDs: []peer.ID{peer1, peer2},
	}

	tests := []struct {
		evt   ConnectionEvent
		match bool
	}{
		{ConnectionEvent{PeerID: peer1, State: StateConnected}, true},
		{ConnectionEvent{PeerID: peer2, State: StateConnected}, true},
		{ConnectionEvent{PeerID: peer3, State: StateConnected}, false},
	}

	for _, tt := range tests {
		got := filter.matches(tt.evt)
		if got != tt.match {
			t.Errorf("filter.matches(%v) = %v, want %v", tt.evt, got, tt.match)
		}
	}
}

func TestEventFilter_Matches_StateFilter(t *testing.T) {
	filter := EventFilter{
		States: []ConnectionState{StateConnected, StateEstablished},
	}

	tests := []struct {
		evt   ConnectionEvent
		match bool
	}{
		{ConnectionEvent{PeerID: peer.ID("p1"), State: StateConnecting}, false},
		{ConnectionEvent{PeerID: peer.ID("p1"), State: StateConnected}, true},
		{ConnectionEvent{PeerID: peer.ID("p1"), State: StateEstablished}, true},
		{ConnectionEvent{PeerID: peer.ID("p1"), State: StateDisconnected}, false},
	}

	for _, tt := range tests {
		got := filter.matches(tt.evt)
		if got != tt.match {
			t.Errorf("filter.matches(state=%v) = %v, want %v", tt.evt.State, got, tt.match)
		}
	}
}

func TestEventFilter_Matches_CombinedFilter(t *testing.T) {
	peer1 := peer.ID("peer1")
	peer2 := peer.ID("peer2")

	filter := EventFilter{
		PeerIDs: []peer.ID{peer1},
		States:  []ConnectionState{StateConnected},
	}

	tests := []struct {
		evt   ConnectionEvent
		match bool
	}{
		// Peer matches, state matches
		{ConnectionEvent{PeerID: peer1, State: StateConnected}, true},
		// Peer matches, state doesn't match
		{ConnectionEvent{PeerID: peer1, State: StateConnecting}, false},
		// Peer doesn't match, state matches
		{ConnectionEvent{PeerID: peer2, State: StateConnected}, false},
		// Neither matches
		{ConnectionEvent{PeerID: peer2, State: StateConnecting}, false},
	}

	for _, tt := range tests {
		got := filter.matches(tt.evt)
		if got != tt.match {
			t.Errorf("filter.matches(peer=%v,state=%v) = %v, want %v",
				tt.evt.PeerID, tt.evt.State, got, tt.match)
		}
	}
}

func TestConnectionEvent_Fields(t *testing.T) {
	now := time.Now()
	err := ErrConnectionFailed

	evt := ConnectionEvent{
		PeerID:    peer.ID("test-peer"),
		State:     StateConnected,
		Error:     err,
		Timestamp: now,
	}

	if evt.PeerID != peer.ID("test-peer") {
		t.Errorf("PeerID = %v, want test-peer", evt.PeerID)
	}
	if evt.State != StateConnected {
		t.Errorf("State = %v, want StateConnected", evt.State)
	}
	if evt.Error != err {
		t.Errorf("Error = %v, want %v", evt.Error, err)
	}
	if !evt.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", evt.Timestamp, now)
	}
}

func TestConnectionState_StringInFilter(t *testing.T) {
	// Verify the states used in filtering work correctly
	states := []ConnectionState{
		StateDisconnected,
		StateConnecting,
		StateConnected,
		StateEstablished,
		StateCooldown,
	}

	for _, s := range states {
		// Just verify String() doesn't panic and returns something
		str := s.String()
		if str == "" {
			t.Errorf("ConnectionState(%d).String() returned empty string", s)
		}
	}
}
