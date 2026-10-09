package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// health 供负载均衡和容器探针使用，不做依赖检查，只表明进程活着。
func (h *Handler) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// info 返回服务的自述信息。
func (h *Handler) info(c *gin.Context) {
	c.JSON(http.StatusOK, h.infoSvc.Info())
}
