package logger

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/trace"
)

type logger struct {
	inner *zerolog.Logger
}

func newZerolog(cfg Cfg) (Logger, error) {
	var l zerolog.Level
	out := io.Writer(os.Stderr)
	if cfg.Bool("observability.log.pretty") {
		out = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.DateTime,
			NoColor:    false,
		}
	}

	switch strings.ToLower(cfg.String("observability.log.level")) {
	case "error":
		l = zerolog.ErrorLevel
	case "warn":
		l = zerolog.WarnLevel
	case "info":
		l = zerolog.InfoLevel
	case "debug":
		l = zerolog.DebugLevel
	default:
		l = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(l)

	log.Logger = zerolog.New(out).
		With().
		Str("service", cfg.String("app.name")).
		Timestamp().
		CallerWithSkipFrameCount(4).
		Logger()

	return &logger{
		inner: &log.Logger,
	}, nil
}

func (l *logger) Debug(message string, args ...any) {
	l.log(zerolog.DebugLevel, message, args...)
}

func (l *logger) Info(message string, args ...any) {
	l.log(zerolog.InfoLevel, message, args...)
}

func (l *logger) Warn(message string, args ...any) {
	l.log(zerolog.WarnLevel, message, args...)
}

func (l *logger) Error(message string, args ...any) {
	l.log(zerolog.ErrorLevel, message, args...)
}

func (l *logger) Fatal(message string, args ...any) {
	l.log(zerolog.FatalLevel, message, args...)

	os.Exit(1)
}

func (l *logger) With(args ...any) Logger {
	newLogger := applyPairs(l.inner.With(), args).Logger()

	return &logger{inner: &newLogger}
}

func (l *logger) WithCtx(ctx context.Context, args ...any) Logger {
	builder := applyPairs(l.inner.With(), args)

	if span := trace.SpanFromContext(ctx); span != nil {
		spanCtx := span.SpanContext()
		if spanCtx.IsValid() {
			builder = builder.
				Str("trace_id", spanCtx.TraceID().String()).
				Str("span_id", spanCtx.SpanID().String())
		}
	}

	newLogger := builder.Logger()

	return &logger{inner: &newLogger}
}

func (l *logger) log(level zerolog.Level, message string, args ...any) {
	if len(args) == 0 {
		l.inner.WithLevel(level).Msg(message)

		return
	}

	applyPairs(l.inner.WithLevel(level), args).Msg(message)
}

// kvSetter is satisfied by both zerolog.Context and *zerolog.Event, letting
// applyPairs build fields on whichever one a caller is assembling.
type kvSetter[T any] interface {
	Str(key, val string) T
	Int(key string, i int) T
	Err(err error) T
	Interface(key string, i any) T
}

func applyPairs[T kvSetter[T]](b T, args []any) T {
	pairsCount := len(args) / 2
	for i := range pairsCount {
		idx := i * 2
		var key string
		if k, ok := args[idx].(string); ok {
			key = k
		} else {
			key = fmt.Sprintf("arg%d", idx)
		}

		value := args[idx+1]

		switch v := value.(type) {
		case string:
			b = b.Str(key, v)
		case int:
			b = b.Int(key, v)
		case error:
			b = b.Err(v)
		default:
			b = b.Interface(key, v)
		}
	}

	if len(args)%2 != 0 {
		lastIdx := len(args) - 1
		key := fmt.Sprintf("arg%d", lastIdx)
		b = b.Interface(key, args[lastIdx])
	}

	return b
}
