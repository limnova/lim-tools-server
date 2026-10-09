package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/limnova/lim-tools-server/internal/logging"
)

// WithRequestLogger 把带 request_id 的子 logger 放进请求 context。
// 下游 handler 用 logging.FromContext(c.Request.Context()) 取，
// 之后每条日志都会自动带上同一个 request_id。
func WithRequestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestLogger := logger.With("request_id", c.GetString(RequestIDKey))

		ctx := logging.WithLogger(c.Request.Context(), requestLogger)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// AccessLog 记录每个请求的结果。
//
// 不记录客户端 IP：它属于个人数据，应用日志里不该留。
// 真要追攻击来源，反向代理那一层有更完整的访问日志。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		// 走到这里时 context 里已经有带 request_id 的 logger 了。
		ctx := c.Request.Context()
		logger := logging.FromContext(ctx)

		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		}
		// 路由模板（如 /api/v1/info）比原始 path 更适合做聚合维度，
		// 但 404 时匹配不到路由，此时 FullPath 为空。
		if route := c.FullPath(); route != "" {
			attrs = append(attrs, "route", route)
		}

		if len(c.Errors) > 0 {
			logger.ErrorContext(ctx, "request failed",
				append(attrs, "errors", c.Errors.String())...)
			return
		}

		logger.InfoContext(ctx, "request", attrs...)
	}
}
