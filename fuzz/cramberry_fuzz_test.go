package fuzz

import (
	"bytes"
	"testing"

	"github.com/blockberries/cramberry/pkg/cramberry"
)

// FuzzMessageIterator tests the Cramberry MessageIterator with malformed data.
// This helps find panics or issues when parsing corrupted messages from peers.
func FuzzMessageIterator(f *testing.F) {
	// Add seed corpus

	// Valid empty message (length prefix 0)
	f.Add([]byte{0x00})

	// Valid single-byte message
	f.Add([]byte{0x01, 0x42})

	// Valid multi-byte message
	f.Add([]byte{0x05, 'h', 'e', 'l', 'l', 'o'})

	// Multiple valid messages
	f.Add([]byte{0x02, 'h', 'i', 0x03, 'b', 'y', 'e'})

	// Truncated length prefix (varint continuation without data)
	f.Add([]byte{0x80})
	f.Add([]byte{0x80, 0x80})
	f.Add([]byte{0x80, 0x80, 0x80})

	// Length prefix but no data
	f.Add([]byte{0x05})
	f.Add([]byte{0x10})

	// Partial data (claims more than available)
	f.Add([]byte{0x05, 'h', 'e', 'l'})

	// Very large length prefix (overflow attempt)
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01})

	// Zero-length varint followed by more data
	f.Add([]byte{0x00, 0x00, 0x00})

	// Empty input
	f.Add([]byte{})

	// Random garbage
	f.Add([]byte{0xDE, 0xAD, 0xBE, 0xEF})

	// Varint overflow (10 bytes with value > max uint64)
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F})

	f.Fuzz(func(t *testing.T, data []byte) {
		reader := bytes.NewReader(data)
		iter := cramberry.NewMessageIterator(reader)

		// This should not panic regardless of input
		var msg []byte
		for iter.Next(&msg) {
			// Successfully read a message, try to process it
			if len(msg) > 0 {
				_ = msg[0]
			}
		}

		// Error is expected for malformed input
		_ = iter.Err()
	})
}

// FuzzStreamReaderVarint tests the Cramberry StreamReader varint parsing.
func FuzzStreamReaderVarint(f *testing.F) {
	// Add seed corpus

	// Valid varints
	f.Add([]byte{0x00})                                                       // 0
	f.Add([]byte{0x01})                                                       // 1
	f.Add([]byte{0x7F})                                                       // 127
	f.Add([]byte{0x80, 0x01})                                                 // 128
	f.Add([]byte{0xFF, 0x7F})                                                 // 16383
	f.Add([]byte{0x80, 0x80, 0x01})                                           // 16384
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01}) // max uint64

	// Invalid varints
	f.Add([]byte{0x80})             // truncated
	f.Add([]byte{0x80, 0x80})       // truncated
	f.Add([]byte{0x80, 0x80, 0x80}) // truncated
	f.Add([]byte{})                 // empty

	// Overflow varints (10th byte > 1)
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x02})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F})

	f.Fuzz(func(t *testing.T, data []byte) {
		reader := cramberry.NewStreamReader(bytes.NewReader(data))

		// This should not panic regardless of input
		_ = reader.ReadUvarint()
		_ = reader.Err()
	})
}

// FuzzStreamReaderString tests the Cramberry StreamReader string parsing.
func FuzzStreamReaderString(f *testing.F) {
	// Add seed corpus

	// Valid strings
	f.Add([]byte{0x00})                          // empty string
	f.Add([]byte{0x05, 'h', 'e', 'l', 'l', 'o'}) // "hello"
	f.Add([]byte{0x01, 'a'})                     // "a"
	f.Add([]byte{0x03, 0xE2, 0x9C, 0x93})        // checkmark UTF-8
	f.Add([]byte{0x04, 0xF0, 0x9F, 0x98, 0x80})  // emoji UTF-8

	// Invalid strings
	f.Add([]byte{0x05})                   // length but no data
	f.Add([]byte{0x05, 'h', 'e', 'l'})    // truncated
	f.Add([]byte{0x80})                   // truncated varint length
	f.Add([]byte{})                       // empty
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // huge length

	// Invalid UTF-8 (if validation is enabled)
	f.Add([]byte{0x01, 0x80})             // invalid UTF-8 sequence
	f.Add([]byte{0x02, 0xC0, 0x80})       // overlong encoding
	f.Add([]byte{0x03, 0xED, 0xA0, 0x80}) // surrogate half

	f.Fuzz(func(t *testing.T, data []byte) {
		reader := cramberry.NewStreamReader(bytes.NewReader(data))

		// This should not panic regardless of input
		_ = reader.ReadString()
		_ = reader.Err()
	})
}

