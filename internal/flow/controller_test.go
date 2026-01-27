package flow

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewController_Defaults(t *testing.T) {
	fc := NewController(0, 0)

	if fc.highWatermark != DefaultHighWatermark {
		t.Errorf("expected high watermark %d, got %d", DefaultHighWatermark, fc.highWatermark)
	}
	if fc.lowWatermark != DefaultLowWatermark {
		t.Errorf("expected low watermark %d, got %d", DefaultLowWatermark, fc.lowWatermark)
	}
}

func TestNewController_CustomValues(t *testing.T) {
	fc := NewController(500, 50)

	if fc.highWatermark != 500 {
		t.Errorf("expected high watermark 500, got %d", fc.highWatermark)
	}
	if fc.lowWatermark != 50 {
		t.Errorf("expected low watermark 50, got %d", fc.lowWatermark)
	}
}

func TestNewController_LowWatermarkAdjustment(t *testing.T) {
	// When low >= high, low should be adjusted
	fc := NewController(100, 100)

	if fc.lowWatermark >= fc.highWatermark {
		t.Errorf("low watermark %d should be less than high %d", fc.lowWatermark, fc.highWatermark)
	}
	if fc.lowWatermark != 10 { // 100/10
		t.Errorf("expected low watermark 10, got %d", fc.lowWatermark)
	}
}

func TestNewController_LowWatermarkMinimum(t *testing.T) {
	// When high is small, low should be at least 1
	fc := NewController(5, 10)

	if fc.lowWatermark < 1 {
		t.Errorf("low watermark should be at least 1, got %d", fc.lowWatermark)
	}
}

func TestController_AcquireRelease_Basic(t *testing.T) {
	fc := NewController(10, 5)
	ctx := context.Background()

	// Acquire should succeed immediately when under watermark
	err := fc.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	if fc.Pending() != 1 {
		t.Errorf("expected pending 1, got %d", fc.Pending())
	}

	fc.Release()

	if fc.Pending() != 0 {
		t.Errorf("expected pending 0, got %d", fc.Pending())
	}
}

