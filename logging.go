package glueberry

// Logger defines the logging interface for Glueberry.
// It is designed to be compatible with standard logging libraries
// such as slog, zap, and zerolog.
//
// Implementations must be safe for concurrent use.
type Logger interface {
	// Debug logs a debug-level message with optional key-value pairs.
	// Used for verbose diagnostic information useful during development.
	Debug(msg string, keysAndValues ...any)

	// Info logs an info-level message with optional key-value pairs.
	// Used for significant events like connection establishment.
	Info(msg string, keysAndValues ...any)

	// Warn logs a warning-level message with optional key-value pairs.
	// Used for recoverable issues like failed handshakes.
	Warn(msg string, keysAndValues ...any)

	// Error logs an error-level message with optional key-value pairs.
	// Used for serious errors that may impact functionality.
	Error(msg string, keysAndValues ...any)
}

// NopLogger is a no-op logger implementation that discards all log messages.
// It is the default logger when no logger is configured.
type NopLogger struct{}

// Ensure NopLogger implements Logger.
var _ Logger = NopLogger{}

// Debug implements Logger.Debug (no-op).
func (NopLogger) Debug(msg string, keysAndValues ...any) {}

// Info implements Logger.Info (no-op).
func (NopLogger) Info(msg string, keysAndValues ...any) {}

// Warn implements Logger.Warn (no-op).
func (NopLogger) Warn(msg string, keysAndValues ...any) {}

// Error implements Logger.Error (no-op).
func (NopLogger) Error(msg string, keysAndValues ...any) {}
