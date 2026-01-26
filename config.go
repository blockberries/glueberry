package glueberry

import (
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/multiformats/go-multiaddr"
)

// Default configuration values.
const (
	DefaultHandshakeTimeout        = 30 * time.Second
	DefaultReconnectBaseDelay      = 1 * time.Second
	DefaultReconnectMaxDelay       = 5 * time.Minute
	DefaultReconnectMaxAttempts    = 10
	DefaultFailedHandshakeCooldown = 1 * time.Minute
	DefaultEventBufferSize         = 100
	DefaultMessageBufferSize       = 1000
)

// Config holds the configuration for a Glueberry node.
type Config struct {
	// PrivateKey is the Ed25519 private key for this node's identity.
	// This is required and must be provided by the application.
	PrivateKey ed25519.PrivateKey

	// AddressBookPath is the file path for persisting the address book.
	// This is required.
	AddressBookPath string

	// ListenAddrs are the multiaddresses this node will listen on.
	// At least one address is required.
	ListenAddrs []multiaddr.Multiaddr

	// HandshakeTimeout is the maximum duration allowed for completing
	// a handshake after connection. If the application doesn't call
	// EstablishEncryptedStreams within this duration, the connection
	// is dropped and the peer enters cooldown.
	HandshakeTimeout time.Duration

	// ReconnectBaseDelay is the initial delay before the first reconnection attempt.
	ReconnectBaseDelay time.Duration

	// ReconnectMaxDelay is the maximum delay between reconnection attempts
	// after exponential backoff.
	ReconnectMaxDelay time.Duration

	// ReconnectMaxAttempts is the maximum number of reconnection attempts.
	// Set to 0 for unlimited attempts.
	ReconnectMaxAttempts int

	// FailedHandshakeCooldown is the duration to wait after a failed handshake
	// (timeout or error) before allowing reconnection to that peer.
	FailedHandshakeCooldown time.Duration

	// EventBufferSize is the buffer size for the connection events channel.
	EventBufferSize int

	// MessageBufferSize is the buffer size for the incoming messages channel.
	MessageBufferSize int

	// Logger is the logger for the node. If nil, a NopLogger is used.
	// The logger must be safe for concurrent use.
	Logger Logger

	// Metrics is the metrics collector for the node. If nil, a NopMetrics is used.
	// The metrics collector must be safe for concurrent use.
	Metrics Metrics
}

// Validate checks that the configuration is valid and returns an error
// describing any problems found.
func (c *Config) Validate() error {
	if c.PrivateKey == nil {
		return ErrMissingPrivateKey
	}
	if len(c.PrivateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("%w: expected %d bytes, got %d",
			ErrInvalidPrivateKey, ed25519.PrivateKeySize, len(c.PrivateKey))
	}
	if c.AddressBookPath == "" {
		return ErrMissingAddressBookPath
	}
	if len(c.ListenAddrs) == 0 {
		return ErrMissingListenAddrs
	}
	if c.HandshakeTimeout < 0 {
		return fmt.Errorf("%w: handshake timeout cannot be negative", ErrInvalidConfig)
	}
	if c.ReconnectBaseDelay < 0 {
		return fmt.Errorf("%w: reconnect base delay cannot be negative", ErrInvalidConfig)
	}
	if c.ReconnectMaxDelay < 0 {
		return fmt.Errorf("%w: reconnect max delay cannot be negative", ErrInvalidConfig)
	}
	if c.ReconnectMaxDelay > 0 && c.ReconnectMaxDelay < c.ReconnectBaseDelay {
		return fmt.Errorf("%w: reconnect max delay cannot be less than base delay", ErrInvalidConfig)
	}
	if c.ReconnectMaxAttempts < 0 {
		return fmt.Errorf("%w: reconnect max attempts cannot be negative", ErrInvalidConfig)
	}
	if c.FailedHandshakeCooldown < 0 {
		return fmt.Errorf("%w: failed handshake cooldown cannot be negative", ErrInvalidConfig)
	}
	if c.EventBufferSize < 0 {
		return fmt.Errorf("%w: event buffer size cannot be negative", ErrInvalidConfig)
	}
	if c.MessageBufferSize < 0 {
		return fmt.Errorf("%w: message buffer size cannot be negative", ErrInvalidConfig)
	}
	return nil
}

