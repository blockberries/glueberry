package connection

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestPeerConnection_ConcurrentStateTransitions tests concurrent state transitions
// to verify there are no race conditions in the TransitionTo method.
func TestPeerConnection_ConcurrentStateTransitions(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	const goroutines = 50
	const iterationsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				// Try various state transitions - some will succeed, some will fail
				_ = conn.TransitionTo(StateConnecting)
				_ = conn.TransitionTo(StateConnected)
				_ = conn.TransitionTo(StateEstablished)
				_ = conn.TransitionTo(StateDisconnected)
			}
		}()
	}

	wg.Wait()
	// If we get here without the race detector complaining, we're good
}

// TestPeerConnection_ConcurrentReadWrite tests concurrent read and write operations
// on PeerConnection fields to verify proper mutex protection.
func TestPeerConnection_ConcurrentReadWrite(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)
	conn := NewPeerConnection(peerID)

	const goroutines = 20
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(goroutines * 5) // 5 types of operations

	// Concurrent state reads
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = conn.GetState()
			}
		}()
	}

	// Concurrent state transitions
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = conn.TransitionTo(StateConnecting)
				_ = conn.TransitionTo(StateDisconnected)
			}
		}()
	}

	// Concurrent shared key operations
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				conn.SetSharedKey([]byte("test-key-32-bytes-long-exactly!!"))
				_ = conn.GetSharedKey()
			}
		}()
	}

	// Concurrent error operations
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				conn.SetError(fmt.Errorf("test error %d", i))
				_ = conn.GetError()
			}
		}()
	}

	// Concurrent IsOutbound operations
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				conn.SetIsOutbound(i%2 == 0)
				_ = conn.GetIsOutbound()
			}
		}()
	}

	wg.Wait()
}

// TestPeerConnection_ConcurrentCooldownOperations tests concurrent cooldown operations
// to verify there are no race conditions in cooldown handling.
func TestPeerConnection_ConcurrentCooldownOperations(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)

	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// Create fresh connection for each iteration to allow valid transitions
				conn := NewPeerConnection(peerID)
				conn.State = StateConnected // Set to state that can transition to cooldown

				// Start cooldown
				_ = conn.StartCooldown(time.Millisecond * 10)

				// Concurrent cooldown checks
				_ = conn.IsInCooldown()
				_ = conn.CooldownRemaining()
			}
		}()
	}

	wg.Wait()
}

// TestPeerConnection_ConcurrentHandshakeTimeout tests concurrent handshake timeout operations.
func TestPeerConnection_ConcurrentHandshakeTimeout(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)

	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				conn := NewPeerConnection(peerID)

				// Set up handshake timeout
				ctx, cancel := context.WithCancel(context.Background())
				conn.HandshakeCancel = cancel
				conn.HandshakeTimer = time.NewTimer(time.Hour)

				// Concurrent CancelHandshakeTimeout calls
				var inner sync.WaitGroup
				inner.Add(3)

				go func() {
					defer inner.Done()
					conn.CancelHandshakeTimeout()
				}()

				go func() {
					defer inner.Done()
					conn.CancelHandshakeTimeout()
				}()

				go func() {
					defer inner.Done()
					conn.CancelHandshakeTimeout()
				}()

				inner.Wait()
				ctx.Done()
			}
		}()
	}

	wg.Wait()
}

// TestPeerConnection_StressCleanup tests that Cleanup is safe to call concurrently with other operations.
func TestPeerConnection_StressCleanup(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)

	const iterations = 100

	for i := 0; i < iterations; i++ {
		conn := NewPeerConnection(peerID)

		// Set up some state
		conn.SetSharedKey([]byte("test-key-32-bytes-long-exactly!!"))
		ctx, cancel := context.WithCancel(context.Background())
		conn.ReconnectState = NewReconnectState(cancel)

		var wg sync.WaitGroup
		wg.Add(5)

		// Concurrent readers
		go func() {
			defer wg.Done()
			_ = conn.GetState()
			_ = conn.GetSharedKey()
		}()

		go func() {
			defer wg.Done()
			_ = conn.GetError()
			_ = conn.GetIsOutbound()
		}()

		go func() {
			defer wg.Done()
			_ = conn.GetStats()
		}()

		go func() {
			defer wg.Done()
			_ = conn.IsInCooldown()
		}()

		// Cleanup
		go func() {
			defer wg.Done()
			conn.Cleanup()
		}()

		wg.Wait()
		ctx.Done()
	}
}

