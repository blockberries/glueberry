package streams

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// mockStream implements network.Stream for testing
type mockStream struct {
	reader   io.Reader
	writer   io.Writer
	deadline time.Time
	closed   bool
	mu       sync.Mutex
}

func newMockStream(r io.Reader, w io.Writer) *mockStream {
	return &mockStream{
		reader: r,
		writer: w,
	}
}

func (m *mockStream) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()

	if closed {
		return 0, io.EOF
	}
	return m.reader.Read(p)
}

func (m *mockStream) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()

	if closed {
		return 0, io.ErrClosedPipe
	}
	return m.writer.Write(p)
}

func (m *mockStream) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockStream) Reset() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockStream) ResetWithError(code network.StreamErrorCode) error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockStream) SetDeadline(t time.Time) error {
	m.deadline = t
	return nil
}

func (m *mockStream) SetReadDeadline(t time.Time) error {
	m.deadline = t
	return nil
}

func (m *mockStream) SetWriteDeadline(t time.Time) error {
	m.deadline = t
	return nil
}

func (m *mockStream) CloseWrite() error {
	return nil
}

func (m *mockStream) CloseRead() error {
	return nil
}

// The following methods satisfy the network.Stream interface but aren't used in tests
func (m *mockStream) ID() string                     { return "test-stream" }
func (m *mockStream) Protocol() protocol.ID          { return "/test/1.0.0" }
func (m *mockStream) SetProtocol(id protocol.ID) error { return nil }
func (m *mockStream) Stat() network.Stats            { return network.Stats{} }
func (m *mockStream) Conn() network.Conn             { return nil }
func (m *mockStream) Scope() network.StreamScope     { return nil }

// Test message types
type TestMessage struct {
	ID      int64  `cramberry:"1,required"`
	Content string `cramberry:"2"`
}

type AnotherMessage struct {
	Value int32 `cramberry:"1"`
}

func TestNewHandshakeStream(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	hs := NewHandshakeStream(stream, timeout)

	if hs == nil {
		t.Fatal("NewHandshakeStream returned nil")
	}
	if hs.closed {
		t.Error("stream should not be closed initially")
	}
	if hs.TimeRemaining() <= 0 {
		t.Error("TimeRemaining should be positive")
	}
	if hs.TimeRemaining() > timeout {
		t.Error("TimeRemaining should not exceed timeout")
	}
}

func TestHandshakeStream_SendReceive(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	hs := NewHandshakeStream(stream, timeout)

	// Send a message
	msg := TestMessage{
		ID:      123,
		Content: "hello handshake",
	}

	err := hs.Send(&msg)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Create a new handshake stream to read (simulating the other side)
	stream2 := newMockStream(&buf, &buf)
	hs2 := NewHandshakeStream(stream2, timeout)

	// Receive the message
	var received TestMessage
	err = hs2.Receive(&received)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if received.ID != msg.ID {
		t.Errorf("ID = %d, want %d", received.ID, msg.ID)
	}
	if received.Content != msg.Content {
		t.Errorf("Content = %q, want %q", received.Content, msg.Content)
	}
}

func TestHandshakeStream_MultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	sender := NewHandshakeStream(stream, timeout)

	// Send multiple messages
	msg1 := TestMessage{ID: 1, Content: "first"}
	msg2 := TestMessage{ID: 2, Content: "second"}
	msg3 := TestMessage{ID: 3, Content: "third"}

	if err := sender.Send(&msg1); err != nil {
		t.Fatalf("Send msg1 failed: %v", err)
	}
	if err := sender.Send(&msg2); err != nil {
		t.Fatalf("Send msg2 failed: %v", err)
	}
	if err := sender.Send(&msg3); err != nil {
		t.Fatalf("Send msg3 failed: %v", err)
	}

	// Receive from other side
	stream2 := newMockStream(&buf, &buf)
	receiver := NewHandshakeStream(stream2, timeout)

	var recv1, recv2, recv3 TestMessage
	if err := receiver.Receive(&recv1); err != nil {
		t.Fatalf("Receive msg1 failed: %v", err)
	}
	if err := receiver.Receive(&recv2); err != nil {
		t.Fatalf("Receive msg2 failed: %v", err)
	}
	if err := receiver.Receive(&recv3); err != nil {
		t.Fatalf("Receive msg3 failed: %v", err)
	}

	if recv1.ID != 1 || recv1.Content != "first" {
		t.Error("msg1 mismatch")
	}
	if recv2.ID != 2 || recv2.Content != "second" {
		t.Error("msg2 mismatch")
	}
	if recv3.ID != 3 || recv3.Content != "third" {
		t.Error("msg3 mismatch")
	}
}