// FuzzStreamReaderBytes tests the Cramberry StreamReader bytes parsing.
func FuzzStreamReaderBytes(f *testing.F) {
	// Add seed corpus

	// Valid byte slices
	f.Add([]byte{0x00})       // empty
	f.Add([]byte{0x01, 0xFF}) // single byte
	f.Add([]byte{0x05, 0x01, 0x02, 0x03, 0x04, 0x05})
	f.Add([]byte{0x10, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15})

	// Invalid byte slices
	f.Add([]byte{0x05})                                                       // length but no data
	f.Add([]byte{0x05, 0x01, 0x02})                                           // truncated
	f.Add([]byte{0x80})                                                       // truncated varint length
	f.Add([]byte{})                                                           // empty
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x01}) // huge length

	f.Fuzz(func(t *testing.T, data []byte) {
		reader := cramberry.NewStreamReader(bytes.NewReader(data))

		// This should not panic regardless of input
		_ = reader.ReadBytes()
		_ = reader.Err()
	})
}

// FuzzStreamWriterReader tests round-trip encoding/decoding.
func FuzzStreamWriterReader(f *testing.F) {
	// Add seed corpus
	f.Add([]byte("hello"), int64(42), uint64(100))
	f.Add([]byte{}, int64(0), uint64(0))
	f.Add([]byte{0xFF}, int64(-1), uint64(1))
	f.Add([]byte{0x00, 0x01, 0x02}, int64(-9223372036854775808), uint64(18446744073709551615))

	f.Fuzz(func(t *testing.T, data []byte, signed int64, unsigned uint64) {
		// Write
		var buf bytes.Buffer
		writer := cramberry.NewStreamWriter(&buf)
		writer.WriteBytes(data)
		writer.WriteSvarint(signed)
		writer.WriteUvarint(unsigned)
		if writer.Err() != nil {
			return // Skip if encoding fails (e.g., limits)
		}
		if err := writer.Flush(); err != nil {
			return
		}

		// Read
		reader := cramberry.NewStreamReader(bytes.NewReader(buf.Bytes()))
		gotData := reader.ReadBytes()
		gotSigned := reader.ReadSvarint()
		gotUnsigned := reader.ReadUvarint()

		if reader.Err() != nil {
			t.Fatalf("read failed: %v", reader.Err())
		}

		// Verify round-trip
		if !bytes.Equal(data, gotData) {
			t.Errorf("bytes mismatch: got %v, want %v", gotData, data)
		}
		if signed != gotSigned {
			t.Errorf("signed mismatch: got %d, want %d", gotSigned, signed)
		}
		if unsigned != gotUnsigned {
			t.Errorf("unsigned mismatch: got %d, want %d", gotUnsigned, unsigned)
		}
	})
}

// FuzzDelimitedMessages tests delimited message reading with malformed data.
func FuzzDelimitedMessages(f *testing.F) {
	// Add seed corpus

	// Valid delimited messages (Cramberry-encoded byte slices)
	validMsg := func(data []byte) []byte {
		var buf bytes.Buffer
		writer := cramberry.NewStreamWriter(&buf)
		writer.WriteMessage(data)
		_ = writer.Flush()
		return buf.Bytes()
	}

	f.Add(validMsg([]byte{}))
	f.Add(validMsg([]byte("hello")))
	f.Add(validMsg([]byte{0, 1, 2, 3, 4}))

	// Invalid data
	f.Add([]byte{})
	f.Add([]byte{0xFF})
	f.Add([]byte{0x05, 0x01}) // truncated

	f.Fuzz(func(t *testing.T, data []byte) {
		reader := cramberry.NewStreamReader(bytes.NewReader(data))

		// This should not panic regardless of input
		_ = reader.ReadMessage()
		_ = reader.Err()
	})
}

// FuzzMarshalUnmarshal tests Cramberry marshal/unmarshal with various types.
func FuzzMarshalUnmarshal(f *testing.F) {
	// Add seed corpus
	f.Add("test", int64(42), true)
	f.Add("", int64(0), false)
	f.Add("hello world", int64(-1), true)
	f.Add("unicode: \u0000\u0001\u0002", int64(9223372036854775807), false)

	f.Fuzz(func(t *testing.T, name string, value int64, enabled bool) {
		type TestMessage struct {
			Name    string `cramberry:"1"`
			Value   int64  `cramberry:"2"`
			Enabled bool   `cramberry:"3"`
		}

		original := TestMessage{
			Name:    name,
			Value:   value,
			Enabled: enabled,
		}

		// Marshal
		data, err := cramberry.Marshal(&original)
		if err != nil {
			return // Skip invalid inputs
		}

		// Unmarshal
		var decoded TestMessage
		err = cramberry.Unmarshal(data, &decoded)
		if err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		// Verify
		if original != decoded {
			t.Errorf("mismatch: got %+v, want %+v", decoded, original)
		}
	})
}
