package glueberry

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestHandshakeState_String(t *testing.T) {
	tests := []struct {
		state HandshakeState
		want  string
	}{
		{HandshakeStateInit, "Init"},
		{HandshakeStateSentRequest, "SentRequest"},
		{HandshakeStateReceivedResponse, "ReceivedResponse"},
		{HandshakeStateSentFinalize, "SentFinalize"},
		{HandshakeStateComplete, "Complete"},
		{HandshakeStateFailed, "Failed"},
		{HandshakeState(99), "HandshakeState(99)"},
	}

	for _, tt := range tests {
		got := tt.state.String()
		if got != tt.want {
			t.Errorf("HandshakeState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestNewHandshakeStateMachine(t *testing.T) {
	hsm := NewHandshakeStateMachine(3)

	if hsm.State() != HandshakeStateInit {
		t.Errorf("initial state = %v, want Init", hsm.State())
	}
	if hsm.Retries() != 0 {
		t.Errorf("initial retries = %d, want 0", hsm.Retries())
	}
	if hsm.LastError() != nil {
		t.Errorf("initial error = %v, want nil", hsm.LastError())
	}
}

func TestNewHandshakeStateMachine_NegativeRetries(t *testing.T) {
	hsm := NewHandshakeStateMachine(-1)

	// Should clamp to 0
	if hsm.maxRetries != 0 {
		t.Errorf("maxRetries = %d, want 0", hsm.maxRetries)
	}
}

func TestHandshakeStateMachine_ValidTransitions(t *testing.T) {
	hsm := NewHandshakeStateMachine(0)

	// Init -> SentRequest
	if err := hsm.Transition(HandshakeStateSentRequest); err != nil {
		t.Errorf("Init -> SentRequest failed: %v", err)
	}
	if hsm.State() != HandshakeStateSentRequest {
		t.Errorf("state = %v, want SentRequest", hsm.State())
	}

	// SentRequest -> ReceivedResponse
	if err := hsm.Transition(HandshakeStateReceivedResponse); err != nil {
		t.Errorf("SentRequest -> ReceivedResponse failed: %v", err)
	}

	// ReceivedResponse -> SentFinalize
	if err := hsm.Transition(HandshakeStateSentFinalize); err != nil {
		t.Errorf("ReceivedResponse -> SentFinalize failed: %v", err)
	}

	// SentFinalize -> Complete
	if err := hsm.Transition(HandshakeStateComplete); err != nil {
		t.Errorf("SentFinalize -> Complete failed: %v", err)
	}

	if !hsm.IsComplete() {
		t.Error("expected IsComplete to be true")
	}
	if !hsm.IsTerminal() {
		t.Error("expected IsTerminal to be true")
	}
}

func TestHandshakeStateMachine_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from HandshakeState
		to   HandshakeState
	}{
		{"Init -> ReceivedResponse", HandshakeStateInit, HandshakeStateReceivedResponse},
		{"Init -> SentFinalize", HandshakeStateInit, HandshakeStateSentFinalize},
		{"Init -> Complete", HandshakeStateInit, HandshakeStateComplete},
		{"SentRequest -> Complete", HandshakeStateSentRequest, HandshakeStateComplete},
		{"Complete -> anything", HandshakeStateComplete, HandshakeStateSentRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hsm := NewHandshakeStateMachine(0)
			hsm.state = tt.from

			err := hsm.Transition(tt.to)
			if err == nil {
				t.Errorf("expected error for transition %v -> %v", tt.from, tt.to)
			}
			if !errors.Is(err, ErrInvalidStateTransition) {
				t.Errorf("expected ErrInvalidStateTransition, got %v", err)
			}
		})
	}
}

func TestHandshakeStateMachine_TransitionWithRetry(t *testing.T) {
	hsm := NewHandshakeStateMachine(2)

	// Init -> SentRequest (not a retry)
	if err := hsm.TransitionWithRetry(HandshakeStateSentRequest); err != nil {
		t.Errorf("first transition failed: %v", err)
	}
	if hsm.Retries() != 0 {
		t.Errorf("retries = %d, want 0", hsm.Retries())
	}

	// SentRequest -> SentRequest (retry 1)
	if err := hsm.TransitionWithRetry(HandshakeStateSentRequest); err != nil {
		t.Errorf("retry 1 failed: %v", err)
	}
	if hsm.Retries() != 1 {
		t.Errorf("retries = %d, want 1", hsm.Retries())
	}

	// SentRequest -> SentRequest (retry 2)
	if err := hsm.TransitionWithRetry(HandshakeStateSentRequest); err != nil {
		t.Errorf("retry 2 failed: %v", err)
	}
	if hsm.Retries() != 2 {
		t.Errorf("retries = %d, want 2", hsm.Retries())
	}

	// SentRequest -> SentRequest (retry 3 - should fail)
	err := hsm.TransitionWithRetry(HandshakeStateSentRequest)
	if !errors.Is(err, ErrHandshakeMaxRetries) {
		t.Errorf("expected ErrHandshakeMaxRetries, got %v", err)
	}
	if !hsm.IsFailed() {
		t.Error("expected state to be Failed after max retries")
	}
}

