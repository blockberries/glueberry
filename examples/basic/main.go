// Package main demonstrates basic Glueberry usage with the symmetric API.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	"github.com/blockberries/glueberry"
	"github.com/blockberries/glueberry/pkg/streams"
	"github.com/multiformats/go-multiaddr"
)

// Handshake message types
const (
	msgHello    byte = 1
	msgPubKey   byte = 2
	msgComplete byte = 3
)

func main() {
	// Generate identity
	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// Configure listen address
	listenAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/9000")

	// Create node
	cfg := glueberry.NewConfig(
		privateKey,
		"./addressbook.json",
		[]multiaddr.Multiaddr{listenAddr},
	)

	node, err := glueberry.New(cfg)
	if err != nil {
		fmt.Printf("Failed to create node: %v\n", err)
		return
	}

	// Start the node
	if err := node.Start(); err != nil {
		fmt.Printf("Failed to start node: %v\n", err)
		return
	}
	defer node.Stop()

	fmt.Printf("Node started\n")
	fmt.Printf("  ID: %s\n", node.PeerID())
	fmt.Printf("  Listening on: %v\n", node.Addrs())
	fmt.Printf("  Public Key: %x\n\n", node.PublicKey())

	// Track handshake state per peer
	type peerState struct {
		gotHello   bool
		gotPubKey  bool
		peerPubKey ed25519.PublicKey
	}
	peerStates := make(map[string]*peerState)

	// Handle incoming messages (both handshake and encrypted)
	go func() {
		for msg := range node.Messages() {
			// Handle handshake messages
			if msg.StreamName == streams.HandshakeStreamName {
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
					fmt.Printf("Received Hello from %s\n", peerID[:8])
					state.gotHello = true

					// Send Hello back
					_ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgHello})

					// Send our public key
					pubKeyMsg := make([]byte, 1+len(publicKey))
					pubKeyMsg[0] = msgPubKey
					copy(pubKeyMsg[1:], publicKey)
					_ = node.Send(msg.PeerID, streams.HandshakeStreamName, pubKeyMsg)

				case msgPubKey:
					if len(msg.Data) > 1 {
						state.peerPubKey = ed25519.PublicKey(msg.Data[1:])
						state.gotPubKey = true
						fmt.Printf("Received PubKey from %s\n", peerID[:8])
					}

				case msgComplete:
					fmt.Printf("Received Complete from %s\n", peerID[:8])
					if state.gotHello && state.gotPubKey {
						// Send Complete back
						_ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgComplete})

						// Complete the handshake
						if err := node.CompleteHandshake(msg.PeerID, state.peerPubKey, []string{"messages"}); err != nil {
							fmt.Printf("CompleteHandshake failed: %v\n", err)
						} else {
							fmt.Printf("Connection established with %s\n", peerID[:8])
						}
						delete(peerStates, peerID)
					}
				}
				continue
			}

			// Handle encrypted messages
			fmt.Printf("Message from %s on stream %s: %s\n",
				msg.PeerID.String()[:8], msg.StreamName, string(msg.Data))
		}
	}()

	// Handle connection events
	go func() {
		for event := range node.Events() {
			fmt.Printf("Event: %s -> %s", event.PeerID.String()[:8], event.State)
			if event.IsError() {
				fmt.Printf(" (error: %v)", event.Error)
			}
			fmt.Println()
		}
	}()

	// Example: Connect to another peer (if you have one)
	// Uncomment and modify to connect to a real peer
	/*
		peerAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/9001/p2p/12D3KooW...")
		peerInfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)

		node.AddPeer(peerInfo.ID, peerInfo.Addrs, nil)

		// Connect triggers auto-handshake via the handlers above
		if err := node.Connect(peerInfo.ID); err != nil {
			fmt.Printf("Connect failed: %v\n", err)
			return
		}

		// Send initial Hello
		node.Send(peerInfo.ID, streams.HandshakeStreamName, []byte{msgHello})

		// Send our public key
		pubKeyMsg := make([]byte, 1+len(publicKey))
		pubKeyMsg[0] = msgPubKey
		copy(pubKeyMsg[1:], publicKey)
		node.Send(peerInfo.ID, streams.HandshakeStreamName, pubKeyMsg)

		// The message handler will complete the handshake when it receives PubKey and Complete

		// After handshake completes, send a message
		// time.Sleep(time.Second) // Wait for handshake
		// node.Send(peerInfo.ID, "messages", []byte("Hello!"))
	*/

	// Keep running
	fmt.Println("Node running... Press Ctrl+C to stop")
	select {}
}
