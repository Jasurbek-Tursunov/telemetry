package metrics

import (
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/metric"
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

func New(cfg Cfg) (metric.Meter, error) {
	provider, err := newMeterProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create meter provider: %w", err)
	}

	return provider.Meter(cfg.String("app.name")), nil
}

func Handler() http.Handler {
	if handler := GetPrometheusHandler(); handler != nil {
		return handler
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
}
