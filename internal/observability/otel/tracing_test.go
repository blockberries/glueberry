package otel

import (
	"context"
	"errors"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestNewTracer(t *testing.T) {
	// Test with nil provider (should use noop)
	tracer := NewTracer(nil)
	if tracer == nil {
		t.Fatal("NewTracer(nil) returned nil")
	}
	if tracer.tracer == nil {
		t.Error("tracer.tracer is nil")
	}

	// Test with real provider
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	tracer = NewTracer(tp)
	if tracer == nil {
		t.Error("NewTracer(tp) returned nil")
	}
}

func TestTracer_StartConnect(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	peerID := peer.ID("test-peer")

	ctx, span := tracer.StartConnect(context.Background(), peerID, "outbound")
	span.End()

	if ctx == nil {
		t.Error("context should not be nil")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != SpanConnect {
		t.Errorf("span name = %q, want %q", spans[0].Name, SpanConnect)
	}

	// Check attributes
	var foundPeerID, foundDirection bool
	for _, attr := range spans[0].Attributes {
		if string(attr.Key) == AttrPeerID && attr.Value.AsString() == peerID.String() {
			foundPeerID = true
		}
		if string(attr.Key) == AttrConnectionDirection && attr.Value.AsString() == "outbound" {
			foundDirection = true
		}
	}
	if !foundPeerID {
		t.Error("peer.id attribute not found")
	}
	if !foundDirection {
		t.Error("connection.direction attribute not found")
	}
}

func TestTracer_StartHandshake(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	peerID := peer.ID("test-peer")

	ctx, span := tracer.StartHandshake(context.Background(), peerID)
	span.End()

	if ctx == nil {
		t.Error("context should not be nil")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != SpanHandshake {
		t.Errorf("span name = %q, want %q", spans[0].Name, SpanHandshake)
	}
}

func TestTracer_StartSend(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	peerID := peer.ID("test-peer")

	ctx, span := tracer.StartSend(context.Background(), peerID, "data", 1024)
	span.End()

	if ctx == nil {
		t.Error("context should not be nil")
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	if spans[0].Name != SpanSend {
		t.Errorf("span name = %q, want %q", spans[0].Name, SpanSend)
	}

	// Check that message size attribute is set
	var foundSize bool
	for _, attr := range spans[0].Attributes {
		if string(attr.Key) == AttrMessageSize && attr.Value.AsInt64() == 1024 {
			foundSize = true
		}
	}
	if !foundSize {
		t.Error("message.size attribute not found or incorrect")
	}
}

func TestTracer_RecordHandshakeResult(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	peerID := peer.ID("test-peer")

	// Test success case
	_, span := tracer.StartHandshake(context.Background(), peerID)
	tracer.RecordHandshakeResult(span, "success", nil)
	span.End()

	spans := exporter.GetSpans()
	if spans[0].Status.Code != codes.Ok {
		t.Errorf("status code = %v, want Ok", spans[0].Status.Code)
	}

	// Test failure case
	exporter.Reset()
	_, span = tracer.StartHandshake(context.Background(), peerID)
	testErr := errors.New("handshake failed")
	tracer.RecordHandshakeResult(span, "failure", testErr)
	span.End()

	spans = exporter.GetSpans()
	if spans[0].Status.Code != codes.Error {
		t.Errorf("status code = %v, want Error", spans[0].Status.Code)
	}
}

func TestTracer_EndSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	peerID := peer.ID("test-peer")

	// Test with no error
	_, span := tracer.StartConnect(context.Background(), peerID, "inbound")
	tracer.EndSpan(span, nil)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Test with error
	exporter.Reset()
	_, span = tracer.StartConnect(context.Background(), peerID, "inbound")
	tracer.EndSpan(span, errors.New("connection failed"))

	spans = exporter.GetSpans()
	if spans[0].Status.Code != codes.Error {
		t.Errorf("status code = %v, want Error", spans[0].Status.Code)
	}
}

func TestNopTracer(t *testing.T) {
	tracer := NewNopTracer()
	peerID := peer.ID("test-peer")

	// All methods should not panic
	ctx, span := tracer.StartConnect(context.Background(), peerID, "outbound")
	if ctx == nil {
		t.Error("context should not be nil")
	}
	span.End()

	_, span = tracer.StartDial(context.Background(), peerID)
	span.End()

	_, span = tracer.StartHandshake(context.Background(), peerID)
	tracer.RecordHandshakeResult(span, "success", nil)
	span.End()

	_, span = tracer.StartKeyDerivation(context.Background(), peerID)
	span.End()

	_, span = tracer.StartStreamSetup(context.Background(), peerID, "data")
	span.End()

	_, span = tracer.StartSend(context.Background(), peerID, "data", 100)
	tracer.EndSpan(span, nil)

	_, span = tracer.StartEncrypt(context.Background(), 100)
	span.End()

	_, span = tracer.StartReceive(context.Background(), peerID, "data")
	span.End()

	_, span = tracer.StartDecrypt(context.Background())
	span.End()

	_, span = tracer.StartDisconnect(context.Background(), peerID)
	tracer.RecordError(span, errors.New("test error"))
	tracer.EndSpan(span, errors.New("test"))
}

func TestTracer_AllSpanTypes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	tracer := NewTracer(tp)
	peerID := peer.ID("test-peer")

	// Test all span types
	tests := []struct {
		name     string
		startFn  func() (context.Context, trace.Span)
		expected string
	}{
		{
			name: "Connect",
			startFn: func() (context.Context, trace.Span) {
				return tracer.StartConnect(context.Background(), peerID, "outbound")
			},
			expected: SpanConnect,
		},
		{
			name:     "Dial",
			startFn:  func() (context.Context, trace.Span) { return tracer.StartDial(context.Background(), peerID) },
			expected: SpanDial,
		},
		{
			name:     "Handshake",
			startFn:  func() (context.Context, trace.Span) { return tracer.StartHandshake(context.Background(), peerID) },
			expected: SpanHandshake,
		},
		{
			name:     "KeyDerivation",
			startFn:  func() (context.Context, trace.Span) { return tracer.StartKeyDerivation(context.Background(), peerID) },
			expected: SpanKeyDerive,
		},
		{
			name: "StreamSetup",
			startFn: func() (context.Context, trace.Span) {
				return tracer.StartStreamSetup(context.Background(), peerID, "data")
			},
			expected: SpanStreamSetup,
		},
		{
			name: "Send",
			startFn: func() (context.Context, trace.Span) {
				return tracer.StartSend(context.Background(), peerID, "data", 100)
			},
			expected: SpanSend,
		},
		{
			name:     "Encrypt",
			startFn:  func() (context.Context, trace.Span) { return tracer.StartEncrypt(context.Background(), 100) },
			expected: SpanEncrypt,
		},
		{
			name:     "Receive",
			startFn:  func() (context.Context, trace.Span) { return tracer.StartReceive(context.Background(), peerID, "data") },
			expected: SpanReceive,
		},
		{
			name:     "Decrypt",
			startFn:  func() (context.Context, trace.Span) { return tracer.StartDecrypt(context.Background()) },
			expected: SpanDecrypt,
		},
		{
			name:     "Disconnect",
			startFn:  func() (context.Context, trace.Span) { return tracer.StartDisconnect(context.Background(), peerID) },
			expected: SpanDisconnect,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter.Reset()
			_, span := tt.startFn()
			span.End()

			spans := exporter.GetSpans()
			if len(spans) != 1 {
				t.Fatalf("expected 1 span, got %d", len(spans))
			}

			if spans[0].Name != tt.expected {
				t.Errorf("span name = %q, want %q", spans[0].Name, tt.expected)
			}
		})
	}
}
