package protocol

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
)

// mockConnForHandlers implements network.Conn for handlers testing
type mockConnForHandlers struct {
	remotePeer peer.ID
}

func (m *mockConnForHandlers) Close() error                                          { return nil }
func (m *mockConnForHandlers) IsClosed() bool                                        { return false }
func (m *mockConnForHandlers) LocalPeer() peer.ID                                    { return "" }
func (m *mockConnForHandlers) RemotePeer() peer.ID                                   { return m.remotePeer }
func (m *mockConnForHandlers) LocalMultiaddr() multiaddr.Multiaddr                   { return nil }
func (m *mockConnForHandlers) RemoteMultiaddr() multiaddr.Multiaddr                  { return nil }
func (m *mockConnForHandlers) LocalPrivateKey() libp2pcrypto.PrivKey                 { return nil }
func (m *mockConnForHandlers) RemotePublicKey() libp2pcrypto.PubKey                  { return nil }
func (m *mockConnForHandlers) ConnState() network.ConnectionState                    { return network.ConnectionState{} }
func (m *mockConnForHandlers) Scope() network.ConnScope                              { return nil }
func (m *mockConnForHandlers) Stat() network.ConnStats                               { return network.ConnStats{} }
func (m *mockConnForHandlers) ID() string                                            { return "test-conn" }
func (m *mockConnForHandlers) NewStream(ctx context.Context) (network.Stream, error) { return nil, nil }
func (m *mockConnForHandlers) GetStreams() []network.Stream                          { return nil }
func (m *mockConnForHandlers) CloseWithError(code network.ConnErrorCode) error       { return nil }
func (m *mockConnForHandlers) As(interface{}) bool                                   { return false }

// mockStreamWithConn extends mockStream with Conn() method
type mockStreamWithConn struct {
	reader     io.Reader
	writer     io.Writer
	deadline   time.Time
	closed     bool
	mu         sync.Mutex
	remotePeer peer.ID
}

func newMockStreamWithConn(r io.Reader, w io.Writer, remotePeer peer.ID) *mockStreamWithConn {
	return &mockStreamWithConn{
		reader:     r,
		writer:     w,
		remotePeer: remotePeer,
	}
}

func (m *mockStreamWithConn) Read(p []byte) (n int, err error) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()

	if closed {
		return 0, io.EOF
	}
	return m.reader.Read(p)
}

func (m *mockStreamWithConn) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()

	if closed {
		return 0, io.ErrClosedPipe
	}
	return m.writer.Write(p)
}

func (m *mockStreamWithConn) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockStreamWithConn) Reset() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockStreamWithConn) ResetWithError(code network.StreamErrorCode) error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockStreamWithConn) SetDeadline(t time.Time) error {
	m.deadline = t
	return nil
}

func (m *mockStreamWithConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockStreamWithConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *mockStreamWithConn) CloseWrite() error {
	return nil
}

func (m *mockStreamWithConn) CloseRead() error {
	return nil
}

func (m *mockStreamWithConn) ID() string                       { return "test-stream" }
func (m *mockStreamWithConn) Protocol() protocol.ID            { return "/test/1.0.0" }
func (m *mockStreamWithConn) SetProtocol(id protocol.ID) error { return nil }
func (m *mockStreamWithConn) Stat() network.Stats              { return network.Stats{} }
func (m *mockStreamWithConn) Scope() network.StreamScope       { return nil }

func (m *mockStreamWithConn) Conn() network.Conn {
	return &mockConnForHandlers{remotePeer: m.remotePeer}
}

func TestStreamHandler_HandleStream(t *testing.T) {
	// StreamHandler is deprecated but we test it doesn't panic
	handler := NewStreamHandler(nil)

	peerID := mustParsePeerID(t, "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")

	var buf bytes.Buffer
	stream := newMockStreamWithConn(&buf, &buf, peerID)

	// Get the handler function
	handlerFn := handler.HandleStream("test")

	// Should not panic
	handlerFn(stream)

	// Stream should be reset (handler is deprecated)
	if !stream.closed {
		t.Error("stream should be closed/reset by deprecated handler")
	}
}

func TestNewStreamHandler(t *testing.T) {
	handler := NewStreamHandler(nil)

	if handler == nil {
		t.Fatal("NewStreamHandler returned nil")
	}
}
