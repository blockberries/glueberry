package prometheus

import (
	"testing"

	"github.com/blockberries/glueberry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestMetricsImplementsInterface verifies that Metrics implements glueberry.Metrics.
func TestMetricsImplementsInterface(t *testing.T) {
	var _ glueberry.Metrics = (*Metrics)(nil)
}

// TestNewMetrics_DefaultNamespace verifies default namespace is used when empty.
func TestNewMetrics_DefaultNamespace(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer("", registry)

	// Record a metric
	m.ConnectionOpened("inbound")

	// Verify metric exists with default namespace
	names, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range names {
		if mf.GetName() == "glueberry_connections_opened_total" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected metric with default namespace 'glueberry'")
	}
}

// TestNewMetrics_CustomNamespace verifies custom namespace is used.
func TestNewMetrics_CustomNamespace(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer("myapp", registry)

	m.ConnectionOpened("outbound")

	names, err := registry.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range names {
		if mf.GetName() == "myapp_connections_opened_total" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected metric with custom namespace 'myapp'")
	}
}

// TestConnectionMetrics tests connection-related metrics.
func TestConnectionMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer("test", registry)

	// Test ConnectionOpened
	m.ConnectionOpened("inbound")
	m.ConnectionOpened("inbound")
	m.ConnectionOpened("outbound")

	if count := testutil.ToFloat64(m.connectionsOpened.WithLabelValues("inbound")); count != 2 {
		t.Errorf("inbound connections = %v, want 2", count)
	}
	if count := testutil.ToFloat64(m.connectionsOpened.WithLabelValues("outbound")); count != 1 {
		t.Errorf("outbound connections = %v, want 1", count)
	}

	// Test ConnectionClosed
	m.ConnectionClosed("inbound")
	if count := testutil.ToFloat64(m.connectionsClosed.WithLabelValues("inbound")); count != 1 {
		t.Errorf("inbound connections closed = %v, want 1", count)
	}

	// Test ConnectionAttempt
	m.ConnectionAttempt("success")
	m.ConnectionAttempt("failure")
	m.ConnectionAttempt("success")

	if count := testutil.ToFloat64(m.connectionAttempts.WithLabelValues("success")); count != 2 {
		t.Errorf("successful attempts = %v, want 2", count)
	}
	if count := testutil.ToFloat64(m.connectionAttempts.WithLabelValues("failure")); count != 1 {
		t.Errorf("failed attempts = %v, want 1", count)
	}
}

// TestHandshakeMetrics tests handshake-related metrics.
func TestHandshakeMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer("test", registry)

	// Test HandshakeDuration
	m.HandshakeDuration(0.5)
	m.HandshakeDuration(1.0)
	m.HandshakeDuration(0.1)

	// Verify histogram has observations
	families, _ := registry.Gather()
	var histFound bool
	for _, mf := range families {
		if mf.GetName() == "test_handshake_duration_seconds" {
			histFound = true
			metrics := mf.GetMetric()
			if len(metrics) == 0 {
				t.Error("expected histogram metrics")
				break
			}
			hist := metrics[0].GetHistogram()
			if hist.GetSampleCount() != 3 {
				t.Errorf("histogram count = %d, want 3", hist.GetSampleCount())
			}
		}
	}
	if !histFound {
		t.Error("handshake_duration_seconds histogram not found")
	}

	// Test HandshakeResult
	m.HandshakeResult("success")
	m.HandshakeResult("failure")
	m.HandshakeResult("timeout")

	if count := testutil.ToFloat64(m.handshakeResults.WithLabelValues("success")); count != 1 {
		t.Errorf("successful handshakes = %v, want 1", count)
	}
	if count := testutil.ToFloat64(m.handshakeResults.WithLabelValues("failure")); count != 1 {
		t.Errorf("failed handshakes = %v, want 1", count)
	}
	if count := testutil.ToFloat64(m.handshakeResults.WithLabelValues("timeout")); count != 1 {
		t.Errorf("timeout handshakes = %v, want 1", count)
	}
}

// TestStreamMetrics tests stream-related metrics.
func TestStreamMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer("test", registry)

	// Test MessageSent
	m.MessageSent("data", 100)
	m.MessageSent("data", 200)
	m.MessageSent("control", 50)

	if count := testutil.ToFloat64(m.messagesSent.WithLabelValues("data")); count != 2 {
		t.Errorf("data messages sent = %v, want 2", count)
	}
	if bytes := testutil.ToFloat64(m.bytesSent.WithLabelValues("data")); bytes != 300 {
		t.Errorf("data bytes sent = %v, want 300", bytes)
	}
	if count := testutil.ToFloat64(m.messagesSent.WithLabelValues("control")); count != 1 {
		t.Errorf("control messages sent = %v, want 1", count)
	}

	// Test MessageReceived
	m.MessageReceived("data", 500)
	m.MessageReceived("data", 300)

	if count := testutil.ToFloat64(m.messagesReceived.WithLabelValues("data")); count != 2 {
		t.Errorf("data messages received = %v, want 2", count)
	}
	if bytes := testutil.ToFloat64(m.bytesReceived.WithLabelValues("data")); bytes != 800 {
		t.Errorf("data bytes received = %v, want 800", bytes)
	}

	// Test StreamOpened/Closed
	m.StreamOpened("data")
	m.StreamOpened("data")
	m.StreamClosed("data")

	if count := testutil.ToFloat64(m.streamsOpened.WithLabelValues("data")); count != 2 {
		t.Errorf("data streams opened = %v, want 2", count)
	}
	if count := testutil.ToFloat64(m.streamsClosed.WithLabelValues("data")); count != 1 {
		t.Errorf("data streams closed = %v, want 1", count)
	}
}

