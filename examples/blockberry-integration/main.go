// Package main demonstrates how a blockchain application (like Blockberry)
// might use Glueberry for P2P networking.
//
// This example shows:
// - Custom handshake protocol with node identification
// - Multiple stream types for different message categories
// - Connection direction handling (important for protocol roles)
// - Peer scoring and management
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

// Protocol constants mimicking a blockchain node
const (
	// Handshake message types (exchanged on handshake stream)
	msgHelloRequest  byte = 1 // Sent by initiator: version, genesis, height
	msgHelloResponse byte = 2 // Sent by responder: version, genesis, height
	msgHelloFinalize byte = 3 // Sent by both to confirm

	// Blockchain streams
	streamConsensus = "consensus" // Consensus messages (votes, proposals)
	streamMempool   = "mempool"   // Transaction gossip
	streamSync      = "sync"      // Block sync requests/responses

	// Protocol version
	protocolVersion = uint32(1)
)

// HelloMessage is exchanged during handshake to verify compatibility
type HelloMessage struct {
	Version     uint32 `json:"version"`
	GenesisHash string `json:"genesis_hash"` // Must match for nodes to connect
	ChainID     string `json:"chain_id"`
	NodeID      string `json:"node_id"`
	Height      uint64 `json:"height"` // Current blockchain height
	PublicKey   []byte `json:"public_key"`
}

// ConsensusMessage represents a consensus protocol message
type ConsensusMessage struct {
	Type      string `json:"type"` // "proposal", "prevote", "precommit"
	Height    uint64 `json:"height"`
	Round     uint32 `json:"round"`
	BlockHash string `json:"block_hash,omitempty"`
	Signature []byte `json:"signature,omitempty"`
	Sender    string `json:"sender"`
}

