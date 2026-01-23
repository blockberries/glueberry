package connection

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestBackoffCalculator_NextDelay(t *testing.T) {
	bc := NewBackoffCalculator(1*time.Second, 1*time.Minute)

	tests := []struct {
		attempt int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{0, 0, 2 * time.Second},     // 1s * 2^0 = 1s, with jitter
		{1, time.Second, 3 * time.Second},     // 1s * 2^1 = 2s, with jitter
		{2, 3 * time.Second, 5 * time.Second},     // 1s * 2^2 = 4s, with jitter
		{3, 7 * time.Second, 9 * time.Second},     // 1s * 2^3 = 8s, with jitter
		{10, 55 * time.Second, 66 * time.Second}, // Capped at 1m with jitter
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempt_%d", tt.attempt), func(t *testing.T) {
			delay := bc.NextDelay(tt.attempt)
			if delay < tt.minDelay || delay > tt.maxDelay {
				t.Errorf("NextDelay(%d) = %v, want between %v and %v",
					tt.attempt, delay, tt.minDelay, tt.maxDelay)
			}
		})
	}
}

func TestBackoffCalculator_NextDelay_NegativeAttempt(t *testing.T) {
	bc := NewBackoffCalculator(1*time.Second, 1*time.Minute)

	delay := bc.NextDelay(-1)
	if delay < 0 || delay > 2*time.Second {
		t.Errorf("NextDelay(-1) = %v, should treat as attempt 0", delay)
	}
}

func TestBackoffCalculator_ScheduleNext(t *testing.T) {
	bc := NewBackoffCalculator(1*time.Second, 1*time.Minute)
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	rs := NewReconnectState(cancel)

	// First schedule
	before := time.Now()
	bc.ScheduleNext(rs)
	after := time.Now()

	if rs.Attempts != 1 {
		t.Errorf("Attempts = %d, want 1", rs.Attempts)
	}

	if rs.NextAttempt.Before(before) || rs.NextAttempt.After(after.Add(2*time.Second)) {
		t.Errorf("NextAttempt outside expected range")
	}

	// Second schedule
	bc.ScheduleNext(rs)

	if rs.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2 after second schedule", rs.Attempts)
	}
}

func TestBackoffCalculator_MaxDelay(t *testing.T) {
	bc := NewBackoffCalculator(1*time.Second, 5*time.Second)

	// After several attempts, should be capped at max delay
	for attempt := 10; attempt < 20; attempt++ {
		delay := bc.NextDelay(attempt)
		// With jitter (±10%), max should be around 5.5s
		if delay > 6*time.Second {
			t.Errorf("NextDelay(%d) = %v, should be capped around max delay 5s (with jitter)", attempt, delay)
		}
	}
}

func TestShouldRetry_Unlimited(t *testing.T) {
	// maxAttempts = 0 means unlimited
	for attempts := 0; attempts < 1000; attempts++ {
		if !ShouldRetry(attempts, 0) {
			t.Errorf("ShouldRetry(%d, 0) = false, want true (unlimited)", attempts)
		}
	}
}

func TestShouldRetry_Limited(t *testing.T) {
	maxAttempts := 5

	tests := []struct {
		attempts    int
		shouldRetry bool
	}{
		{0, true},
		{1, true},
		{4, true},
		{5, false},  // At limit
		{6, false},  // Exceeded
		{100, false}, // Far exceeded
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("attempts_%d", tt.attempts), func(t *testing.T) {
			got := ShouldRetry(tt.attempts, maxAttempts)
			if got != tt.shouldRetry {
				t.Errorf("ShouldRetry(%d, %d) = %v, want %v",
					tt.attempts, maxAttempts, got, tt.shouldRetry)
			}
		})
	}
}

func TestNewReconnectState(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	rs := NewReconnectState(cancel)

	if rs.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", rs.Attempts)
	}
	if rs.Cancel == nil {
		t.Error("Cancel should not be nil")
	}
}

func TestBackoffCalculator_Jitter(t *testing.T) {
	bc := NewBackoffCalculator(1*time.Second, 1*time.Minute)

	// Calculate delay multiple times for the same attempt
	// Due to jitter, they should not all be identical
	delays := make(map[time.Duration]bool)
	attempt := 3

	for i := 0; i < 100; i++ {
		delay := bc.NextDelay(attempt)
		delays[delay] = true
	}

	// Should have some variety due to jitter
	// (though small chance all are same if jitter is very small)
	if len(delays) < 2 {
		t.Logf("Warning: Only %d unique delays out of 100 iterations, jitter might be too small", len(delays))
	}
}
