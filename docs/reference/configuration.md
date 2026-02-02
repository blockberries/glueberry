# Configuration Reference

Complete reference for Glueberry configuration options.

---

## Config Struct

```go
type Config struct {
    // Required Fields
    PrivateKey      ed25519.PrivateKey
    AddressBookPath string
    ListenAddrs     []multiaddr.Multiaddr

    // Optional Fields (with defaults)
    Logger                   Logger
    Metrics                  Metrics
    HandshakeTimeout         time.Duration
    MaxMessageSize           int
    ReconnectInitialBackoff  time.Duration
    ReconnectMaxBackoff      time.Duration
    ReconnectMaxAttempts     int
    EventBufferSize          int
    MessageBufferSize        int
    DisableBackpressure      bool
    HighWatermark            int64
    LowWatermark             int64
    MaxStreamNameLength      int
    OnDecryptionError        func(peer.ID, error)
}
```

---

## Required Fields

### PrivateKey

```go
PrivateKey ed25519.PrivateKey
```

**Description:** Ed25519 private key for node identity (32 bytes seed + 32 bytes public key).

**Generation:**
```go
import (
    "crypto/ed25519"
    "crypto/rand"
)

publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
```

**Security:**
- **Never commit private keys** to version control
- Store securely (environment variables, key management service, etc.)
- Generate fresh keys for testing

### AddressBookPath

```go
AddressBookPath string
```

**Description:** File path for JSON-persisted peer address book.

**Example:**
```go
AddressBookPath: "./data/addressbook.json"
AddressBookPath: "/var/lib/myapp/addressbook.json"
AddressBookPath: os.Getenv("HOME") + "/.myapp/addressbook.json"
```

**Notes:**
- Parent directory must exist or be writable
- File is created if it doesn't exist
- Automatically flushed every 30 seconds

### ListenAddrs

```go
ListenAddrs []multiaddr.Multiaddr
```

**Description:** List of multiaddresses to listen on for incoming connections.

**Examples:**
```go
// Listen on all interfaces, port 9000
listenAddr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9000")

// Listen on specific IP
listenAddr, _ := multiaddr.NewMultiaddr("/ip4/192.168.1.100/tcp/9000")

// Listen on random port (OS-assigned)
listenAddr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/0")

// Multiple addresses
listenAddrs := []multiaddr.Multiaddr{
    multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9000"),
    multiaddr.NewMultiaddr("/ip6/::/tcp/9000"),
}
```

---

## Optional Fields

### Logger

```go
Logger Logger
```

**Default:** `NopLogger{}` (discards all logs)

**Description:** Structured logger for node operations.

**Compatible Loggers:**
- `log/slog` (Go 1.21+)
- `go.uber.org/zap`
- `github.com/rs/zerolog`

**Example (slog):**
```go
import "log/slog"

logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug,
}))

cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithLogger(logger),
)
```

**Log Levels:**
- **Debug:** Verbose diagnostics (stream operations, crypto)
- **Info:** Significant events (connections, handshakes)
- **Warn:** Recoverable issues (handshake failures, decryption errors)
- **Error:** Serious errors (connection failures, crypto errors)

### Metrics

```go
Metrics Metrics
```

**Default:** `NopMetrics{}` (discards all metrics)

**Description:** Metrics collector for Prometheus-style metrics.

**Example (Prometheus):**
```go
import "github.com/blockberries/glueberry/internal/observability/prometheus"

metrics := prometheus.NewMetrics()

cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithMetrics(metrics),
)
```

**Metrics Collected:**
- Connection lifecycle (opened, closed, attempts)
- Handshake duration and results
- Message send/receive (bytes, counts)
- Encryption/decryption errors
- Backpressure events
- Event/message drops

### HandshakeTimeout

```go
HandshakeTimeout time.Duration
```

**Default:** `60 * time.Second`

**Description:** Maximum time allowed for handshake completion. If exceeded, connection transitions to `StateCooldown`.

**Examples:**
```go
// Short timeout for local testing
glueberry.WithHandshakeTimeout(10 * time.Second)

// Long timeout for high-latency networks
glueberry.WithHandshakeTimeout(2 * time.Minute)

// Disable timeout (not recommended)
glueberry.WithHandshakeTimeout(0) // 0 = no timeout
```

