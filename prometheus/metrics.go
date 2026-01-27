// Package prometheus provides a Prometheus implementation of the glueberry.Metrics interface.
//
// This package enables integration with Prometheus monitoring systems. All metrics
// are registered with the default Prometheus registry and follow Prometheus naming
// conventions.
//
// # Metric Names
//
// All metrics use the configured namespace prefix (default: "glueberry"). The full
// metric name follows the pattern: {namespace}_{subsystem}_{name}
//
// # Counters
//
//	glueberry_connections_opened_total{direction="inbound|outbound"}
//	glueberry_connections_closed_total{direction="inbound|outbound"}
//	glueberry_connection_attempts_total{result="success|failure"}
//	glueberry_handshake_results_total{result="success|failure|timeout"}
//	glueberry_messages_sent_total{stream="<name>"}
//	glueberry_messages_received_total{stream="<name>"}
//	glueberry_bytes_sent_total{stream="<name>"}
//	glueberry_bytes_received_total{stream="<name>"}
//	glueberry_streams_opened_total{stream="<name>"}
//	glueberry_streams_closed_total{stream="<name>"}
//	glueberry_encryption_errors_total
//	glueberry_decryption_errors_total
//	glueberry_key_derivations_total{cached="true|false"}
//	glueberry_events_emitted_total{state="<state>"}
//	glueberry_events_dropped_total
//	glueberry_messages_dropped_total
//	glueberry_backpressure_engaged_total{stream="<name>"}
//
// # Histograms
//
//	glueberry_handshake_duration_seconds
//	glueberry_backpressure_wait_seconds{stream="<name>"}
//
// # Gauges
//
//	glueberry_pending_messages{stream="<name>"}
//
// # Example Usage
//
//	import (
//	    "github.com/blockberries/glueberry"
//	    prommetrics "github.com/blockberries/glueberry/prometheus"
//	    "github.com/prometheus/client_golang/prometheus/promhttp"
//	)
//
//	func main() {
//	    metrics := prommetrics.NewMetrics("myapp")
//
//	    cfg := glueberry.NewConfig(key, path, addrs,
//	        glueberry.WithMetrics(metrics),
//	    )
//
//	    node, err := glueberry.New(cfg)
//	    // ...
//
//	    // Expose metrics endpoint
//	    http.Handle("/metrics", promhttp.Handler())
//	    http.ListenAndServe(":9090", nil)
//	}
package prometheus

import (
	"strconv"

	"github.com/blockberries/glueberry"
	"github.com/prometheus/client_golang/prometheus"
)

// DefaultNamespace is the default namespace for all metrics.
const DefaultNamespace = "glueberry"

// Metrics implements the glueberry.Metrics interface using Prometheus metrics.
// All metrics are registered with the default Prometheus registry.
//
// Metrics is safe for concurrent use.
type Metrics struct {
	// Connection metrics
	connectionsOpened *prometheus.CounterVec
	connectionsClosed *prometheus.CounterVec
	connectionAttempts *prometheus.CounterVec
	handshakeDuration prometheus.Histogram
	handshakeResults  *prometheus.CounterVec

	// Stream metrics
	messagesSent     *prometheus.CounterVec
	messagesReceived *prometheus.CounterVec
	bytesSent        *prometheus.CounterVec
	bytesReceived    *prometheus.CounterVec
	streamsOpened    *prometheus.CounterVec
	streamsClosed    *prometheus.CounterVec

	// Crypto metrics
	encryptionErrors *prometheus.Counter
	decryptionErrors *prometheus.Counter
	keyDerivations   *prometheus.CounterVec

	// Event metrics
	eventsEmitted   *prometheus.CounterVec
	eventsDropped   prometheus.Counter
	messagesDropped prometheus.Counter

	// Flow control metrics
	backpressureEngaged *prometheus.CounterVec
	backpressureWait    *prometheus.HistogramVec
	pendingMessages     *prometheus.GaugeVec
}

