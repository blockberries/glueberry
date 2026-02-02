# Glueberry Quick Start Tutorial

This tutorial will walk you through creating your first Glueberry P2P application in 10 minutes.

---

## Prerequisites

- Go 1.25+ installed
- Basic understanding of Go channels and goroutines
- Text editor or IDE

---

## Step 1: Install Glueberry

Create a new Go module:

```bash
mkdir my-p2p-app
cd my-p2p-app
go mod init my-p2p-app
```

Install Glueberry:

```bash
go get github.com/blockberries/glueberry@latest
```

---

## Step 2: Create Your First Node

Create `main.go`:

```go
package main

import (
    "crypto/ed25519"
    "crypto/rand"
    "fmt"
    "log"

    "github.com/blockberries/glueberry"
    "github.com/multiformats/go-multiaddr"
)

func main() {
    // Generate identity key
    _, privateKey, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        log.Fatalf("Failed to generate key: %v", err)
    }

    // Configure listen address
    listenAddr, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9000")
    if err != nil {
        log.Fatalf("Failed to create multiaddr: %v", err)
    }

    // Create node configuration
    cfg := glueberry.NewConfig(
        privateKey,
        "./addressbook.json",
        []multiaddr.Multiaddr{listenAddr},
    )

    // Create node
    node, err := glueberry.New(cfg)
    if err != nil {
        log.Fatalf("Failed to create node: %v", err)
    }

    // Start node
    if err := node.Start(); err != nil {
        log.Fatalf("Failed to start node: %v", err)
    }
    defer node.Stop()

    fmt.Println("Node started successfully!")
    fmt.Printf("  Peer ID: %s\n", node.PeerID())
    fmt.Printf("  Listening on: %v\n", node.Addrs())

    // Keep running
    select {}
}
```

Run it:

```bash
go run main.go
```

You should see:
```
Node started successfully!
  Peer ID: 12D3KooW...
  Listening on: [/ip4/0.0.0.0/tcp/9000]
```

Congratulations! You've created your first Glueberry node. 🎉

---

## Step 3: Handle Connection Events

Now let's monitor connection events. Add this before `select {}`:

```go
// Handle connection events
go func() {
    for event := range node.Events() {
        fmt.Printf("[EVENT] Peer %s: %s", event.PeerID.String()[:8], event.State)
        if event.IsError() {
            fmt.Printf(" (error: %v)", event.Error)
        }
        fmt.Println()
    }
}()
```

---

## Step 4: Implement Two-Phase Handshake

Add handshake logic to establish secure connections:

```go
import (
    "github.com/blockberries/glueberry/pkg/streams"
)

// Handshake message types
const (
    msgHello    byte = 1
    msgPubKey   byte = 2
    msgComplete byte = 3
)

publicKey := privateKey.Public().(ed25519.PublicKey)

// Track handshake state
type peerState struct {
    gotHello        bool
    gotPubKey       bool
    peerPubKey      ed25519.PublicKey
    streamsPrepared bool
    sentComplete    bool
    gotComplete     bool
}
peerStates := make(map[string]*peerState)

// Handle connection events
go func() {
    for event := range node.Events() {
        fmt.Printf("[EVENT] %s: %s\n", event.PeerID.String()[:8], event.State)

        // When connected, send Hello to start handshake
        if event.State == glueberry.StateConnected {
            helloMsg := []byte{msgHello}
            if err := node.Send(event.PeerID, streams.HandshakeStreamName, helloMsg); err != nil {
                log.Printf("Failed to send Hello: %v", err)
            }
        }
    }
}()

// Handle handshake messages
go func() {
    for msg := range node.Messages() {
        if msg.StreamName != streams.HandshakeStreamName {
            // Skip non-handshake messages for now
            continue
        }

        peerID := msg.PeerID.String()
        state, ok := peerStates[peerID]
        if !ok {
            state = &peerState{}
            peerStates[peerID] = state
        }

        if len(msg.Data) == 0 {
            continue
        }

        switch msg.Data[0] {
        case msgHello:
            fmt.Printf("[HANDSHAKE] Received Hello from %s\n", peerID[:8])
            state.gotHello = true
            // Send Hello back
            _ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgHello})
            // Send our public key
            pubKeyMsg := append([]byte{msgPubKey}, publicKey...)
            _ = node.Send(msg.PeerID, streams.HandshakeStreamName, pubKeyMsg)

        case msgPubKey:
            if len(msg.Data) > 1 {
                state.peerPubKey = ed25519.PublicKey(msg.Data[1:])
                state.gotPubKey = true
                fmt.Printf("[HANDSHAKE] Received PubKey from %s\n", peerID[:8])

                // Phase 1: Prepare streams
                if err := node.PrepareStreams(msg.PeerID, state.peerPubKey, []string{"messages"}); err != nil {
                    log.Printf("PrepareStreams failed: %v", err)
                    continue
                }
                state.streamsPrepared = true

                // Send Complete
                _ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgComplete})
                state.sentComplete = true
            }

        case msgComplete:
            fmt.Printf("[HANDSHAKE] Received Complete from %s\n", peerID[:8])
            state.gotComplete = true
        }

        // Phase 2: Finalize when both sides are ready
        if state.streamsPrepared && state.sentComplete && state.gotComplete {
            if err := node.FinalizeHandshake(msg.PeerID); err != nil {
                log.Printf("FinalizeHandshake failed: %v", err)
                continue
            }
            fmt.Printf("[HANDSHAKE] ✅ Handshake complete with %s\n", peerID[:8])
            delete(peerStates, peerID)
        }
    }
}()
```

