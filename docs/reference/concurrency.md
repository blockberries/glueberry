# Concurrency Safety Guide

This guide explains Glueberry's concurrency model and thread-safety guarantees.

---

## Thread-Safety Guarantees

### All Public APIs are Thread-Safe

**All public methods on `Node` are safe to call concurrently from multiple goroutines:**

```go
// Safe to call from multiple goroutines
go func() {
    node.Send(peerID, "stream1", data1)
}()

go func() {
    node.Send(peerID, "stream2", data2)
}()

go func() {
    stats := node.PeerStatistics(peerID)
}()
```

### Channels are Single-Consumer

**Channels returned by `Messages()` and `Events()` should only be read by ONE goroutine:**

```go
// ✅ CORRECT: Single consumer
go func() {
    for msg := range node.Messages() {
        handleMessage(msg)
    }
}()

// ❌ INCORRECT: Multiple consumers reading from same channel
go func() {
    for msg := range node.Messages() { // BAD!
        handleMessage1(msg)
    }
}()
go func() {
    for msg := range node.Messages() { // BAD! - same channel
        handleMessage2(msg)
    }
}()
```

**Solution: Use event subscriptions for multiple consumers:**

```go
// ✅ CORRECT: Multiple subscriptions
sub1 := node.SubscribeEvents()
sub2 := node.SubscribeEvents()

go func() {
    for evt := range sub1.Events() {
        handleEvents1(evt)
    }
}()

go func() {
    for evt := range sub2.Events() {
        handleEvents2(evt)
    }
}()
```

---

## Concurrency Patterns

### Pattern 1: Fan-Out Event Handling

Use subscriptions to fan out events to multiple handlers:

```go
// Connection monitor
sub1 := node.SubscribeEvents()
go func() {
    for evt := range sub1.Events() {
        if evt.State == glueberry.StateDisconnected {
            log.Printf("Connection lost: %s", evt.PeerID)
        }
    }
}()

// Handshake initiator
sub2 := node.EventsForStates(glueberry.StateConnected)
go func() {
    for evt := range sub2.Events() {
        startHandshake(evt.PeerID)
    }
}()

// Metrics collector
sub3 := node.SubscribeEvents()
go func() {
    for evt := range sub3.Events() {
        metrics.RecordEvent(evt)
    }
}()
```

### Pattern 2: Message Routing by Stream

Route messages to different handlers based on stream name:

```go
type MessageRouter struct {
    node     *glueberry.Node
    handlers map[string]func(streams.IncomingMessage)
}

func (r *MessageRouter) Run() {
    for msg := range r.node.Messages() {
        if handler, ok := r.handlers[msg.StreamName]; ok {
            // Dispatch to handler goroutine
            go handler(msg)
        }
    }
}

router := &MessageRouter{
    node: node,
    handlers: map[string]func(streams.IncomingMessage){
        streams.HandshakeStreamName: handleHandshake,
        "messages":                  handleMessages,
        "consensus":                 handleConsensus,
    },
}
go router.Run()
```

### Pattern 3: Worker Pool for Message Processing

Process messages concurrently with a worker pool:

```go
type MessageProcessor struct {
    node       *glueberry.Node
    workerPool chan struct{} // Semaphore for concurrency limit
    wg         sync.WaitGroup
}

func NewMessageProcessor(node *glueberry.Node, maxWorkers int) *MessageProcessor {
    return &MessageProcessor{
        node:       node,
        workerPool: make(chan struct{}, maxWorkers),
    }
}

func (p *MessageProcessor) Run() {
    for msg := range p.node.Messages() {
        p.workerPool <- struct{}{} // Acquire worker
        p.wg.Add(1)

        go func(m streams.IncomingMessage) {
            defer func() {
                <-p.workerPool // Release worker
                p.wg.Done()
            }()
            processMessage(m)
        }(msg)
    }
}

func (p *MessageProcessor) Shutdown() {
    p.wg.Wait() // Wait for all workers to finish
}
```

### Pattern 4: Rate-Limited Sends

Limit send rate using a ticker:

```go
func rateLimitedSend(node *glueberry.Node, peerID peer.ID, stream string, messages <-chan []byte) {
    ticker := time.NewTicker(100 * time.Millisecond) // Max 10 msg/sec
    defer ticker.Stop()

    for msg := range messages {
        <-ticker.C // Wait for rate limit
        if err := node.Send(peerID, stream, msg); err != nil {
            log.Printf("Send failed: %v", err)
        }
    }
}
```

---

## Internal Concurrency Architecture