// Ensure Metrics implements glueberry.Metrics.
var _ glueberry.Metrics = (*Metrics)(nil)

// NewMetrics creates a new Prometheus metrics collector with the given namespace.
// If namespace is empty, DefaultNamespace ("glueberry") is used.
//
// All metrics are automatically registered with the default Prometheus registry.
// If registration fails (e.g., metrics already registered), this function will panic.
// To avoid panics, use NewMetricsWithRegisterer with a custom registry.
func NewMetrics(namespace string) *Metrics {
	return NewMetricsWithRegisterer(namespace, prometheus.DefaultRegisterer)
}

// NewMetricsWithRegisterer creates a new Prometheus metrics collector with the given
// namespace and registerer. This allows using a custom registry for testing or
// to avoid conflicts with other metrics.
//
// If namespace is empty, DefaultNamespace ("glueberry") is used.
// If registerer is nil, metrics will not be registered automatically.
func NewMetricsWithRegisterer(namespace string, registerer prometheus.Registerer) *Metrics {
	if namespace == "" {
		namespace = DefaultNamespace
	}

	encryptionErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "encryption_errors_total",
		Help:      "Total number of encryption errors",
	})

	decryptionErrors := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "decryption_errors_total",
		Help:      "Total number of decryption errors",
	})

	eventsDropped := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_dropped_total",
		Help:      "Total number of events dropped due to buffer full",
	})

	messagesDropped := prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "messages_dropped_total",
		Help:      "Total number of messages dropped due to buffer full",
	})

	m := &Metrics{
		connectionsOpened: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "connections_opened_total",
				Help:      "Total number of connections opened",
			},
			[]string{"direction"},
		),
		connectionsClosed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "connections_closed_total",
				Help:      "Total number of connections closed",
			},
			[]string{"direction"},
		),
		connectionAttempts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "connection_attempts_total",
				Help:      "Total number of connection attempts by result",
			},
			[]string{"result"},
		),
		handshakeDuration: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "handshake_duration_seconds",
				Help:      "Histogram of successful handshake durations",
				Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
		),
		handshakeResults: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "handshake_results_total",
				Help:      "Total number of handshake results by outcome",
			},
			[]string{"result"},
		),
		messagesSent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "messages_sent_total",
				Help:      "Total number of messages sent per stream",
			},
			[]string{"stream"},
		),
		messagesReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "messages_received_total",
				Help:      "Total number of messages received per stream",
			},
			[]string{"stream"},
		),
		bytesSent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "bytes_sent_total",
				Help:      "Total bytes sent per stream",
			},
			[]string{"stream"},
		),
		bytesReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "bytes_received_total",
				Help:      "Total bytes received per stream",
			},
			[]string{"stream"},
		),
		streamsOpened: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "streams_opened_total",
				Help:      "Total number of streams opened per stream type",
			},
			[]string{"stream"},
		),
		streamsClosed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "streams_closed_total",
				Help:      "Total number of streams closed per stream type",
			},
			[]string{"stream"},
		),
		encryptionErrors: &encryptionErrors,
		decryptionErrors: &decryptionErrors,
		keyDerivations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "key_derivations_total",
				Help:      "Total number of key derivations by cache status",
			},
			[]string{"cached"},
		),
		eventsEmitted: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "events_emitted_total",
				Help:      "Total number of events emitted by state",
			},
			[]string{"state"},
		),
		eventsDropped:   eventsDropped,
		messagesDropped: messagesDropped,
		backpressureEngaged: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "backpressure_engaged_total",
				Help:      "Total times backpressure was engaged per stream",
			},
			[]string{"stream"},
		),
		backpressureWait: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Name:      "backpressure_wait_seconds",
				Help:      "Histogram of time spent waiting due to backpressure",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10},
			},
			[]string{"stream"},
		),
		pendingMessages: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "pending_messages",
				Help:      "Current number of pending messages per stream",
			},
			[]string{"stream"},
		),
	}

	if registerer != nil {
		registerer.MustRegister(
			m.connectionsOpened,
			m.connectionsClosed,
			m.connectionAttempts,
			m.handshakeDuration,
			m.handshakeResults,
			m.messagesSent,
			m.messagesReceived,
			m.bytesSent,
			m.bytesReceived,
			m.streamsOpened,
			m.streamsClosed,
			encryptionErrors,
			decryptionErrors,
			m.keyDerivations,
			m.eventsEmitted,
			eventsDropped,
			messagesDropped,
			m.backpressureEngaged,
			m.backpressureWait,
			m.pendingMessages,
		)
	}

	return m
}

