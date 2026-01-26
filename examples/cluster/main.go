// Package main demonstrates multi-node cluster coordination with Glueberry.
// This example shows how to implement peer discovery, gossip-based membership,
// and broadcast messaging across multiple nodes in a cluster.
package main

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blockberries/glueberry"
	"github.com/blockberries/glueberry/pkg/streams"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// Protocol constants
const (
	// Handshake message types
	msgHello    byte = 1
	msgPubKey   byte = 2
	msgComplete byte = 3

	// Cluster message types
	msgGossip    byte = 30 // Gossip about known peers
	msgBroadcast byte = 31 // Broadcast message to all peers
	msgPing      byte = 32 // Ping for liveness
	msgPong      byte = 33 // Pong response

	// Stream name
	streamCluster = "cluster"

	// Gossip interval
	gossipInterval = 10 * time.Second
	pingInterval   = 5 * time.Second
)

// PeerInfo represents information about a cluster peer
type PeerInfo struct {
	PeerID    string   `json:"peer_id"`
	Addrs     []string `json:"addrs"`
	Name      string   `json:"name"`
	JoinedAt  int64    `json:"joined_at"`
	LastSeen  int64    `json:"last_seen"`
	PublicKey []byte   `json:"public_key,omitempty"`
}

// GossipMessage contains peer discovery information
type GossipMessage struct {
	Sender     string      `json:"sender"`
	KnownPeers []*PeerInfo `json:"known_peers"`
}