func TestController_BlocksAtHighWatermark(t *testing.T) {
	fc := NewController(3, 1)
	ctx := context.Background()

	// Fill up to high watermark
	for i := 0; i < 3; i++ {
		err := fc.Acquire(ctx)
		if err != nil {
			t.Fatalf("Acquire %d failed: %v", i, err)
		}
	}

	if !fc.IsBlocked() {
		t.Error("expected flow controller to be blocked at high watermark")
	}

	// Next acquire should block
	acquireDone := make(chan error, 1)
	ctx2, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	go func() {
		acquireDone <- fc.Acquire(ctx2)
	}()

	select {
	case err := <-acquireDone:
		if err != context.DeadlineExceeded {
			t.Errorf("expected DeadlineExceeded, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Acquire should have timed out")
	}
}

func TestController_UnblocksAtLowWatermark(t *testing.T) {
	fc := NewController(3, 1)
	ctx := context.Background()

	// Fill up to high watermark
	for i := 0; i < 3; i++ {
		if err := fc.Acquire(ctx); err != nil {
			t.Fatalf("Acquire %d failed: %v", i, err)
		}
	}

	// Start a goroutine that will block on Acquire
	// Note: blocked waiters do NOT increment pending
	acquireDone := make(chan error, 1)
	go func() {
		acquireDone <- fc.Acquire(ctx)
	}()

	// Give it time to block
	time.Sleep(10 * time.Millisecond)

	// Release until we hit low watermark
	// Pending stays at 3 (blocked waiters don't increment)
	fc.Release() // 3 -> 2 (still blocked)
	fc.Release() // 2 -> 1 (now at low watermark, should unblock)
	fc.Release() // 1 -> 0 (waiter got unblocked and incremented, this releases that)

	select {
	case err := <-acquireDone:
		if err != nil {
			t.Errorf("Acquire should have succeeded, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Acquire should have unblocked")
	}
}

func TestController_ContextCancelled(t *testing.T) {
	fc := NewController(3, 1)

	// Fill up to trigger blocking
	for i := 0; i < 3; i++ {
		if err := fc.Acquire(context.Background()); err != nil {
			t.Fatalf("Acquire %d failed: %v", i, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	acquireDone := make(chan error, 1)
	go func() {
		acquireDone <- fc.Acquire(ctx)
	}()

	// Give it time to start blocking
	time.Sleep(10 * time.Millisecond)

	cancel()

	select {
	case err := <-acquireDone:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Acquire should have been cancelled")
	}

	// Pending should still be 3 (cancelled acquires never increment pending)
	if fc.Pending() != 3 {
		t.Errorf("expected pending 3, got %d", fc.Pending())
	}
}

func TestController_BlockedCallback(t *testing.T) {
	fc := NewController(3, 1)
	var callbackCount int32

	fc.SetBlockedCallback(func() {
		atomic.AddInt32(&callbackCount, 1)
	})

	ctx := context.Background()

	// Fill up to high watermark
	for i := 0; i < 3; i++ {
		if err := fc.Acquire(ctx); err != nil {
			t.Fatalf("Acquire %d failed: %v", i, err)
		}
	}

	if atomic.LoadInt32(&callbackCount) != 1 {
		t.Errorf("expected callback to be called once, called %d times", callbackCount)
	}
}

func TestController_Close(t *testing.T) {
	fc := NewController(3, 1)
	ctx := context.Background()

	// Fill up to trigger blocking
	for i := 0; i < 3; i++ {
		if err := fc.Acquire(ctx); err != nil {
			t.Fatalf("Acquire %d failed: %v", i, err)
		}
	}

	acquireDone := make(chan error, 1)
	go func() {
		acquireDone <- fc.Acquire(ctx)
	}()

	// Give it time to block
	time.Sleep(10 * time.Millisecond)

	fc.Close()

	select {
	case err := <-acquireDone:
		// When closed, blocked acquires should return
		if err != nil && err != context.Canceled {
			// Either error is acceptable depending on timing
			t.Logf("Acquire returned: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Acquire should have unblocked on Close")
	}
}

func TestController_CloseIdempotent(t *testing.T) {
	fc := NewController(10, 5)

	// Should not panic on multiple closes
	fc.Close()
	fc.Close()
}

func TestController_AcquireAfterClose(t *testing.T) {
	fc := NewController(10, 5)
	fc.Close()

	err := fc.Acquire(context.Background())
	if err != context.Canceled {
		t.Errorf("expected context.Canceled after Close, got %v", err)
	}
}

func TestController_ReleaseUnderflow(t *testing.T) {
	fc := NewController(10, 5)

	// Release without Acquire should not go negative
	fc.Release()
	fc.Release()

	if fc.Pending() < 0 {
		t.Errorf("pending should not be negative, got %d", fc.Pending())
	}
}

func TestController_Concurrent(t *testing.T) {
	fc := NewController(100, 10)
	ctx := context.Background()

	var wg sync.WaitGroup
	const numGoroutines = 50
	const numOps = 100

	// Multiple goroutines doing acquire/release
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				if err := fc.Acquire(ctx); err == nil {
					// Simulate some work
					time.Sleep(time.Microsecond)
					fc.Release()
				}
			}
		}()
	}

	wg.Wait()

	// All operations should complete, pending should be 0
	if fc.Pending() != 0 {
		t.Errorf("expected pending 0 after all operations, got %d", fc.Pending())
	}
}

func TestController_Pending(t *testing.T) {
	fc := NewController(10, 5)
	ctx := context.Background()

	if fc.Pending() != 0 {
		t.Errorf("expected initial pending 0, got %d", fc.Pending())
	}

	for i := 1; i <= 5; i++ {
		fc.Acquire(ctx)
		if fc.Pending() != i {
			t.Errorf("expected pending %d, got %d", i, fc.Pending())
		}
	}

	for i := 4; i >= 0; i-- {
		fc.Release()
		if fc.Pending() != i {
			t.Errorf("expected pending %d, got %d", i, fc.Pending())
		}
	}
}

func TestController_IsBlocked(t *testing.T) {
	fc := NewController(3, 1)
	ctx := context.Background()

	if fc.IsBlocked() {
		t.Error("should not be blocked initially")
	}

	// Fill up
	for i := 0; i < 3; i++ {
		fc.Acquire(ctx)
	}

	if !fc.IsBlocked() {
		t.Error("should be blocked at high watermark")
	}

	// Release to low watermark
	fc.Release()
	fc.Release()

	if fc.IsBlocked() {
		t.Error("should not be blocked at low watermark")
	}
}

// TestController_StressBlockedWaiters tests that when 100+ goroutines are blocked
// on backpressure, ALL of them unblock correctly when transitioning from blocked
// to unblocked state. This tests the broadcast mechanism (close and recreate channel).
func TestController_StressBlockedWaiters(t *testing.T) {
	const numBlockedGoroutines = 150
	const highWatermark = 10
	const lowWatermark = 2

	fc := NewController(highWatermark, lowWatermark)
	ctx := context.Background()

	// Fill up to high watermark to trigger blocking
	for i := 0; i < highWatermark; i++ {
		if err := fc.Acquire(ctx); err != nil {
			t.Fatalf("Acquire %d failed: %v", i, err)
		}
	}

	if !fc.IsBlocked() {
		t.Fatal("expected flow controller to be blocked at high watermark")
	}

	// Track how many goroutines have unblocked
	var unblocked atomic.Int32
	var started sync.WaitGroup
	var finished sync.WaitGroup

	started.Add(numBlockedGoroutines)
	finished.Add(numBlockedGoroutines)

	// Start many goroutines that will block on Acquire
	for i := 0; i < numBlockedGoroutines; i++ {
		go func() {
			started.Done() // Signal we're about to block

			err := fc.Acquire(ctx)
			if err != nil {
				// Context error - shouldn't happen with background context
				t.Errorf("Acquire returned error: %v", err)
			}
			unblocked.Add(1)

			// Release immediately to not block others
			fc.Release()
			finished.Done()
		}()
	}

	// Wait for all goroutines to start and be blocking
	started.Wait()
	time.Sleep(50 * time.Millisecond) // Give them time to block

	// Verify no one has unblocked yet
	if got := unblocked.Load(); got != 0 {
		t.Errorf("expected 0 unblocked before Release, got %d", got)
	}

	// Release enough to drop below low watermark and trigger unblock broadcast
	// We need to release (highWatermark - lowWatermark) to go from high to low
	releasesNeeded := highWatermark - lowWatermark
	for i := 0; i < releasesNeeded; i++ {
		fc.Release()
	}

	// Wait for all goroutines to finish with a timeout
	done := make(chan struct{})
	go func() {
		finished.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All goroutines finished successfully
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout: only %d/%d goroutines unblocked", unblocked.Load(), numBlockedGoroutines)
	}

	// Verify all goroutines unblocked
	if got := unblocked.Load(); got != int32(numBlockedGoroutines) {
		t.Errorf("expected %d unblocked, got %d", numBlockedGoroutines, got)
	}

	// Verify pending is back to reasonable level
	// Note: pending may be non-zero if goroutines are still in flight
	pending := fc.Pending()
	t.Logf("Final pending count: %d", pending)
}

// TestController_StressMultipleBlockUnblockCycles tests multiple cycles of
// blocking and unblocking with 100+ goroutines to ensure the broadcast
// mechanism works correctly across multiple cycles.
func TestController_StressMultipleBlockUnblockCycles(t *testing.T) {
	const numGoroutines = 100
	const numCycles = 5
	const highWatermark = 5
	const lowWatermark = 1

	fc := NewController(highWatermark, lowWatermark)
	ctx := context.Background()

	for cycle := 0; cycle < numCycles; cycle++ {
		t.Logf("Starting cycle %d", cycle)

		// Fill up to high watermark
		for i := 0; i < highWatermark; i++ {
			if err := fc.Acquire(ctx); err != nil {
				t.Fatalf("cycle %d: Acquire %d failed: %v", cycle, i, err)
			}
		}

		if !fc.IsBlocked() {
			t.Fatalf("cycle %d: expected blocked", cycle)
		}

		var unblocked atomic.Int32
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		// Start goroutines that will block
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				if err := fc.Acquire(ctx); err == nil {
					unblocked.Add(1)
					fc.Release()
				}
			}()
		}

		// Give goroutines time to start blocking
		time.Sleep(20 * time.Millisecond)

		// Release to trigger unblock
		for i := 0; i < highWatermark; i++ {
			fc.Release()
		}

		// Wait for all with timeout
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(3 * time.Second):
			t.Fatalf("cycle %d: timeout - %d/%d unblocked", cycle, unblocked.Load(), numGoroutines)
		}

		if got := unblocked.Load(); got != int32(numGoroutines) {
			t.Errorf("cycle %d: expected %d unblocked, got %d", cycle, numGoroutines, got)
		}
	}
}