// TestStateTransition_ConcurrentValidation tests concurrent state validation.
func TestStateTransition_ConcurrentValidation(t *testing.T) {
	const goroutines = 50
	const iterations = 200

	states := []ConnectionState{
		StateDisconnected,
		StateConnecting,
		StateConnected,
		StateEstablished,
		StateReconnecting,
		StateCooldown,
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				for _, from := range states {
					for _, to := range states {
						_ = from.ValidateTransition(to)
						_ = from.CanTransitionTo(to)
						_ = from.IsActive()
						_ = from.String()
					}
				}
			}
		}()
	}

	wg.Wait()
}

// TestBackoffCalculator_ConcurrentAccess tests concurrent access to BackoffCalculator.
func TestBackoffCalculator_ConcurrentAccess(t *testing.T) {
	const goroutines = 20
	const iterations = 100

	bc := NewBackoffCalculator(time.Second, time.Minute)

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = bc.NextDelay(i % 10)
			}
		}()
	}

	wg.Wait()
}

// TestMultiplePeerConnections_ConcurrentOperations tests operating on multiple peer connections concurrently.
func TestMultiplePeerConnections_ConcurrentOperations(t *testing.T) {
	// Create multiple peer connections
	peerID := mustParsePeerID(t, testPeerID)

	connections := make([]*PeerConnection, 10)
	for i := 0; i < 10; i++ {
		connections[i] = NewPeerConnection(peerID)
	}

	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// Operate on random connections
				for _, conn := range connections {
					_ = conn.GetState()
					conn.SetSharedKey([]byte("test-key-32-bytes-long-exactly!!"))
					_ = conn.GetSharedKey()
					_ = conn.TransitionTo(StateConnecting)
					_ = conn.TransitionTo(StateDisconnected)
				}
			}
		}()
	}

	wg.Wait()
}

// TestPeerConnection_RapidStateChanges tests rapid state changes to stress the state machine.
func TestPeerConnection_RapidStateChanges(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)

	const iterations = 1000

	for i := 0; i < iterations; i++ {
		conn := NewPeerConnection(peerID)

		// Rapid state transitions following valid paths
		_ = conn.TransitionTo(StateConnecting)
		_ = conn.TransitionTo(StateConnected)
		_ = conn.TransitionTo(StateEstablished)
		_ = conn.TransitionTo(StateDisconnected)

		// Or with reconnection
		conn2 := NewPeerConnection(peerID)
		_ = conn2.TransitionTo(StateConnecting)
		_ = conn2.TransitionTo(StateConnected)
		_ = conn2.TransitionTo(StateReconnecting)
		_ = conn2.TransitionTo(StateConnecting)
		_ = conn2.TransitionTo(StateDisconnected)
	}
}

// TestPeerConnection_ConcurrentSetAndCleanup tests setting values and cleanup running concurrently.
func TestPeerConnection_ConcurrentSetAndCleanup(t *testing.T) {
	peerID := mustParsePeerID(t, testPeerID)

	const goroutines = 10
	const iterations = 100

	for i := 0; i < iterations; i++ {
		conn := NewPeerConnection(peerID)

		var wg sync.WaitGroup
		wg.Add(goroutines + 1)

		// Concurrent setters
		for g := 0; g < goroutines; g++ {
			go func() {
				defer wg.Done()
				conn.SetSharedKey([]byte("test-key-32-bytes-long-exactly!!"))
				conn.SetError(fmt.Errorf("test error"))
				conn.SetIsOutbound(true)
			}()
		}

		// Cleanup running concurrently
		go func() {
			defer wg.Done()
			conn.Cleanup()
		}()

		wg.Wait()
	}
}
