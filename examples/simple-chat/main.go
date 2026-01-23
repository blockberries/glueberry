package main

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"

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

// peerState tracks handshake progress for a peer
type peerState struct {
	name       string
	gotHello   bool
	gotPubKey  bool
	peerPubKey ed25519.PublicKey
}

var (
	peerStates   = make(map[peer.ID]*peerState)
	peerStatesMu sync.Mutex
)

func main() {
	// Parse command line flags
	port := flag.Int("port", 9000, "Port to listen on")
	name := flag.String("name", "anonymous", "Your name")
	keyFile := flag.String("key", "./node.key", "Path to private key file")
	abFile := flag.String("addressbook", "./addressbook.json", "Path to address book")
	peerAddr := flag.String("peer", "", "Peer multiaddr to connect to (optional)")
	flag.Parse()

	// Load or generate private key
	privateKey, err := loadOrGenerateKey(*keyFile)
	if err != nil {
		fmt.Printf("Failed to load key: %v\n", err)
		return
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// Create listen address
	listenAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port))

	// Create node configuration
	cfg := glueberry.NewConfig(
		privateKey,
		*abFile,
		[]multiaddr.Multiaddr{listenAddr},
	)

	// Create and start node
	node, err := glueberry.New(cfg)
	if err != nil {
		fmt.Printf("Failed to create node: %v\n", err)
		return
	}

	if err := node.Start(); err != nil {
		fmt.Printf("Failed to start node: %v\n", err)
		return
	}
	defer node.Stop()

	fmt.Printf("========================================\n")
	fmt.Printf("      Glueberry Simple Chat Node        \n")
	fmt.Printf("========================================\n\n")
	fmt.Printf("Your Name: %s\n", *name)
	fmt.Printf("Peer ID:   %s\n", node.PeerID())
	fmt.Printf("Listening: %v\n\n", node.Addrs())

	// Handle incoming messages (both handshake and chat)
	go handleMessages(node, *name, publicKey)

	// Handle connection events
	go handleEvents(node)

	// If peer address provided, connect
	if *peerAddr != "" {
		go connectToPeer(node, *peerAddr, *name, publicKey)
	}

	// Interactive command loop
	fmt.Println("Commands:")
	fmt.Println("  connect <multiaddr> - Connect to a peer")
	fmt.Println("  send <peer-id> <message> - Send a message")
	fmt.Println("  list - List connected peers")
	fmt.Println("  quit - Exit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		cmd := parts[0]
		switch cmd {
		case "quit", "exit":
			fmt.Println("Goodbye!")
			return

		case "connect":
			if len(parts) < 2 {
				fmt.Println("Usage: connect <multiaddr>")
				continue
			}
			go connectToPeer(node, parts[1], *name, publicKey)

		case "send":
			if len(parts) < 3 {
				fmt.Println("Usage: send <peer-id> <message>")
				continue
			}
			peerIDStr := parts[1]
			message := strings.Join(parts[2:], " ")
			sendMessage(node, peerIDStr, *name, message)

		case "list":
			listPeers(node)

		default:
			fmt.Printf("Unknown command: %s\n", cmd)
		}
	}
}

func loadOrGenerateKey(path string) (ed25519.PrivateKey, error) {
	// Try to load existing key
	if data, err := os.ReadFile(path); err == nil {
		if len(data) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(data), nil
		}
	}

	// Generate new key
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	// Save for next time
	if err := os.WriteFile(path, priv, 0600); err != nil {
		fmt.Printf("Warning: failed to save key: %v\n", err)
	}

	return priv, nil
}

func handleMessages(node *glueberry.Node, myName string, myPubKey ed25519.PublicKey) {
	for msg := range node.Messages() {
		// Handle handshake messages
		if msg.StreamName == streams.HandshakeStreamName {
			handleHandshakeMessage(node, msg, myName, myPubKey)
			continue
		}

		// Handle chat messages
		fmt.Printf("\n[%s]: %s\n> ", msg.PeerID.String()[:8], string(msg.Data))
	}
}

func handleHandshakeMessage(node *glueberry.Node, msg streams.IncomingMessage, myName string, myPubKey ed25519.PublicKey) {
	if len(msg.Data) == 0 {
		return
	}

	peerStatesMu.Lock()
	state, ok := peerStates[msg.PeerID]
	if !ok {
		state = &peerState{}
		peerStates[msg.PeerID] = state
	}
	peerStatesMu.Unlock()

	switch msg.Data[0] {
	case msgHello:
		state.gotHello = true
		if len(msg.Data) > 1 {
			state.name = string(msg.Data[1:])
		}
		fmt.Printf("\nHandshake: Hello from %s\n> ", state.name)

		// Send Hello back with our name
		helloMsg := make([]byte, 1+len(myName))
		helloMsg[0] = msgHello
		copy(helloMsg[1:], myName)
		_ = node.Send(msg.PeerID, streams.HandshakeStreamName, helloMsg)

		// Send our public key
		pubKeyMsg := make([]byte, 1+len(myPubKey))
		pubKeyMsg[0] = msgPubKey
		copy(pubKeyMsg[1:], myPubKey)
		_ = node.Send(msg.PeerID, streams.HandshakeStreamName, pubKeyMsg)

	case msgPubKey:
		if len(msg.Data) > 1 {
			state.peerPubKey = ed25519.PublicKey(msg.Data[1:])
			state.gotPubKey = true
			fmt.Printf("\nHandshake: PubKey received from %s\n> ", msg.PeerID.String()[:8])
		}

	case msgComplete:
		fmt.Printf("\nHandshake: Complete received from %s\n> ", msg.PeerID.String()[:8])
		if state.gotHello && state.gotPubKey {
			// Send Complete back
			_ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgComplete})

			// Complete the handshake
			if err := node.CompleteHandshake(msg.PeerID, state.peerPubKey, []string{"chat"}); err != nil {
				fmt.Printf("CompleteHandshake failed: %v\n> ", err)
			} else {
				fmt.Printf("Secure connection established with %s\n> ", state.name)
			}

			peerStatesMu.Lock()
			delete(peerStates, msg.PeerID)
			peerStatesMu.Unlock()
		}
	}
}