### Goroutines Started by Node

Glueberry starts several background goroutines in `node.Start()`:

1. **Message forwarding:** `node.forwardMessagesWithStats()`
   - Reads from internal message channel
   - Records statistics
   - Forwards to external `Messages()` channel

2. **Event forwarding:** `node.forwardEvents()`
   - Reads from internal event channel
   - Fans out to all subscriptions
   - Forwards to external `Events()` channel

3. **Peer stats cleanup:** `node.cleanupStalePeerStats()`
   - Periodically removes stats for disconnected peers

4. **Stream readers:** (per stream)
   - Continuously read from libp2p streams
   - Decrypt and demultiplex messages

5. **Reconnection timers:** (per peer in reconnect state)
   - Wait for backoff duration
   - Attempt reconnection

### Locking Strategy

Glueberry uses **fine-grained locking** to minimize contention:

```go
type Node struct {
    // Separate mutexes per concern
    eventSubsMu       sync.RWMutex // Event subscriptions
    peerStatsMu       sync.RWMutex // Peer statistics
    flowControllersMu sync.RWMutex // Flow controllers
    startMu           sync.Mutex   // Start/stop state
}
```

**Benefits:**
- Operations on different maps don't block each other
- Read-heavy workloads use `RLock()` for parallelism
- Write operations acquire `Lock()` only when necessary

### Non-Blocking Channel Operations

Glueberry uses non-blocking sends to prevent deadlocks:

```go
// Non-blocking send - drop if channel full
select {
case ch <- msg:
    // Message sent
default:
    // Channel full, drop message
    metrics.MessageDropped()
}
```

This ensures:
- Connection state changes never block on slow consumers
- Message processing never blocks on full channels
- System remains responsive under load

---

## Race Condition Prevention

### Double-Checked Locking

Used for lazy initialization of shared resources:

```go
func (n *Node) getOrCreateFlowController(streamName string) *flow.Controller {
    // Fast path: check if exists (read lock)
    n.flowControllersMu.RLock()
    fc := n.flowControllers[streamName]
    n.flowControllersMu.RUnlock()

    if fc != nil {
        return fc
    }

    // Slow path: create new (write lock)
    n.flowControllersMu.Lock()
    defer n.flowControllersMu.Unlock()

    // Double-check after acquiring write lock
    if fc = n.flowControllers[streamName]; fc != nil {
        return fc
    }

    fc = flow.NewController(...)
    n.flowControllers[streamName] = fc
    return fc
}
```

### Idempotent Close with sync.Once

Ensures channels are closed only once:

```go
type EventSubscription struct {
    ch        chan ConnectionEvent
    closeOnce sync.Once
}

func (s *EventSubscription) closeChannel() {
    s.closeOnce.Do(func() {
        close(s.ch)
    })
}
```

### Copy Before Release Lock

Prevents data races when returning data from locked maps:

```go
func (m *Module) getCachedKey(remoteX25519 []byte) ([]byte, bool) {
    m.peerKeysMu.RLock()
    key, ok := m.peerKeys[string(remoteX25519)]
    if !ok {
        m.peerKeysMu.RUnlock()
        return nil, false
    }
    // Copy before releasing lock
    result := make([]byte, len(key))
    copy(result, key)
    m.peerKeysMu.RUnlock()
    return result, true
}
```

---

## Context Cancellation

### Hierarchical Contexts

Glueberry uses hierarchical contexts for graceful shutdown:

```go
func New(cfg *Config) (*Node, error) {
    // Root context for node lifetime
    ctx, cancel := context.WithCancel(context.Background())

    // Component contexts derived from root
    connections := connection.NewManager(ctx, ...)
    streamManager := streams.NewManager(ctx, ...)

    return &Node{
        ctx:    ctx,
        cancel: cancel,
    }
}

func (n *Node) Stop() error {
    n.cancel() // Cascades to all components
    // Wait for goroutines to finish...
}
```

### Context-Aware Operations

All blocking operations respect context cancellation:

```go
func (fc *Controller) Acquire(ctx context.Context) error {
    for {
        fc.mu.Lock()
        if fc.pending < fc.highWater {
            fc.pending++
            fc.mu.Unlock()
            return nil
        }
        waitCh := fc.unblockCh
        fc.mu.Unlock()

        // Wait for unblock OR context cancellation
        select {
        case <-ctx.Done():
            return ctx.Err()
        case <-waitCh:
            continue // Retry
        }
    }
}
```

---

## Deadlock Prevention

### Lock Ordering

Always acquire locks in consistent order:

