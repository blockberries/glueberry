// Package main demonstrates large data transfer with Glueberry.
// This example shows how to implement file transfer with progress reporting
// and chunked data transmission over encrypted streams.
package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

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

	// File transfer message types
	msgFileStart byte = 10 // Start file transfer: [type][filename_len][filename][file_size]
	msgFileChunk byte = 11 // File chunk: [type][chunk_index][chunk_data]
	msgFileEnd   byte = 12 // End file transfer: [type][checksum]
	msgFileAck   byte = 13 // Acknowledgment: [type][chunk_index]

	// Stream names
	streamFile = "file-transfer"

	// Transfer settings
	chunkSize = 64 * 1024 // 64KB chunks
)

// FileTransfer tracks an ongoing file transfer
type FileTransfer struct {
	Filename    string
	TotalSize   int64
	ChunksTotal int
	ChunksRecv  int
	Data        *bytes.Buffer
	Checksum    []byte
}

var (
	transfers   = make(map[peer.ID]*FileTransfer)
	transfersMu sync.Mutex
)

func main() {
	// Parse command line flags
	port := flag.Int("port", 9000, "Port to listen on")
	peerAddr := flag.String("peer", "", "Peer multiaddr to connect to")
	sendFile := flag.String("send", "", "File to send after connecting")
	outputDir := flag.String("output", ".", "Directory to save received files")
	flag.Parse()

	// Generate identity
	_, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	publicKey := privateKey.Public().(ed25519.PublicKey)

	// Create listen address
	listenAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", *port))

	// Create node
	cfg := glueberry.NewConfig(
		privateKey,
		"./addressbook-transfer.json",
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
	fmt.Printf("   Glueberry File Transfer Example\n")
	fmt.Printf("=========================================\n\n")
	fmt.Printf("Peer ID: %s\n", node.PeerID())
	fmt.Printf("Listen:  %v\n\n", node.Addrs())

	// Print connection info
	for _, addr := range node.Addrs() {
		fullAddr := fmt.Sprintf("%s/p2p/%s", addr, node.PeerID())
		fmt.Printf("Connect with: --peer %s\n", fullAddr)
	}
	fmt.Println()

	// Handshake state tracking
	type peerState struct {
		gotPubKey  bool
		peerPubKey ed25519.PublicKey
	}
	peerStates := make(map[peer.ID]*peerState)
	peerStatesMu := sync.Mutex{}

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
					fmt.Printf("Received Hello from %s\n", msg.PeerID.String()[:8])
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
						fmt.Printf("Received PubKey from %s\n", msg.PeerID.String()[:8])
					}

				case msgComplete:
					fmt.Printf("Received Complete from %s\n", msg.PeerID.String()[:8])
					if state.gotPubKey {
						_ = node.Send(msg.PeerID, streams.HandshakeStreamName, []byte{msgComplete})
						if err := node.CompleteHandshake(msg.PeerID, state.peerPubKey, []string{streamFile}); err != nil {
							fmt.Printf("CompleteHandshake failed: %v\n", err)
						} else {
							fmt.Printf("Secure connection established with %s\n", msg.PeerID.String()[:8])
						}
						peerStatesMu.Lock()
						delete(peerStates, msg.PeerID)
						peerStatesMu.Unlock()
					}
				}
				continue
			}

			// Handle file transfer messages
			if msg.StreamName == streamFile && len(msg.Data) > 0 {
				handleFileTransferMessage(node, msg, *outputDir)
			}
		}
	}()

	// Handle events
	go func() {
		for event := range node.Events() {
			switch event.State {
			case glueberry.StateConnected:
				fmt.Printf("Connected to %s\n", event.PeerID.String()[:8])
			case glueberry.StateEstablished:
				fmt.Printf("Secure connection ready with %s\n", event.PeerID.String()[:8])
				// If we have a file to send, send it now
				if *sendFile != "" && *peerAddr != "" {
					go func(peerID peer.ID, filename string) {
						if err := sendFileTransfer(node, peerID, filename); err != nil {
							fmt.Printf("Failed to send file: %v\n", err)
						}
					}(event.PeerID, *sendFile)
				}
			case glueberry.StateDisconnected:
				fmt.Printf("Disconnected from %s\n", event.PeerID.String()[:8])
			}
		}
	}()

	// Connect to peer if specified
	if *peerAddr != "" {
		go func() {
			addr, err := multiaddr.NewMultiaddr(*peerAddr)
			if err != nil {
				fmt.Printf("Invalid peer address: %v\n", err)
				return
			}

			peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
			if err != nil {
				fmt.Printf("Failed to parse peer info: %v\n", err)
				return
			}

			peerStatesMu.Lock()
			peerStates[peerInfo.ID] = &peerState{}
			peerStatesMu.Unlock()

			node.AddPeer(peerInfo.ID, peerInfo.Addrs, nil)

			if err := node.Connect(peerInfo.ID); err != nil {
				fmt.Printf("Connect failed: %v\n", err)
				return
			}

			// Send Hello
			_ = node.Send(peerInfo.ID, streams.HandshakeStreamName, []byte{msgHello})
			// Send PubKey
			pubKeyMsg := make([]byte, 1+len(publicKey))
			pubKeyMsg[0] = msgPubKey
			copy(pubKeyMsg[1:], publicKey)
			_ = node.Send(peerInfo.ID, streams.HandshakeStreamName, pubKeyMsg)

			// Wait for peer's pubkey and send Complete
			for i := 0; i < 100; i++ {
				peerStatesMu.Lock()
				state := peerStates[peerInfo.ID]
				if state != nil && state.gotPubKey {
					peerStatesMu.Unlock()
					_ = node.Send(peerInfo.ID, streams.HandshakeStreamName, []byte{msgComplete})
					if err := node.CompleteHandshake(peerInfo.ID, state.peerPubKey, []string{streamFile}); err != nil {
						fmt.Printf("CompleteHandshake failed: %v\n", err)
					}
					peerStatesMu.Lock()
					delete(peerStates, peerInfo.ID)
					peerStatesMu.Unlock()
					break
				}
				peerStatesMu.Unlock()
			}
		}()
	}

	// Interactive mode if no file specified
	if *sendFile == "" {
		fmt.Println("\nCommands:")
		fmt.Println("  send <peer-id> <filepath> - Send a file to a peer")
		fmt.Println("  quit - Exit")
		fmt.Println()

		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Print("> ")
			if !scanner.Scan() {
				break
			}

			var cmd, arg1, arg2 string
			line := scanner.Text()
			fmt.Sscanf(line, "%s %s %s", &cmd, &arg1, &arg2)

			switch cmd {
			case "quit", "exit":
				return
			case "send":
				if arg1 == "" || arg2 == "" {
					fmt.Println("Usage: send <peer-id> <filepath>")
					continue
				}
				peerID, err := peer.Decode(arg1)
				if err != nil {
					fmt.Printf("Invalid peer ID: %v\n", err)
					continue
				}
				go func() {
					if err := sendFileTransfer(node, peerID, arg2); err != nil {
						fmt.Printf("Failed to send file: %v\n", err)
					}
				}()
			default:
				fmt.Println("Unknown command")
			}
		}
	} else {
		// Wait for transfer to complete
		select {}
	}
}

