package glueberry

import (
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// NonceExhaustionWarningThreshold is the number of messages sent to a peer
	// after which a warning is emitted about potential nonce exhaustion.
	//
	// ChaCha20-Poly1305 uses 96-bit random nonces. Due to the birthday paradox,
	// after ~2^48 messages there's a 50% chance of nonce collision. At 2^40
	// messages, the collision probability is approximately 2^-16 (1 in 65536).
	//
	// When this threshold is crossed, applications should consider:
	// - Rotating the shared key via a new handshake
	// - Monitoring for unusual traffic patterns
	//
	// This is a conservative threshold providing a safety margin.
	NonceExhaustionWarningThreshold int64 = 1 << 40 // ~1 trillion messages
)

// StreamStats contains statistics for a single stream.
type StreamStats struct {
	// Name is the stream name.
	Name string

	// MessagesSent is the number of messages sent on this stream.
	MessagesSent int64

	// MessagesReceived is the number of messages received on this stream.
	MessagesReceived int64

	// BytesSent is the total bytes sent on this stream.
	BytesSent int64

	// BytesReceived is the total bytes received on this stream.
	BytesReceived int64

	// LastSentAt is when a message was last sent on this stream.
	LastSentAt time.Time

	// LastReceivedAt is when a message was last received on this stream.
	LastReceivedAt time.Time
}

// PeerStats contains statistics for a peer connection.
// All fields are safe to read without synchronization once returned
// from the API, as they are snapshot copies.
type PeerStats struct {
	// PeerID is the peer identifier.
	PeerID peer.ID

	// Connected indicates whether the peer is currently connected.
	Connected bool

	// IsOutbound indicates whether we initiated this connection.
	IsOutbound bool

	// ConnectedAt is when the current connection was established.
	// Zero value if not connected.
	ConnectedAt time.Time

	// TotalConnectTime is the cumulative duration of all connections.
	TotalConnectTime time.Duration

	// MessagesSent is the total number of messages sent to this peer.
	MessagesSent int64

	// MessagesReceived is the total number of messages received from this peer.
	MessagesReceived int64

	// BytesSent is the total bytes sent to this peer.
	BytesSent int64

	// BytesReceived is the total bytes received from this peer.
	BytesReceived int64

	// StreamStats contains per-stream statistics.
	StreamStats map[string]*StreamStats

	// LastMessageAt is when a message was last sent or received.
	LastMessageAt time.Time

	// ConnectionCount is the total number of connections (including reconnects).
	ConnectionCount int

	// FailureCount is the number of failed connection/handshake attempts.
	FailureCount int
}

// NonceExhaustionCallback is called when message count exceeds the warning threshold.
// It receives the peer ID and the current message count.
type NonceExhaustionCallback func(peerID peer.ID, messageCount int64)

// PeerStatsTracker is the internal mutable stats tracker.
// It implements connection.StatsRecorder and is stored per-peer.
type PeerStatsTracker struct {
	mu sync.RWMutex

	peerID peer.ID // stored for callback invocation

	connectedAt      time.Time
	totalConnectTime time.Duration

	messagesSent     int64
	messagesReceived int64
	bytesSent        int64
	bytesReceived    int64

	streamStats map[string]*streamStatsInternal

	lastMessageAt   time.Time
	connectionCount int
	failureCount    int

	// nonceWarningEmitted tracks whether the nonce exhaustion warning was emitted
	nonceWarningEmitted bool

	// onNonceExhaustion is called when the message count exceeds the warning threshold
	onNonceExhaustion NonceExhaustionCallback
}

// streamStatsInternal is the internal mutable stream stats tracker.
type streamStatsInternal struct {
	messagesSent     int64
	messagesReceived int64
	bytesSent        int64
	bytesReceived    int64
	lastSentAt       time.Time
	lastReceivedAt   time.Time
}

// NewPeerStatsTracker creates a new stats tracker for a peer.
func NewPeerStatsTracker(peerID peer.ID, onNonceExhaustion NonceExhaustionCallback) *PeerStatsTracker {
	return &PeerStatsTracker{
		peerID:            peerID,
		streamStats:       make(map[string]*streamStatsInternal),
		onNonceExhaustion: onNonceExhaustion,
	}
}

