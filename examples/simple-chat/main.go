package main

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/blockberries/glueberry"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// HandshakeMessage is the message exchanged during handshake
type HandshakeMessage struct {
	NodeName  string `cramberry:"1,required"`
	PublicKey []byte `cramberry:"2,required"`
}

// ChatMessage is a text chat message
type ChatMessage struct {
	Sender  string `cramberry:"1,required"`
	Content string `cramberry:"2,required"`
}

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

	fmt.Printf("╔════════════════════════════════════════╗\n")
	fmt.Printf("║      Glueberry Simple Chat Node       ║\n")
	fmt.Printf("╚════════════════════════════════════════╝\n\n")
	fmt.Printf("Your Name: %s\n", *name)
	fmt.Printf("Peer ID:   %s\n", node.PeerID())
	fmt.Printf("Listening: %v\n\n", node.Addrs())

	// Handle incoming handshakes
	go handleIncomingHandshakes(node, *name)

	// Handle incoming messages
	go handleIncomingMessages(node)

	// Handle connection events
	go handleEvents(node)

	// If peer address provided, connect
	if *peerAddr != "" {
		go connectToPeer(node, *peerAddr, *name)
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
			go connectToPeer(node, parts[1], *name)

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

func handleIncomingHandshakes(node *glueberry.Node, myName string) {
	for incoming := range node.IncomingHandshakes() {
		fmt.Printf("\n📞 Incoming connection from %s\n", incoming.PeerID)

		// Receive handshake
		var msg HandshakeMessage
		if err := incoming.HandshakeStream.Receive(&msg); err != nil {
			fmt.Printf("  ❌ Handshake failed: %v\n", err)
			incoming.HandshakeStream.Close()
			continue
		}

		fmt.Printf("  Peer name: %s\n", msg.NodeName)

		// Send our handshake
		ourMsg := HandshakeMessage{
			NodeName:  myName,
			PublicKey: node.PublicKey(),
		}
		if err := incoming.HandshakeStream.Send(&ourMsg); err != nil {
			fmt.Printf("  ❌ Failed to send handshake: %v\n", err)
			incoming.HandshakeStream.Close()
			continue
		}

		// Establish encrypted streams
		streamNames := []string{"chat"}
		remotePubKey := ed25519.PublicKey(msg.PublicKey)
		if err := node.EstablishEncryptedStreams(incoming.PeerID, remotePubKey, streamNames); err != nil {
			fmt.Printf("  ❌ Failed to establish streams: %v\n", err)
			continue
		}

		fmt.Printf("  ✅ Connected to %s\n\n", msg.NodeName)
	}
}

func handleIncomingMessages(node *glueberry.Node) {
	for msg := range node.Messages() {
		// Deserialize chat message
		// For simplicity, we're just treating it as raw bytes
		fmt.Printf("\n💬 [%s]: %s\n> ", msg.PeerID, string(msg.Data))
	}
}

func handleEvents(node *glueberry.Node) {
	for event := range node.Events() {
		switch event.State {
		case glueberry.StateConnected:
			fmt.Printf("\n🔗 Connected to %s\n> ", event.PeerID)
		case glueberry.StateEstablished:
			fmt.Printf("\n✅ Secure connection established with %s\n> ", event.PeerID)
		case glueberry.StateDisconnected:
			if event.IsError() {
				fmt.Printf("\n❌ Disconnected from %s: %v\n> ", event.PeerID, event.Error)
			} else {
				fmt.Printf("\n👋 Disconnected from %s\n> ", event.PeerID)
			}
		case glueberry.StateReconnecting:
			fmt.Printf("\n🔄 Reconnecting to %s...\n> ", event.PeerID)
		}
	}
}

func connectToPeer(node *glueberry.Node, addrStr string, myName string) {
	fmt.Printf("Connecting to %s...\n", addrStr)

	// Parse multiaddr
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		fmt.Printf("❌ Invalid multiaddr: %v\n", err)
		return
	}

	// Extract peer ID from multiaddr
	peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		fmt.Printf("❌ Failed to parse peer info: %v\n", err)
		return
	}

	// Add to address book
	node.AddPeer(peerInfo.ID, peerInfo.Addrs, map[string]string{"name": "peer"})

	// Connect
	hs, err := node.Connect(peerInfo.ID)
	if err != nil {
		fmt.Printf("❌ Connect failed: %v\n", err)
		return
	}

	// Send handshake
	ourMsg := HandshakeMessage{
		NodeName:  myName,
		PublicKey: node.PublicKey(),
	}
	if err := hs.Send(&ourMsg); err != nil {
		fmt.Printf("❌ Handshake send failed: %v\n", err)
		return
	}

	// Receive handshake response
	var response HandshakeMessage
	if err := hs.Receive(&response); err != nil {
		fmt.Printf("❌ Handshake receive failed: %v\n", err)
		return
	}

	fmt.Printf("✅ Handshake complete with: %s\n", response.NodeName)

	// Establish encrypted streams
	streamNames := []string{"chat"}
	remotePubKey := ed25519.PublicKey(response.PublicKey)
	if err := node.EstablishEncryptedStreams(peerInfo.ID, remotePubKey, streamNames); err != nil {
		fmt.Printf("❌ Failed to establish streams: %v\n", err)
		return
	}

	fmt.Printf("✅ Secure connection established\n")
}

func sendMessage(node *glueberry.Node, peerIDStr, sender, content string) {
	peerID, err := peer.Decode(peerIDStr)
	if err != nil {
		fmt.Printf("❌ Invalid peer ID: %v\n", err)
		return
	}

	// For simplicity, just send raw bytes
	// In production, you'd use Cramberry to serialize a ChatMessage struct
	message := fmt.Sprintf("[%s]: %s", sender, content)

	if err := node.Send(peerID, "chat", []byte(message)); err != nil {
		fmt.Printf("❌ Send failed: %v\n", err)
		return
	}

	fmt.Printf("Sent to %s\n", peerID)
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
