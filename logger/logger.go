package logger

import (
	"context"
	"fmt"
)

type Logger interface {
	Debug(message string, args ...any)
	Info(message string, args ...any)
	Warn(message string, args ...any)
	Error(message string, args ...any)
	Fatal(message string, args ...any)
	With(args ...any) Logger
	WithCtx(ctx context.Context, args ...any) Logger
}

type Cfg interface {
	Bool(key string) bool
	String(key string) string
}

func New(cfg Cfg) (Logger, error) {
	if !cfg.Bool("observability.log.enable") {
		return NewMockLogger(), nil
	}

	provider := cfg.String("observability.log.provider")
	switch provider {
	case "slog":
		return newSlog(cfg)
	case "zerolog":
		return newZerolog(cfg)
	case "otlp":
		return newOTELLog(cfg)
	case "mock":
		return NewMockLogger(), nil
	default:
		return nil, fmt.Errorf("unknown log provider: %s", provider)
	}
}