func TestHandshakeStream_DifferentMessageTypes(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	sender := NewHandshakeStream(stream, timeout)

	// Send different message types
	msg1 := TestMessage{ID: 100, Content: "test"}
	msg2 := AnotherMessage{Value: 42}

	if err := sender.Send(&msg1); err != nil {
		t.Fatalf("Send TestMessage failed: %v", err)
	}
	if err := sender.Send(&msg2); err != nil {
		t.Fatalf("Send AnotherMessage failed: %v", err)
	}

	// Receive
	stream2 := newMockStream(&buf, &buf)
	receiver := NewHandshakeStream(stream2, timeout)

	var recv1 TestMessage
	var recv2 AnotherMessage

	if err := receiver.Receive(&recv1); err != nil {
		t.Fatalf("Receive TestMessage failed: %v", err)
	}
	if err := receiver.Receive(&recv2); err != nil {
		t.Fatalf("Receive AnotherMessage failed: %v", err)
	}

	if recv1.ID != 100 || recv1.Content != "test" {
		t.Error("TestMessage mismatch")
	}
	if recv2.Value != 42 {
		t.Error("AnotherMessage mismatch")
	}
}

func TestHandshakeStream_Close(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	hs := NewHandshakeStream(stream, timeout)

	// Close should succeed
	err := hs.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Stream should be marked as closed
	if !hs.IsClosed() {
		t.Error("stream should be closed after Close()")
	}

	// Subsequent operations should fail
	msg := TestMessage{ID: 1, Content: "test"}
	err = hs.Send(&msg)
	if err == nil {
		t.Error("Send should fail on closed stream")
	}

	err = hs.Receive(&msg)
	if err == nil {
		t.Error("Receive should fail on closed stream")
	}
}

func TestHandshakeStream_CloseMultiple(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	hs := NewHandshakeStream(stream, timeout)

	// Multiple Close calls should be safe
	err := hs.Close()
	if err != nil {
		t.Errorf("first Close failed: %v", err)
	}

	err = hs.Close()
	if err != nil {
		t.Errorf("second Close should be safe: %v", err)
	}

	err = hs.Close()
	if err != nil {
		t.Errorf("third Close should be safe: %v", err)
	}
}

func TestHandshakeStream_CloseWrite(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	hs := NewHandshakeStream(stream, timeout)

	err := hs.CloseWrite()
	if err != nil {
		t.Errorf("CloseWrite failed: %v", err)
	}

	// Stream should not be marked as fully closed
	if hs.IsClosed() {
		t.Error("stream should not be fully closed after CloseWrite()")
	}
}

func TestHandshakeStream_TimeRemaining(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 100 * time.Millisecond

	hs := NewHandshakeStream(stream, timeout)

	// Initially should have nearly full timeout
	remaining := hs.TimeRemaining()
	if remaining <= 0 || remaining > timeout {
		t.Errorf("TimeRemaining = %v, expected between 0 and %v", remaining, timeout)
	}

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Should have less time remaining
	remaining2 := hs.TimeRemaining()
	if remaining2 >= remaining {
		t.Error("TimeRemaining should decrease over time")
	}

	// Wait for deadline to pass
	time.Sleep(100 * time.Millisecond)

	// Should return 0 after deadline
	remaining3 := hs.TimeRemaining()
	if remaining3 != 0 {
		t.Errorf("TimeRemaining after deadline = %v, want 0", remaining3)
	}
}

func TestHandshakeStream_DeadlineExpired_Send(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 10 * time.Millisecond

	hs := NewHandshakeStream(stream, timeout)

	// Wait for deadline to pass
	time.Sleep(20 * time.Millisecond)

	// Send should fail
	msg := TestMessage{ID: 1, Content: "too late"}
	err := hs.Send(&msg)
	if err == nil {
		t.Error("Send should fail after deadline")
	}
}

func TestHandshakeStream_DeadlineExpired_Receive(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 10 * time.Millisecond

	hs := NewHandshakeStream(stream, timeout)

	// Wait for deadline to pass
	time.Sleep(20 * time.Millisecond)

	// Receive should fail
	var msg TestMessage
	err := hs.Receive(&msg)
	if err == nil {
		t.Error("Receive should fail after deadline")
	}
}

func TestHandshakeStream_Deadline(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 1 * time.Hour

	before := time.Now()
	hs := NewHandshakeStream(stream, timeout)
	after := time.Now()

	deadline := hs.Deadline()

	// Deadline should be approximately timeout from now
	expectedMin := before.Add(timeout)
	expectedMax := after.Add(timeout)

	if deadline.Before(expectedMin) || deadline.After(expectedMax) {
		t.Errorf("Deadline = %v, expected between %v and %v", deadline, expectedMin, expectedMax)
	}
}

func TestHandshakeStream_Stream(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	hs := NewHandshakeStream(stream, timeout)

	// Should return the underlying stream
	if hs.Stream() != stream {
		t.Error("Stream() should return underlying stream")
	}
}

func TestHandshakeStream_ReceiveEOF(t *testing.T) {
	// Empty buffer simulates closed connection
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	hs := NewHandshakeStream(stream, timeout)

	var msg TestMessage
	err := hs.Receive(&msg)
	if err == nil {
		t.Error("Receive should fail on EOF")
	}
}