func handleEvents(node *glueberry.Node) {
	for event := range node.Events() {
		switch event.State {
		case glueberry.StateConnected:
			fmt.Printf("\nConnected to %s\n> ", event.PeerID.String()[:8])
		case glueberry.StateEstablished:
			fmt.Printf("\nSecure connection established with %s\n> ", event.PeerID.String()[:8])
		case glueberry.StateDisconnected:
			if event.IsError() {
				fmt.Printf("\nDisconnected from %s: %v\n> ", event.PeerID.String()[:8], event.Error)
			} else {
				fmt.Printf("\nDisconnected from %s\n> ", event.PeerID.String()[:8])
			}
		case glueberry.StateReconnecting:
			fmt.Printf("\nReconnecting to %s...\n> ", event.PeerID.String()[:8])
		}
	}
}

func connectToPeer(node *glueberry.Node, addrStr string, myName string, myPubKey ed25519.PublicKey) {
	fmt.Printf("Connecting to %s...\n", addrStr)

	// Parse multiaddr
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		fmt.Printf("Invalid multiaddr: %v\n> ", err)
		return
	}

	// Extract peer ID from multiaddr
	peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		fmt.Printf("Failed to parse peer info: %v\n> ", err)
		return
	}

	// Initialize peer state
	peerStatesMu.Lock()
	peerStates[peerInfo.ID] = &peerState{}
	peerStatesMu.Unlock()

	// Add to address book
	node.AddPeer(peerInfo.ID, peerInfo.Addrs, map[string]string{"name": "peer"})

	// Connect
	if err := node.Connect(peerInfo.ID); err != nil {
		fmt.Printf("Connect failed: %v\n> ", err)
		return
	}

	// Send Hello with our name
	helloMsg := make([]byte, 1+len(myName))
	helloMsg[0] = msgHello
	copy(helloMsg[1:], myName)
	if err := node.Send(peerInfo.ID, streams.HandshakeStreamName, helloMsg); err != nil {
		fmt.Printf("Failed to send Hello: %v\n> ", err)
		return
	}

	// Send our public key
	pubKeyMsg := make([]byte, 1+len(myPubKey))
	pubKeyMsg[0] = msgPubKey
	copy(pubKeyMsg[1:], myPubKey)
	if err := node.Send(peerInfo.ID, streams.HandshakeStreamName, pubKeyMsg); err != nil {
		fmt.Printf("Failed to send PubKey: %v\n> ", err)
		return
	}

	// Wait for response and send Complete when ready
	// (handled by the message handler)
	go func() {
		peerStatesMu.Lock()
		state := peerStates[peerInfo.ID]
		peerStatesMu.Unlock()

		if state == nil {
			return
		}

		// Wait for PubKey to arrive (poll - in production use channels)
		for i := 0; i < 100; i++ {
			peerStatesMu.Lock()
			state = peerStates[peerInfo.ID]
			if state != nil && state.gotPubKey {
				peerStatesMu.Unlock()
				break
			}
			peerStatesMu.Unlock()
			// time.Sleep(50 * time.Millisecond)
		}

		peerStatesMu.Lock()
		state = peerStates[peerInfo.ID]
		if state == nil || !state.gotPubKey {
			peerStatesMu.Unlock()
			return
		}
		peerPubKey := state.peerPubKey
		peerStatesMu.Unlock()

		// Send Complete
		_ = node.Send(peerInfo.ID, streams.HandshakeStreamName, []byte{msgComplete})

		// Complete handshake will be called when we receive Complete back
		// But if we're the initiator, we can also complete here
		if err := node.CompleteHandshake(peerInfo.ID, peerPubKey, []string{"chat"}); err != nil {
			fmt.Printf("CompleteHandshake failed: %v\n> ", err)
			return
		}

		peerStatesMu.Lock()
		name := state.name
		if name == "" {
			name = peerInfo.ID.String()[:8]
		}
		delete(peerStates, peerInfo.ID)
		peerStatesMu.Unlock()

		fmt.Printf("Secure connection established with %s\n> ", name)
	}()
}

func sendMessage(node *glueberry.Node, peerIDStr, sender, content string) {
	peerID, err := peer.Decode(peerIDStr)
	if err != nil {
		fmt.Printf("Invalid peer ID: %v\n", err)
		return
	}

	// Send message
	message := fmt.Sprintf("[%s]: %s", sender, content)

	if err := node.Send(peerID, "chat", []byte(message)); err != nil {
		fmt.Printf("Send failed: %v\n", err)
		return
	}

	fmt.Printf("Sent to %s\n", peerIDStr[:8])
}

func listPeers(node *glueberry.Node) {
	peers := node.ListPeers()
	if len(peers) == 0 {
		fmt.Println("No peers in address book")
		return
	}

	fmt.Printf("\nPeers (%d):\n", len(peers))
	for _, p := range peers {
		state := node.ConnectionState(p.PeerID)
		fmt.Printf("  %s - %s\n", p.PeerID, state)
		if len(p.Metadata) > 0 {
			for k, v := range p.Metadata {
				fmt.Printf("    %s: %s\n", k, v)
			}
		}
	}
	fmt.Println()
}