---

## Step 5: Send and Receive Messages

Add message handling after handshake:

```go
// Handle all messages
go func() {
    for msg := range node.Messages() {
        if msg.StreamName == streams.HandshakeStreamName {
            // Handle handshake (code from Step 4)
            handleHandshake(msg)
            continue
        }

        // Handle application messages
        if msg.StreamName == "messages" {
            fmt.Printf("[MESSAGE] From %s: %s\n", msg.PeerID.String()[:8], string(msg.Data))
        }
    }
}()
```

Send a message after handshake completes:

```go
// In the handshake completion block:
if state.streamsPrepared && state.sentComplete && state.gotComplete {
    if err := node.FinalizeHandshake(msg.PeerID); err != nil {
        log.Printf("FinalizeHandshake failed: %v", err)
        continue
    }
    fmt.Printf("[HANDSHAKE] ✅ Handshake complete with %s\n", peerID[:8])

    // Send a test message
    testMsg := []byte("Hello from Node!")
    if err := node.Send(msg.PeerID, "messages", testMsg); err != nil {
        log.Printf("Failed to send message: %v", err)
    }

    delete(peerStates, peerID)
}
```

---

## Step 6: Connect Two Nodes

Now let's run two nodes and connect them.

**Terminal 1 (Node A):**
```bash
go run main.go
```

Note the **Peer ID** and **listen address** printed.

**Terminal 2 (Node B):**

Modify `main.go` to add Node A as a peer before starting:

```go
// After creating node, before starting
peerAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/9000/p2p/12D3KooW...")  // Replace with Node A's peer ID
peerInfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
node.AddPeer(peerInfo.ID, peerInfo.Addrs, nil)
```

Then run:
```bash
go run main.go -port 9001  # (Add flag parsing if needed)
```

You should see handshake messages exchanged and "Handshake complete" on both terminals!

---

## Complete Example

Here's the complete working example:

