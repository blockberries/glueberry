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
	DefaultHighWatermark           = 1000    // Messages per stream before backpressure
	DefaultLowWatermark            = 100     // Messages per stream to resume sending
	DefaultMaxMessageSize          = 1 << 20 // 1MB max message size
	DefaultConnMgrLowWatermark     = 100     // Connection manager low watermark
	DefaultConnMgrHighWatermark    = 400     // Connection manager high watermark
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

	// HighWatermark is the number of pending messages per stream before
	// backpressure engages. When reached, Send operations will block until
	// messages are acknowledged. Set to 0 to use DefaultHighWatermark.
	HighWatermark int

	// LowWatermark is the number of pending messages per stream at which
	// backpressure disengages, allowing sends to proceed again.
	// Set to 0 to use DefaultLowWatermark.
	LowWatermark int

	// MaxMessageSize is the maximum size in bytes of a single message.
	// Messages exceeding this size will be rejected. Set to 0 to use
	// DefaultMaxMessageSize (1MB).
	MaxMessageSize int

	// DisableBackpressure controls whether flow control is disabled.
	// When false (default), sends will block when the high watermark is reached.
	// When true, backpressure is disabled and messages may be dropped if buffers are full.
	DisableBackpressure bool

	// ConnMgrLowWatermark is the low watermark for the libp2p connection manager.
	// The connection manager will start pruning connections when the number of
	// connections exceeds ConnMgrHighWatermark, down to this level.
	// Set to 0 to use DefaultConnMgrLowWatermark (100).
	ConnMgrLowWatermark int

	// ConnMgrHighWatermark is the high watermark for the libp2p connection manager.
	// When the number of connections exceeds this, the connection manager will
	// prune connections down to ConnMgrLowWatermark.
	// Set to 0 to use DefaultConnMgrHighWatermark (400).
	ConnMgrHighWatermark int
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
	if c.HighWatermark < 0 {
		return fmt.Errorf("%w: high watermark cannot be negative", ErrInvalidConfig)
	}
	if c.LowWatermark < 0 {
		return fmt.Errorf("%w: low watermark cannot be negative", ErrInvalidConfig)
	}
	if c.LowWatermark > 0 && c.HighWatermark > 0 && c.LowWatermark >= c.HighWatermark {
		return fmt.Errorf("%w: low watermark must be less than high watermark", ErrInvalidConfig)
	}
	if c.MaxMessageSize < 0 {
		return fmt.Errorf("%w: max message size cannot be negative", ErrInvalidConfig)
	}
	if c.ConnMgrLowWatermark < 0 {
		return fmt.Errorf("%w: connection manager low watermark cannot be negative", ErrInvalidConfig)
	}
	if c.ConnMgrHighWatermark < 0 {
		return fmt.Errorf("%w: connection manager high watermark cannot be negative", ErrInvalidConfig)
	}
	if c.ConnMgrLowWatermark > 0 && c.ConnMgrHighWatermark > 0 && c.ConnMgrLowWatermark >= c.ConnMgrHighWatermark {
		return fmt.Errorf("%w: connection manager low watermark must be less than high watermark", ErrInvalidConfig)
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
	if c.HighWatermark == 0 {
		c.HighWatermark = DefaultHighWatermark
	}
	if c.LowWatermark == 0 {
		c.LowWatermark = DefaultLowWatermark
	}
	if c.MaxMessageSize == 0 {
		c.MaxMessageSize = DefaultMaxMessageSize
	}
	if c.ConnMgrLowWatermark == 0 {
		c.ConnMgrLowWatermark = DefaultConnMgrLowWatermark
	}
	if c.ConnMgrHighWatermark == 0 {
		c.ConnMgrHighWatermark = DefaultConnMgrHighWatermark
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

// WithHighWatermark sets the high watermark for flow control.
// When the number of pending messages reaches this value, new sends will block.
func WithHighWatermark(n int) ConfigOption {
	return func(c *Config) {
		c.HighWatermark = n
	}
}

// WithLowWatermark sets the low watermark for flow control.
// When the number of pending messages drops to this value, blocked sends can proceed.
func WithLowWatermark(n int) ConfigOption {
	return func(c *Config) {
		c.LowWatermark = n
	}
}

// WithMaxMessageSize sets the maximum size of a single message in bytes.
func WithMaxMessageSize(n int) ConfigOption {
	return func(c *Config) {
		c.MaxMessageSize = n
	}
}

// WithBackpressureDisabled disables flow control backpressure.
// When disabled, sends will not block when buffers are full.
func WithBackpressureDisabled() ConfigOption {
	return func(c *Config) {
		c.DisableBackpressure = true
	}
}

// WithConnMgrLowWatermark sets the connection manager low watermark.
// The connection manager will prune connections down to this level when
// the high watermark is exceeded.
func WithConnMgrLowWatermark(n int) ConfigOption {
	return func(c *Config) {
		c.ConnMgrLowWatermark = n
	}
}

// WithConnMgrHighWatermark sets the connection manager high watermark.
// When the number of connections exceeds this value, the connection manager
// will start pruning connections down to the low watermark.
func WithConnMgrHighWatermark(n int) ConfigOption {
	return func(c *Config) {
		c.ConnMgrHighWatermark = n
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
