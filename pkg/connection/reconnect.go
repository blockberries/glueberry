package connection

import (
	"context"
	"math/rand"
	"time"
)

// ReconnectState tracks the reconnection attempts for a peer.
type ReconnectState struct {
	// Attempts is the number of reconnection attempts made.
	Attempts int

	// NextAttempt is the time of the next reconnection attempt.
	NextAttempt time.Time

	// CurrentDelay is the current backoff delay.
	CurrentDelay time.Duration

	// Cancel cancels the reconnection goroutine.
	Cancel context.CancelFunc
}

// NewReconnectState creates a new reconnection state.
func NewReconnectState(cancel context.CancelFunc) *ReconnectState {
	return &ReconnectState{
		Attempts: 0,
		Cancel:   cancel,
	}
}

// BackoffCalculator calculates the next backoff delay with exponential backoff.
type BackoffCalculator struct {
	BaseDelay time.Duration
	MaxDelay  time.Duration
}

// NewBackoffCalculator creates a new backoff calculator.
func NewBackoffCalculator(baseDelay, maxDelay time.Duration) *BackoffCalculator {
	return &BackoffCalculator{
		BaseDelay: baseDelay,
		MaxDelay:  maxDelay,
	}
}

// NextDelay calculates the next backoff delay for the given attempt number.
// It uses exponential backoff with jitter to prevent thundering herd.
func (bc *BackoffCalculator) NextDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// Calculate exponential backoff: baseDelay * 2^attempt
	delay := bc.BaseDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay >= bc.MaxDelay {
			delay = bc.MaxDelay
			break
		}
	}

	// Cap at max delay
	if delay > bc.MaxDelay {
		delay = bc.MaxDelay
	}

	// Add jitter (±10%) to prevent synchronized reconnections
	jitter := time.Duration(float64(delay) * 0.1 * (2*rand.Float64() - 1))
	delay += jitter

	// Ensure delay is non-negative
	if delay < 0 {
		delay = bc.BaseDelay
	}

	return delay
}

// ScheduleNext schedules the next reconnection attempt.
// Updates the ReconnectState with the next attempt time and current delay.
func (bc *BackoffCalculator) ScheduleNext(rs *ReconnectState) {
	rs.CurrentDelay = bc.NextDelay(rs.Attempts)
	rs.NextAttempt = time.Now().Add(rs.CurrentDelay)
	rs.Attempts++
}

// ShouldRetry determines if reconnection should be attempted based on
// the maximum attempt limit. Returns true if more attempts should be made.
func ShouldRetry(attempts, maxAttempts int) bool {
	// maxAttempts == 0 means unlimited retries
	if maxAttempts == 0 {
		return true
	}
	return attempts < maxAttempts
}