// applyDefaults sets default values for any unset optional fields.
func (c *Config) applyDefaults() {
	if c.HandshakeTimeout == 0 {
		c.HandshakeTimeout = DefaultHandshakeTimeout
	}
	if c.ReconnectBaseDelay == 0 {
		c.ReconnectBaseDelay = DefaultReconnectBaseDelay
	}
	if c.ReconnectMaxDelay == 0 {
		c.ReconnectMaxDelay = DefaultReconnectMaxDelay
	}
	if c.ReconnectMaxAttempts == 0 {
		c.ReconnectMaxAttempts = DefaultReconnectMaxAttempts
	}
	if c.FailedHandshakeCooldown == 0 {
		c.FailedHandshakeCooldown = DefaultFailedHandshakeCooldown
	}
	if c.EventBufferSize == 0 {
		c.EventBufferSize = DefaultEventBufferSize
	}
	if c.MessageBufferSize == 0 {
		c.MessageBufferSize = DefaultMessageBufferSize
	}
	if c.Logger == nil {
		c.Logger = NopLogger{}
	}
	if c.Metrics == nil {
		c.Metrics = NopMetrics{}
	}
}

// ConfigOption is a functional option for configuring a Node.
type ConfigOption func(*Config)

// WithHandshakeTimeout sets the handshake timeout duration.
func WithHandshakeTimeout(d time.Duration) ConfigOption {
	return func(c *Config) {
		c.HandshakeTimeout = d
	}
}

// WithReconnectBaseDelay sets the initial reconnection delay.
func WithReconnectBaseDelay(d time.Duration) ConfigOption {
	return func(c *Config) {
		c.ReconnectBaseDelay = d
	}
}

// WithReconnectMaxDelay sets the maximum reconnection delay after backoff.
func WithReconnectMaxDelay(d time.Duration) ConfigOption {
	return func(c *Config) {
		c.ReconnectMaxDelay = d
	}
}

// WithReconnectMaxAttempts sets the maximum number of reconnection attempts.
// Set to 0 for unlimited attempts.
func WithReconnectMaxAttempts(n int) ConfigOption {
	return func(c *Config) {
		c.ReconnectMaxAttempts = n
	}
}

// WithFailedHandshakeCooldown sets the cooldown duration after a failed handshake.
func WithFailedHandshakeCooldown(d time.Duration) ConfigOption {
	return func(c *Config) {
		c.FailedHandshakeCooldown = d
	}
}

// WithEventBufferSize sets the buffer size for the events channel.
func WithEventBufferSize(size int) ConfigOption {
	return func(c *Config) {
		c.EventBufferSize = size
	}
}

// WithMessageBufferSize sets the buffer size for the messages channel.
func WithMessageBufferSize(size int) ConfigOption {
	return func(c *Config) {
		c.MessageBufferSize = size
	}
}

// WithLogger sets the logger for the node.
// The logger must be safe for concurrent use.
func WithLogger(l Logger) ConfigOption {
	return func(c *Config) {
		c.Logger = l
	}
}

// WithMetrics sets the metrics collector for the node.
// The metrics collector must be safe for concurrent use.
func WithMetrics(m Metrics) ConfigOption {
	return func(c *Config) {
		c.Metrics = m
	}
}

// NewConfig creates a new Config with the required fields and applies
// any provided options. It applies defaults for unset optional fields
// but does not validate the configuration.
func NewConfig(
	privateKey ed25519.PrivateKey,
	addressBookPath string,
	listenAddrs []multiaddr.Multiaddr,
	opts ...ConfigOption,
) *Config {
	c := &Config{
		PrivateKey:      privateKey,
		AddressBookPath: addressBookPath,
		ListenAddrs:     listenAddrs,
	}
	for _, opt := range opts {
		opt(c)
	}
	c.applyDefaults()
	return c
}
