package glueberry

import (
	"sync"
	"testing"
)

func TestNopMetrics_Implements_Metrics(t *testing.T) {
	var _ Metrics = NopMetrics{}
}

func TestNopMetrics_Methods_DoNotPanic(t *testing.T) {
	m := NopMetrics{}

	// Should not panic with any arguments
	m.ConnectionOpened("inbound")
	m.ConnectionOpened("outbound")
	m.ConnectionClosed("inbound")
	m.ConnectionClosed("outbound")
	m.ConnectionAttempt("success")
	m.ConnectionAttempt("failure")
	m.HandshakeDuration(1.5)
	m.HandshakeResult("success")
	m.HandshakeResult("failure")
	m.HandshakeResult("timeout")
	m.MessageSent("data", 1024)
	m.MessageReceived("data", 2048)
	m.StreamOpened("data")
	m.StreamClosed("data")
	m.EncryptionError()
	m.DecryptionError()
	m.KeyDerivation(true)
	m.KeyDerivation(false)
	m.EventEmitted("connected")
	m.EventDropped()
	m.MessageDropped()
	m.BackpressureEngaged("data")
	m.BackpressureWait("data", 0.5)
	m.PendingMessages("data", 100)
}

// TestMetrics is a test metrics implementation that records calls.
type TestMetrics struct {
	mu sync.Mutex

	ConnectionsOpened    map[string]int
	ConnectionsClosed    map[string]int
	ConnectionAttempts   map[string]int
	HandshakeDurations   []float64
	HandshakeResults     map[string]int
	MessagesSent         map[string]int
	BytesSent            map[string]int
	MessagesReceived     map[string]int
	BytesReceived        map[string]int
	StreamsOpened        map[string]int
	StreamsClosed        map[string]int
	EncryptionErrors     int
	DecryptionErrors     int
	KeyDerivations       map[bool]int
	EventsEmitted        map[string]int
	EventsDropped        int
	MessagesDropped      int
	BackpressureEvents   map[string]int
	BackpressureWaits    map[string][]float64
	PendingMessagesGauge map[string]int
}

func NewTestMetrics() *TestMetrics {
	return &TestMetrics{
		ConnectionsOpened:    make(map[string]int),
		ConnectionsClosed:    make(map[string]int),
		ConnectionAttempts:   make(map[string]int),
		HandshakeDurations:   make([]float64, 0),
		HandshakeResults:     make(map[string]int),
		MessagesSent:         make(map[string]int),
		BytesSent:            make(map[string]int),
		MessagesReceived:     make(map[string]int),
		BytesReceived:        make(map[string]int),
		StreamsOpened:        make(map[string]int),
		StreamsClosed:        make(map[string]int),
		KeyDerivations:       make(map[bool]int),
		EventsEmitted:        make(map[string]int),
		BackpressureEvents:   make(map[string]int),
		BackpressureWaits:    make(map[string][]float64),
		PendingMessagesGauge: make(map[string]int),
	}
}

func (m *TestMetrics) ConnectionOpened(direction string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConnectionsOpened[direction]++
}

func (m *TestMetrics) ConnectionClosed(direction string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConnectionsClosed[direction]++
}

func (m *TestMetrics) ConnectionAttempt(result string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ConnectionAttempts[result]++
}

func (m *TestMetrics) HandshakeDuration(seconds float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HandshakeDurations = append(m.HandshakeDurations, seconds)
}

func (m *TestMetrics) HandshakeResult(result string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.HandshakeResults[result]++
}

func (m *TestMetrics) MessageSent(stream string, bytes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MessagesSent[stream]++
	m.BytesSent[stream] += bytes
}

func (m *TestMetrics) MessageReceived(stream string, bytes int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MessagesReceived[stream]++
	m.BytesReceived[stream] += bytes
}

func (m *TestMetrics) StreamOpened(stream string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StreamsOpened[stream]++
}

func (m *TestMetrics) StreamClosed(stream string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.StreamsClosed[stream]++
}

func (m *TestMetrics) EncryptionError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EncryptionErrors++
}

func (m *TestMetrics) DecryptionError() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DecryptionErrors++
}

func (m *TestMetrics) KeyDerivation(cached bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.KeyDerivations[cached]++
}

func (m *TestMetrics) EventEmitted(state string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EventsEmitted[state]++
}

func (m *TestMetrics) EventDropped() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.EventsDropped++
}

func (m *TestMetrics) MessageDropped() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.MessagesDropped++
}

func (m *TestMetrics) BackpressureEngaged(stream string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BackpressureEvents[stream]++
}

func (m *TestMetrics) BackpressureWait(stream string, seconds float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BackpressureWaits[stream] = append(m.BackpressureWaits[stream], seconds)
}

func (m *TestMetrics) PendingMessages(stream string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PendingMessagesGauge[stream] = count
}

