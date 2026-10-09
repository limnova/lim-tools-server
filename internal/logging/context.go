package logging

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

// WithLogger 把带请求级属性的 logger 放进 context。
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// FromContext 取出请求级 logger；取不到时回落到全局默认，绝不返回 nil。
//
// 用 slog.InfoContext(ctx, ...) 而不是 slog.Info(...)，
// 是为了让日志跟随 context —— 接上 OpenTelemetry 后 trace_id / span_id 会自动注入。
func FromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}