// RecordConnectionStart records that a connection started.
// Implements connection.StatsRecorder.
func (s *PeerStatsTracker) RecordConnectionStart() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.connectedAt = time.Now()
	s.connectionCount++
}

// RecordConnectionEnd records that a connection ended.
// Implements connection.StatsRecorder.
func (s *PeerStatsTracker) RecordConnectionEnd() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connectedAt.IsZero() {
		s.totalConnectTime += time.Since(s.connectedAt)
		s.connectedAt = time.Time{}
	}
}

// RecordFailure records a connection or handshake failure.
// Implements connection.StatsRecorder.
func (s *PeerStatsTracker) RecordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failureCount++
}

// RecordMessageSent records a message being sent.
// Implements connection.StatsRecorder.
func (s *PeerStatsTracker) RecordMessageSent(streamName string, size int) {
	s.mu.Lock()

	now := time.Now()

	s.messagesSent++
	s.bytesSent += int64(size)
	s.lastMessageAt = now

	ss := s.streamStats[streamName]
	if ss == nil {
		ss = &streamStatsInternal{}
		s.streamStats[streamName] = ss
	}
	ss.messagesSent++
	ss.bytesSent += int64(size)
	ss.lastSentAt = now

	// Check for nonce exhaustion warning
	shouldWarn := !s.nonceWarningEmitted &&
		s.messagesSent >= NonceExhaustionWarningThreshold &&
		s.onNonceExhaustion != nil
	if shouldWarn {
		s.nonceWarningEmitted = true
	}
	peerID := s.peerID
	messageCount := s.messagesSent
	callback := s.onNonceExhaustion

	s.mu.Unlock()

	// Call callback outside the lock to avoid deadlocks
	if shouldWarn && callback != nil {
		callback(peerID, messageCount)
	}
}

// RecordMessageReceived records a message being received.
// Implements connection.StatsRecorder.
func (s *PeerStatsTracker) RecordMessageReceived(streamName string, size int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	s.messagesReceived++
	s.bytesReceived += int64(size)
	s.lastMessageAt = now

	ss := s.streamStats[streamName]
	if ss == nil {
		ss = &streamStatsInternal{}
		s.streamStats[streamName] = ss
	}
	ss.messagesReceived++
	ss.bytesReceived += int64(size)
	ss.lastReceivedAt = now
}

// Snapshot returns a copy of the stats for external consumption.
func (s *PeerStatsTracker) Snapshot(peerID peer.ID, connected bool, isOutbound bool) *PeerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &PeerStats{
		PeerID:           peerID,
		Connected:        connected,
		IsOutbound:       isOutbound,
		ConnectedAt:      s.connectedAt,
		TotalConnectTime: s.totalConnectTime,
		MessagesSent:     s.messagesSent,
		MessagesReceived: s.messagesReceived,
		BytesSent:        s.bytesSent,
		BytesReceived:    s.bytesReceived,
		StreamStats:      make(map[string]*StreamStats, len(s.streamStats)),
		LastMessageAt:    s.lastMessageAt,
		ConnectionCount:  s.connectionCount,
		FailureCount:     s.failureCount,
	}

	// If currently connected, add the current session duration
	if connected && !s.connectedAt.IsZero() {
		stats.TotalConnectTime += time.Since(s.connectedAt)
	}

	// Copy stream stats
	for name, ss := range s.streamStats {
		stats.StreamStats[name] = &StreamStats{
			Name:             name,
			MessagesSent:     ss.messagesSent,
			MessagesReceived: ss.messagesReceived,
			BytesSent:        ss.bytesSent,
			BytesReceived:    ss.bytesReceived,
			LastSentAt:       ss.lastSentAt,
			LastReceivedAt:   ss.lastReceivedAt,
		}
	}

	return stats
}

// LastActivity returns the time of last activity for this peer.
// This is the most recent of connectedAt or lastMessageAt.
func (s *PeerStatsTracker) LastActivity() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// If currently connected, use now as last activity
	if !s.connectedAt.IsZero() {
		return time.Now()
	}

	// Otherwise, use the last message time
	return s.lastMessageAt
}
