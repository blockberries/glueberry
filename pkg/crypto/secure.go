// Package crypto provides cryptographic operations for Glueberry including
// key conversion, ECDH key exchange, and symmetric encryption.
package crypto

// SecureZero overwrites the provided byte slice with zeros to prevent
// sensitive key material from lingering in memory.
//
// This function should be called on all sensitive data (private keys,
// shared secrets, derived keys) when they are no longer needed.
//
// Note: Go's garbage collector does not guarantee memory is zeroed when
// freed, so explicit zeroing is necessary for security-sensitive data.
// While this provides defense in depth, it cannot prevent all memory
// disclosure scenarios (e.g., memory dumps taken before zeroing).
func SecureZero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// SecureZeroMultiple zeros multiple byte slices.
// Useful for cleaning up multiple keys at once.
func SecureZeroMultiple(slices ...[]byte) {
	for _, b := range slices {
		SecureZero(b)
	}
}
