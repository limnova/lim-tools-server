package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/limnova/lim-tools-server/internal/logging"
)

// Recovery 捕获 panic，用 slog 记录堆栈，并返回 500。
//
// 关闭 Gin 自带的文本日志通道，统一由请求级 slog 记录。
// 仍沿用 Gin 对连接断开和 http.ErrAbortHandler 的处理。
func Recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		ctx := c.Request.Context()

		logging.FromContext(ctx).ErrorContext(ctx, "panic recovered",
			"panic", fmt.Sprint(recovered),
			"stack", string(debug.Stack()),
		)

		c.AbortWithStatus(http.StatusInternalServerError)
	})
}
