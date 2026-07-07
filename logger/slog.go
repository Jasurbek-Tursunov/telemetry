package logger

import (
	"context"
	"log/slog"
	"os"
)

type slogLogger struct {
	logger *slog.Logger
}

func newSlog(cfg Cfg) (Logger, error) {
	env := cfg.String("observability.log.env")

	var handler slog.Handler

	switch env {
	case "dev":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	case "prod":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	default:
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}

	return &slogLogger{
		logger: slog.New(handler),
	}, nil
}

func (l *slogLogger) Debug(message string, args ...any) {
	l.logger.Debug(message, args...)
}

func (l *slogLogger) Info(message string, args ...any) {
	l.logger.Info(message, args...)
}

func (l *slogLogger) Warn(message string, args ...any) {
	l.logger.Warn(message, args...)
}

func (l *slogLogger) Error(message string, args ...any) {
	l.logger.Error(message, args...)
}

func (l *slogLogger) Fatal(message string, args ...any) {
	l.logger.Error(message, args...)
	os.Exit(1)
}

func (l *slogLogger) With(args ...any) Logger {
	return &slogLogger{
		logger: l.logger.With(args...),
	}
}

func (l *slogLogger) WithCtx(ctx context.Context, args ...any) Logger {
	newLogger := l.logger.With(args...)

	if requestID := ctx.Value("request_id"); requestID != nil {
		newLogger = newLogger.With("request_id", requestID)
	}

	return &slogLogger{
		logger: newLogger,
	}
}
