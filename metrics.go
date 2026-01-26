package glueberry

// Metrics defines the metrics collection interface for Glueberry.
// It is designed to be compatible with Prometheus and other metrics systems.
//
// Implementations must be safe for concurrent use.
//
// Metric naming convention:
//   - Counters: <name>_total (e.g., connections_total)
//   - Histograms: <name>_seconds or <name>_bytes (e.g., handshake_duration_seconds)
//   - Gauges: current_<name> (e.g., current_connections)
type Metrics interface {
	// Connection metrics

	// ConnectionOpened increments when a connection is established.
	// Labels: direction (inbound, outbound)
	ConnectionOpened(direction string)

	// ConnectionClosed increments when a connection is closed.
	// Labels: direction (inbound, outbound)
	ConnectionClosed(direction string)

	// ConnectionAttempt records a connection attempt result.
	// Labels: result (success, failure)
	ConnectionAttempt(result string)

	// HandshakeDuration records the duration of a successful handshake.
	HandshakeDuration(seconds float64)

	// HandshakeResult records the result of a handshake attempt.
	// Labels: result (success, failure, timeout)
	HandshakeResult(result string)

	// Stream metrics

	// MessageSent records a message being sent.
	// Labels: stream (the stream name)
	MessageSent(stream string, bytes int)

	// MessageReceived records a message being received.
	// Labels: stream (the stream name)
	MessageReceived(stream string, bytes int)

	// StreamOpened increments when a stream is opened.
	// Labels: stream (the stream name)
	StreamOpened(stream string)

	// StreamClosed increments when a stream is closed.
	// Labels: stream (the stream name)
	StreamClosed(stream string)

	// Crypto metrics

	// EncryptionError records an encryption failure.
	EncryptionError()

	// DecryptionError records a decryption failure.
	DecryptionError()

	// KeyDerivation records a key derivation operation.
	// Labels: cached (true, false)
	KeyDerivation(cached bool)

	// Event metrics

	// EventEmitted records an event being emitted.
	// Labels: state (the connection state)
	EventEmitted(state string)

	// EventDropped records an event being dropped due to buffer full.
	EventDropped()

	// MessageDropped records a message being dropped due to buffer full.
	MessageDropped()
}

// NopMetrics is a no-op metrics implementation that discards all metrics.
// It is the default when no metrics collector is configured.
type NopMetrics struct{}

// Ensure NopMetrics implements Metrics.
var _ Metrics = NopMetrics{}

// ConnectionOpened implements Metrics.ConnectionOpened (no-op).
func (NopMetrics) ConnectionOpened(direction string) {}

// ConnectionClosed implements Metrics.ConnectionClosed (no-op).
func (NopMetrics) ConnectionClosed(direction string) {}

// ConnectionAttempt implements Metrics.ConnectionAttempt (no-op).
func (NopMetrics) ConnectionAttempt(result string) {}

// HandshakeDuration implements Metrics.HandshakeDuration (no-op).
func (NopMetrics) HandshakeDuration(seconds float64) {}

// HandshakeResult implements Metrics.HandshakeResult (no-op).
func (NopMetrics) HandshakeResult(result string) {}

// MessageSent implements Metrics.MessageSent (no-op).
func (NopMetrics) MessageSent(stream string, bytes int) {}

// MessageReceived implements Metrics.MessageReceived (no-op).
func (NopMetrics) MessageReceived(stream string, bytes int) {}

// StreamOpened implements Metrics.StreamOpened (no-op).
func (NopMetrics) StreamOpened(stream string) {}

// StreamClosed implements Metrics.StreamClosed (no-op).
func (NopMetrics) StreamClosed(stream string) {}

// EncryptionError implements Metrics.EncryptionError (no-op).
func (NopMetrics) EncryptionError() {}

// DecryptionError implements Metrics.DecryptionError (no-op).
func (NopMetrics) DecryptionError() {}

// KeyDerivation implements Metrics.KeyDerivation (no-op).
func (NopMetrics) KeyDerivation(cached bool) {}

// EventEmitted implements Metrics.EventEmitted (no-op).
func (NopMetrics) EventEmitted(state string) {}

// EventDropped implements Metrics.EventDropped (no-op).
func (NopMetrics) EventDropped() {}

// MessageDropped implements Metrics.MessageDropped (no-op).
func (NopMetrics) MessageDropped() {}