// sendFileTransfer sends a file to a peer with progress reporting
func sendFileTransfer(node *glueberry.Node, peerID peer.ID, filepath string) error {
	// Open file
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	filename := info.Name()
	fileSize := info.Size()
	chunksTotal := int((fileSize + chunkSize - 1) / chunkSize)

	fmt.Printf("Sending file: %s (%d bytes, %d chunks)\n", filename, fileSize, chunksTotal)

	// Calculate checksum while reading
	hasher := sha256.New()

	// Send file start message
	startMsg := make([]byte, 1+1+len(filename)+8)
	startMsg[0] = msgFileStart
	startMsg[1] = byte(len(filename))
	copy(startMsg[2:], filename)
	binary.BigEndian.PutUint64(startMsg[2+len(filename):], uint64(fileSize))

	if err := node.Send(peerID, streamFile, startMsg); err != nil {
		return fmt.Errorf("failed to send file start: %w", err)
	}

	// Send chunks
	buffer := make([]byte, chunkSize)
	chunkIndex := 0

	for {
		n, err := file.Read(buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		// Update checksum
		hasher.Write(buffer[:n])

		// Build chunk message
		chunkMsg := make([]byte, 1+4+n)
		chunkMsg[0] = msgFileChunk
		binary.BigEndian.PutUint32(chunkMsg[1:5], uint32(chunkIndex))
		copy(chunkMsg[5:], buffer[:n])

		if err := node.Send(peerID, streamFile, chunkMsg); err != nil {
			return fmt.Errorf("failed to send chunk %d: %w", chunkIndex, err)
		}

		chunkIndex++
		progress := float64(chunkIndex) / float64(chunksTotal) * 100
		fmt.Printf("\rSending: %.1f%% (%d/%d chunks)", progress, chunkIndex, chunksTotal)
	}

	fmt.Println()

	// Send file end with checksum
	checksum := hasher.Sum(nil)
	endMsg := make([]byte, 1+len(checksum))
	endMsg[0] = msgFileEnd
	copy(endMsg[1:], checksum)

	if err := node.Send(peerID, streamFile, endMsg); err != nil {
		return fmt.Errorf("failed to send file end: %w", err)
	}

	fmt.Printf("File transfer complete. Checksum: %x\n", checksum[:8])
	return nil
}

// handleFileTransferMessage handles incoming file transfer messages
func handleFileTransferMessage(node *glueberry.Node, msg streams.IncomingMessage, outputDir string) {
	switch msg.Data[0] {
	case msgFileStart:
		// Parse: [type][filename_len][filename][file_size]
		if len(msg.Data) < 3 {
			return
		}
		filenameLen := int(msg.Data[1])
		if len(msg.Data) < 2+filenameLen+8 {
			return
		}
		filename := string(msg.Data[2 : 2+filenameLen])
		fileSize := int64(binary.BigEndian.Uint64(msg.Data[2+filenameLen:]))
		chunksTotal := int((fileSize + chunkSize - 1) / chunkSize)

		fmt.Printf("\nReceiving file: %s (%d bytes, %d chunks)\n", filename, fileSize, chunksTotal)

		transfersMu.Lock()
		transfers[msg.PeerID] = &FileTransfer{
			Filename:    filename,
			TotalSize:   fileSize,
			ChunksTotal: chunksTotal,
			Data:        bytes.NewBuffer(make([]byte, 0, fileSize)),
		}
		transfersMu.Unlock()

	case msgFileChunk:
		// Parse: [type][chunk_index][chunk_data]
		if len(msg.Data) < 5 {
			return
		}
		chunkIndex := binary.BigEndian.Uint32(msg.Data[1:5])
		chunkData := msg.Data[5:]

		transfersMu.Lock()
		transfer := transfers[msg.PeerID]
		if transfer == nil {
			transfersMu.Unlock()
			return
		}
		transfer.Data.Write(chunkData)
		transfer.ChunksRecv++
		progress := float64(transfer.ChunksRecv) / float64(transfer.ChunksTotal) * 100
		transfersMu.Unlock()

		fmt.Printf("\rReceiving: %.1f%% (chunk %d)", progress, chunkIndex)

		// Send acknowledgment
		ackMsg := make([]byte, 5)
		ackMsg[0] = msgFileAck
		binary.BigEndian.PutUint32(ackMsg[1:], chunkIndex)
		_ = node.Send(msg.PeerID, streamFile, ackMsg)

	case msgFileEnd:
		// Parse: [type][checksum]
		if len(msg.Data) < 33 {
			return
		}
		receivedChecksum := msg.Data[1:33]

		transfersMu.Lock()
		transfer := transfers[msg.PeerID]
		if transfer == nil {
			transfersMu.Unlock()
			return
		}
		delete(transfers, msg.PeerID)
		transfersMu.Unlock()

		fmt.Println()

		// Verify checksum
		hasher := sha256.New()
		hasher.Write(transfer.Data.Bytes())
		calculatedChecksum := hasher.Sum(nil)

		if !bytes.Equal(receivedChecksum, calculatedChecksum) {
			fmt.Printf("Checksum mismatch! Expected %x, got %x\n",
				receivedChecksum[:8], calculatedChecksum[:8])
			return
		}

		// Save file
		outputPath := filepath.Join(outputDir, transfer.Filename)
		if err := os.WriteFile(outputPath, transfer.Data.Bytes(), 0644); err != nil {
			fmt.Printf("Failed to save file: %v\n", err)
			return
		}

		fmt.Printf("File saved: %s (checksum verified: %x)\n", outputPath, calculatedChecksum[:8])

	case msgFileAck:
		// Acknowledgment received - could be used for flow control
		// In this simple example we just fire-and-forget
	}
}
