package flow

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestController_MultipleWaiters verifies that all blocked waiters are properly
// unblocked when pending drops to the low watermark. This is a regression test
// for a bug where only one waiter was unblocked due to using a buffered channel
// instead of a broadcast mechanism.
func TestController_MultipleWaiters(t *testing.T) {
	// High=3, Low=1: first 3 acquires succeed, then we block
	fc := NewController(3, 1)
	ctx := context.Background()

	// Fill up to high watermark - these all succeed
	for i := 0; i < 3; i++ {
		if err := fc.Acquire(ctx); err != nil {
			t.Fatalf("Initial Acquire %d failed: %v", i, err)
		}
	}
	// pending=3, blocked=true

	// Start 3 waiters - they will all block
	var wg sync.WaitGroup
	var succeeded int32
	numWaiters := 3

	for i := 0; i < numWaiters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			err := fc.Acquire(waitCtx)
			if err == nil {
				atomic.AddInt32(&succeeded, 1)
				// Release after successful acquire to avoid affecting pending count
				fc.Release()
			}
		}(i)
	}

	// Give waiters time to block
	time.Sleep(50 * time.Millisecond)

	// pending should still be 3 (waiters don't increment pending while blocked)
	if pending := fc.Pending(); pending != 3 {
		t.Errorf("Expected pending=3 while waiters blocked, got %d", pending)
	}

	// Release the initial 3 acquires - this should unblock all waiters
	for i := 0; i < 3; i++ {
		fc.Release()
	}

	// Wait for all waiters
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All done
	case <-time.After(3 * time.Second):
		t.Fatalf("Timeout waiting for waiters: only %d/%d succeeded", succeeded, numWaiters)
	}

	// All 3 waiters should have succeeded
	if succeeded != int32(numWaiters) {
		t.Errorf("Expected all %d waiters to succeed, only %d did", numWaiters, succeeded)
	}
}

// TestController_StressConcurrent runs many concurrent acquire/release operations
// to verify no races or deadlocks occur under load.
func TestController_StressConcurrent(t *testing.T) {
	fc := NewController(50, 10)
	ctx := context.Background()

	var wg sync.WaitGroup
	const numGoroutines = 100
	const numOps = 500

	var successCount int64
	var failCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				opCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
				if err := fc.Acquire(opCtx); err == nil {
					atomic.AddInt64(&successCount, 1)
					// Simulate some work
					time.Sleep(time.Microsecond)
					fc.Release()
				} else {
					atomic.AddInt64(&failCount, 1)
				}
				cancel()
			}
		}()
	}

	wg.Wait()

	t.Logf("Completed: %d successful, %d failed (timeouts)", successCount, failCount)

	// All operations should complete, pending should be 0
	if fc.Pending() != 0 {
		t.Errorf("Expected pending=0 after all operations, got %d", fc.Pending())
	}
}
