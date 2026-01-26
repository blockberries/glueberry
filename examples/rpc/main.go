// Package main demonstrates request/response (RPC) patterns with Glueberry.
// This example shows how to implement a simple RPC-style communication layer
// on top of Glueberry's encrypted streams.
package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
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

	// RPC message types
	msgRequest  byte = 20
	msgResponse byte = 21

	// Stream name
	streamRPC = "rpc"

	// Default timeout
	defaultTimeout = 10 * time.Second
)

// RPCRequest represents an RPC request
type RPCRequest struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// RPCResponse represents an RPC response
type RPCResponse struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

// RPCError represents an error in an RPC response
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// RPCClient provides RPC call functionality over Glueberry
type RPCClient struct {
	node      *glueberry.Node
	nextID    uint64
	pending   map[uint64]chan *RPCResponse
	pendingMu sync.Mutex
}

// NewRPCClient creates a new RPC client
func NewRPCClient(node *glueberry.Node) *RPCClient {
	return &RPCClient{
		node:    node,
		pending: make(map[uint64]chan *RPCResponse),
	}
}

// Call makes an RPC call and waits for the response
func (c *RPCClient) Call(ctx context.Context, peerID peer.ID, method string, params any) (json.RawMessage, error) {
	// Generate request ID
	reqID := atomic.AddUint64(&c.nextID, 1)

	// Create response channel
	respCh := make(chan *RPCResponse, 1)
	c.pendingMu.Lock()
	c.pending[reqID] = respCh
	c.pendingMu.Unlock()

	// Cleanup on exit
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, reqID)
		c.pendingMu.Unlock()
	}()

	// Marshal params
	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	// Create request
	req := RPCRequest{
		ID:     reqID,
		Method: method,
		Params: paramsJSON,
	}

	// Marshal request
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build message: [type][request_json]
	msg := make([]byte, 1+len(reqJSON))
	msg[0] = msgRequest
	copy(msg[1:], reqJSON)

	// Send request
	if err := c.node.SendCtx(ctx, peerID, streamRPC, msg); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, fmt.Errorf("RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// HandleResponse processes an incoming RPC response
func (c *RPCClient) HandleResponse(data []byte) {
	var resp RPCResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return
	}

	c.pendingMu.Lock()
	ch, ok := c.pending[resp.ID]
	c.pendingMu.Unlock()

	if ok {
		select {
		case ch <- &resp:
		default:
		}
	}
}

// RPCServer handles incoming RPC requests
type RPCServer struct {
	node     *glueberry.Node
	handlers map[string]RPCHandler
	mu       sync.RWMutex
}

// RPCHandler is a function that handles an RPC request
type RPCHandler func(params json.RawMessage) (any, error)

// NewRPCServer creates a new RPC server
func NewRPCServer(node *glueberry.Node) *RPCServer {
	return &RPCServer{
		node:     node,
		handlers: make(map[string]RPCHandler),
	}
}

// Register registers an RPC handler
func (s *RPCServer) Register(method string, handler RPCHandler) {
	s.mu.Lock()
	s.handlers[method] = handler
	s.mu.Unlock()
}

// HandleRequest processes an incoming RPC request
func (s *RPCServer) HandleRequest(peerID peer.ID, data []byte) {
	var req RPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		s.sendError(peerID, 0, -32700, "Parse error")
		return
	}

	s.mu.RLock()
	handler, ok := s.handlers[req.Method]
	s.mu.RUnlock()

	if !ok {
		s.sendError(peerID, req.ID, -32601, "Method not found: "+req.Method)
		return
	}

	result, err := handler(req.Params)
	if err != nil {
		s.sendError(peerID, req.ID, -32000, err.Error())
		return
	}

	s.sendResult(peerID, req.ID, result)
}

func (s *RPCServer) sendResult(peerID peer.ID, reqID uint64, result any) {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		s.sendError(peerID, reqID, -32603, "Internal error")
		return
	}

	resp := RPCResponse{
		ID:     reqID,
		Result: resultJSON,
	}

	respJSON, _ := json.Marshal(resp)
	msg := make([]byte, 1+len(respJSON))
	msg[0] = msgResponse
	copy(msg[1:], respJSON)

	_ = s.node.Send(peerID, streamRPC, msg)
}

