package tracing

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type stubCfg struct {
	enable bool
	name   string
}

func (cfg stubCfg) Validate(map[string]string) (map[string][]string, error) { return nil, nil }

func (cfg stubCfg) Bool(key string) bool {
	return key == "observability.trace.enable" && cfg.enable
}

func (cfg stubCfg) String(key string) string {
	if key == "app.name" {
		return cfg.name
	}

	return ""
}

func (cfg stubCfg) Int(string) int                { return 0 }
func (cfg stubCfg) Float(string) float64          { return 0 }
func (cfg stubCfg) Duration(string) time.Duration { return 0 }
func (cfg stubCfg) Slice(string) []string         { return nil }

func newTracer(t *testing.T, cfg stubCfg) trace.Tracer {
	t.Helper()

	tracer, cleanup, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(cleanup)

	return tracer
}

// A service with tracing off still has to say which lines belong together, so a
// disabled tracer mints ids and exports nothing. Every consumer of a span
// context in this module -- WithCtx in the logger, TraceIDFromContext here --
// gates on the context being valid rather than sampled, which is what makes the
// two halves meet.
func TestNew_DisabledStillMintsATraceIDToCorrelateLogsBy(t *testing.T) {
	tracer := newTracer(t, stubCfg{enable: false, name: "svc"})

	ctx, span := tracer.Start(context.Background(), "op")
	defer span.End()

	spanCtx := span.SpanContext()
	if !spanCtx.IsValid() {
		t.Fatal("a disabled tracer handed back an invalid span context: nothing downstream will print a trace id, so the logs of one operation cannot be told from another's")
	}
	if spanCtx.IsSampled() {
		t.Error("the span is sampled although tracing is disabled: it would be handed to an exporter that was never configured")
	}
	if span.IsRecording() {
		t.Error("the span is recording although tracing is disabled: attributes and events would be kept in memory for a span nobody exports")
	}
	if TraceIDFromContext(ctx) == "" {
		t.Error("TraceIDFromContext came back empty for a live span: the helper the services log through sees no id")
	}
}

// Correlation is only worth anything if it survives the hops of one operation
// and stops at the edge of the next.
func TestNew_DisabledCarriesOneTraceAndKeepsRootsApart(t *testing.T) {
	tracer := newTracer(t, stubCfg{enable: false, name: "svc"})

	ctx, parent := tracer.Start(context.Background(), "parent")
	defer parent.End()

	_, child := tracer.Start(ctx, "child")
	defer child.End()

	if child.SpanContext().TraceID() != parent.SpanContext().TraceID() {
		t.Error("the child left its parent's trace: work of one operation would be scattered across trace ids")
	}
	if child.SpanContext().SpanID() == parent.SpanContext().SpanID() {
		t.Error("the child reused its parent's span id: the two are indistinguishable in a log line")
	}

	_, root := tracer.Start(ctx, "root", trace.WithNewRoot())
	defer root.End()

	if root.SpanContext().TraceID() == parent.SpanContext().TraceID() {
		t.Error("WithNewRoot stayed on the caller's trace: deferred work would keep extending a trace that never ends")
	}
}

// The sampler is explicit rather than left to the environment, so that a config
// saying tracing is off cannot be contradicted by a stray OTEL_TRACES_SAMPLER in
// a deployment. NewTracerProvider applies the environment first and the options
// after, which is what makes the explicit one win.
func TestNew_DisabledCannotBeSampledBackOnByEnvironment(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")

	tracer := newTracer(t, stubCfg{enable: false, name: "svc"})

	_, span := tracer.Start(context.Background(), "op")
	defer span.End()

	if span.SpanContext().IsSampled() || span.IsRecording() {
		t.Error("the environment sampled a span the config disabled: half the switch lives in the yaml and half in the deployment, which is how a service starts exporting by accident")
	}
	if !span.SpanContext().IsValid() {
		t.Error("the span context is invalid: the environment cost us the correlation ids too")
	}
}

// Enabled, the environment stays in charge: the sampler and the exporter are the
// operator's call, and none of the above may quietly override them.
func TestNew_EnabledLeavesTheSamplerToTheEnvironment(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "none")
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")

	tracer := newTracer(t, stubCfg{enable: true, name: "svc"})

	_, span := tracer.Start(context.Background(), "op")
	defer span.End()

	if span.SpanContext().IsSampled() {
		t.Error("always_off was ignored: an operator cannot turn sampling down without a redeploy of the config")
	}
	if !span.SpanContext().IsValid() {
		t.Error("an enabled tracer handed back an invalid span context")
	}
}
