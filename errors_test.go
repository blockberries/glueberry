package glueberry

import (
	"errors"
	"testing"
)

func TestErrorsAreSentinels(t *testing.T) {
	// Verify all errors are distinct and can be used with errors.Is
	allErrors := []error{
		// Peer and address book errors
		ErrPeerNotFound,
		ErrPeerBlacklisted,
		ErrPeerAlreadyExists,
		// Connection errors
		ErrNotConnected,
		ErrAlreadyConnected,
		ErrConnectionFailed,
		ErrHandshakeTimeout,
		ErrHandshakeNotComplete,
		ErrReconnectionCancelled,
		ErrInCooldown,
		// Stream errors
		ErrStreamNotFound,
		ErrStreamClosed,
		ErrStreamsAlreadyEstablished,
		ErrNoStreamsRequested,
		// Crypto errors
		ErrInvalidPublicKey,
		ErrInvalidPrivateKey,
		ErrEncryptionFailed,
		ErrDecryptionFailed,
		ErrKeyDerivationFailed,
		// Config errors
		ErrInvalidConfig,
		ErrMissingPrivateKey,
		ErrMissingAddressBookPath,
		ErrMissingListenAddrs,
		// Node errors
		ErrNodeNotStarted,
		ErrNodeAlreadyStarted,
		ErrNodeStopped,
	}

	// Each error should match itself
	for _, err := range allErrors {
		if !errors.Is(err, err) {
			t.Errorf("error %v should match itself with errors.Is", err)
		}
	}

	// Each error should be distinct from others
	for i, err1 := range allErrors {
		for j, err2 := range allErrors {
			if i != j && errors.Is(err1, err2) {
				t.Errorf("error %v should not match %v", err1, err2)
			}
		}
	}
}

func TestErrorsHaveMessages(t *testing.T) {
	// Verify all errors have non-empty messages
	allErrors := []error{
		ErrPeerNotFound,
		ErrPeerBlacklisted,
		ErrPeerAlreadyExists,
		ErrNotConnected,
		ErrAlreadyConnected,
		ErrConnectionFailed,
		ErrHandshakeTimeout,
		ErrHandshakeNotComplete,
		ErrReconnectionCancelled,
		ErrInCooldown,
		ErrStreamNotFound,
		ErrStreamClosed,
		ErrStreamsAlreadyEstablished,
		ErrNoStreamsRequested,
		ErrInvalidPublicKey,
		ErrInvalidPrivateKey,
		ErrEncryptionFailed,
		ErrDecryptionFailed,
		ErrKeyDerivationFailed,
		ErrInvalidConfig,
		ErrMissingPrivateKey,
		ErrMissingAddressBookPath,
		ErrMissingListenAddrs,
		ErrNodeNotStarted,
		ErrNodeAlreadyStarted,
		ErrNodeStopped,
	}

	for _, err := range allErrors {
		if err.Error() == "" {
			t.Errorf("error %v should have a non-empty message", err)
		}
	}
}
