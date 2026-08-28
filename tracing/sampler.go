package tracing

import (
	"strings"

	"go.opentelemetry.io/otel/attribute"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func ignoredAttributes(rules []string) map[attribute.Key]struct{} {
	ignored := make(map[attribute.Key]struct{}, len(rules))
	for _, name := range rules {
		if name = strings.TrimSpace(name); name != "" {
			ignored[attribute.Key(name)] = struct{}{}
		}
	}

	return ignored
}

type dropIgnored struct {
	sdk.Sampler
	ignored map[attribute.Key]struct{}
}

func (s dropIgnored) ShouldSample(p sdk.SamplingParameters) sdk.SamplingResult {
	for _, attr := range p.Attributes {
		if _, ok := s.ignored[attr.Key]; !ok {
			continue
		}

		if attr.Value.Type() == attribute.BOOL && attr.Value.AsBool() {
			return sdk.SamplingResult{
				Decision:   sdk.Drop,
				Tracestate: trace.SpanContextFromContext(p.ParentContext).TraceState(),
			}
		}
	}

	return s.Sampler.ShouldSample(p)
}

func (s dropIgnored) Description() string {
	return "DropIgnored{" + s.Sampler.Description() + "}"
}
