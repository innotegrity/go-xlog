package xlog

import (
	"context"
	"log/slog"
)

// loggerCtxKey is just a key for storing a logger in a context.
type loggerCtxKey struct{}

// AddToContext adds the given logger to the existing context and returns a new context.
func AddToContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey{}, logger)
}

// FromContext returns the [slog.Logger] object stored in the context.
//
// If no logger is stored in the context, the [slog.Default] logger is returned.
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerCtxKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