// TestCryptoMetrics tests crypto-related metrics.
func TestCryptoMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer("test", registry)

	// Test EncryptionError
	m.EncryptionError()
	m.EncryptionError()

	if count := testutil.ToFloat64(*m.encryptionErrors); count != 2 {
		t.Errorf("encryption errors = %v, want 2", count)
	}

	// Test DecryptionError
	m.DecryptionError()
	m.DecryptionError()
	m.DecryptionError()

	if count := testutil.ToFloat64(*m.decryptionErrors); count != 3 {
		t.Errorf("decryption errors = %v, want 3", count)
	}

	// Test KeyDerivation
	m.KeyDerivation(true)
	m.KeyDerivation(true)
	m.KeyDerivation(false)

	if count := testutil.ToFloat64(m.keyDerivations.WithLabelValues("true")); count != 2 {
		t.Errorf("cached key derivations = %v, want 2", count)
	}
	if count := testutil.ToFloat64(m.keyDerivations.WithLabelValues("false")); count != 1 {
		t.Errorf("non-cached key derivations = %v, want 1", count)
	}
}

// TestEventMetrics tests event-related metrics.
func TestEventMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer("test", registry)

	// Test EventEmitted
	m.EventEmitted("connected")
	m.EventEmitted("connected")
	m.EventEmitted("disconnected")

	if count := testutil.ToFloat64(m.eventsEmitted.WithLabelValues("connected")); count != 2 {
		t.Errorf("connected events = %v, want 2", count)
	}
	if count := testutil.ToFloat64(m.eventsEmitted.WithLabelValues("disconnected")); count != 1 {
		t.Errorf("disconnected events = %v, want 1", count)
	}

	// Test EventDropped
	m.EventDropped()
	m.EventDropped()

	if count := testutil.ToFloat64(m.eventsDropped); count != 2 {
		t.Errorf("events dropped = %v, want 2", count)
	}

	// Test MessageDropped
	m.MessageDropped()

	if count := testutil.ToFloat64(m.messagesDropped); count != 1 {
		t.Errorf("messages dropped = %v, want 1", count)
	}
}

// TestFlowControlMetrics tests flow control-related metrics.
func TestFlowControlMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer("test", registry)

	// Test BackpressureEngaged
	m.BackpressureEngaged("data")
	m.BackpressureEngaged("data")

	if count := testutil.ToFloat64(m.backpressureEngaged.WithLabelValues("data")); count != 2 {
		t.Errorf("backpressure engaged = %v, want 2", count)
	}

	// Test BackpressureWait
	m.BackpressureWait("data", 0.1)
	m.BackpressureWait("data", 0.5)

	// Test PendingMessages (gauge)
	m.PendingMessages("data", 100)
	if count := testutil.ToFloat64(m.pendingMessages.WithLabelValues("data")); count != 100 {
		t.Errorf("pending messages = %v, want 100", count)
	}

	m.PendingMessages("data", 50)
	if count := testutil.ToFloat64(m.pendingMessages.WithLabelValues("data")); count != 50 {
		t.Errorf("pending messages after decrease = %v, want 50", count)
	}
}

// TestNewMetricsWithNilRegisterer verifies metrics work without registration.
func TestNewMetricsWithNilRegisterer(t *testing.T) {
	// Should not panic
	m := NewMetricsWithRegisterer("test", nil)

	// All operations should work
	m.ConnectionOpened("inbound")
	m.ConnectionClosed("outbound")
	m.ConnectionAttempt("success")
	m.HandshakeDuration(0.5)
	m.HandshakeResult("success")
	m.MessageSent("data", 100)
	m.MessageReceived("data", 200)
	m.StreamOpened("data")
	m.StreamClosed("data")
	m.EncryptionError()
	m.DecryptionError()
	m.KeyDerivation(true)
	m.EventEmitted("connected")
	m.EventDropped()
	m.MessageDropped()
	m.BackpressureEngaged("data")
	m.BackpressureWait("data", 0.1)
	m.PendingMessages("data", 10)
}

// TestConcurrentMetricUpdates tests that metrics are safe for concurrent use.
func TestConcurrentMetricUpdates(t *testing.T) {
	registry := prometheus.NewRegistry()
	m := NewMetricsWithRegisterer("test", registry)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				m.ConnectionOpened("inbound")
				m.ConnectionClosed("inbound")
				m.MessageSent("data", 100)
				m.MessageReceived("data", 200)
				m.EncryptionError()
				m.DecryptionError()
				m.EventEmitted("connected")
				m.BackpressureEngaged("data")
				m.PendingMessages("data", j)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify counts are as expected
	if count := testutil.ToFloat64(m.connectionsOpened.WithLabelValues("inbound")); count != 1000 {
		t.Errorf("concurrent connections opened = %v, want 1000", count)
	}
	if count := testutil.ToFloat64(m.messagesSent.WithLabelValues("data")); count != 1000 {
		t.Errorf("concurrent messages sent = %v, want 1000", count)
	}
}
