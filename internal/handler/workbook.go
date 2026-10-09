package handler

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/limnova/lim-tools-server/internal/logging"
	"github.com/limnova/lim-tools-server/internal/service"
)

type workbookRequest struct {
	Name     string          `json:"name"`
	Revision int64           `json:"revision"`
	Snapshot json.RawMessage `json:"snapshot"`
}

func (h *Handler) listWorkbooks(c *gin.Context) {
	items, err := h.workbooks.List(c.Request.Context())
	if err != nil {
		h.workbookError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) createWorkbook(c *gin.Context) {
	var input workbookRequest
	if !decodeWorkbookRequest(c, &input) {
		return
	}
	book, err := h.workbooks.Create(c.Request.Context(), input.Name, input.Snapshot)
	if err != nil {
		h.workbookError(c, err)
		return
	}
	c.Header("Location", "/api/v1/workbooks/"+book.ID)
	c.JSON(http.StatusCreated, book)
}

func (h *Handler) getWorkbook(c *gin.Context) {
	book, err := h.workbooks.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.workbookError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, book)
}

func (h *Handler) updateWorkbook(c *gin.Context) {
	var input workbookRequest
	if !decodeWorkbookRequest(c, &input) {
		return
	}
	book, err := h.workbooks.Update(c.Request.Context(), c.Param("id"), input.Name, input.Revision, input.Snapshot)
	if err != nil {
		h.workbookError(c, err)
		return
	}
	c.JSON(http.StatusOK, book.WorkbookSummary)
}

func (h *Handler) deleteWorkbook(c *gin.Context) {
	revision, err := strconv.ParseInt(c.Query("revision"), 10, 64)
	if err != nil || revision < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_workbook", "message": "删除需要当前文件版本，请刷新列表后重试。"})
		return
	}
	if err := h.workbooks.Delete(c.Request.Context(), c.Param("id"), revision); err != nil {
		h.workbookError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func decodeWorkbookRequest(c *gin.Context, input *workbookRequest) bool {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"code": "unsupported_media_type", "message": "请使用 JSON 格式提交表格。"})
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxWorkbookBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	err = decoder.Decode(input)
	if err == nil {
		var trailing any
		if next := decoder.Decode(&trailing); !errors.Is(next, io.EOF) {
			err = next
			if err == nil {
				err = errors.New("unexpected trailing request data")
			}
		}
	}
	if err != nil {
		status, code, message := http.StatusBadRequest, "invalid_workbook", "表格数据格式不正确，请检查导入文件。"
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			status, code, message = http.StatusRequestEntityTooLarge, "workbook_too_large", "表格超过 10 MB，请减少数据后重试。"
		}
		c.JSON(status, gin.H{"code": code, "message": message})
		return false
	}
	return true
}

func (h *Handler) workbookError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "storage_error", "无法读取或保存表格，请稍后重试。"
	switch {
	case errors.Is(err, service.ErrWorkbookNotFound):
		status, code, message = http.StatusNotFound, "workbook_not_found", "表格不存在或已被删除。"
	case errors.Is(err, service.ErrWorkbookConflict):
		status, code, message = http.StatusConflict, "revision_conflict", "其他页面已更新这份表格。请备份当前内容，再重新打开最新版本。"
	case errors.Is(err, service.ErrInvalidWorkbook):
		status, code, message = http.StatusBadRequest, "invalid_workbook", "表格数据不符合要求：请检查名称、工作表和数据大小。"
	default:
		ctx := c.Request.Context()
		logging.FromContext(ctx).ErrorContext(ctx, "workbook storage failed", "error", err)
	}
	c.JSON(status, gin.H{"code": code, "message": message})
}
