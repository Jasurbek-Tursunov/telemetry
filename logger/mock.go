package logger

import "context"

type mockLogger struct{}

func NewMockLogger() Logger {
	return &mockLogger{}
}

func (l *mockLogger) Debug(message string, args ...any)               {}
func (l *mockLogger) Info(message string, args ...any)                {}
func (l *mockLogger) Warn(message string, args ...any)                {}
func (l *mockLogger) Error(message string, args ...any)               {}
func (l *mockLogger) Fatal(message string, args ...any)               {}
func (l *mockLogger) With(args ...any) Logger                         { return l }
func (l *mockLogger) WithCtx(ctx context.Context, args ...any) Logger { return l }
