// Package main demonstrates basic Glueberry usage.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	"github.com/blockberries/glueberry"
	"github.com/multiformats/go-multiaddr"
)

// HandshakeMsg is exchanged during connection handshake
type HandshakeMsg struct {
	Version   int32  `cramberry:"1,required"`
	PublicKey []byte `cramberry:"2,required"`
}

func main() {
	// Generate identity
	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)

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

	// Handle incoming messages
	go func() {
		for msg := range node.Messages() {
			fmt.Printf("📨 Message from %s on stream %s: %s\n",
				msg.PeerID, msg.StreamName, string(msg.Data))
		}
	}()

	// Handle connection events
	go func() {
		for event := range node.Events() {
			fmt.Printf("🔔 Event: %s -> %s", event.PeerID, event.State)
			if event.IsError() {
				fmt.Printf(" (error: %v)", event.Error)
			}
			fmt.Println()
		}
	}()

	// Handle incoming handshakes
	go func() {
		for incoming := range node.IncomingHandshakes() {
			fmt.Printf("📞 Incoming handshake from: %s\n", incoming.PeerID)

			// Receive handshake
			var msg HandshakeMsg
			if err := incoming.HandshakeStream.Receive(&msg); err != nil {
				fmt.Printf("  ❌ Handshake receive failed: %v\n", err)
				incoming.HandshakeStream.Close()
				continue
			}

			fmt.Printf("  Protocol version: %d\n", msg.Version)

			// Send response
			response := HandshakeMsg{
				Version:   1,
				PublicKey: node.PublicKey(),
			}
			if err := incoming.HandshakeStream.Send(&response); err != nil {
				fmt.Printf("  ❌ Handshake send failed: %v\n", err)
				incoming.HandshakeStream.Close()
				continue
			}

			// Establish encrypted streams
			remotePubKey := ed25519.PublicKey(msg.PublicKey)
			streamNames := []string{"messages"}
			if err := node.EstablishEncryptedStreams(incoming.PeerID, remotePubKey, streamNames); err != nil {
				fmt.Printf("  ❌ Failed to establish streams: %v\n", err)
				continue
			}

			fmt.Printf("  ✅ Connection established\n")
		}
	}()

	// Example: Connect to another peer (if you have one)
	// Uncomment and modify to connect to a real peer
	/*
	peerAddr, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/9001/p2p/12D3KooW...")
	peerInfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)

	node.AddPeer(peerInfo.ID, peerInfo.Addrs, nil)

	hs, err := node.Connect(peerInfo.ID)
	if err != nil {
		fmt.Printf("Connect failed: %v\n", err)
		return
	}

	// Send handshake
	hs.Send(&HandshakeMsg{Version: 1, PublicKey: node.PublicKey()})

	// Receive response
	var response HandshakeMsg
	hs.Receive(&response)

	// Establish streams
	remotePubKey := ed25519.PublicKey(response.PublicKey)
	node.EstablishEncryptedStreams(peerInfo.ID, remotePubKey, []string{"messages"})

	// Send a message
	node.Send(peerInfo.ID, "messages", []byte("Hello!"))
	*/

	// Keep running
	fmt.Println("Node running... Press Ctrl+C to stop")
	select {}
}
