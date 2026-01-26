// Package eventdispatch provides event dispatching for connection state changes.
package eventdispatch

import (
	"sync"
	"time"

	"github.com/blockberries/glueberry/pkg/connection"
	"github.com/libp2p/go-libp2p/core/peer"
)

// ConnectionEvent represents a connection state change event.
// This is the same as the public glueberry.ConnectionEvent type.
type ConnectionEvent struct {
	PeerID    peer.ID
	State     connection.ConnectionState
	Error     error
	Timestamp time.Time
}

// IsError returns true if this event represents an error condition.
func (e ConnectionEvent) IsError() bool {
	return e.Error != nil
}

// Dispatcher manages event emission to a buffered channel.
// It implements non-blocking sends to prevent slow consumers from
// blocking connection operations.
type Dispatcher struct {
	events chan ConnectionEvent
	mu     sync.Mutex
	closed bool
}

// NewDispatcher creates a new event dispatcher with the given buffer size.
func NewDispatcher(bufferSize int) *Dispatcher {
	return &Dispatcher{
		events: make(chan ConnectionEvent, bufferSize),
	}
}

// EmitEvent emits a connection event.
// This is a non-blocking operation - if the channel is full, the event is dropped.
func (d *Dispatcher) EmitEvent(event connection.Event) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return
	}

	// Convert to ConnectionEvent type
	evt := ConnectionEvent{
		PeerID:    event.PeerID,
		State:     event.State,
		Error:     event.Error,
		Timestamp: event.Timestamp,
	}

	// Non-blocking send
	select {
	case d.events <- evt:
		// Event sent
	default:
		// Channel full, drop event
		// In production, might want to log this
	}
}

// Events returns the events channel for the application to consume.
// The channel is closed when the dispatcher is closed.
func (d *Dispatcher) Events() <-chan ConnectionEvent {
	return d.events
}

// Close closes the events channel.
// It is safe to call Close multiple times.
func (d *Dispatcher) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.closed {
		d.closed = true
		close(d.events)
	}
}

// IsClosed returns true if the dispatcher has been closed.
func (d *Dispatcher) IsClosed() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.closed
}
