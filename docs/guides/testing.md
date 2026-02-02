# Testing Guide for Glueberry Applications

This guide covers best practices for testing applications built with Glueberry.

---

## Table of Contents

- [Unit Testing](#unit-testing)
- [Integration Testing](#integration-testing)
- [Testing Handshakes](#testing-handshakes)
- [Mocking](#mocking)
- [Benchmarking](#benchmarking)
- [Test Utilities](#test-utilities)
- [CI/CD Integration](#cicd-integration)

---

## Unit Testing

### Testing Node Creation

```go
func TestNodeCreation(t *testing.T) {
    _, privateKey, err := ed25519.GenerateKey(rand.Reader)
    require.NoError(t, err)

    listenAddr, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
    require.NoError(t, err)

    cfg := glueberry.NewConfig(
        privateKey,
        t.TempDir() + "/addressbook.json",
        []multiaddr.Multiaddr{listenAddr},
    )

    node, err := glueberry.New(cfg)
    require.NoError(t, err)
    defer node.Stop()

    assert.NotNil(t, node)
    assert.Equal(t, privateKey.Public(), node.PublicKey())
}
```

### Testing Configuration Validation

```go
func TestConfigValidation(t *testing.T) {
    tests := []struct {
        name    string
        cfg     *glueberry.Config
        wantErr bool
    }{
        {
            name: "valid config",
            cfg: &glueberry.Config{
                PrivateKey:      testPrivateKey(),
                AddressBookPath: "/tmp/addressbook.json",
                ListenAddrs:     []multiaddr.Multiaddr{testMultiaddr()},
            },
            wantErr: false,
        },
        {
            name: "missing private key",
            cfg: &glueberry.Config{
                AddressBookPath: "/tmp/addressbook.json",
                ListenAddrs:     []multiaddr.Multiaddr{testMultiaddr()},
            },
            wantErr: true,
        },
        {
            name: "missing listen addrs",
            cfg: &glueberry.Config{
                PrivateKey:      testPrivateKey(),
                AddressBookPath: "/tmp/addressbook.json",
            },
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.cfg.Validate()
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

---

## Integration Testing

### Two-Node Connection Test

```go
func TestTwoNodeConnection(t *testing.T) {
    // Create node A
    nodeA := createTestNode(t, 0) // Port 0 = random port
    defer nodeA.Stop()

    err := nodeA.Start()
    require.NoError(t, err)

    // Create node B
    nodeB := createTestNode(t, 0)
    defer nodeB.Stop()

    err = nodeB.Start()
    require.NoError(t, err)

    // Add node A to node B's address book
    nodeB.AddPeer(nodeA.PeerID(), nodeA.Addrs(), nil)

    // Connect B → A
    err = nodeB.Connect(nodeA.PeerID())
    require.NoError(t, err)

    // Wait for connection
    assert.Eventually(t, func() bool {
        state := nodeB.ConnectionState(nodeA.PeerID())
        return state == glueberry.StateConnected
    }, 5*time.Second, 100*time.Millisecond, "Should reach StateConnected")
}

func createTestNode(t *testing.T, port int) *glueberry.Node {
    _, privateKey, err := ed25519.GenerateKey(rand.Reader)
    require.NoError(t, err)

    listenAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port))
    require.NoError(t, err)

    cfg := glueberry.NewConfig(
        privateKey,
        t.TempDir() + "/addressbook.json",
        []multiaddr.Multiaddr{listenAddr},
    )

    node, err := glueberry.New(cfg)
    require.NoError(t, err)

    return node
}
```

---

## Testing Handshakes

### Complete Handshake Test

```go
func TestCompleteHandshake(t *testing.T) {
    nodeA := createTestNode(t, 0)
    nodeB := createTestNode(t, 0)
    defer nodeA.Stop()
    defer nodeB.Stop()

    require.NoError(t, nodeA.Start())
    require.NoError(t, nodeB.Start())

    // Setup handshake handlers
    pubKeyA := nodeA.PublicKey()
    pubKeyB := nodeB.PublicKey()

    handshakeA := newHandshakeHandler(nodeA, pubKeyA)
    handshakeB := newHandshakeHandler(nodeB, pubKeyB)

    go handshakeA.run()
    go handshakeB.run()

    // Connect A → B
    nodeA.AddPeer(nodeB.PeerID(), nodeB.Addrs(), nil)
    require.NoError(t, nodeA.Connect(nodeB.PeerID()))

    // Wait for handshake completion
    assert.Eventually(t, func() bool {
        return nodeA.ConnectionState(nodeB.PeerID()) == glueberry.StateEstablished
    }, 10*time.Second, 100*time.Millisecond)

    assert.Eventually(t, func() bool {
        return nodeB.ConnectionState(nodeA.PeerID()) == glueberry.StateEstablished
    }, 10*time.Second, 100*time.Millisecond)
}

type handshakeHandler struct {
    node      *glueberry.Node
    publicKey ed25519.PublicKey
}

func newHandshakeHandler(node *glueberry.Node, publicKey ed25519.PublicKey) *handshakeHandler {
    return &handshakeHandler{node: node, publicKey: publicKey}
}

func (h *handshakeHandler) run() {
    // Implement handshake protocol (see examples/simple-chat)
    // ...
}
```

### Testing Handshake Timeout

```go
func TestHandshakeTimeout(t *testing.T) {
    nodeA := createTestNode(t, 0)
    nodeB := createTestNode(t, 0)
    defer nodeA.Stop()
    defer nodeB.Stop()

    // Set short timeout for test
    cfg := nodeB.(*glueberry.Node).Config()
    cfg.HandshakeTimeout = 2 * time.Second

    require.NoError(t, nodeA.Start())
    require.NoError(t, nodeB.Start())

    // Connect but DON'T complete handshake
    nodeA.AddPeer(nodeB.PeerID(), nodeB.Addrs(), nil)
    require.NoError(t, nodeA.Connect(nodeB.PeerID()))

    // Wait for timeout
    assert.Eventually(t, func() bool {
        state := nodeA.ConnectionState(nodeB.PeerID())
        return state == glueberry.StateCooldown
    }, 5*time.Second, 100*time.Millisecond)
}
```

---

## Mocking

### Mock Logger

```go
type mockLogger struct {
    mu   sync.Mutex
    logs []logEntry
}

type logEntry struct {
    level  string
    msg    string
    fields map[string]interface{}
}

func newMockLogger() *mockLogger {
    return &mockLogger{logs: []logEntry{}}
}

func (l *mockLogger) Debug(msg string, keysAndValues ...any) {
    l.log("debug", msg, keysAndValues)
}

func (l *mockLogger) Info(msg string, keysAndValues ...any) {
    l.log("info", msg, keysAndValues)
}

func (l *mockLogger) Warn(msg string, keysAndValues ...any) {
    l.log("warn", msg, keysAndValues)
}

func (l *mockLogger) Error(msg string, keysAndValues ...any) {
    l.log("error", msg, keysAndValues)
}

func (l *mockLogger) log(level, msg string, keysAndValues []any) {
    l.mu.Lock()
    defer l.mu.Unlock()

    fields := make(map[string]interface{})
    for i := 0; i < len(keysAndValues); i += 2 {
        if i+1 < len(keysAndValues) {
            fields[keysAndValues[i].(string)] = keysAndValues[i+1]
        }
    }

    l.logs = append(l.logs, logEntry{level: level, msg: msg, fields: fields})
}

func (l *mockLogger) assertLogged(t *testing.T, level, msgContains string) {
    l.mu.Lock()
    defer l.mu.Unlock()

    for _, entry := range l.logs {
        if entry.level == level && strings.Contains(entry.msg, msgContains) {
            return
        }
    }
    t.Errorf("Expected log entry with level=%s msg contains=%q not found", level, msgContains)
}
```

### Mock Metrics

```go
type mockMetrics struct {
    mu                sync.Mutex
    connectionsOpened int
    messagesSent      map[string]int
}

func newMockMetrics() *mockMetrics {
    return &mockMetrics{messagesSent: make(map[string]int)}
}

func (m *mockMetrics) ConnectionOpened(direction string) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.connectionsOpened++
}

func (m *mockMetrics) MessageSent(stream string, bytes int) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.messagesSent[stream] += bytes
}

// ... implement other Metrics methods
```

---

## Benchmarking

### Benchmark Node Creation

```go
func BenchmarkNodeCreation(b *testing.B) {
    _, privateKey, _ := ed25519.GenerateKey(rand.Reader)
    listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        cfg := glueberry.NewConfig(
            privateKey,
            "/tmp/addressbook.json",
            []multiaddr.Multiaddr{listenAddr},
        )
        node, _ := glueberry.New(cfg)
        node.Stop()
    }
}
```

### Benchmark Message Send

```go
func BenchmarkMessageSend(b *testing.B) {
    nodeA, nodeB := setupConnectedNodes(b)
    defer nodeA.Stop()
    defer nodeB.Stop()

    // Complete handshake
    completeHandshake(b, nodeA, nodeB)

    // Prepare test message
    msg := make([]byte, 1024)
    rand.Read(msg)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        err := nodeA.Send(nodeB.PeerID(), "messages", msg)
        if err != nil {
            b.Fatalf("Send failed: %v", err)
        }
    }
}
```

---

## Test Utilities

### Helper Functions

```go
// testPrivateKey generates a test Ed25519 private key
func testPrivateKey() ed25519.PrivateKey {
    _, priv, _ := ed25519.GenerateKey(rand.Reader)
    return priv
}

// testMultiaddr creates a test multiaddr
func testMultiaddr() multiaddr.Multiaddr {
    addr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
    return addr
}

// waitForState waits for a peer to reach a specific connection state
func waitForState(t *testing.T, node *glueberry.Node, peerID peer.ID, state glueberry.ConnectionState, timeout time.Duration) {
    assert.Eventually(t, func() bool {
        return node.ConnectionState(peerID) == state
    }, timeout, 100*time.Millisecond, "Expected state %v", state)
}

// drainMessages drains messages from a channel with timeout
func drainMessages(msgCh <-chan streams.IncomingMessage, timeout time.Duration) []streams.IncomingMessage {
    var msgs []streams.IncomingMessage
    timer := time.After(timeout)
    for {
        select {
        case msg := <-msgCh:
            msgs = append(msgs, msg)
        case <-timer:
            return msgs
        }
    }
}
```

---

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.25'

      - name: Run tests
        run: go test -v -race -coverprofile=coverage.out ./...

      - name: Upload coverage
        uses: codecov/codecov-action@v3
        with:
          files: ./coverage.out
```

### Makefile Targets

```makefile
.PHONY: test test-short test-race test-integration test-bench

test:
\tgo test -v -race -coverprofile=coverage.out ./...

test-short:
\tgo test -v -short ./...

test-race:
\tgo test -v -race ./...

test-integration:
\tgo test -v -run TestIntegration ./...

test-bench:
\tgo test -bench=. -benchmem ./...
```

---

## Best Practices

### 1. Use Table-Driven Tests

```go
func TestSend(t *testing.T) {
    tests := []struct {
        name    string
        stream  string
        data    []byte
        wantErr bool
    }{
        {"valid message", "messages", []byte("hello"), false},
        {"empty message", "messages", []byte{}, false},
        {"large message", "messages", make([]byte, 1024*1024), false},
        {"invalid stream", "", []byte("hello"), true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := node.Send(peerID, tt.stream, tt.data)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### 2. Use testify for Assertions

```go
import (
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestExample(t *testing.T) {
    // Use require for fatal errors (stops test)
    node, err := glueberry.New(cfg)
    require.NoError(t, err)
    require.NotNil(t, node)

    // Use assert for non-fatal checks (continues test)
    assert.Equal(t, expectedValue, actualValue)
}
```

### 3. Clean Up Resources

```go
func TestWithCleanup(t *testing.T) {
    node := createTestNode(t, 0)
    t.Cleanup(func() {
        node.Stop()
    })

    // Or use defer
    defer node.Stop()

    // Test code here
}
```

### 4. Use Temporary Directories

```go
func TestAddressBook(t *testing.T) {
    tempDir := t.TempDir() // Auto-cleaned up after test
    addressBookPath := tempDir + "/addressbook.json"

    // Use addressBookPath in test
}
```

---

## Summary

✅ Unit tests for individual components
✅ Integration tests for multi-node scenarios
✅ Handshake testing with timeout scenarios
✅ Mocking for logger and metrics
✅ Benchmarking for performance
✅ Test utilities for common patterns
✅ CI/CD integration examples

For more examples, see the Glueberry test suite in `*_test.go` files.
