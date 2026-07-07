package logger

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/exporters/autoexport"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
)

type otelLogger struct {
	ctx   context.Context
	inner otellog.Logger
	attrs []otellog.KeyValue
}

func newOTELLog(cfg Cfg) (Logger, error) {
	ctx := context.Background()

	exporter, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP logger exporter: %w", err)
	}

	processor := log.NewBatchProcessor(exporter)

	provider := log.NewLoggerProvider(
		log.WithProcessor(processor),
	)

	global.SetLoggerProvider(provider)

	return &otelLogger{
		ctx:   ctx,
		inner: provider.Logger(cfg.String("app.name")),
	}, nil
}

func parseSeverity(level string) otellog.Severity {
	switch strings.ToLower(level) {
	case "debug":
		return otellog.SeverityDebug
	case "info":
		return otellog.SeverityInfo
	case "warn", "warning":
		return otellog.SeverityWarn
	case "error":
		return otellog.SeverityError
	default:
		return otellog.SeverityInfo
	}
}

func (l *otelLogger) Debug(message string, args ...any) {
	l.emit(otellog.SeverityDebug, message, args...)
}
func (l *otelLogger) Info(message string, args ...any) {
	l.emit(otellog.SeverityInfo, message, args...)
}
func (l *otelLogger) Warn(message string, args ...any) {
	l.emit(otellog.SeverityWarn, message, args...)
}
func (l *otelLogger) Error(message string, args ...any) {
	l.emit(otellog.SeverityError, message, args...)
}
func (l *otelLogger) Fatal(message string, args ...any) {
	l.emit(otellog.SeverityFatal, message, args...)
	os.Exit(1)
}

func (l *otelLogger) With(args ...any) Logger {
	newAttrs := append([]otellog.KeyValue{}, l.attrs...)
	newAttrs = append(newAttrs, attrsFromArgs(args)...)

	return &otelLogger{
		ctx:   l.ctx,
		inner: l.inner,
		attrs: newAttrs,
	}
}

func (l *otelLogger) WithCtx(ctx context.Context, args ...any) Logger {
	newAttrs := append([]otellog.KeyValue{}, l.attrs...)
	newAttrs = append(newAttrs, attrsFromArgs(args)...)

	return &otelLogger{
		ctx:   ctx,
		inner: l.inner,
		attrs: newAttrs,
	}
}

func toAttribute(key string, value any) otellog.KeyValue {
	switch v := value.(type) {
	case string:
		return otellog.String(key, v)
	case int:
		return otellog.Int(key, v)
	case int64:
		return otellog.Int64(key, v)
	case float64:
		return otellog.Float64(key, v)
	case bool:
		return otellog.Bool(key, v)
	default:
		return otellog.String(key, fmt.Sprint(v))
	}
}

func attrsFromArgs(args []any) []otellog.KeyValue {
	attrs := make([]otellog.KeyValue, 0, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		attrs = append(attrs, toAttribute(key, args[i+1]))
	}

	return attrs
}

func (l *otelLogger) emit(sev otellog.Severity, msg string, args ...any) {
	attrList := append([]otellog.KeyValue{}, l.attrs...)
	attrList = append(attrList, attrsFromArgs(args)...)

	var r otellog.Record

	r.SetTimestamp(time.Now())
	r.SetSeverity(sev)
	r.SetBody(otellog.StringValue(msg))
	r.AddAttributes(attrList...)

	l.inner.Emit(l.ctx, r)
}