// ConnectionOpened implements glueberry.Metrics.
func (m *Metrics) ConnectionOpened(direction string) {
	m.connectionsOpened.WithLabelValues(direction).Inc()
}

// ConnectionClosed implements glueberry.Metrics.
func (m *Metrics) ConnectionClosed(direction string) {
	m.connectionsClosed.WithLabelValues(direction).Inc()
}

// ConnectionAttempt implements glueberry.Metrics.
func (m *Metrics) ConnectionAttempt(result string) {
	m.connectionAttempts.WithLabelValues(result).Inc()
}

// HandshakeDuration implements glueberry.Metrics.
func (m *Metrics) HandshakeDuration(seconds float64) {
	m.handshakeDuration.Observe(seconds)
}

// HandshakeResult implements glueberry.Metrics.
func (m *Metrics) HandshakeResult(result string) {
	m.handshakeResults.WithLabelValues(result).Inc()
}

// MessageSent implements glueberry.Metrics.
func (m *Metrics) MessageSent(stream string, bytes int) {
	m.messagesSent.WithLabelValues(stream).Inc()
	m.bytesSent.WithLabelValues(stream).Add(float64(bytes))
}

// MessageReceived implements glueberry.Metrics.
func (m *Metrics) MessageReceived(stream string, bytes int) {
	m.messagesReceived.WithLabelValues(stream).Inc()
	m.bytesReceived.WithLabelValues(stream).Add(float64(bytes))
}

// StreamOpened implements glueberry.Metrics.
func (m *Metrics) StreamOpened(stream string) {
	m.streamsOpened.WithLabelValues(stream).Inc()
}

// StreamClosed implements glueberry.Metrics.
func (m *Metrics) StreamClosed(stream string) {
	m.streamsClosed.WithLabelValues(stream).Inc()
}

// EncryptionError implements glueberry.Metrics.
func (m *Metrics) EncryptionError() {
	(*m.encryptionErrors).Inc()
}

// DecryptionError implements glueberry.Metrics.
func (m *Metrics) DecryptionError() {
	(*m.decryptionErrors).Inc()
}

// KeyDerivation implements glueberry.Metrics.
func (m *Metrics) KeyDerivation(cached bool) {
	m.keyDerivations.WithLabelValues(strconv.FormatBool(cached)).Inc()
}

// EventEmitted implements glueberry.Metrics.
func (m *Metrics) EventEmitted(state string) {
	m.eventsEmitted.WithLabelValues(state).Inc()
}

// EventDropped implements glueberry.Metrics.
func (m *Metrics) EventDropped() {
	m.eventsDropped.Inc()
}

// MessageDropped implements glueberry.Metrics.
func (m *Metrics) MessageDropped() {
	m.messagesDropped.Inc()
}

// BackpressureEngaged implements glueberry.Metrics.
func (m *Metrics) BackpressureEngaged(stream string) {
	m.backpressureEngaged.WithLabelValues(stream).Inc()
}

// BackpressureWait implements glueberry.Metrics.
func (m *Metrics) BackpressureWait(stream string, seconds float64) {
	m.backpressureWait.WithLabelValues(stream).Observe(seconds)
}

// PendingMessages implements glueberry.Metrics.
func (m *Metrics) PendingMessages(stream string, count int) {
	m.pendingMessages.WithLabelValues(stream).Set(float64(count))
}
