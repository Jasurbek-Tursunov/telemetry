package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type Cfg interface {
	Validate(rules map[string]string) (map[string][]string, error)

	Bool(key string) bool

	Int(key string) int

	Float(key string) float64

	String(key string) string

	Duration(key string) time.Duration

	Slice(key string) []string
}

func New(cfg Cfg) (trace.Tracer, func(), error) {
	if !cfg.Bool("observability.trace.enable") {
		// Disabled exports nothing but still mints trace ids: WithCtx and
		// TraceIDFromContext gate on the span context being valid, not sampled.
		tp := sdk.NewTracerProvider(sdk.WithSampler(sdk.NeverSample()))

		return tp.Tracer(cfg.String("app.name")), func() {}, nil
	}

	exporter, err := autoexport.NewSpanExporter(context.Background())
	if err != nil {
		return nil, nil, err
	}

	tp := sdk.NewTracerProvider(
		sdk.WithBatcher(exporter),
	)

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(shutdownCtx)
	}

	return tp.Tracer(cfg.String("app.name")), cleanup, nil
}
