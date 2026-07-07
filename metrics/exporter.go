package metrics

import (
	"context"
	"fmt"
	"net/http"

	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

var (
	prometheusRegistry *prom.Registry
)

func GetPrometheusHandler() http.Handler {
	if prometheusRegistry != nil {
		return promhttp.HandlerFor(prometheusRegistry, promhttp.HandlerOpts{})
	}

	return nil
}

func newMeterProvider(cfg Cfg) (*metric.MeterProvider, error) {
	if !cfg.Bool("observability.metrics.enable") {
		return &metric.MeterProvider{}, nil
	}

	res, err := resource.New(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	provider := cfg.String("observability.metrics.provider")
	switch provider {
	case "otlp":
		return newOTLPMeterProvider(res)
	case "prometheus":
		return newPrometheusMeterProvider(res)
	case "mock":
		return newMockMeterProvider(res)
	default:
		return nil, fmt.Errorf("unknown provider type: '%s'", provider)
	}
}

func newPrometheusMeterProvider(res *resource.Resource) (*metric.MeterProvider, error) {
	registry := prom.NewRegistry()
	prometheusRegistry = registry

	exporter, err := prometheus.New(
		prometheus.WithRegisterer(registry),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create prometheus exporter: %w", err)
	}

	provider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(exporter),
	)

	return provider, nil
}

func newOTLPMeterProvider(res *resource.Resource) (*metric.MeterProvider, error) {
	ctx := context.Background()

	reader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric reader: %w", err)
	}

	provider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(reader),
	)

	return provider, nil
}

func newMockMeterProvider(res *resource.Resource) (*metric.MeterProvider, error) {
	provider := metric.NewMeterProvider(
		metric.WithResource(res),
	)

	return provider, nil
}
