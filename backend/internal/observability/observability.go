// Package observability wires OpenTelemetry tracing. By default (no endpoint) it
// installs nothing, so the global tracer stays a no-op and the app runs with no
// external dependencies. Point config.Observability.Endpoint at an OTLP/gRPC
// collector (host:port) to export traces; otelgin and otelpgx then light up.
package observability

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Provider struct {
	tp *sdktrace.TracerProvider
}

// Init configures the global tracer provider. When endpoint is empty it is a
// no-op; otherwise it exports via OTLP/gRPC (insecure — front with a collector
// or set up TLS for production).
func Init(serviceName, endpoint string) *Provider {
	if endpoint == "" {
		log.Printf("observability: no-op tracer (set observability.endpoint to export)")
		return &Provider{}
	}

	ctx := context.Background()
	exp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		log.Printf("observability: exporter init failed (%v); continuing without export", err)
		return &Provider{}
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", serviceName)),
	)
	if err != nil {
		log.Printf("observability: resource init failed (%v); using default", err)
		res = resource.Default()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	log.Printf("observability: exporting traces for %q to %s", serviceName, endpoint)
	return &Provider{tp: tp}
}

// TracerProvider returns the installed tracer provider so other subsystems can
// attach their instrumentation to the same pipeline — e.g. otelpgx for the
// Postgres pool, or a Redis/HTTP client later. When Init ran no-op, this falls
// back to the global provider (also a no-op), so callers never special-case it.
func (p *Provider) TracerProvider() trace.TracerProvider {
	if p.tp != nil {
		return p.tp
	}
	return otel.GetTracerProvider()
}

// Shutdown flushes and stops the exporter. Safe to call on a no-op provider.
func (p *Provider) Shutdown() {
	if p.tp == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.tp.Shutdown(ctx); err != nil {
		log.Printf("observability: shutdown: %v", err)
	}
}
