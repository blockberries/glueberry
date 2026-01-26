package glueberry

import (
	"errors"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestErrorCode_String(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want string
	}{
		{ErrCodeUnknown, "Unknown"},
		{ErrCodeConnectionFailed, "ConnectionFailed"},
		{ErrCodeHandshakeFailed, "HandshakeFailed"},
		{ErrCodeHandshakeTimeout, "HandshakeTimeout"},
		{ErrCodeStreamClosed, "StreamClosed"},
		{ErrCodeEncryptionFailed, "EncryptionFailed"},
		{ErrCodeDecryptionFailed, "DecryptionFailed"},
		{ErrCodePeerNotFound, "PeerNotFound"},
		{ErrCodePeerBlacklisted, "PeerBlacklisted"},
		{ErrCodeBufferFull, "BufferFull"},
		{ErrCodeContextCanceled, "ContextCanceled"},
		{ErrCodeInvalidConfig, "InvalidConfig"},
		{ErrCodeNodeNotStarted, "NodeNotStarted"},
		{ErrCodeNodeAlreadyStarted, "NodeAlreadyStarted"},
		{ErrorCode(999), "ErrorCode(999)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.code.String(); got != tt.want {
				t.Errorf("ErrorCode.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestError_Error(t *testing.T) {
	t.Run("without cause", func(t *testing.T) {
		err := &Error{
			Code:    ErrCodeConnectionFailed,
			Message: "connection refused",
		}
		want := "glueberry: connection refused"
		if got := err.Error(); got != want {
			t.Errorf("Error.Error() = %v, want %v", got, want)
		}
	})

	t.Run("with cause", func(t *testing.T) {
		cause := errors.New("dial timeout")
		err := &Error{
			Code:    ErrCodeConnectionFailed,
			Message: "connection refused",
			Cause:   cause,
		}
		want := "glueberry: connection refused: dial timeout"
		if got := err.Error(); got != want {
			t.Errorf("Error.Error() = %v, want %v", got, want)
		}
	})
}

func TestError_Unwrap(t *testing.T) {
	t.Run("with cause", func(t *testing.T) {
		cause := errors.New("underlying error")
		err := &Error{
			Code:    ErrCodeUnknown,
			Message: "wrapper",
			Cause:   cause,
		}
		if unwrapped := err.Unwrap(); unwrapped != cause {
			t.Errorf("Error.Unwrap() = %v, want %v", unwrapped, cause)
		}
	})

	t.Run("without cause", func(t *testing.T) {
		err := &Error{
			Code:    ErrCodeUnknown,
			Message: "no cause",
		}
		if unwrapped := err.Unwrap(); unwrapped != nil {
			t.Errorf("Error.Unwrap() = %v, want nil", unwrapped)
		}
	})
}

func TestError_Is(t *testing.T) {
	t.Run("same code", func(t *testing.T) {
		err1 := &Error{Code: ErrCodeConnectionFailed, Message: "error 1"}
		err2 := &Error{Code: ErrCodeConnectionFailed, Message: "error 2"}
		if !err1.Is(err2) {
			t.Error("Error.Is() should return true for same error code")
		}
	})

	t.Run("different code", func(t *testing.T) {
		err1 := &Error{Code: ErrCodeConnectionFailed}
		err2 := &Error{Code: ErrCodeHandshakeFailed}
		if err1.Is(err2) {
			t.Error("Error.Is() should return false for different error codes")
		}
	})

	t.Run("non-Error target", func(t *testing.T) {
		err1 := &Error{Code: ErrCodeConnectionFailed}
		err2 := errors.New("regular error")
		if err1.Is(err2) {
			t.Error("Error.Is() should return false for non-Error target")
		}
	})
}

func TestError_ErrorsIs(t *testing.T) {
	t.Run("wrapped error matches", func(t *testing.T) {
		inner := &Error{Code: ErrCodePeerNotFound}
		outer := &Error{Code: ErrCodeConnectionFailed, Cause: inner}

		// errors.Is should find inner error
		if !errors.Is(outer, &Error{Code: ErrCodePeerNotFound}) {
			t.Error("errors.Is should find wrapped Error by code")
		}
	})

	t.Run("direct match", func(t *testing.T) {
		err := &Error{Code: ErrCodeHandshakeTimeout}
		target := &Error{Code: ErrCodeHandshakeTimeout}
		if !errors.Is(err, target) {
			t.Error("errors.Is should match same error code")
		}
	})
}

func TestError_ErrorsAs(t *testing.T) {
	t.Run("unwraps to Error", func(t *testing.T) {
		original := &Error{Code: ErrCodeStreamClosed, Message: "stream was closed"}
		wrapped := errors.New("wrapper: " + original.Error())
		_ = wrapped // We can't wrap *Error with errors.New in a way that As will work

		// Direct extraction
		var gErr *Error
		if !errors.As(original, &gErr) {
			t.Error("errors.As should extract Error")
		}
		if gErr.Code != ErrCodeStreamClosed {
			t.Errorf("extracted error code = %v, want %v", gErr.Code, ErrCodeStreamClosed)
		}
	})
}

func TestIsRetriable(t *testing.T) {
	t.Run("retriable error", func(t *testing.T) {
		err := &Error{
			Code:      ErrCodeConnectionFailed,
			Retriable: true,
		}
		if !IsRetriable(err) {
			t.Error("IsRetriable() should return true for retriable error")
		}
	})

	t.Run("non-retriable error", func(t *testing.T) {
		err := &Error{
			Code:      ErrCodePeerBlacklisted,
			Retriable: false,
		}
		if IsRetriable(err) {
			t.Error("IsRetriable() should return false for non-retriable error")
		}
	})

	t.Run("non-Error", func(t *testing.T) {
		err := errors.New("regular error")
		if IsRetriable(err) {
			t.Error("IsRetriable() should return false for non-Error")
		}
	})

	t.Run("nil error", func(t *testing.T) {
		if IsRetriable(nil) {
			t.Error("IsRetriable() should return false for nil")
		}
	})
}

func TestIsPermanent(t *testing.T) {
	t.Run("blacklisted peer", func(t *testing.T) {
		err := &Error{Code: ErrCodePeerBlacklisted}
		if !IsPermanent(err) {
			t.Error("IsPermanent() should return true for blacklisted peer")
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		err := &Error{Code: ErrCodeInvalidConfig}
		if !IsPermanent(err) {
			t.Error("IsPermanent() should return true for invalid config")
		}
	})

	t.Run("connection failed (not permanent)", func(t *testing.T) {
		err := &Error{Code: ErrCodeConnectionFailed}
		if IsPermanent(err) {
			t.Error("IsPermanent() should return false for connection failed")
		}
	})

	t.Run("non-Error", func(t *testing.T) {
		err := errors.New("regular error")
		if IsPermanent(err) {
			t.Error("IsPermanent() should return false for non-Error")
		}
	})

	t.Run("nil error", func(t *testing.T) {
		if IsPermanent(nil) {
			t.Error("IsPermanent() should return false for nil")
		}
	})
}

func TestNewError(t *testing.T) {
	err := NewError(ErrCodeConnectionFailed, "connection refused")
	if err.Code != ErrCodeConnectionFailed {
		t.Errorf("Code = %v, want %v", err.Code, ErrCodeConnectionFailed)
	}
	if err.Message != "connection refused" {
		t.Errorf("Message = %v, want %v", err.Message, "connection refused")
	}
	if err.Cause != nil {
		t.Errorf("Cause = %v, want nil", err.Cause)
	}
}

func TestNewErrorWithCause(t *testing.T) {
	cause := errors.New("dial failed")
	err := NewErrorWithCause(ErrCodeConnectionFailed, "connection refused", cause)

	if err.Code != ErrCodeConnectionFailed {
		t.Errorf("Code = %v, want %v", err.Code, ErrCodeConnectionFailed)
	}
	if err.Message != "connection refused" {
		t.Errorf("Message = %v, want %v", err.Message, "connection refused")
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestNewPeerError(t *testing.T) {
	peerID := peer.ID("12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN")
	err := NewPeerError(ErrCodePeerNotFound, "peer not found", peerID)

	if err.Code != ErrCodePeerNotFound {
		t.Errorf("Code = %v, want %v", err.Code, ErrCodePeerNotFound)
	}
	if err.Message != "peer not found" {
		t.Errorf("Message = %v, want %v", err.Message, "peer not found")
	}
	if err.PeerID != peerID {
		t.Errorf("PeerID = %v, want %v", err.PeerID, peerID)
	}
}

func TestNewStreamError(t *testing.T) {
	peerID := peer.ID("12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN")
	streamName := "data"
	err := NewStreamError(ErrCodeStreamClosed, "stream closed", peerID, streamName)

	if err.Code != ErrCodeStreamClosed {
		t.Errorf("Code = %v, want %v", err.Code, ErrCodeStreamClosed)
	}
	if err.Message != "stream closed" {
		t.Errorf("Message = %v, want %v", err.Message, "stream closed")
	}
	if err.PeerID != peerID {
		t.Errorf("PeerID = %v, want %v", err.PeerID, peerID)
	}
	if err.Stream != streamName {
		t.Errorf("Stream = %v, want %v", err.Stream, streamName)
	}
}

func TestError_Fields(t *testing.T) {
	peerID := peer.ID("12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN")
	cause := errors.New("underlying error")

	err := &Error{
		Code:      ErrCodeHandshakeFailed,
		Message:   "handshake failed",
		PeerID:    peerID,
		Stream:    "handshake",
		Cause:     cause,
		Retriable: true,
	}

	if err.Code != ErrCodeHandshakeFailed {
		t.Errorf("Code = %v, want %v", err.Code, ErrCodeHandshakeFailed)
	}
	if err.Message != "handshake failed" {
		t.Errorf("Message = %v, want %v", err.Message, "handshake failed")
	}
	if err.PeerID != peerID {
		t.Errorf("PeerID = %v, want %v", err.PeerID, peerID)
	}
	if err.Stream != "handshake" {
		t.Errorf("Stream = %v, want %v", err.Stream, "handshake")
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
	if !err.Retriable {
		t.Error("Retriable should be true")
	}
}
