package fuzz

import (
	"bytes"
	"testing"

	"github.com/blockberries/cramberry/pkg/cramberry"
)

// HandshakeMessage represents a typical handshake message structure.
// Applications define their own handshake protocol; this mirrors common patterns.
type HandshakeMessage struct {
	Version      uint32   `cramberry:"1"`
	PublicKey    []byte   `cramberry:"2"`
	Nonce        []byte   `cramberry:"3"`
	Signature    []byte   `cramberry:"4"`
	ChainID      string   `cramberry:"5"`
	NodeID       string   `cramberry:"6"`
	Capabilities []string `cramberry:"7"`
}

// FuzzHandshakeMessageParsing tests parsing of handshake messages with malformed data.
// This simulates what happens when a malicious peer sends corrupted handshake data.
func FuzzHandshakeMessageParsing(f *testing.F) {
	// Add seed corpus

	// Valid handshake message
	validMsg := &HandshakeMessage{
		Version:      1,
		PublicKey:    make([]byte, 32),
		Nonce:        make([]byte, 32),
		Signature:    make([]byte, 64),
		ChainID:      "testnet",
		NodeID:       "12D3KooWtest",
		Capabilities: []string{"v1", "encryption"},
	}
	validData, _ := cramberry.Marshal(validMsg)
	f.Add(validData)

	// Empty message
	emptyData, _ := cramberry.Marshal(&HandshakeMessage{})
	f.Add(emptyData)

	// Only version
	onlyVersion, _ := cramberry.Marshal(&HandshakeMessage{Version: 1})
	f.Add(onlyVersion)

	// Large public key (oversized)
	largeKey := &HandshakeMessage{
		Version:   1,
		PublicKey: make([]byte, 10000),
	}
	largeData, _ := cramberry.Marshal(largeKey)
	f.Add(largeData)

	// Many capabilities
	manyCapabilities := &HandshakeMessage{
		Version:      1,
		Capabilities: make([]string, 100),
	}
	for i := range manyCapabilities.Capabilities {
		manyCapabilities.Capabilities[i] = "cap"
	}
	manyData, _ := cramberry.Marshal(manyCapabilities)
	f.Add(manyData)

	// Random garbage
	f.Add([]byte{0xDE, 0xAD, 0xBE, 0xEF})
	f.Add([]byte{})
	f.Add([]byte{0xFF, 0xFF, 0xFF})
	f.Add([]byte{0x00})

	// Truncated data
	if len(validData) > 10 {
		f.Add(validData[:10])
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// This should not panic regardless of input
		var msg HandshakeMessage
		_ = cramberry.Unmarshal(data, &msg)

		// If we got a result, try to re-marshal it
		_, _ = cramberry.Marshal(&msg)
	})
}

// FuzzHandshakeDelimited tests delimited handshake message reading.
// This simulates reading handshake messages from a stream.
func FuzzHandshakeDelimited(f *testing.F) {
	// Helper to create delimited message
	createDelimited := func(msg *HandshakeMessage) []byte {
		data, err := cramberry.Marshal(msg)
		if err != nil {
			return nil
		}
		var buf bytes.Buffer
		writer := cramberry.NewStreamWriter(&buf)
		writer.WriteMessage(data)
		_ = writer.Flush()
		return buf.Bytes()
	}

	// Add seed corpus
	f.Add(createDelimited(&HandshakeMessage{Version: 1}))
	f.Add(createDelimited(&HandshakeMessage{
		Version:   1,
		PublicKey: make([]byte, 32),
		Nonce:     make([]byte, 32),
	}))

	// Malformed data
	f.Add([]byte{})
	f.Add([]byte{0x05})                   // length but no data
	f.Add([]byte{0x80})                   // truncated varint
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // huge length

	f.Fuzz(func(t *testing.T, data []byte) {
		reader := cramberry.NewStreamReader(bytes.NewReader(data))

		// Read the delimited message
		msgData := reader.ReadMessage()
		if reader.Err() != nil {
			return // Expected for malformed input
		}

		// Try to parse as handshake message
		var msg HandshakeMessage
		_ = cramberry.Unmarshal(msgData, &msg)
	})
}