func TestHandshakeStateMachine_Fail(t *testing.T) {
	hsm := NewHandshakeStateMachine(0)
	hsm.Transition(HandshakeStateSentRequest)

	testErr := errors.New("test error")
	hsm.Fail(testErr)

	if !hsm.IsFailed() {
		t.Error("expected IsFailed to be true")
	}
	if hsm.LastError() != testErr {
		t.Errorf("LastError = %v, want %v", hsm.LastError(), testErr)
	}
}

func TestHandshakeStateMachine_CanRetry(t *testing.T) {
	hsm := NewHandshakeStateMachine(1)

	// Initial state - can retry
	if !hsm.CanRetry() {
		t.Error("expected CanRetry to be true initially")
	}

	hsm.Transition(HandshakeStateSentRequest)
	hsm.TransitionWithRetry(HandshakeStateSentRequest) // Use one retry

	// No more retries left
	if hsm.CanRetry() {
		t.Error("expected CanRetry to be false after max retries")
	}
}

func TestHandshakeStateMachine_CanRetry_TerminalStates(t *testing.T) {
	// Complete state
	hsm := NewHandshakeStateMachine(5)
	hsm.state = HandshakeStateComplete
	if hsm.CanRetry() {
		t.Error("expected CanRetry to be false in Complete state")
	}

	// Failed state
	hsm.state = HandshakeStateFailed
	if hsm.CanRetry() {
		t.Error("expected CanRetry to be false in Failed state")
	}
}

func TestHandshakeStateMachine_Reset(t *testing.T) {
	hsm := NewHandshakeStateMachine(3)
	hsm.Transition(HandshakeStateSentRequest)
	hsm.TransitionWithRetry(HandshakeStateSentRequest)
	hsm.Fail(errors.New("test"))

	hsm.Reset()

	if hsm.State() != HandshakeStateInit {
		t.Errorf("state = %v, want Init after reset", hsm.State())
	}
	if hsm.Retries() != 0 {
		t.Errorf("retries = %d, want 0 after reset", hsm.Retries())
	}
	if hsm.LastError() != nil {
		t.Errorf("lastError = %v, want nil after reset", hsm.LastError())
	}
}

func TestHandshakeStateMachine_Duration(t *testing.T) {
	hsm := NewHandshakeStateMachine(0)

	time.Sleep(10 * time.Millisecond)

	duration := hsm.Duration()
	if duration < 10*time.Millisecond {
		t.Errorf("duration = %v, want >= 10ms", duration)
	}
}

func TestHandshakeStateMachine_IsTerminal(t *testing.T) {
	hsm := NewHandshakeStateMachine(0)

	if hsm.IsTerminal() {
		t.Error("expected IsTerminal to be false in Init state")
	}

	hsm.Transition(HandshakeStateSentRequest)
	if hsm.IsTerminal() {
		t.Error("expected IsTerminal to be false in SentRequest state")
	}

	hsm.Fail(nil)
	if !hsm.IsTerminal() {
		t.Error("expected IsTerminal to be true in Failed state")
	}
}

func TestHandshakeStateMachine_TransitionToFailed(t *testing.T) {
	// Can transition to Failed from any non-terminal state
	states := []HandshakeState{
		HandshakeStateInit,
		HandshakeStateSentRequest,
		HandshakeStateReceivedResponse,
		HandshakeStateSentFinalize,
	}

	for _, state := range states {
		t.Run(state.String(), func(t *testing.T) {
			hsm := NewHandshakeStateMachine(0)
			hsm.state = state

			err := hsm.Transition(HandshakeStateFailed)
			if err != nil {
				t.Errorf("%v -> Failed failed: %v", state, err)
			}
		})
	}
}

func TestHandshakeStateMachine_Concurrent(t *testing.T) {
	hsm := NewHandshakeStateMachine(100)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = hsm.State()
				_ = hsm.Retries()
				_ = hsm.LastError()
				_ = hsm.Duration()
				_ = hsm.CanRetry()
				_ = hsm.IsComplete()
				_ = hsm.IsFailed()
				_ = hsm.IsTerminal()
			}
		}()
	}

	wg.Wait()
}
