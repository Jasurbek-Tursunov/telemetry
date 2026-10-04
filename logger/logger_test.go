package logger

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	otellog "go.opentelemetry.io/otel/log"
)

const (
	secretMarker = "SECRET"
	passwordTail = "pass@"
	leakyLink    = "https://api.test/x?access_token=SECRETTOKEN&a=1"
)

type testCfg map[string]string

func (c testCfg) Bool(key string) bool { return c[key] == "true" }

func (c testCfg) String(key string) string { return c[key] }

func leakyError() error {
	return &url.Error{
		Op:  "Get",
		URL: "http://user:pass@market.test/money-send/SECRETPATH?key=SECRETKEY",
		Err: errors.New("dropped"),
	}
}

func captureOutput(t *testing.T, target **os.File, build func() Logger, emit func(Logger)) string {
	t.Helper()

	original := *target

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	*target = writer

	t.Cleanup(func() { *target = original })

	log := build()

	emit(log)

	*target = original

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}

	return string(output)
}

func assertNoSecrets(t *testing.T, output string) {
	t.Helper()

	if strings.Contains(output, secretMarker) || strings.Contains(output, passwordTail) {
		t.Fatalf("output still carries a secret: %s", output)
	}
}

func newLogger(t *testing.T, cfg testCfg) Logger {
	t.Helper()

	log, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	return log
}

func TestZerologMasksSecretsAndKeepsTheCaller(t *testing.T) {
	cfg := testCfg{
		"observability.log.enable":   "true",
		"observability.log.provider": "zerolog",
		"observability.log.level":    "debug",
		"observability.log.pretty":   "false",
		"app.name":                   "test",
	}

	var wantLine int

	output := captureOutput(t, &os.Stderr, func() Logger { return newLogger(t, cfg) }, func(log Logger) {
		_, _, wantLine, _ = runtime.Caller(0)
		log.Warn("request failed", "err", leakyError(), "link", leakyLink)
	})
	wantLine++

	assertNoSecrets(t, output)

	var record map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &record); err != nil {
		t.Fatalf("output %q is no JSON: %v", output, err)
	}

	caller, _ := record["caller"].(string)

	_, file, _, _ := runtime.Caller(0)
	wantSuffix := file[strings.LastIndex(file, "/")+1:] + ":" + strconv.Itoa(wantLine)

	if !strings.HasSuffix(caller, wantSuffix) {
		t.Fatalf("caller = %q, want suffix %q", caller, wantSuffix)
	}
}

func TestZerologMasksBoundArgs(t *testing.T) {
	cfg := testCfg{
		"observability.log.enable":   "true",
		"observability.log.provider": "zerolog",
		"observability.log.level":    "debug",
		"observability.log.pretty":   "false",
		"app.name":                   "test",
	}

	output := captureOutput(t, &os.Stderr, func() Logger { return newLogger(t, cfg) }, func(log Logger) {
		log.With("link", leakyLink, "err", leakyError()).Info("with")
		log.WithCtx(context.Background(), "link", leakyLink, "err", leakyError()).Info("with ctx")
		log.Info("odd", "link", leakyLink, leakyLink)
	})

	assertNoSecrets(t, output)

	lines := bufio.NewScanner(strings.NewReader(output))
	count := 0

	for lines.Scan() {
		count++
	}

	if count != 3 {
		t.Fatalf("got %d lines, want 3: %s", count, output)
	}
}

func TestSlogMasksSecrets(t *testing.T) {
	cfg := testCfg{
		"observability.log.enable":   "true",
		"observability.log.provider": "slog",
		"observability.log.env":      "prod",
	}

	output := captureOutput(t, &os.Stdout, func() Logger { return newLogger(t, cfg) }, func(log Logger) {
		log.Warn("request failed", "err", leakyError(), "link", leakyLink)
		log.With("link", leakyLink, "err", leakyError()).Warn("bound")
		log.WithCtx(context.Background(), "link", leakyLink).Warn("bound ctx")
	})

	assertNoSecrets(t, output)

	if !strings.Contains(output, "xxxxx") {
		t.Fatalf("output carries no mask: %s", output)
	}
}

func TestOtelAttrsFromArgsMaskSecrets(t *testing.T) {
	attrs := attrsFromArgs([]any{"err", leakyError(), "link", leakyLink, "count", 2})

	if len(attrs) != 3 {
		t.Fatalf("got %d attributes, want 3", len(attrs))
	}

	for _, attr := range attrs[:2] {
		text := attr.Value.AsString()
		if strings.Contains(text, secretMarker) || strings.Contains(text, passwordTail) {
			t.Fatalf("attribute %s = %q, still carries a secret", attr.Key, text)
		}
	}

	if attrs[2].Value.Kind() != otellog.KindInt64 {
		t.Fatalf("count kind = %v, want int64", attrs[2].Value.Kind())
	}
}