**Recommendations:**
- **Local network:** 30-60 seconds
- **Internet:** 60-120 seconds
- **High-latency:** 2-5 minutes

### MaxMessageSize

```go
MaxMessageSize int
```

**Default:** `10 * 1024 * 1024` (10MB)

**Description:** Maximum message size in bytes. Messages exceeding this are rejected.

**Examples:**
```go
// Small messages only (1MB)
glueberry.WithMaxMessageSize(1 * 1024 * 1024)

// Large messages (50MB)
glueberry.WithMaxMessageSize(50 * 1024 * 1024)

// Very large (100MB) - use with caution
glueberry.WithMaxMessageSize(100 * 1024 * 1024)
```

**Recommendations:**
- **Chat/messaging:** 1-10 MB
- **File transfer:** 50-100 MB
- **Streaming:** Consider chunking instead of large messages

### ReconnectInitialBackoff

```go
ReconnectInitialBackoff time.Duration
```

**Default:** `1 * time.Second`

**Description:** Initial delay before first reconnection attempt.

**Example:**
```go
glueberry.WithReconnectInitialBackoff(500 * time.Millisecond)
```

### ReconnectMaxBackoff

```go
ReconnectMaxBackoff time.Duration
```

**Default:** `5 * time.Minute`

**Description:** Maximum delay between reconnection attempts (exponential backoff cap).

**Example:**
```go
glueberry.WithReconnectMaxBackoff(10 * time.Minute)
```

**Backoff Formula:**
```
backoff = min(InitialBackoff * 2^attempt, MaxBackoff) * (1 + jitter)
```

Where `jitter` is a random value in range `[-0.25, +0.25]`.

### ReconnectMaxAttempts

```go
ReconnectMaxAttempts int
```

**Default:** `10`

**Description:** Maximum number of reconnection attempts before giving up.

**Examples:**
```go
// Retry forever (not recommended)
glueberry.WithReconnectMaxAttempts(0) // 0 = unlimited

// Few retries (fast failure)
glueberry.WithReconnectMaxAttempts(3)

// Many retries (persistent)
glueberry.WithReconnectMaxAttempts(20)
```

### EventBufferSize

```go
EventBufferSize int
```

**Default:** `100`

**Description:** Buffer size for connection event channels.

**Tuning:**
- **Low activity:** 50-100 events
- **High activity:** 200-500 events
- **Burst traffic:** 500-1000 events

**Trade-offs:**
- **Larger buffer:** Tolerates burst traffic, uses more memory
- **Smaller buffer:** Lower memory, may drop events under load

### MessageBufferSize

```go
MessageBufferSize int
```

**Default:** `1000`

**Description:** Buffer size for incoming message channels.

**Tuning:**
- **Low throughput:** 100-500 messages
- **Medium throughput:** 1000-5000 messages
- **High throughput:** 5000-10000 messages

### DisableBackpressure

```go
DisableBackpressure bool
```

**Default:** `false` (backpressure enabled)

**Description:** If `true`, disables flow control (backpressure mechanism).

**When to Disable:**
- Low message rates
- Messages are small
- Send failures are acceptable

**When to Enable (default):**
- High message rates
- Risk of overwhelming peers
- Need delivery guarantees

### HighWatermark

```go
HighWatermark int64
```

**Default:** `1000`

**Description:** Number of pending messages that triggers backpressure (blocks sending).

**Tuning:**
```go
// Low memory, strict backpressure
glueberry.WithHighWatermark(500)

// High throughput, relaxed backpressure
glueberry.WithHighWatermark(5000)
```

### LowWatermark

```go
LowWatermark int64
```

**Default:** `500`

**Description:** Number of pending messages at which backpressure is released.

**Relationship:** Must satisfy `LowWatermark < HighWatermark`

**Tuning:**
```go
// Tight hysteresis (frequent blocking/unblocking)
glueberry.WithHighWatermark(1000)
glueberry.WithLowWatermark(900)

// Wide hysteresis (stable blocking/unblocking)
glueberry.WithHighWatermark(1000)
glueberry.WithLowWatermark(500)
```

### MaxStreamNameLength

```go
MaxStreamNameLength int
```

**Default:** `128`

**Description:** Maximum length for stream names.

**Example:**
```go
glueberry.WithMaxStreamNameLength(256)
```

### OnDecryptionError

```go
OnDecryptionError func(peer.ID, error)
```

**Default:** `nil` (no callback)