// FuzzHandshakeMessageRoundTrip tests round-trip encoding of handshake messages.
func FuzzHandshakeMessageRoundTrip(f *testing.F) {
	f.Add(uint32(1), "chain1", "node1")
	f.Add(uint32(0), "", "")
	f.Add(uint32(4294967295), "long-chain-id-with-special-chars-!@#$%", "12D3KooWtest")

	f.Fuzz(func(t *testing.T, version uint32, chainID string, nodeID string) {
		original := HandshakeMessage{
			Version:   version,
			ChainID:   chainID,
			NodeID:    nodeID,
			PublicKey: []byte{1, 2, 3, 4},
			Nonce:     []byte{5, 6, 7, 8},
		}

		// Marshal
		data, err := cramberry.Marshal(&original)
		if err != nil {
			return // Skip invalid inputs
		}

		// Unmarshal
		var decoded HandshakeMessage
		err = cramberry.Unmarshal(data, &decoded)
		if err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		// Verify
		if original.Version != decoded.Version {
			t.Errorf("version mismatch: got %d, want %d", decoded.Version, original.Version)
		}
		if original.ChainID != decoded.ChainID {
			t.Errorf("chainID mismatch: got %q, want %q", decoded.ChainID, original.ChainID)
		}
		if original.NodeID != decoded.NodeID {
			t.Errorf("nodeID mismatch: got %q, want %q", decoded.NodeID, original.NodeID)
		}
		if !bytes.Equal(original.PublicKey, decoded.PublicKey) {
			t.Errorf("publicKey mismatch")
		}
		if !bytes.Equal(original.Nonce, decoded.Nonce) {
			t.Errorf("nonce mismatch")
		}
	})
}

// FuzzCryptoMaterial tests parsing of cryptographic material in handshakes.
// This focuses on the key and signature fields that are security-critical.
func FuzzCryptoMaterial(f *testing.F) {
	// Add seed corpus with various key sizes
	f.Add(make([]byte, 0), make([]byte, 0), make([]byte, 0))     // empty
	f.Add(make([]byte, 32), make([]byte, 32), make([]byte, 64))  // typical Ed25519
	f.Add(make([]byte, 33), make([]byte, 32), make([]byte, 65))  // typical secp256k1
	f.Add(make([]byte, 64), make([]byte, 64), make([]byte, 128)) // oversized

	f.Fuzz(func(t *testing.T, pubKey, nonce, signature []byte) {
		msg := &HandshakeMessage{
			Version:   1,
			PublicKey: pubKey,
			Nonce:     nonce,
			Signature: signature,
		}

		// Marshal
		data, err := cramberry.Marshal(msg)
		if err != nil {
			return // Skip if encoding fails (e.g., size limits)
		}

		// Unmarshal
		var decoded HandshakeMessage
		err = cramberry.Unmarshal(data, &decoded)
		if err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		// Verify crypto material preserved
		if !bytes.Equal(pubKey, decoded.PublicKey) {
			t.Errorf("publicKey mismatch: lengths %d vs %d", len(pubKey), len(decoded.PublicKey))
		}
		if !bytes.Equal(nonce, decoded.Nonce) {
			t.Errorf("nonce mismatch: lengths %d vs %d", len(nonce), len(decoded.Nonce))
		}
		if !bytes.Equal(signature, decoded.Signature) {
			t.Errorf("signature mismatch: lengths %d vs %d", len(signature), len(decoded.Signature))
		}
	})
}

// FuzzMultipleHandshakeMessages tests parsing multiple handshake messages in sequence.
// This simulates a conversation during handshake.
func FuzzMultipleHandshakeMessages(f *testing.F) {
	// Create multiple messages
	createMultiple := func(msgs ...*HandshakeMessage) []byte {
		var buf bytes.Buffer
		writer := cramberry.NewStreamWriter(&buf)
		for _, msg := range msgs {
			data, err := cramberry.Marshal(msg)
			if err != nil {
				continue
			}
			writer.WriteMessage(data)
		}
		_ = writer.Flush()
		return buf.Bytes()
	}

	f.Add(createMultiple(
		&HandshakeMessage{Version: 1},
		&HandshakeMessage{Version: 1, ChainID: "test"},
	))

	// Random data
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x01, 0x02})

	f.Fuzz(func(t *testing.T, data []byte) {
		reader := cramberry.NewStreamReader(bytes.NewReader(data))
		count := 0
		maxMessages := 100 // Prevent infinite loop

		for count < maxMessages {
			msgData := reader.ReadMessage()
			if reader.Err() != nil {
				break
			}

			var msg HandshakeMessage
			_ = cramberry.Unmarshal(msgData, &msg)
			count++
		}
	})
}