func TestTestMetrics_RecordsCalls(t *testing.T) {
	m := NewTestMetrics()

	m.ConnectionOpened("inbound")
	m.ConnectionOpened("outbound")
	m.ConnectionOpened("outbound")
	m.ConnectionClosed("inbound")
	m.ConnectionAttempt("success")
	m.ConnectionAttempt("failure")
	m.ConnectionAttempt("failure")
	m.HandshakeDuration(1.5)
	m.HandshakeDuration(2.5)
	m.HandshakeResult("success")
	m.MessageSent("data", 100)
	m.MessageSent("data", 200)
	m.MessageReceived("data", 150)
	m.StreamOpened("data")
	m.StreamClosed("data")
	m.EncryptionError()
	m.DecryptionError()
	m.DecryptionError()
	m.KeyDerivation(true)
	m.KeyDerivation(false)
	m.KeyDerivation(false)
	m.EventEmitted("connected")
	m.EventDropped()
	m.MessageDropped()
	m.MessageDropped()

	// Verify counts
	if m.ConnectionsOpened["inbound"] != 1 {
		t.Errorf("expected 1 inbound connection, got %d", m.ConnectionsOpened["inbound"])
	}
	if m.ConnectionsOpened["outbound"] != 2 {
		t.Errorf("expected 2 outbound connections, got %d", m.ConnectionsOpened["outbound"])
	}
	if m.ConnectionsClosed["inbound"] != 1 {
		t.Errorf("expected 1 closed connection, got %d", m.ConnectionsClosed["inbound"])
	}
	if m.ConnectionAttempts["success"] != 1 {
		t.Errorf("expected 1 success attempt, got %d", m.ConnectionAttempts["success"])
	}
	if m.ConnectionAttempts["failure"] != 2 {
		t.Errorf("expected 2 failure attempts, got %d", m.ConnectionAttempts["failure"])
	}
	if len(m.HandshakeDurations) != 2 {
		t.Errorf("expected 2 handshake durations, got %d", len(m.HandshakeDurations))
	}
	if m.MessagesSent["data"] != 2 {
		t.Errorf("expected 2 messages sent, got %d", m.MessagesSent["data"])
	}
	if m.BytesSent["data"] != 300 {
		t.Errorf("expected 300 bytes sent, got %d", m.BytesSent["data"])
	}
	if m.DecryptionErrors != 2 {
		t.Errorf("expected 2 decryption errors, got %d", m.DecryptionErrors)
	}
	if m.KeyDerivations[true] != 1 {
		t.Errorf("expected 1 cached key derivation, got %d", m.KeyDerivations[true])
	}
	if m.KeyDerivations[false] != 2 {
		t.Errorf("expected 2 uncached key derivations, got %d", m.KeyDerivations[false])
	}
	if m.MessagesDropped != 2 {
		t.Errorf("expected 2 messages dropped, got %d", m.MessagesDropped)
	}
}

func TestTestMetrics_IsThreadSafe(t *testing.T) {
	m := NewTestMetrics()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(6)
		go func() {
			defer wg.Done()
			m.ConnectionOpened("inbound")
		}()
		go func() {
			defer wg.Done()
			m.MessageSent("data", 100)
		}()
		go func() {
			defer wg.Done()
			m.MessageReceived("data", 200)
		}()
		go func() {
			defer wg.Done()
			m.DecryptionError()
		}()
		go func() {
			defer wg.Done()
			m.EventEmitted("connected")
		}()
		go func() {
			defer wg.Done()
			m.HandshakeDuration(1.0)
		}()
	}
	wg.Wait()

	// Verify all calls were recorded
	if m.ConnectionsOpened["inbound"] != 100 {
		t.Errorf("expected 100 connections, got %d", m.ConnectionsOpened["inbound"])
	}
	if m.MessagesSent["data"] != 100 {
		t.Errorf("expected 100 messages sent, got %d", m.MessagesSent["data"])
	}
}

func TestWithMetrics_SetsMetrics(t *testing.T) {
	testMetrics := NewTestMetrics()

	cfg := &Config{}
	opt := WithMetrics(testMetrics)
	opt(cfg)

	if cfg.Metrics != testMetrics {
		t.Error("WithMetrics should set the metrics")
	}
}

func TestConfig_DefaultsToNopMetrics(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()

	if cfg.Metrics == nil {
		t.Error("applyDefaults should set NopMetrics")
	}

	// Verify it's a NopMetrics by type assertion
	_, ok := cfg.Metrics.(NopMetrics)
	if !ok {
		t.Error("default metrics should be NopMetrics")
	}
}

func TestConfig_WithMetrics_OverridesDefault(t *testing.T) {
	testMetrics := NewTestMetrics()

	cfg := &Config{Metrics: testMetrics}
	cfg.applyDefaults()

	// Should not override when already set
	if cfg.Metrics != testMetrics {
		t.Error("applyDefaults should not override existing metrics")
	}
}
