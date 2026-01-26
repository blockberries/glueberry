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
	// This will increment pending to 4 before blocking
	acquireDone := make(chan error, 1)
	go func() {
		acquireDone <- fc.Acquire(ctx)
	}()

	// Give it time to block
	time.Sleep(10 * time.Millisecond)

	// Release until we hit low watermark
	// The blocked Acquire incremented pending to 4
	fc.Release() // 4 -> 3 (still blocked)
	fc.Release() // 3 -> 2 (still blocked)
	fc.Release() // 2 -> 1 (now at low watermark, should unblock)

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

	// Pending should be back to 3 (the cancelled acquire decrements)
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