// BroadcastMessage is a message broadcast to all cluster members
type BroadcastMessage struct {
	Sender    string `json:"sender"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
	MessageID string `json:"message_id"`
}

// ClusterNode manages cluster membership and messaging
type ClusterNode struct {
	node      *glueberry.Node
	name      string
	publicKey ed25519.PublicKey

	// Cluster membership
	members   map[peer.ID]*PeerInfo
	membersMu sync.RWMutex

	// Handshake tracking
	handshakes   map[peer.ID]*handshakeState
	handshakesMu sync.Mutex

	// Message deduplication
	seenMessages   map[string]bool
	seenMessagesMu sync.Mutex

	// Lifecycle
	stopCh chan struct{}
}

type handshakeState struct {
	gotPubKey  bool
	peerPubKey ed25519.PublicKey
	name       string
}

// NewClusterNode creates a new cluster node
func NewClusterNode(node *glueberry.Node, name string, publicKey ed25519.PublicKey) *ClusterNode {
	return &ClusterNode{
		node:         node,
		name:         name,
		publicKey:    publicKey,
		members:      make(map[peer.ID]*PeerInfo),
		handshakes:   make(map[peer.ID]*handshakeState),
		seenMessages: make(map[string]bool),
		stopCh:       make(chan struct{}),
	}
}

// Start begins cluster operations
func (c *ClusterNode) Start() {
	// Start message handler
	go c.handleMessages()

	// Start event handler
	go c.handleEvents()

	// Start gossip routine
	go c.gossipRoutine()

	// Start ping routine
	go c.pingRoutine()
}

// Stop stops cluster operations
func (c *ClusterNode) Stop() {
	close(c.stopCh)
}

// ConnectPeer connects to a peer and performs handshake
func (c *ClusterNode) ConnectPeer(addrStr string) error {
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return fmt.Errorf("failed to parse peer info: %w", err)
	}

	c.handshakesMu.Lock()
	c.handshakes[peerInfo.ID] = &handshakeState{}
	c.handshakesMu.Unlock()

	c.node.AddPeer(peerInfo.ID, peerInfo.Addrs, nil)

	if err := c.node.Connect(peerInfo.ID); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	// Send Hello with name
	helloMsg := make([]byte, 1+len(c.name))
	helloMsg[0] = msgHello
	copy(helloMsg[1:], c.name)
	_ = c.node.Send(peerInfo.ID, streams.HandshakeStreamName, helloMsg)

	// Send PubKey
	pubKeyMsg := make([]byte, 1+len(c.publicKey))
	pubKeyMsg[0] = msgPubKey
	copy(pubKeyMsg[1:], c.publicKey)
	_ = c.node.Send(peerInfo.ID, streams.HandshakeStreamName, pubKeyMsg)

	// Wait for response and complete
	go func() {
		for i := 0; i < 100; i++ {
			c.handshakesMu.Lock()
			state := c.handshakes[peerInfo.ID]
			if state != nil && state.gotPubKey {
				c.handshakesMu.Unlock()
				_ = c.node.Send(peerInfo.ID, streams.HandshakeStreamName, []byte{msgComplete})
				_ = c.node.CompleteHandshake(peerInfo.ID, state.peerPubKey, []string{streamCluster})

				// Add to members
				c.addMember(peerInfo.ID, state.peerPubKey, state.name, peerInfo.Addrs)

				c.handshakesMu.Lock()
				delete(c.handshakes, peerInfo.ID)
				c.handshakesMu.Unlock()
				return
			}
			c.handshakesMu.Unlock()
			time.Sleep(50 * time.Millisecond)
		}
	}()

	return nil
}

// Broadcast sends a message to all cluster members
func (c *ClusterNode) Broadcast(content string) {
	msg := BroadcastMessage{
		Sender:    c.node.PeerID().String(),
		Name:      c.name,
		Content:   content,
		Timestamp: time.Now().UnixNano(),
		MessageID: fmt.Sprintf("%s-%d", c.node.PeerID().String()[:8], time.Now().UnixNano()),
	}

	// Mark as seen
	c.seenMessagesMu.Lock()
	c.seenMessages[msg.MessageID] = true
	c.seenMessagesMu.Unlock()

	data, _ := json.Marshal(msg)
	fullMsg := make([]byte, 1+len(data))
	fullMsg[0] = msgBroadcast
	copy(fullMsg[1:], data)

	c.membersMu.RLock()
	members := make([]peer.ID, 0, len(c.members))
	for pid := range c.members {
		members = append(members, pid)
	}
	c.membersMu.RUnlock()

	for _, pid := range members {
		_ = c.node.Send(pid, streamCluster, fullMsg)
	}
}

// Members returns current cluster members
func (c *ClusterNode) Members() []*PeerInfo {
	c.membersMu.RLock()
	defer c.membersMu.RUnlock()

	result := make([]*PeerInfo, 0, len(c.members))
	for _, m := range c.members {
		result = append(result, m)
	}
	return result
}

func (c *ClusterNode) addMember(peerID peer.ID, pubKey ed25519.PublicKey, name string, addrs []multiaddr.Multiaddr) {
	c.membersMu.Lock()
	defer c.membersMu.Unlock()

	addrStrs := make([]string, len(addrs))
	for i, a := range addrs {
		addrStrs[i] = a.String()
	}

	c.members[peerID] = &PeerInfo{
		PeerID:    peerID.String(),
		Addrs:     addrStrs,
		Name:      name,
		JoinedAt:  time.Now().Unix(),
		LastSeen:  time.Now().Unix(),
		PublicKey: pubKey,
	}

	fmt.Printf("\n[Cluster] Member joined: %s (%s)\n> ", name, peerID.String()[:8])
}

func (c *ClusterNode) removeMember(peerID peer.ID) {
	c.membersMu.Lock()
	member, ok := c.members[peerID]
	delete(c.members, peerID)
	c.membersMu.Unlock()

	if ok {
		fmt.Printf("\n[Cluster] Member left: %s (%s)\n> ", member.Name, peerID.String()[:8])
	}
}

func (c *ClusterNode) handleMessages() {
	for msg := range c.node.Messages() {
		// Handle handshake messages
		if msg.StreamName == streams.HandshakeStreamName {
			c.handleHandshakeMessage(msg)
			continue
		}

		// Handle cluster messages
		if msg.StreamName == streamCluster && len(msg.Data) > 0 {
			c.handleClusterMessage(msg)
		}
	}
}

func (c *ClusterNode) handleHandshakeMessage(msg streams.IncomingMessage) {
	if len(msg.Data) == 0 {
		return
	}

	c.handshakesMu.Lock()
	state, ok := c.handshakes[msg.PeerID]
	if !ok {
		state = &handshakeState{}
		c.handshakes[msg.PeerID] = state
	}
	c.handshakesMu.Unlock()

	switch msg.Data[0] {
	case msgHello:
		if len(msg.Data) > 1 {
			state.name = string(msg.Data[1:])
		}
		// Send Hello back with name
		helloMsg := make([]byte, 1+len(c.name))
		helloMsg[0] = msgHello
		copy(helloMsg[1:], c.name)
		_ = c.node.Send(msg.PeerID, streams.HandshakeStreamName, helloMsg)

		// Send PubKey
		pubKeyMsg := make([]byte, 1+len(c.publicKey))
		pubKeyMsg[0] = msgPubKey
		copy(pubKeyMsg[1:], c.publicKey)
		_ = c.node.Send(msg.PeerID, streams.HandshakeStreamName, pubKeyMsg)

	case msgPubKey:
		if len(msg.Data) > 1 {
			state.peerPubKey = ed25519.PublicKey(msg.Data[1:])
			state.gotPubKey = true
		}

	case msgComplete:
		if state.gotPubKey {
			_ = c.node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgComplete})
			if err := c.node.CompleteHandshake(msg.PeerID, state.peerPubKey, []string{streamCluster}); err != nil {
				fmt.Printf("CompleteHandshake failed: %v\n", err)
				return
			}

			// Get addresses from peerstore
			addrs := c.node.PeerAddrs(msg.PeerID)
			c.addMember(msg.PeerID, state.peerPubKey, state.name, addrs)

			c.handshakesMu.Lock()
			delete(c.handshakes, msg.PeerID)
			c.handshakesMu.Unlock()
		}
	}
}

func (c *ClusterNode) handleClusterMessage(msg streams.IncomingMessage) {
	switch msg.Data[0] {
	case msgGossip:
		var gossip GossipMessage
		if err := json.Unmarshal(msg.Data[1:], &gossip); err != nil {
			return
		}
		c.handleGossip(msg.PeerID, &gossip)

	case msgBroadcast:
		var broadcast BroadcastMessage
		if err := json.Unmarshal(msg.Data[1:], &broadcast); err != nil {
			return
		}
		c.handleBroadcast(msg.PeerID, &broadcast)

	case msgPing:
		// Send pong
		_ = c.node.Send(msg.PeerID, streamCluster, []byte{msgPong})

	case msgPong:
		// Update last seen
		c.membersMu.Lock()
		if member := c.members[msg.PeerID]; member != nil {
			member.LastSeen = time.Now().Unix()
		}
		c.membersMu.Unlock()
	}
}

func (c *ClusterNode) handleGossip(from peer.ID, gossip *GossipMessage) {
	// Update last seen for sender
	c.membersMu.Lock()
	if member := c.members[from]; member != nil {
		member.LastSeen = time.Now().Unix()
	}
	c.membersMu.Unlock()

	// Check for unknown peers
	for _, peerInfo := range gossip.KnownPeers {
		peerID, err := peer.Decode(peerInfo.PeerID)
		if err != nil {
			continue
		}

		// Skip self
		if peerID == c.node.PeerID() {
			continue
		}

		// Skip already known
		c.membersMu.RLock()
		_, known := c.members[peerID]
		c.membersMu.RUnlock()
		if known {
			continue
		}

		// Try to connect to new peer
		if len(peerInfo.Addrs) > 0 {
			fullAddr := peerInfo.Addrs[0] + "/p2p/" + peerInfo.PeerID
			fmt.Printf("\n[Gossip] Discovered new peer: %s (%s)\n> ", peerInfo.Name, peerID.String()[:8])
			go c.ConnectPeer(fullAddr)
		}
	}
}

func (c *ClusterNode) handleBroadcast(from peer.ID, broadcast *BroadcastMessage) {
	// Check if already seen
	c.seenMessagesMu.Lock()
	if c.seenMessages[broadcast.MessageID] {
		c.seenMessagesMu.Unlock()
		return
	}
	c.seenMessages[broadcast.MessageID] = true
	c.seenMessagesMu.Unlock()

	// Display message
	fmt.Printf("\n[%s] %s\n> ", broadcast.Name, broadcast.Content)

	// Forward to other members (gossip-style broadcast)
	data, _ := json.Marshal(broadcast)
	fullMsg := make([]byte, 1+len(data))
	fullMsg[0] = msgBroadcast
	copy(fullMsg[1:], data)

	c.membersMu.RLock()
	for pid := range c.members {
		if pid != from {
			_ = c.node.Send(pid, streamCluster, fullMsg)
		}
	}
	c.membersMu.RUnlock()
}

func (c *ClusterNode) gossipRoutine() {
	ticker := time.NewTicker(gossipInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.sendGossip()
		}
	}
}

func (c *ClusterNode) sendGossip() {
	c.membersMu.RLock()
	members := make([]*PeerInfo, 0, len(c.members))
	peerIDs := make([]peer.ID, 0, len(c.members))
	for pid, m := range c.members {
		members = append(members, m)
		peerIDs = append(peerIDs, pid)
	}
	c.membersMu.RUnlock()

	// Add self to gossip
	selfAddrs := make([]string, 0)
	for _, a := range c.node.Addrs() {
		selfAddrs = append(selfAddrs, a.String())
	}
	members = append(members, &PeerInfo{
		PeerID:   c.node.PeerID().String(),
		Addrs:    selfAddrs,
		Name:     c.name,
		LastSeen: time.Now().Unix(),
	})

	gossip := GossipMessage{
		Sender:     c.node.PeerID().String(),
		KnownPeers: members,
	}

	data, _ := json.Marshal(gossip)
	fullMsg := make([]byte, 1+len(data))
	fullMsg[0] = msgGossip
	copy(fullMsg[1:], data)

	for _, pid := range peerIDs {
		_ = c.node.Send(pid, streamCluster, fullMsg)
	}
}

func (c *ClusterNode) pingRoutine() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.sendPings()
		}
	}
}

func (c *ClusterNode) sendPings() {
	c.membersMu.RLock()
	peerIDs := make([]peer.ID, 0, len(c.members))
	for pid := range c.members {
		peerIDs = append(peerIDs, pid)
	}
	c.membersMu.RUnlock()

	for _, pid := range peerIDs {
		_ = c.node.Send(pid, streamCluster, []byte{msgPing})
	}
}

func (c *ClusterNode) handleEvents() {
	for event := range c.node.Events() {
		switch event.State {
		case glueberry.StateDisconnected:
			c.removeMember(event.PeerID)
		}
	}
}

func main() {
	// Parse flags
	port := flag.Int("port", 9000, "Port to listen on")
	name := flag.String("name", "node", "Node name")
	bootstrap := flag.String("bootstrap", "", "Bootstrap peer address")
	flag.Parse()

	// Generate identity
	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// Create listen address
	listenAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port))

	// Create node
	cfg := glueberry.NewConfig(
		privateKey,
		fmt.Sprintf("./addressbook-cluster-%d.json", *port),
		[]multiaddr.Multiaddr{listenAddr},
	)

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

	fmt.Printf("=========================================\n")
	fmt.Printf("   Glueberry Cluster Example\n")
	fmt.Printf("=========================================\n\n")
	fmt.Printf("Node Name: %s\n", *name)
	fmt.Printf("Peer ID:   %s\n", node.PeerID())
	fmt.Printf("Listen:    %v\n\n", node.Addrs())

	// Print connection info
	for _, addr := range node.Addrs() {
		fullAddr := fmt.Sprintf("%s/p2p/%s", addr, node.PeerID())
		fmt.Printf("Bootstrap with: --bootstrap %s\n", fullAddr)
	}
	fmt.Println()

	// Create cluster node
	cluster := NewClusterNode(node, *name, publicKey)
	cluster.Start()
	defer cluster.Stop()

	// Connect to bootstrap peer
	if *bootstrap != "" {
		fmt.Printf("Connecting to bootstrap peer...\n")
		if err := cluster.ConnectPeer(*bootstrap); err != nil {
			fmt.Printf("Failed to connect to bootstrap: %v\n", err)
		}
	}

	// Interactive mode
	fmt.Println("\nCommands:")
	fmt.Println("  msg <message>     - Broadcast a message")
	fmt.Println("  connect <addr>    - Connect to a peer")
	fmt.Println("  members           - List cluster members")
	fmt.Println("  quit              - Exit")
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

		parts := strings.SplitN(line, " ", 2)
		cmd := parts[0]

		switch cmd {
		case "quit", "exit":
			return

		case "msg":
			if len(parts) < 2 {
				fmt.Println("Usage: msg <message>")
				continue
			}
			cluster.Broadcast(parts[1])
			fmt.Println("Message broadcast sent")

		case "connect":
			if len(parts) < 2 {
				fmt.Println("Usage: connect <multiaddr>")
				continue
			}
			if err := cluster.ConnectPeer(parts[1]); err != nil {
				fmt.Printf("Connect failed: %v\n", err)
			}

		case "members":
			members := cluster.Members()
			fmt.Printf("\nCluster members (%d):\n", len(members))
			for _, m := range members {
				fmt.Printf("  %s (%s) - last seen: %s\n",
					m.Name, m.PeerID[:8],
					time.Unix(m.LastSeen, 0).Format("15:04:05"))
			}

		default:
			fmt.Println("Unknown command")
		}
	}
}
