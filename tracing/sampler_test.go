package tracing

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func filtered(t *testing.T, rules ...string) (trace.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()
	provider := sdk.NewTracerProvider(
		sdk.WithSpanProcessor(sdk.NewSimpleSpanProcessor(exporter)),
		sdk.WithSampler(dropIgnored{
			Sampler: sdk.ParentBased(sdk.AlwaysSample()),
			ignored: ignoredAttributes(rules),
		}),
	)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	return provider.Tracer("test"), exporter
}

func exported(exporter *tracetest.InMemoryExporter) []string {
	names := make([]string, 0, len(exporter.GetSpans()))
	for _, span := range exporter.GetSpans() {
		names = append(names, span.Name)
	}

	return names
}

func TestIgnoredAttributes_SkipsBlanksAndTrimsNames(t *testing.T) {
	ignored := ignoredAttributes([]string{" stream ", "", "   ", "bulk"})

	if len(ignored) != 2 {
		t.Fatalf("parsed %d names from a list with blanks and padding, want 2: %v", len(ignored), ignored)
	}
	if _, ok := ignored["stream"]; !ok {
		t.Fatal(`" stream " did not become the key "stream": a padded config entry silently stops matching anything`)
	}
}

func TestDropIgnored_RefusesTheMarkedSpanAndKeepsTheRest(t *testing.T) {
	tracer, exporter := filtered(t, "stream")

	_, marked := tracer.Start(context.Background(), "stream.op",
		trace.WithAttributes(attribute.Bool("stream", true)))
	marked.End()

	_, plain := tracer.Start(context.Background(), "request.op")
	plain.End()

	if names := exported(exporter); len(names) != 1 || names[0] != "request.op" {
		t.Fatalf("exported %v, want only request.op: the rule has to drop the marked span and leave everything else alone", names)
	}
}

func TestDropIgnored_KeepsTheSpanWhenTheMarkerIsFalse(t *testing.T) {
	tracer, exporter := filtered(t, "stream")

	_, span := tracer.Start(context.Background(), "op",
		trace.WithAttributes(attribute.Bool("stream", false)))
	span.End()

	if names := exported(exporter); len(names) != 1 {
		t.Fatalf("exported %v, want the span: stream=false must not be read as marked", names)
	}
}

func TestDropIgnored_IgnoresANonBooleanAttributeOfTheSameName(t *testing.T) {
	tracer, exporter := filtered(t, "stream")

	_, span := tracer.Start(context.Background(), "op",
		trace.WithAttributes(attribute.Int("stream", 1)))
	span.End()

	if names := exported(exporter); len(names) != 1 {
		t.Fatalf("exported %v, want the span: an int attribute must not be read as the boolean marker", names)
	}
}

func TestDropIgnored_LeavesTheDroppedSpanContextValid(t *testing.T) {
	tracer, _ := filtered(t, "stream")

	_, span := tracer.Start(context.Background(), "op",
		trace.WithAttributes(attribute.Bool("stream", true)))
	defer span.End()

	spanCtx := span.SpanContext()
	if !spanCtx.IsValid() {
		t.Fatal("a dropped span handed back an invalid span context: the log lines of that path lose their trace id, which is the one thing the filter must not take away")
	}
	if spanCtx.IsSampled() {
		t.Fatal("a dropped span is still marked sampled: the exporter will be asked for it after all")
	}
}

func TestDropIgnored_TakesTheSubtreeWithIt(t *testing.T) {
	tracer, exporter := filtered(t, "stream")

	ctx, parent := tracer.Start(context.Background(), "stream.op",
		trace.WithAttributes(attribute.Bool("stream", true)))
	_, child := tracer.Start(ctx, "child.op")
	child.End()
	parent.End()

	if names := exported(exporter); len(names) != 0 {
		t.Fatalf("exported %v, want nothing: the children of a dropped span have to go with it, or marking one entry would not be enough", names)
	}
}

func TestDropIgnored_KeepsAnIndependentRootUnderADroppedSpan(t *testing.T) {
	tracer, exporter := filtered(t, "stream")

	ctx, parent := tracer.Start(context.Background(), "stream.op",
		trace.WithAttributes(attribute.Bool("stream", true)))
	_, root := tracer.Start(ctx, "dispatch.op", trace.WithNewRoot())
	root.End()
	parent.End()

	if names := exported(exporter); len(names) != 1 || names[0] != "dispatch.op" {
		t.Fatalf("exported %v, want dispatch.op: a span that starts its own root is not the dropped span's child and must be sampled on its own", names)
	}
}

func TestDropIgnored_DefersToTheSamplerUnderneath(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdk.NewTracerProvider(
		sdk.WithSpanProcessor(sdk.NewSimpleSpanProcessor(exporter)),
		sdk.WithSampler(dropIgnored{
			Sampler: sdk.NeverSample(),
			ignored: ignoredAttributes([]string{"stream"}),
		}),
	)
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	_, span := provider.Tracer("test").Start(context.Background(), "op")
	span.End()

	if names := exported(exporter); len(names) != 0 {
		t.Fatalf("exported %v with NeverSample underneath: an unmarked span must be left to the sampler below, not sampled by this one", names)
	}
}

func TestNew_WithoutRulesLeavesTheEnvironmentSamplerAlone(t *testing.T) {
	t.Setenv("OTEL_TRACES_SAMPLER", "always_off")

	tracer := newTracer(t, stubCfg{enable: true, name: "svc"})

	_, span := tracer.Start(context.Background(), "op")
	defer span.End()

	if span.IsRecording() {
		t.Fatal("OTEL_TRACES_SAMPLER=always_off did not take: the provider was handed a sampler of ours and overrode the environment config")
	}
}

func TestNew_WithRulesStillRefusesTheMarkedSpan(t *testing.T) {
	tracer := newTracer(t, stubCfg{enable: true, name: "svc", rules: []string{"stream"}})

	_, marked := tracer.Start(context.Background(), "op",
		trace.WithAttributes(attribute.Bool("stream", true)))
	defer marked.End()

	if marked.IsRecording() {
		t.Fatal("the marked span is recording: sample_rules did not reach the provider")
	}

	_, plain := tracer.Start(context.Background(), "op")
	defer plain.End()

	if !plain.IsRecording() {
		t.Fatal("an unmarked span stopped recording: the rule is dropping more than it was given")
	}
}