func (s *RPCServer) sendError(peerID peer.ID, reqID uint64, code int, message string) {
	resp := RPCResponse{
		ID: reqID,
		Error: &RPCError{
			Code:    code,
			Message: message,
		},
	}

	respJSON, _ := json.Marshal(resp)
	msg := make([]byte, 1+len(respJSON))
	msg[0] = msgResponse
	copy(msg[1:], respJSON)

	_ = s.node.Send(peerID, streamRPC, msg)
}

// Example RPC methods
type EchoParams struct {
	Message string `json:"message"`
}

type AddParams struct {
	A int `json:"a"`
	B int `json:"b"`
}

type GetTimeResult struct {
	Time   string `json:"time"`
	Unix   int64  `json:"unix"`
	PeerID string `json:"peer_id"`
}

func main() {
	// Parse command line flags
	port := flag.Int("port", 9000, "Port to listen on")
	peerAddr := flag.String("peer", "", "Peer multiaddr to connect to")
	flag.Parse()

	// Generate identity
	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// Create listen address
	listenAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port))

	// Create node
	cfg := glueberry.NewConfig(
		privateKey,
		"./addressbook-rpc.json",
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
	fmt.Printf("     Glueberry RPC Example\n")
	fmt.Printf("=========================================\n\n")
	fmt.Printf("Peer ID: %s\n", node.PeerID())
	fmt.Printf("Listen:  %v\n\n", node.Addrs())

	// Print connection info
	for _, addr := range node.Addrs() {
		fullAddr := fmt.Sprintf("%s/p2p/%s", addr, node.PeerID())
		fmt.Printf("Connect with: --peer %s\n", fullAddr)
	}
	fmt.Println()

	// Create RPC client and server
	rpcClient := NewRPCClient(node)
	rpcServer := NewRPCServer(node)

	// Register RPC handlers
	rpcServer.Register("echo", func(params json.RawMessage) (any, error) {
		var p EchoParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return map[string]string{"echo": p.Message}, nil
	})

	rpcServer.Register("add", func(params json.RawMessage) (any, error) {
		var p AddParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return map[string]int{"sum": p.A + p.B}, nil
	})

	rpcServer.Register("getTime", func(params json.RawMessage) (any, error) {
		now := time.Now()
		return GetTimeResult{
			Time:   now.Format(time.RFC3339),
			Unix:   now.Unix(),
			PeerID: node.PeerID().String()[:8],
		}, nil
	})

	rpcServer.Register("getPeers", func(params json.RawMessage) (any, error) {
		peers := node.ListPeers()
		peerList := make([]string, len(peers))
		for i, p := range peers {
			peerList[i] = p.PeerID.String()
		}
		return map[string]any{
			"count": len(peers),
			"peers": peerList,
		}, nil
	})

	// Handshake state tracking
	type peerState struct {
		gotPubKey  bool
		peerPubKey ed25519.PublicKey
	}
	peerStates := make(map[peer.ID]*peerState)
	peerStatesMu := sync.Mutex{}

	// Track connected peers for RPC calls
	var connectedPeer peer.ID
	connectedPeerMu := sync.Mutex{}

	// Handle messages
	go func() {
		for msg := range node.Messages() {
			// Handle handshake messages
			if msg.StreamName == streams.HandshakeStreamName {
				peerStatesMu.Lock()
				state, ok := peerStates[msg.PeerID]
				if !ok {
					state = &peerState{}
					peerStates[msg.PeerID] = state
				}
				peerStatesMu.Unlock()

				if len(msg.Data) == 0 {
					continue
				}

				switch msg.Data[0] {
				case msgHello:
					_ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgHello})
					pubKeyMsg := make([]byte, 1+len(publicKey))
					pubKeyMsg[0] = msgPubKey
					copy(pubKeyMsg[1:], publicKey)
					_ = node.Send(msg.PeerID, streams.HandshakeStreamName, pubKeyMsg)

				case msgPubKey:
					if len(msg.Data) > 1 {
						state.peerPubKey = ed25519.PublicKey(msg.Data[1:])
						state.gotPubKey = true
					}

				case msgComplete:
					if state.gotPubKey {
						_ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgComplete})
						if err := node.CompleteHandshake(msg.PeerID, state.peerPubKey, []string{streamRPC}); err != nil {
							fmt.Printf("CompleteHandshake failed: %v\n", err)
						}
						peerStatesMu.Lock()
						delete(peerStates, msg.PeerID)
						peerStatesMu.Unlock()
					}
				}
				continue
			}

			// Handle RPC messages
			if msg.StreamName == streamRPC && len(msg.Data) > 1 {
				switch msg.Data[0] {
				case msgRequest:
					rpcServer.HandleRequest(msg.PeerID, msg.Data[1:])
				case msgResponse:
					rpcClient.HandleResponse(msg.Data[1:])
				}
			}
		}
	}()

	// Handle events
	go func() {
		for event := range node.Events() {
			switch event.State {
			case glueberry.StateEstablished:
				fmt.Printf("Connected to %s - ready for RPC\n", event.PeerID.String()[:8])
				connectedPeerMu.Lock()
				connectedPeer = event.PeerID
				connectedPeerMu.Unlock()
			case glueberry.StateDisconnected:
				fmt.Printf("Disconnected from %s\n", event.PeerID.String()[:8])
				connectedPeerMu.Lock()
				if connectedPeer == event.PeerID {
					connectedPeer = ""
				}
				connectedPeerMu.Unlock()
			}
		}
	}()

	// Connect to peer if specified
	if *peerAddr != "" {
		go func() {
			addr, _ := multiaddr.NewMultiaddr(*peerAddr)
			peerInfo, _ := peer.AddrInfoFromP2pAddr(addr)

			peerStatesMu.Lock()
			peerStates[peerInfo.ID] = &peerState{}
			peerStatesMu.Unlock()

			node.AddPeer(peerInfo.ID, peerInfo.Addrs, nil)
			if err := node.Connect(peerInfo.ID); err != nil {
				fmt.Printf("Connect failed: %v\n", err)
				return
			}

			_ = node.Send(peerInfo.ID, streams.HandshakeStreamName, []byte{msgHello})
			pubKeyMsg := make([]byte, 1+len(publicKey))
			pubKeyMsg[0] = msgPubKey
			copy(pubKeyMsg[1:], publicKey)
			_ = node.Send(peerInfo.ID, streams.HandshakeStreamName, pubKeyMsg)

			for i := 0; i < 100; i++ {
				peerStatesMu.Lock()
				state := peerStates[peerInfo.ID]
				if state != nil && state.gotPubKey {
					peerStatesMu.Unlock()
					_ = node.Send(peerInfo.ID, streams.HandshakeStreamName, []byte{msgComplete})
					_ = node.CompleteHandshake(peerInfo.ID, state.peerPubKey, []string{streamRPC})
					peerStatesMu.Lock()
					delete(peerStates, peerInfo.ID)
					peerStatesMu.Unlock()
					break
				}
				peerStatesMu.Unlock()
			}
		}()
	}

	// Interactive mode
	fmt.Println("RPC Methods available:")
	fmt.Println("  echo <message>  - Echo back the message")
	fmt.Println("  add <a> <b>     - Add two numbers")
	fmt.Println("  time            - Get server time")
	fmt.Println("  peers           - Get peer list")
	fmt.Println("  quit            - Exit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		line := scanner.Text()
		var cmd string
		fmt.Sscanf(line, "%s", &cmd)

		connectedPeerMu.Lock()
		targetPeer := connectedPeer
		connectedPeerMu.Unlock()

		if cmd == "quit" || cmd == "exit" {
			return
		}

		if targetPeer == "" {
			fmt.Println("No peer connected. Use --peer to connect.")
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)

		switch cmd {
		case "echo":
			var msg string
			fmt.Sscanf(line, "echo %s", &msg)
			if msg == "" {
				msg = "hello"
			}
			result, err := rpcClient.Call(ctx, targetPeer, "echo", EchoParams{Message: msg})
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Printf("Result: %s\n", string(result))
			}

		case "add":
			var a, b int
			fmt.Sscanf(line, "add %d %d", &a, &b)
			result, err := rpcClient.Call(ctx, targetPeer, "add", AddParams{A: a, B: b})
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Printf("Result: %s\n", string(result))
			}

		case "time":
			result, err := rpcClient.Call(ctx, targetPeer, "getTime", nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Printf("Result: %s\n", string(result))
			}

		case "peers":
			result, err := rpcClient.Call(ctx, targetPeer, "getPeers", nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Printf("Result: %s\n", string(result))
			}

		default:
			fmt.Println("Unknown command")
		}

		cancel()
	}
}