```go
// ✅ CORRECT: Consistent lock order
func (n *Node) updatePeerStats(peerID peer.ID) {
    n.peerStatsMu.Lock()         // Lock A
    defer n.peerStatsMu.Unlock()

    n.flowControllersMu.Lock()   // Lock B
    defer n.flowControllersMu.Unlock()
    // ...
}

// ❌ INCORRECT: Inconsistent lock order (can deadlock)
func (n *Node) someOtherMethod() {
    n.flowControllersMu.Lock()   // Lock B first
    defer n.flowControllersMu.Unlock()

    n.peerStatsMu.Lock()         // Lock A second - DEADLOCK!
    defer n.peerStatsMu.Unlock()
}
```

### Avoid Nested Locks

Release locks before calling functions that may acquire other locks:

```go
// ✅ CORRECT: Release lock before external call
func (m *Manager) handleEvent(event Event) {
    m.mu.Lock()
    conn := m.connections[event.PeerID]
    m.mu.Unlock() // Release BEFORE external call

    if conn != nil {
        conn.HandleEvent(event) // May acquire other locks
    }
}

// ❌ INCORRECT: Hold lock during external call
func (m *Manager) handleEventBad(event Event) {
    m.mu.Lock()
    defer m.mu.Unlock()

    conn := m.connections[event.PeerID]
    if conn != nil {
        conn.HandleEvent(event) // DANGEROUS - may deadlock
    }
}
```

---

## Performance Considerations

### RWMutex for Read-Heavy Workloads

Use `RWMutex` when reads greatly outnumber writes:

```go
type Cache struct {
    mu    sync.RWMutex
    items map[string]interface{}
}

// Read-heavy operation
func (c *Cache) Get(key string) (interface{}, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    val, ok := c.items[key]
    return val, ok
}

// Rare write operation
func (c *Cache) Set(key string, val interface{}) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.items[key] = val
}
```

### Minimize Critical Sections

Release locks as soon as possible:

```go
// ✅ GOOD: Short critical section
func (m *Manager) process() {
    m.mu.Lock()
    data := make([]byte, len(m.data))
    copy(data, m.data)
    m.mu.Unlock() // Release immediately

    // Expensive processing WITHOUT lock
    result := expensiveComputation(data)

    m.mu.Lock()
    m.result = result
    m.mu.Unlock()
}

// ❌ BAD: Long critical section
func (m *Manager) processBad() {
    m.mu.Lock()
    defer m.mu.Unlock()

    data := m.data
    // Expensive processing WHILE HOLDING LOCK - blocks others!
    result := expensiveComputation(data)
    m.result = result
}
```

---

## Race Detection

### Enable Race Detector in Tests

Always run tests with `-race` flag:

```bash
go test -race ./...
```

### Common Pitfalls

**Pitfall 1: Accessing shared state without locks**

```go
// ❌ RACE: Concurrent access to shared map
type BadCache struct {
    items map[string]string // Unprotected!
}

func (c *BadCache) Set(key, val string) {
    c.items[key] = val // RACE!
}

// ✅ FIXED: Add mutex
type GoodCache struct {
    mu    sync.Mutex
    items map[string]string
}

func (c *GoodCache) Set(key, val string) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.items[key] = val
}
```

**Pitfall 2: Closing channel from multiple goroutines**

```go
// ❌ RACE: Multiple closes
close(ch) // Goroutine 1
close(ch) // Goroutine 2 - PANIC!

// ✅ FIXED: Use sync.Once
var closeOnce sync.Once
closeOnce.Do(func() { close(ch) })
```

**Pitfall 3: Iterating map while modifying**

```go
// ❌ RACE: Iterate while modifying
for k := range m.items { // Read
    go func(key string) {
        delete(m.items, key) // Concurrent write - RACE!
    }(k)
}

// ✅ FIXED: Copy keys first
keys := make([]string, 0, len(m.items))
for k := range m.items {
    keys = append(keys, k)
}
for _, k := range keys {
    delete(m.items, k) // Safe
}
```

---

## Summary

✅ All public APIs are thread-safe
✅ Use event subscriptions for multiple consumers
✅ Implement worker pools for concurrent processing
✅ Use fine-grained locking to minimize contention
✅ Respect context cancellation for graceful shutdown
✅ Avoid deadlocks with consistent lock ordering
✅ Use RWMutex for read-heavy workloads
✅ Always test with `-race` flag

For more details, see [ARCHITECTURE.md - Concurrency Architecture](../../ARCHITECTURE.md#6-concurrency-architecture).