```go
package main

import (
    "crypto/ed25519"
    "crypto/rand"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/blockberries/glueberry"
    "github.com/blockberries/glueberry/pkg/streams"
    "github.com/libp2p/go-libp2p/core/peer"
    "github.com/multiformats/go-multiaddr"
)

// Handshake message types
const (
    msgHello    byte = 1
    msgPubKey   byte = 2
    msgComplete byte = 3
)

type peerState struct {
    gotHello        bool
    gotPubKey       bool
    peerPubKey      ed25519.PublicKey
    streamsPrepared bool
    sentComplete    bool
    gotComplete     bool
}

func main() {
    // Generate identity key
    _, privateKey, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        log.Fatalf("Failed to generate key: %v", err)
    }
    publicKey := privateKey.Public().(ed25519.PublicKey)

    // Configure listen address
    listenAddr, err := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9000")
    if err != nil {
        log.Fatalf("Failed to create multiaddr: %v", err)
    }

    // Create node configuration
    cfg := glueberry.NewConfig(
        privateKey,
        "./addressbook.json",
        []multiaddr.Multiaddr{listenAddr},
    )

    // Create node
    node, err := glueberry.New(cfg)
    if err != nil {
        log.Fatalf("Failed to create node: %v", err)
    }

    // Start node
    if err := node.Start(); err != nil {
        log.Fatalf("Failed to start node: %v", err)
    }
    defer node.Stop()

    fmt.Println("Node started successfully!")
    fmt.Printf("  Peer ID: %s\n", node.PeerID())
    fmt.Printf("  Listening on: %v\n", node.Addrs())
    fmt.Println("\nTo connect from another node:")
    fmt.Printf("  peerAddr := \"%s/p2p/%s\"\n\n", node.Addrs()[0], node.PeerID())

    // Track handshake state
    peerStates := make(map[string]*peerState)

    // Handle connection events
    go func() {
        for event := range node.Events() {
            fmt.Printf("[EVENT] %s: %s\n", event.PeerID.String()[:8], event.State)

            if event.State == glueberry.StateConnected {
                // Start handshake
                helloMsg := []byte{msgHello}
                _ = node.Send(event.PeerID, streams.HandshakeStreamName, helloMsg)
            }
        }
    }()

    // Handle messages
    go func() {
        for msg := range node.Messages() {
            if msg.StreamName == streams.HandshakeStreamName {
                handleHandshake(node, msg, publicKey, peerStates)
                continue
            }

            // Handle application messages
            if msg.StreamName == "messages" {
                fmt.Printf("[MESSAGE] From %s: %s\n", msg.PeerID.String()[:8], string(msg.Data))
            }
        }
    }()

    // Wait for interrupt signal
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    fmt.Println("\nShutting down...")
}

func handleHandshake(node *glueberry.Node, msg streams.IncomingMessage, publicKey ed25519.PublicKey, peerStates map[string]*peerState) {
    peerID := msg.PeerID.String()
    state, ok := peerStates[peerID]
    if !ok {
        state = &peerState{}
        peerStates[peerID] = state
    }

    if len(msg.Data) == 0 {
        return
    }

    switch msg.Data[0] {
    case msgHello:
        fmt.Printf("[HANDSHAKE] Received Hello from %s\n", peerID[:8])
        state.gotHello = true
        _ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgHello})
        pubKeyMsg := append([]byte{msgPubKey}, publicKey...)
        _ = node.Send(msg.PeerID, streams.HandshakeStreamName, pubKeyMsg)

    case msgPubKey:
        if len(msg.Data) > 1 {
            state.peerPubKey = ed25519.PublicKey(msg.Data[1:])
            state.gotPubKey = true
            fmt.Printf("[HANDSHAKE] Received PubKey from %s\n", peerID[:8])

            if err := node.PrepareStreams(msg.PeerID, state.peerPubKey, []string{"messages"}); err != nil {
                log.Printf("PrepareStreams failed: %v", err)
                return
            }
            state.streamsPrepared = true
            _ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgComplete})
            state.sentComplete = true
        }

    case msgComplete:
        fmt.Printf("[HANDSHAKE] Received Complete from %s\n", peerID[:8])
        state.gotComplete = true
    }

    // Finalize handshake
    if state.streamsPrepared && state.sentComplete && state.gotComplete {
        if err := node.FinalizeHandshake(msg.PeerID); err != nil {
            log.Printf("FinalizeHandshake failed: %v", err)
            return
        }
        fmt.Printf("[HANDSHAKE] ✅ Handshake complete with %s\n", peerID[:8])

        // Send test message
        testMsg := []byte("Hello from Glueberry!")
        _ = node.Send(msg.PeerID, "messages", testMsg)

        delete(peerStates, peerID)
    }
}
```

---

## Next Steps

Now that you have a working Glueberry application, explore more:

1. **[Advanced Usage Tutorial](./advanced-usage.md)** - Multiple streams, custom handshake protocols
2. **[Examples](../../examples/)** - Complete example applications
   - `simple-chat` - Interactive chat
   - `file-transfer` - Large file transfer
   - `rpc` - RPC-style communication
3. **[API Reference](../api/index.md)** - Complete API documentation
4. **[ARCHITECTURE.md](../../ARCHITECTURE.md)** - Deep dive into architecture

---

## Troubleshooting

### Port Already in Use
If you get "address already in use", change the listen port:
```go
listenAddr, _ := multiaddr.NewMultiaddr("/ip4/0.0.0.0/tcp/9001")
```

### Connection Refused
Ensure both nodes are running and firewall allows connections.

### Handshake Timeout
Default timeout is 60 seconds. Increase if needed:
```go
cfg := glueberry.NewConfig(
    privateKey,
    addressBookPath,
    listenAddrs,
    glueberry.WithHandshakeTimeout(120*time.Second),
)
```

---

## Summary

You've learned:
✅ How to create a Glueberry node
✅ How to handle connection events
✅ How to implement two-phase handshake
✅ How to send and receive encrypted messages
✅ How to connect two nodes

Happy P2P coding! 🚀