func TestHandshakeStream_SendAfterClose(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	hs := NewHandshakeStream(stream, timeout)
	hs.Close()

	msg := TestMessage{ID: 1, Content: "test"}
	err := hs.Send(&msg)
	if err == nil {
		t.Error("Send should fail on closed stream")
	}
}

func TestHandshakeStream_ReceiveAfterClose(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	hs := NewHandshakeStream(stream, timeout)
	hs.Close()

	var msg TestMessage
	err := hs.Receive(&msg)
	if err == nil {
		t.Error("Receive should fail on closed stream")
	}
}

func TestHandshakeStream_LargeMessage(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	sender := NewHandshakeStream(stream, timeout)

	// Create a large message
	largeContent := string(make([]byte, 1024*100)) // 100KB
	msg := TestMessage{
		ID:      999,
		Content: largeContent,
	}

	err := sender.Send(&msg)
	if err != nil {
		t.Fatalf("Send large message failed: %v", err)
	}

	// Receive
	stream2 := newMockStream(&buf, &buf)
	receiver := NewHandshakeStream(stream2, timeout)

	var received TestMessage
	err = receiver.Receive(&received)
	if err != nil {
		t.Fatalf("Receive large message failed: %v", err)
	}

	if received.ID != msg.ID {
		t.Error("large message ID mismatch")
	}
	if len(received.Content) != len(msg.Content) {
		t.Errorf("large message content length = %d, want %d", len(received.Content), len(msg.Content))
	}
}

func TestHandshakeStream_EmptyContent(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	sender := NewHandshakeStream(stream, timeout)

	// Send message with required ID but empty content
	msg := TestMessage{ID: 1} // ID is required, content is optional

	err := sender.Send(&msg)
	if err != nil {
		t.Fatalf("Send message with empty content failed: %v", err)
	}

	// Receive
	stream2 := newMockStream(&buf, &buf)
	receiver := NewHandshakeStream(stream2, timeout)

	var received TestMessage
	err = receiver.Receive(&received)
	if err != nil {
		t.Fatalf("Receive message with empty content failed: %v", err)
	}

	if received.ID != 1 {
		t.Errorf("message ID = %d, want 1", received.ID)
	}
	if received.Content != "" {
		t.Errorf("message Content = %q, want empty", received.Content)
	}
}

func TestHandshakeStream_BidirectionalCommunication(t *testing.T) {
	// Use two buffers for bidirectional communication
	var buf1to2 bytes.Buffer
	var buf2to1 bytes.Buffer

	stream1 := newMockStream(&buf2to1, &buf1to2)
	stream2 := newMockStream(&buf1to2, &buf2to1)

	timeout := 30 * time.Second
	hs1 := NewHandshakeStream(stream1, timeout)
	hs2 := NewHandshakeStream(stream2, timeout)

	// hs1 sends to hs2
	msg1 := TestMessage{ID: 100, Content: "from hs1"}
	if err := hs1.Send(&msg1); err != nil {
		t.Fatalf("hs1 Send failed: %v", err)
	}

	var recv1 TestMessage
	if err := hs2.Receive(&recv1); err != nil {
		t.Fatalf("hs2 Receive failed: %v", err)
	}
	if recv1.ID != 100 {
		t.Error("hs2 received wrong message from hs1")
	}

	// hs2 sends to hs1
	msg2 := TestMessage{ID: 200, Content: "from hs2"}
	if err := hs2.Send(&msg2); err != nil {
		t.Fatalf("hs2 Send failed: %v", err)
	}

	var recv2 TestMessage
	if err := hs1.Receive(&recv2); err != nil {
		t.Fatalf("hs1 Receive failed: %v", err)
	}
	if recv2.ID != 200 {
		t.Error("hs1 received wrong message from hs2")
	}
}

func TestHandshakeStream_TimeRemaining_ZeroAfterExpiry(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 1 * time.Millisecond

	hs := NewHandshakeStream(stream, timeout)

	// Wait for timeout
	time.Sleep(5 * time.Millisecond)

	if hs.TimeRemaining() != 0 {
		t.Errorf("TimeRemaining after expiry = %v, want 0", hs.TimeRemaining())
	}
}

func TestHandshakeStream_CloseWrite_ThenClose(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	hs := NewHandshakeStream(stream, timeout)

	// CloseWrite first
	err := hs.CloseWrite()
	if err != nil {
		t.Errorf("CloseWrite failed: %v", err)
	}

	// Should still be able to fully Close
	err = hs.Close()
	if err != nil {
		t.Errorf("Close after CloseWrite failed: %v", err)
	}

	if !hs.IsClosed() {
		t.Error("stream should be closed")
	}
}

func TestHandshakeStream_CloseWrite_AfterFullClose(t *testing.T) {
	var buf bytes.Buffer
	stream := newMockStream(&buf, &buf)
	timeout := 30 * time.Second

	hs := NewHandshakeStream(stream, timeout)

	// Full close first
	hs.Close()

	// CloseWrite should fail
	err := hs.CloseWrite()
	if err == nil {
		t.Error("CloseWrite should fail after full Close()")
	}
}