**Description:** Callback invoked when message decryption fails.

**Use Cases:**
- Log decryption failures
- Blacklist peers sending invalid encrypted data
- Trigger alerts

**Example:**
```go
cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithOnDecryptionError(func(peerID peer.ID, err error) {
        log.Printf("Decryption error from %s: %v", peerID, err)
        // Optionally blacklist peer after N failures
    }),
)
```

---

## Configuration Patterns

### Development Configuration

```go
cfg := glueberry.NewConfig(
    privateKey,
    "./addressbook.json",
    []multiaddr.Multiaddr{listenAddr},
    glueberry.WithLogger(slog.Default()),
    glueberry.WithHandshakeTimeout(10*time.Second), // Short timeout
    glueberry.WithReconnectMaxAttempts(3),          // Fast failure
)
```

### Production Configuration

```go
cfg := glueberry.NewConfig(
    privateKey,
    "/var/lib/myapp/addressbook.json",
    listenAddrs,
    glueberry.WithLogger(productionLogger),
    glueberry.WithMetrics(prometheusMetrics),
    glueberry.WithHandshakeTimeout(120*time.Second), // Generous timeout
    glueberry.WithReconnectMaxAttempts(20),          // Persistent
    glueberry.WithMaxMessageSize(50*1024*1024),      // 50MB
    glueberry.WithHighWatermark(5000),               // High throughput
    glueberry.WithLowWatermark(2500),
)
```

### High-Performance Configuration

```go
cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithMessageBufferSize(10000),     // Large buffer
    glueberry.WithEventBufferSize(1000),
    glueberry.WithDisableBackpressure(false),   // Keep enabled
    glueberry.WithHighWatermark(10000),         // Very high
    glueberry.WithLowWatermark(5000),
)
```

### Low-Memory Configuration

```go
cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithMessageBufferSize(100),       // Small buffer
    glueberry.WithEventBufferSize(50),
    glueberry.WithHighWatermark(500),           // Strict backpressure
    glueberry.WithLowWatermark(250),
)
```

---

## Environment Variable Integration

Glueberry doesn't read environment variables directly. Applications should handle this:

```go
func configFromEnv() *glueberry.Config {
    // Private key
    keyBytes, _ := hex.DecodeString(os.Getenv("GLUEBERRY_PRIVATE_KEY"))
    privateKey := ed25519.PrivateKey(keyBytes)

    // Listen address
    listenAddr, _ := multiaddr.NewMultiaddr(
        os.Getenv("GLUEBERRY_LISTEN_ADDR"),
    )

    // Address book path
    addressBookPath := os.Getenv("GLUEBERRY_ADDRESS_BOOK_PATH")

    // Handshake timeout
    timeout := 60 * time.Second
    if val := os.Getenv("GLUEBERRY_HANDSHAKE_TIMEOUT"); val != "" {
        if d, err := time.ParseDuration(val); err == nil {
            timeout = d
        }
    }

    return glueberry.NewConfig(
        privateKey,
        addressBookPath,
        []multiaddr.Multiaddr{listenAddr},
        glueberry.WithHandshakeTimeout(timeout),
    )
}
```

---

## Validation

All configuration is validated in `cfg.Validate()` called by `New()`:

```go
func (c *Config) Validate() error {
    if c.PrivateKey == nil {
        return fmt.Errorf("private key is required")
    }
    if c.AddressBookPath == "" {
        return fmt.Errorf("address book path is required")
    }
    if len(c.ListenAddrs) == 0 {
        return fmt.Errorf("at least one listen address is required")
    }
    if c.HandshakeTimeout < 0 {
        return fmt.Errorf("handshake timeout must be non-negative")
    }
    if c.MaxMessageSize <= 0 {
        return fmt.Errorf("max message size must be positive")
    }
    if c.LowWatermark >= c.HighWatermark {
        return fmt.Errorf("low watermark must be < high watermark")
    }
    // ... more validation
    return nil
}
```

---

## Summary

✅ Three required fields: `PrivateKey`, `AddressBookPath`, `ListenAddrs`
✅ Sensible defaults for all optional fields
✅ Functional options pattern for customization
✅ Validation on node creation
✅ Environment variable integration (app-level)
✅ Configuration patterns for different scenarios

For more details, see [ARCHITECTURE.md - Configuration](../../ARCHITECTURE.md#8-configuration).
