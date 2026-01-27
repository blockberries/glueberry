// Package crypto provides cryptographic operations for Glueberry including
// key conversion, ECDH key exchange, and symmetric encryption.
package crypto

// memclrFunc is a function variable that performs the actual zeroing.
// Using a function variable prevents the Go compiler from recognizing
// the zeroing pattern and potentially optimizing it away as a "dead store"
// when the memory is not read afterward. The compiler cannot devirtualize
// function variable calls or prove the variable won't be reassigned,
// so it must preserve the writes.
//
//nolint:gochecknoglobals // Intentional: prevents compiler optimization
var memclrFunc = func(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// SecureZero overwrites the provided byte slice with zeros to prevent
// sensitive key material from lingering in memory.
//
// This function should be called on all sensitive data (private keys,
// shared secrets, derived keys) when they are no longer needed.
//
// Implementation note: This function uses a function variable indirection
// to prevent the compiler from optimizing away the zeroing. Without this,
// the Go compiler could theoretically eliminate the writes as "dead stores"
// if it can prove the memory is never read afterward.
//
// Note: Go's garbage collector does not guarantee memory is zeroed when
// freed, so explicit zeroing is necessary for security-sensitive data.
// While this provides defense in depth, it cannot prevent all memory
// disclosure scenarios (e.g., memory dumps taken before zeroing).
func SecureZero(b []byte) {
	memclrFunc(b)
}

// SecureZeroMultiple zeros multiple byte slices.
// Useful for cleaning up multiple keys at once.
func SecureZeroMultiple(slices ...[]byte) {
	for _, b := range slices {
		SecureZero(b)
	}
}