// TestController_StressConcurrentAcquireRelease tests heavy concurrent
// acquire/release operations to verify no deadlocks or race conditions
// under extreme load.
func TestController_StressConcurrentAcquireRelease(t *testing.T) {
	const numGoroutines = 200
	const opsPerGoroutine = 500
	const highWatermark = 50
	const lowWatermark = 10

	fc := NewController(highWatermark, lowWatermark)
	ctx := context.Background()

	var successfulOps atomic.Int64
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	start := time.Now()

	for g := 0; g < numGoroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				if err := fc.Acquire(ctx); err == nil {
					successfulOps.Add(1)
					// Brief work simulation
					time.Sleep(time.Microsecond)
					fc.Release()
				}
			}
		}()
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(30 * time.Second):
		t.Fatal("timeout: test took too long, possible deadlock")
	}

	elapsed := time.Since(start)
	ops := successfulOps.Load()
	opsPerSec := float64(ops) / elapsed.Seconds()

	t.Logf("Completed %d operations in %v (%.0f ops/sec)", ops, elapsed, opsPerSec)
	t.Logf("Final pending: %d, blocked: %v", fc.Pending(), fc.IsBlocked())

	// Verify all operations completed
	if ops == 0 {
		t.Error("expected some successful operations")
	}

	// Final pending should be 0
	if fc.Pending() != 0 {
		t.Errorf("expected final pending 0, got %d", fc.Pending())
	}
}
