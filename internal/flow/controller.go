// Package flow provides flow control and backpressure mechanisms.
package flow

import (
	"context"
	"sync"
	"time"
)

// Default flow control values.
const (
	DefaultHighWatermark = 1000
	DefaultLowWatermark  = 100
)

// Controller implements flow control using high and low watermarks.
// When the pending count reaches the high watermark, new sends block
// until the pending count drops to the low watermark.
// All methods are safe for concurrent use.
type Controller struct {
	mu            sync.Mutex
	highWatermark int
	lowWatermark  int
	pending       int
	blocked       bool
	closed        bool

	// unblockCh is closed when transitioning from blocked to unblocked.
	// A new channel is created after each unblock event.
	unblockCh chan struct{}

	// Metrics callback (optional)
	onBlocked func()
}

// NewController creates a new flow controller with the given watermarks.
// If high <= 0, DefaultHighWatermark is used.
// If low <= 0, DefaultLowWatermark is used.
// If low >= high, low is set to high/10 (minimum 1).
func NewController(high, low int) *Controller {
	if high <= 0 {
		high = DefaultHighWatermark
	}
	if low <= 0 {
		low = DefaultLowWatermark
	}
	if low >= high {
		low = max(high/10, 1)
	}

	return &Controller{
		highWatermark: high,
		lowWatermark:  low,
		unblockCh:     make(chan struct{}),
	}
}

// SetBlockedCallback sets a callback that is called when flow control
// becomes blocked. This is useful for metrics.
func (fc *Controller) SetBlockedCallback(fn func()) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.onBlocked = fn
}

// Acquire attempts to acquire permission to send a message.
// If the pending count is at or above the high watermark, this blocks
// until the pending count drops to the low watermark or the context is cancelled.
// Returns nil on success, or context.Canceled/context.DeadlineExceeded on failure.
func (fc *Controller) Acquire(ctx context.Context) error {
	for {
		fc.mu.Lock()

		if fc.closed {
			fc.mu.Unlock()
			return context.Canceled
		}

		// If not blocked, we can proceed immediately
		if !fc.blocked {
			fc.pending++

			// Check if we just hit the high watermark
			if fc.pending >= fc.highWatermark && !fc.blocked {
				fc.blocked = true
				if fc.onBlocked != nil {
					fc.onBlocked()
				}
			}

			fc.mu.Unlock()
			return nil
		}

		// We're blocked - get the current unblock channel
		// Note: we do NOT increment pending while waiting
		waitCh := fc.unblockCh
		fc.mu.Unlock()

		// Wait for either context cancellation or unblock signal
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
			// Channel was closed (unblock broadcast) - loop and retry
			continue
		}
	}
}

// Release decrements the pending count and potentially unblocks waiting senders.
// This should be called after a message has been sent successfully.
func (fc *Controller) Release() {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if fc.pending > 0 {
		fc.pending--
	}

	// Check if we should unblock
	if fc.blocked && fc.pending <= fc.lowWatermark {
		fc.blocked = false
		// Broadcast to ALL waiters by closing the channel
		close(fc.unblockCh)
		// Create a new channel for the next blocking cycle
		fc.unblockCh = make(chan struct{})
	}
}

// Pending returns the current number of pending messages.
func (fc *Controller) Pending() int {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.pending
}

// IsBlocked returns true if flow control is currently blocking new sends.
func (fc *Controller) IsBlocked() bool {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return fc.blocked
}

// Close closes the flow controller, unblocking any waiting Acquire calls.
func (fc *Controller) Close() {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if fc.closed {
		return
	}
	fc.closed = true

	// Unblock any waiting senders by closing the channel
	close(fc.unblockCh)
}

// WaitWithTimeout is a helper for tests that need to wait with a timeout.
// It's not part of the normal flow control API.
func (fc *Controller) WaitWithTimeout(timeout time.Duration) bool {
	fc.mu.Lock()
	if !fc.blocked {
		fc.mu.Unlock()
		return true
	}
	waitCh := fc.unblockCh
	fc.mu.Unlock()

	select {
	case <-waitCh:
		return true
	case <-time.After(timeout):
		return false
	}
}
