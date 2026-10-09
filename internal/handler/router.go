// Package handler 负责 HTTP 层：解析请求、调用 service、组装响应。
// 业务逻辑不写在这里。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/limnova/lim-tools-server/internal/service"
)

// Handler 持有各 HTTP 处理器需要的依赖。
// 依赖用手动构造函数注入，见 New。
type Handler struct {
	infoSvc   *service.InfoService
	workbooks *service.WorkbookService
}

// New 构造 Handler。
func New(infoSvc *service.InfoService, workbooks *service.WorkbookService) *Handler {
	return &Handler{infoSvc: infoSvc, workbooks: workbooks}
}

// Register 把所有路由挂到给定的引擎上。
// 新增接口时在这里加一行，路由全貌一眼可见。
func (h *Handler) Register(r gin.IRouter) {
	r.GET("/healthz", h.health)

	v1 := r.Group("/api/v1")
	v1.GET("/info", h.info)
	if h.workbooks != nil {
		v1.GET("/workbooks", h.listWorkbooks)
		v1.POST("/workbooks", h.createWorkbook)
		v1.GET("/workbooks/:id", h.getWorkbook)
		v1.PUT("/workbooks/:id", h.updateWorkbook)
		v1.DELETE("/workbooks/:id", h.deleteWorkbook)
	}
}
