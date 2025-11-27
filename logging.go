package agendadistribuida

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	loggerOnce sync.Once
	baseLogger *slog.Logger
	levelVar   = &slog.LevelVar{}
)

type ctxKeyRequestID struct{}

// Logger returns the singleton slog logger configured from environment variables.
func Logger() *slog.Logger {
	loggerOnce.Do(func() {
		levelVar.Set(determineLevel(os.Getenv("LOG_LEVEL")))
		handler := buildHandler(os.Getenv("LOG_FORMAT"), os.Getenv("LOG_DEST"))
		baseLogger = slog.New(handler).With("app", "distributed-agenda")
	})
	return baseLogger
}

func determineLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func buildHandler(format, dest string) slog.Handler {
	writer := selectWriter(dest)
	opts := &slog.HandlerOptions{Level: levelVar}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "text":
		return slog.NewTextHandler(writer, opts)
	default:
		return slog.NewJSONHandler(writer, opts)
	}
}

func selectWriter(dest string) io.Writer {
	switch strings.ToLower(strings.TrimSpace(dest)) {
	case "stderr":
		return os.Stderr
	case "stdout", "":
		return os.Stdout
	default:
		if strings.HasPrefix(dest, "file:") {
			path := strings.TrimPrefix(dest, "file:")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			if err == nil {
				return f
			}
			slog.Default().Warn("failed to open log file, falling back to stderr", "path", path, "err", err)
			return os.Stderr
		}
		return os.Stdout
	}
}

// WithRequestID ensures the context carries a request id and returns the updated context + id.
func WithRequestID(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if id, ok := ctx.Value(ctxKeyRequestID{}).(string); ok && id != "" {
		return ctx, id
	}
	id := newRequestID()
	ctx = context.WithValue(ctx, ctxKeyRequestID{}, id)
	return ctx, id
}

// RequestIDFromContext returns the request id stored in the context, if any.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(ctxKeyRequestID{}).(string); ok {
		return id
	}
	return ""
}

func newRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(buf)
}
