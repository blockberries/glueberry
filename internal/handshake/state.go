package handshake

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// HandshakeState represents the current state of a handshake.
type HandshakeState int

const (
	// HandshakeStateInit is the initial state before handshake begins.
	HandshakeStateInit HandshakeState = iota

	// HandshakeStateSentRequest indicates a handshake request has been sent.
	HandshakeStateSentRequest

	// HandshakeStateReceivedResponse indicates a response has been received.
	HandshakeStateReceivedResponse

	// HandshakeStateSentFinalize indicates the final message has been sent.
	HandshakeStateSentFinalize

	// HandshakeStateComplete indicates the handshake completed successfully.
	HandshakeStateComplete

	// HandshakeStateFailed indicates the handshake has failed.
	HandshakeStateFailed
)

// String returns a human-readable name for the handshake state.
func (s HandshakeState) String() string {
	switch s {
	case HandshakeStateInit:
		return "Init"
	case HandshakeStateSentRequest:
		return "SentRequest"
	case HandshakeStateReceivedResponse:
		return "ReceivedResponse"
	case HandshakeStateSentFinalize:
		return "SentFinalize"
	case HandshakeStateComplete:
		return "Complete"
	case HandshakeStateFailed:
		return "Failed"
	default:
		return fmt.Sprintf("HandshakeState(%d)", s)
	}
}

// Sentinel errors for handshake state machine.
var (
	// ErrInvalidStateTransition indicates an invalid handshake state transition was attempted.
	ErrInvalidStateTransition = errors.New("invalid handshake state transition")

	// ErrHandshakeMaxRetries indicates the maximum number of handshake retries was exceeded.
	ErrHandshakeMaxRetries = errors.New("handshake max retries exceeded")
)

// HandshakeStateMachine tracks handshake progress and enforces valid state transitions.
// This is a utility type that applications can use to implement robust handshakes
// with retry logic.
//
// All methods are safe for concurrent use.
type HandshakeStateMachine struct {
	mu         sync.Mutex
	state      HandshakeState
	retries    int
	maxRetries int
	lastError  error
	startTime  time.Time
}

// NewHandshakeStateMachine creates a new handshake state machine.
// maxRetries specifies the maximum number of retry attempts (0 = no retries).
func NewHandshakeStateMachine(maxRetries int) *HandshakeStateMachine {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &HandshakeStateMachine{
		state:      HandshakeStateInit,
		maxRetries: maxRetries,
		startTime:  time.Now(),
	}
}

// State returns the current handshake state.
func (hsm *HandshakeStateMachine) State() HandshakeState {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()
	return hsm.state
}

// Retries returns the number of retries attempted.
func (hsm *HandshakeStateMachine) Retries() int {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()
	return hsm.retries
}

// LastError returns the last error encountered, if any.
func (hsm *HandshakeStateMachine) LastError() error {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()
	return hsm.lastError
}

// Duration returns the time elapsed since the handshake started.
func (hsm *HandshakeStateMachine) Duration() time.Duration {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()
	return time.Since(hsm.startTime)
}

// Transition attempts to transition to a new state.
// Returns an error if the transition is invalid.
func (hsm *HandshakeStateMachine) Transition(to HandshakeState) error {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()

	if !isValidTransition(hsm.state, to) {
		return fmt.Errorf("%w: %v -> %v", ErrInvalidStateTransition, hsm.state, to)
	}

	hsm.state = to
	return nil
}

// TransitionWithRetry attempts to transition to a new state, incrementing
// the retry counter. Returns an error if retries are exhausted or the
// transition is invalid.
func (hsm *HandshakeStateMachine) TransitionWithRetry(to HandshakeState) error {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()

	if !isValidTransition(hsm.state, to) {
		return fmt.Errorf("%w: %v -> %v", ErrInvalidStateTransition, hsm.state, to)
	}

	// Check if this is a retry (transitioning to a previous state)
	if isRetryTransition(hsm.state, to) {
		if hsm.retries >= hsm.maxRetries {
			hsm.state = HandshakeStateFailed
			return ErrHandshakeMaxRetries
		}
		hsm.retries++
	}

	hsm.state = to
	return nil
}

// Fail marks the handshake as failed with the given error.
func (hsm *HandshakeStateMachine) Fail(err error) {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()
	hsm.state = HandshakeStateFailed
	hsm.lastError = err
}

// CanRetry returns true if the state machine allows a retry from the current state.
func (hsm *HandshakeStateMachine) CanRetry() bool {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()

	if hsm.state == HandshakeStateFailed || hsm.state == HandshakeStateComplete {
		return false
	}

	return hsm.retries < hsm.maxRetries
}

// IsComplete returns true if the handshake completed successfully.
func (hsm *HandshakeStateMachine) IsComplete() bool {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()
	return hsm.state == HandshakeStateComplete
}

// IsFailed returns true if the handshake has failed.
func (hsm *HandshakeStateMachine) IsFailed() bool {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()
	return hsm.state == HandshakeStateFailed
}

// IsTerminal returns true if the handshake is in a terminal state (Complete or Failed).
func (hsm *HandshakeStateMachine) IsTerminal() bool {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()
	return hsm.state == HandshakeStateComplete || hsm.state == HandshakeStateFailed
}

// Reset resets the state machine to its initial state.
// This can be used to restart a failed handshake.
func (hsm *HandshakeStateMachine) Reset() {
	hsm.mu.Lock()
	defer hsm.mu.Unlock()
	hsm.state = HandshakeStateInit
	hsm.retries = 0
	hsm.lastError = nil
	hsm.startTime = time.Now()
}

// isValidTransition checks if a state transition is valid.
// Valid transitions:
//
//	Init -> SentRequest
//	SentRequest -> ReceivedResponse, SentRequest (retry), Failed
//	ReceivedResponse -> SentFinalize, ReceivedResponse (retry), Failed
//	SentFinalize -> Complete, SentFinalize (retry), Failed
//	Complete -> (terminal)
//	Failed -> (terminal, but can be reset)
func isValidTransition(from, to HandshakeState) bool {
	switch from {
	case HandshakeStateInit:
		return to == HandshakeStateSentRequest || to == HandshakeStateFailed
	case HandshakeStateSentRequest:
		return to == HandshakeStateReceivedResponse || to == HandshakeStateSentRequest || to == HandshakeStateFailed
	case HandshakeStateReceivedResponse:
		return to == HandshakeStateSentFinalize || to == HandshakeStateReceivedResponse || to == HandshakeStateFailed
	case HandshakeStateSentFinalize:
		return to == HandshakeStateComplete || to == HandshakeStateSentFinalize || to == HandshakeStateFailed
	case HandshakeStateComplete:
		return false // Terminal state
	case HandshakeStateFailed:
		return false // Terminal state (use Reset to start over)
	default:
		return false
	}
}

// isRetryTransition checks if a transition represents a retry (same state).
func isRetryTransition(from, to HandshakeState) bool {
	return from == to && from != HandshakeStateInit
}
