// Package otel provides OpenTelemetry tracing integration for Glueberry.
//
// This package enables distributed tracing of Glueberry operations using
// OpenTelemetry. Traces provide visibility into connection lifecycle,
// handshake operations, and message flow.
//
// # Span Hierarchy
//
// The following spans are created during normal operation:
//
//	glueberry.connect
//	├── glueberry.dial                 (outbound connections)
//	├── glueberry.handshake
//	│   ├── glueberry.key_derivation
//	│   └── glueberry.stream_setup
//	└── glueberry.established
//
//	glueberry.send
//	├── glueberry.encrypt
//	└── glueberry.write
//
//	glueberry.receive
//	├── glueberry.read
//	└── glueberry.decrypt
//
// # Attributes
//
// Common span attributes include:
//   - peer.id: The remote peer's ID
//   - stream.name: The stream name for message operations
//   - message.size: Size of sent/received messages
//   - connection.direction: "inbound" or "outbound"
//   - handshake.result: "success", "failure", or "timeout"
//
// # Example Usage
//
//	import (
//	    "github.com/blockberries/glueberry"
//	    glueberryotel "github.com/blockberries/glueberry/otel"
//	    "go.opentelemetry.io/otel"
//	)
//
//	func main() {
//	    tp := otel.GetTracerProvider()
//	    tracer := glueberryotel.NewTracer(tp)
//
//	    cfg := glueberry.NewConfig(key, path, addrs,
//	        glueberry.WithTracer(tracer),
//	    )
//
//	    node, err := glueberry.New(cfg)
//	    // ...
//	}
package otel

import (
	"context"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	// TracerName is the name used for the OpenTelemetry tracer.
	TracerName = "github.com/blockberries/glueberry"

	// Span names
	SpanConnect     = "glueberry.connect"
	SpanDial        = "glueberry.dial"
	SpanHandshake   = "glueberry.handshake"
	SpanKeyDerive   = "glueberry.key_derivation"
	SpanStreamSetup = "glueberry.stream_setup"
	SpanEstablished = "glueberry.established"
	SpanSend        = "glueberry.send"
	SpanEncrypt     = "glueberry.encrypt"
	SpanWrite       = "glueberry.write"
	SpanReceive     = "glueberry.receive"
	SpanRead        = "glueberry.read"
	SpanDecrypt     = "glueberry.decrypt"
	SpanDisconnect  = "glueberry.disconnect"

	// Attribute keys
	AttrPeerID              = "peer.id"
	AttrStreamName          = "stream.name"
	AttrMessageSize         = "message.size"
	AttrConnectionDirection = "connection.direction"
	AttrHandshakeResult     = "handshake.result"
	AttrErrorMessage        = "error.message"
)

// Tracer provides OpenTelemetry tracing for Glueberry operations.
// It wraps an OpenTelemetry TracerProvider and creates spans for
// connection lifecycle, handshakes, and message operations.
//
// Tracer is safe for concurrent use.
type Tracer struct {
	tracer trace.Tracer
}

// NewTracer creates a new Tracer using the given TracerProvider.
// If provider is nil, a no-op tracer is used.
func NewTracer(provider trace.TracerProvider) *Tracer {
	if provider == nil {
		return &Tracer{tracer: noop.NewTracerProvider().Tracer(TracerName)}
	}
	return &Tracer{tracer: provider.Tracer(TracerName)}
}

// StartConnect starts a span for a connection attempt.
func (t *Tracer) StartConnect(ctx context.Context, peerID peer.ID, direction string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, SpanConnect,
		trace.WithAttributes(
			attribute.String(AttrPeerID, peerID.String()),
			attribute.String(AttrConnectionDirection, direction),
		),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// StartDial starts a span for dialing a peer.
func (t *Tracer) StartDial(ctx context.Context, peerID peer.ID) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, SpanDial,
		trace.WithAttributes(
			attribute.String(AttrPeerID, peerID.String()),
		),
	)
}

// StartHandshake starts a span for a handshake operation.
func (t *Tracer) StartHandshake(ctx context.Context, peerID peer.ID) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, SpanHandshake,
		trace.WithAttributes(
			attribute.String(AttrPeerID, peerID.String()),
		),
	)
}

// StartKeyDerivation starts a span for key derivation.
func (t *Tracer) StartKeyDerivation(ctx context.Context, peerID peer.ID) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, SpanKeyDerive,
		trace.WithAttributes(
			attribute.String(AttrPeerID, peerID.String()),
		),
	)
}

// StartStreamSetup starts a span for stream setup.
func (t *Tracer) StartStreamSetup(ctx context.Context, peerID peer.ID, streamName string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, SpanStreamSetup,
		trace.WithAttributes(
			attribute.String(AttrPeerID, peerID.String()),
			attribute.String(AttrStreamName, streamName),
		),
	)
}

// StartSend starts a span for sending a message.
func (t *Tracer) StartSend(ctx context.Context, peerID peer.ID, streamName string, size int) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, SpanSend,
		trace.WithAttributes(
			attribute.String(AttrPeerID, peerID.String()),
			attribute.String(AttrStreamName, streamName),
			attribute.Int(AttrMessageSize, size),
		),
		trace.WithSpanKind(trace.SpanKindProducer),
	)
}

// StartEncrypt starts a span for encryption.
func (t *Tracer) StartEncrypt(ctx context.Context, size int) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, SpanEncrypt,
		trace.WithAttributes(
			attribute.Int(AttrMessageSize, size),
		),
	)
}

// StartReceive starts a span for receiving a message.
func (t *Tracer) StartReceive(ctx context.Context, peerID peer.ID, streamName string) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, SpanReceive,
		trace.WithAttributes(
			attribute.String(AttrPeerID, peerID.String()),
			attribute.String(AttrStreamName, streamName),
		),
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
}

// StartDecrypt starts a span for decryption.
func (t *Tracer) StartDecrypt(ctx context.Context) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, SpanDecrypt)
}

// StartDisconnect starts a span for disconnection.
func (t *Tracer) StartDisconnect(ctx context.Context, peerID peer.ID) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, SpanDisconnect,
		trace.WithAttributes(
			attribute.String(AttrPeerID, peerID.String()),
		),
	)
}

// RecordHandshakeResult records the result of a handshake on the given span.
func (t *Tracer) RecordHandshakeResult(span trace.Span, result string, err error) {
	span.SetAttributes(attribute.String(AttrHandshakeResult, result))
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String(AttrErrorMessage, err.Error()))
	} else {
		span.SetStatus(codes.Ok, "")
	}
}

// RecordError records an error on the given span.
func (t *Tracer) RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// EndSpan ends a span, optionally recording an error.
func (t *Tracer) EndSpan(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// NopTracer is a no-op tracer that does nothing.
// It is used when tracing is disabled.
// NopTracer wraps the real Tracer with a noop provider.
type NopTracer struct {
	*Tracer
}

// NewNopTracer creates a new no-op tracer.
func NewNopTracer() *NopTracer {
	return &NopTracer{
		Tracer: NewTracer(nil),
	}
}
