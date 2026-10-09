// Package middleware 存放 gin 中间件。
package middleware

import (
	"crypto/rand"

	"github.com/gin-gonic/gin"
)

const (
	// RequestIDHeader 是承载请求 ID 的 HTTP 头。
	// 上游网关（nginx / CDN）一般会带，带了就沿用，这样跨服务能串起来。
	RequestIDHeader = "X-Request-ID"

	// RequestIDKey 是请求 ID 在 gin.Context 里的键。
	RequestIDKey = "request_id"
)

// RequestID 为每个请求确定一个 ID：上游带了就沿用，没带就生成，
// 并回写到响应头，方便端到端排查。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			// crypto/rand.Text 是 Go 1.24 起标准库提供的随机串，无需自己拼。
			id = rand.Text()
		}

		c.Set(RequestIDKey, id)
		c.Header(RequestIDHeader, id)
		c.Next()
	}
}
