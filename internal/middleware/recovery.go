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
// 不用 gin.Recovery()：它把堆栈写到 gin 自己的 writer，格式与我们的结构化日志不一致；
// 而且在 release 模式下它几乎不输出，panic 会被静默吞掉 —— 线上出了问题什么都看不到。
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		ctx := c.Request.Context()

		logging.FromContext(ctx).ErrorContext(ctx, "panic recovered",
			"panic", fmt.Sprint(recovered),
			"stack", string(debug.Stack()),
		)

		c.AbortWithStatus(http.StatusInternalServerError)
	})
}