// Transaction represents a transaction to be gossiped
type Transaction struct {
	Hash      string `json:"hash"`
	Data      []byte `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

// SyncRequest requests blocks from a peer
type SyncRequest struct {
	FromHeight uint64 `json:"from_height"`
	ToHeight   uint64 `json:"to_height"`
}

// BlockchainNode simulates a blockchain node using Glueberry
type BlockchainNode struct {
	node      *glueberry.Node
	chainID   string
	genesis   string
	publicKey ed25519.PublicKey

	// Blockchain state (simplified)
	height    uint64
	heightMu  sync.RWMutex

	// Peer management
	peers     map[peer.ID]*PeerState
	peersMu   sync.RWMutex
	handshakes   map[peer.ID]*handshakeProgress
	handshakesMu sync.Mutex

	// Mempool (simplified)
	mempool   map[string]*Transaction
	mempoolMu sync.RWMutex
}

// PeerState tracks peer information
type PeerState struct {
	NodeID     string
	Height     uint64
	IsOutbound bool
	Score      int // Simple scoring: higher is better
	LastSeen   time.Time
}

type handshakeProgress struct {
	gotHello   bool
	sentHello  bool
	peerInfo   *HelloMessage
	peerPubKey ed25519.PublicKey
}

// NewBlockchainNode creates a new blockchain node
func NewBlockchainNode(node *glueberry.Node, chainID, genesis string, publicKey ed25519.PublicKey) *BlockchainNode {
	return &BlockchainNode{
		node:       node,
		chainID:    chainID,
		genesis:    genesis,
		publicKey:  publicKey,
		height:     1, // Start at block 1
		peers:      make(map[peer.ID]*PeerState),
		handshakes: make(map[peer.ID]*handshakeProgress),
		mempool:    make(map[string]*Transaction),
	}
}

// Start begins blockchain node operations
func (b *BlockchainNode) Start() {
	go b.handleMessages()
	go b.handleEvents()
}

// Height returns current blockchain height
func (b *BlockchainNode) Height() uint64 {
	b.heightMu.RLock()
	defer b.heightMu.RUnlock()
	return b.height
}

// SetHeight sets the blockchain height (simulating block production)
func (b *BlockchainNode) SetHeight(h uint64) {
	b.heightMu.Lock()
	b.height = h
	b.heightMu.Unlock()
}

// Connect connects to a peer using blockchain handshake protocol
func (b *BlockchainNode) Connect(addrStr string) error {
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}

	peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return fmt.Errorf("failed to parse peer info: %w", err)
	}

	b.handshakesMu.Lock()
	b.handshakes[peerInfo.ID] = &handshakeProgress{}
	b.handshakesMu.Unlock()

	b.node.AddPeer(peerInfo.ID, peerInfo.Addrs, nil)

	if err := b.node.Connect(peerInfo.ID); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}

	// As initiator, send HelloRequest
	if err := b.sendHelloRequest(peerInfo.ID); err != nil {
		return fmt.Errorf("failed to send hello: %w", err)
	}

	return nil
}

// BroadcastTransaction gossips a transaction to all peers
func (b *BlockchainNode) BroadcastTransaction(tx *Transaction) {
	// Add to local mempool
	b.mempoolMu.Lock()
	b.mempool[tx.Hash] = tx
	b.mempoolMu.Unlock()

	// Send to all peers
	data, _ := json.Marshal(tx)
	msg := make([]byte, 1+len(data))
	msg[0] = 1 // TX_GOSSIP message type
	copy(msg[1:], data)

	b.peersMu.RLock()
	peers := make([]peer.ID, 0, len(b.peers))
	for pid := range b.peers {
		peers = append(peers, pid)
	}
	b.peersMu.RUnlock()

	for _, pid := range peers {
		_ = b.node.Send(pid, streamMempool, msg)
	}
}

// ProposeBlock simulates proposing a new block
func (b *BlockchainNode) ProposeBlock() {
	height := b.Height()
	msg := ConsensusMessage{
		Type:      "proposal",
		Height:    height + 1,
		Round:     0,
		BlockHash: fmt.Sprintf("block-%d", height+1),
		Sender:    b.node.PeerID().String()[:8],
	}

	data, _ := json.Marshal(msg)
	fullMsg := make([]byte, 1+len(data))
	fullMsg[0] = 1 // PROPOSAL message type
	copy(fullMsg[1:], data)

	b.peersMu.RLock()
	peers := make([]peer.ID, 0, len(b.peers))
	for pid := range b.peers {
		peers = append(peers, pid)
	}
	b.peersMu.RUnlock()

	for _, pid := range peers {
		_ = b.node.Send(pid, streamConsensus, fullMsg)
	}

	fmt.Printf("[Consensus] Proposed block at height %d\n", height+1)
}

// Peers returns connected peer states
func (b *BlockchainNode) Peers() map[peer.ID]*PeerState {
	b.peersMu.RLock()
	defer b.peersMu.RUnlock()
	result := make(map[peer.ID]*PeerState)
	for k, v := range b.peers {
		result[k] = v
	}
	return result
}

func (b *BlockchainNode) sendHelloRequest(peerID peer.ID) error {
	hello := HelloMessage{
		Version:     protocolVersion,
		GenesisHash: b.genesis,
		ChainID:     b.chainID,
		NodeID:      b.node.PeerID().String()[:8],
		Height:      b.Height(),
		PublicKey:   b.publicKey,
	}

	data, _ := json.Marshal(hello)
	msg := make([]byte, 1+len(data))
	msg[0] = msgHelloRequest
	copy(msg[1:], data)

	b.handshakesMu.Lock()
	if progress := b.handshakes[peerID]; progress != nil {
		progress.sentHello = true
	}
	b.handshakesMu.Unlock()

	return b.node.Send(peerID, streams.HandshakeStreamName, msg)
}

func (b *BlockchainNode) sendHelloResponse(peerID peer.ID) error {
	hello := HelloMessage{
		Version:     protocolVersion,
		GenesisHash: b.genesis,
		ChainID:     b.chainID,
		NodeID:      b.node.PeerID().String()[:8],
		Height:      b.Height(),
		PublicKey:   b.publicKey,
	}

	data, _ := json.Marshal(hello)
	msg := make([]byte, 1+len(data))
	msg[0] = msgHelloResponse
	copy(msg[1:], data)

	b.handshakesMu.Lock()
	if progress := b.handshakes[peerID]; progress != nil {
		progress.sentHello = true
	}
	b.handshakesMu.Unlock()

	return b.node.Send(peerID, streams.HandshakeStreamName, msg)
}

func (b *BlockchainNode) sendHelloFinalize(peerID peer.ID) error {
	msg := []byte{msgHelloFinalize}
	return b.node.Send(peerID, streams.HandshakeStreamName, msg)
}

func (b *BlockchainNode) handleMessages() {
	for msg := range b.node.Messages() {
		switch msg.StreamName {
		case streams.HandshakeStreamName:
			b.handleHandshakeMessage(msg)
		case streamConsensus:
			b.handleConsensusMessage(msg)
		case streamMempool:
			b.handleMempoolMessage(msg)
		case streamSync:
			b.handleSyncMessage(msg)
		}
	}
}

func (b *BlockchainNode) handleHandshakeMessage(msg streams.IncomingMessage) {
	if len(msg.Data) == 0 {
		return
	}

	b.handshakesMu.Lock()
	progress, ok := b.handshakes[msg.PeerID]
	if !ok {
		progress = &handshakeProgress{}
		b.handshakes[msg.PeerID] = progress
	}
	b.handshakesMu.Unlock()

	switch msg.Data[0] {
	case msgHelloRequest:
		// We received a request - we are the responder
		var hello HelloMessage
		if err := json.Unmarshal(msg.Data[1:], &hello); err != nil {
			fmt.Printf("[Handshake] Invalid HelloRequest from %s\n", msg.PeerID.String()[:8])
			return
		}

		// Validate genesis/chain
		if hello.GenesisHash != b.genesis {
			fmt.Printf("[Handshake] Genesis mismatch with %s\n", msg.PeerID.String()[:8])
			_ = b.node.Disconnect(msg.PeerID)
			return
		}

		if hello.ChainID != b.chainID {
			fmt.Printf("[Handshake] Chain ID mismatch with %s\n", msg.PeerID.String()[:8])
			_ = b.node.Disconnect(msg.PeerID)
			return
		}

		progress.gotHello = true
		progress.peerInfo = &hello
		progress.peerPubKey = ed25519.PublicKey(hello.PublicKey)

		fmt.Printf("[Handshake] Received HelloRequest from %s (height: %d)\n",
			hello.NodeID, hello.Height)

		// Send our response
		_ = b.sendHelloResponse(msg.PeerID)

	case msgHelloResponse:
		// We received a response - we are the initiator
		var hello HelloMessage
		if err := json.Unmarshal(msg.Data[1:], &hello); err != nil {
			fmt.Printf("[Handshake] Invalid HelloResponse from %s\n", msg.PeerID.String()[:8])
			return
		}

		// Validate
		if hello.GenesisHash != b.genesis || hello.ChainID != b.chainID {
			fmt.Printf("[Handshake] Chain mismatch with %s\n", msg.PeerID.String()[:8])
			_ = b.node.Disconnect(msg.PeerID)
			return
		}

		progress.gotHello = true
		progress.peerInfo = &hello
		progress.peerPubKey = ed25519.PublicKey(hello.PublicKey)

		fmt.Printf("[Handshake] Received HelloResponse from %s (height: %d)\n",
			hello.NodeID, hello.Height)

		// Both sides got hello, prepare streams and send finalize
		if err := b.node.PrepareStreams(msg.PeerID, progress.peerPubKey,
			[]string{streamConsensus, streamMempool, streamSync}); err != nil {
			fmt.Printf("[Handshake] PrepareStreams failed: %v\n", err)
			return
		}

		_ = b.sendHelloFinalize(msg.PeerID)

	case msgHelloFinalize:
		if !progress.gotHello {
			return
		}

		// Check if we've sent hello (we're the responder)
		if progress.sentHello && progress.peerPubKey != nil {
			// Prepare our streams and send finalize
			if err := b.node.PrepareStreams(msg.PeerID, progress.peerPubKey,
				[]string{streamConsensus, streamMempool, streamSync}); err != nil {
				fmt.Printf("[Handshake] PrepareStreams failed: %v\n", err)
				return
			}
			_ = b.sendHelloFinalize(msg.PeerID)
		}

		// Finalize the handshake
		if err := b.node.FinalizeHandshake(msg.PeerID); err != nil {
			fmt.Printf("[Handshake] FinalizeHandshake failed: %v\n", err)
			return
		}

		// Determine connection direction
		isOutbound, _ := b.node.IsOutbound(msg.PeerID)

		// Add to peers
		b.peersMu.Lock()
		b.peers[msg.PeerID] = &PeerState{
			NodeID:     progress.peerInfo.NodeID,
			Height:     progress.peerInfo.Height,
			IsOutbound: isOutbound,
			Score:      100, // Initial score
			LastSeen:   time.Now(),
		}
		b.peersMu.Unlock()

		direction := "inbound"
		if isOutbound {
			direction = "outbound"
		}
		fmt.Printf("[Handshake] Complete with %s (%s connection)\n",
			progress.peerInfo.NodeID, direction)

		b.handshakesMu.Lock()
		delete(b.handshakes, msg.PeerID)
		b.handshakesMu.Unlock()
	}
}

func (b *BlockchainNode) handleConsensusMessage(msg streams.IncomingMessage) {
	if len(msg.Data) < 2 {
		return
	}

	var consensus ConsensusMessage
	if err := json.Unmarshal(msg.Data[1:], &consensus); err != nil {
		return
	}

	fmt.Printf("[Consensus] Received %s for height %d from %s\n",
		consensus.Type, consensus.Height, consensus.Sender)

	// Update peer's last seen
	b.peersMu.Lock()
	if peer := b.peers[msg.PeerID]; peer != nil {
		peer.LastSeen = time.Now()
	}
	b.peersMu.Unlock()
}

func (b *BlockchainNode) handleMempoolMessage(msg streams.IncomingMessage) {
	if len(msg.Data) < 2 {
		return
	}

	var tx Transaction
	if err := json.Unmarshal(msg.Data[1:], &tx); err != nil {
		return
	}

	b.mempoolMu.Lock()
	if _, exists := b.mempool[tx.Hash]; !exists {
		b.mempool[tx.Hash] = &tx
		fmt.Printf("[Mempool] Received tx %s\n", tx.Hash[:8])

		// Re-gossip to other peers
		go func() {
			b.peersMu.RLock()
			peers := make([]peer.ID, 0, len(b.peers))
			for pid := range b.peers {
				if pid != msg.PeerID {
					peers = append(peers, pid)
				}
			}
			b.peersMu.RUnlock()

			for _, pid := range peers {
				_ = b.node.Send(pid, streamMempool, msg.Data)
			}
		}()
	}
	b.mempoolMu.Unlock()
}

func (b *BlockchainNode) handleSyncMessage(msg streams.IncomingMessage) {
	if len(msg.Data) < 2 {
		return
	}

	var req SyncRequest
	if err := json.Unmarshal(msg.Data[1:], &req); err != nil {
		return
	}

	fmt.Printf("[Sync] Received sync request from %s: heights %d-%d\n",
		msg.PeerID.String()[:8], req.FromHeight, req.ToHeight)

	// In a real implementation, we would send blocks here
}

func (b *BlockchainNode) handleEvents() {
	for event := range b.node.Events() {
		switch event.State {
		case glueberry.StateDisconnected:
			b.peersMu.Lock()
			if peer := b.peers[event.PeerID]; peer != nil {
				fmt.Printf("[Network] Peer %s disconnected\n", peer.NodeID)
			}
			delete(b.peers, event.PeerID)
			b.peersMu.Unlock()

			b.handshakesMu.Lock()
			delete(b.handshakes, event.PeerID)
			b.handshakesMu.Unlock()
		}
	}
}

func main() {
	port := flag.Int("port", 9000, "Port to listen on")
	chainID := flag.String("chain", "testnet-1", "Chain ID")
	genesis := flag.String("genesis", "genesis-hash-abc123", "Genesis hash")
	peerAddr := flag.String("peer", "", "Peer to connect to")
	flag.Parse()

	// Generate identity
	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// Create listen address
	listenAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port))

	// Create Glueberry node
	cfg := glueberry.NewConfig(
		privateKey,
		fmt.Sprintf("./addressbook-blockchain-%d.json", *port),
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
	fmt.Printf(" Glueberry Blockchain Integration Example\n")
	fmt.Printf("=========================================\n\n")
	fmt.Printf("Chain ID:  %s\n", *chainID)
	fmt.Printf("Genesis:   %s\n", *genesis)
	fmt.Printf("Peer ID:   %s\n", node.PeerID())
	fmt.Printf("Listen:    %v\n\n", node.Addrs())

	for _, addr := range node.Addrs() {
		fullAddr := fmt.Sprintf("%s/p2p/%s", addr, node.PeerID())
		fmt.Printf("Connect with: --peer %s\n", fullAddr)
	}
	fmt.Println()

	// Create blockchain node
	blockchain := NewBlockchainNode(node, *chainID, *genesis, publicKey)
	blockchain.Start()

	// Connect to peer if specified
	if *peerAddr != "" {
		fmt.Printf("Connecting to peer...\n")
		if err := blockchain.Connect(*peerAddr); err != nil {
			fmt.Printf("Connect failed: %v\n", err)
		}
	}

	// Interactive mode
	fmt.Println("\nCommands:")
	fmt.Println("  connect <addr>  - Connect to a peer")
	fmt.Println("  tx <data>       - Broadcast a transaction")
	fmt.Println("  propose         - Propose a new block")
	fmt.Println("  height <n>      - Set blockchain height")
	fmt.Println("  peers           - List connected peers")
	fmt.Println("  mempool         - Show mempool")
	fmt.Println("  quit            - Exit")
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

		case "connect":
			if len(parts) < 2 {
				fmt.Println("Usage: connect <multiaddr>")
				continue
			}
			if err := blockchain.Connect(parts[1]); err != nil {
				fmt.Printf("Connect failed: %v\n", err)
			}

		case "tx":
			data := "test transaction"
			if len(parts) > 1 {
				data = parts[1]
			}
			tx := &Transaction{
				Hash:      fmt.Sprintf("%x", time.Now().UnixNano())[:16],
				Data:      []byte(data),
				Timestamp: time.Now().Unix(),
			}
			blockchain.BroadcastTransaction(tx)
			fmt.Printf("Broadcast tx: %s\n", tx.Hash)

		case "propose":
			blockchain.ProposeBlock()

		case "height":
			if len(parts) < 2 {
				fmt.Printf("Current height: %d\n", blockchain.Height())
				continue
			}
			var h uint64
			fmt.Sscanf(parts[1], "%d", &h)
			blockchain.SetHeight(h)
			fmt.Printf("Height set to %d\n", h)

		case "peers":
			peers := blockchain.Peers()
			fmt.Printf("\nConnected peers (%d):\n", len(peers))
			for pid, p := range peers {
				direction := "inbound"
				if p.IsOutbound {
					direction = "outbound"
				}
				fmt.Printf("  %s (%s) - height: %d, score: %d, %s\n",
					p.NodeID, pid.String()[:8], p.Height, p.Score, direction)
			}

		case "mempool":
			blockchain.mempoolMu.RLock()
			fmt.Printf("\nMempool (%d txs):\n", len(blockchain.mempool))
			for hash, tx := range blockchain.mempool {
				fmt.Printf("  %s: %s\n", hash[:8], string(tx.Data))
			}
			blockchain.mempoolMu.RUnlock()

		default:
			fmt.Println("Unknown command")
		}
	}
}
