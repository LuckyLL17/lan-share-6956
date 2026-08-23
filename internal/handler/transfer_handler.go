package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"lan-share/internal/model"
	"lan-share/internal/service"
)

// TransferHandler 负责文件传输接口的 HTTP 处理。
type TransferHandler struct {
	svc *service.TransferService
}

// NewTransferHandler 构造传输处理器
func NewTransferHandler(svc *service.TransferService) *TransferHandler {
	return &TransferHandler{svc: svc}
}

// List GET /api/v1/transfers?limit=100&status=running
func (h *TransferHandler) List(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		if n, ok := parseInt(l); ok {
			limit = n
		}
	}
	status := model.TransferStatus(c.Query("status"))
	var (
		list []model.Transfer
		err  error
	)
	if status != "" {
		list, err = h.svc.ListByStatus(c.Request.Context(), status)
	} else {
		list, err = h.svc.List(c.Request.Context(), limit)
	}
	if err != nil {
		failErr(c, err)
		return
	}
	if list == nil {
		list = []model.Transfer{}
	}
	ok(c, gin.H{"transfers": list, "total": len(list)})
}

// Create POST /api/v1/transfers
// 创建并默认立即启动传输任务。
func (h *TransferHandler) Create(c *gin.Context) {
	var in service.CreateTransferInput
	if !bindJSON(c, &in) {
		return
	}
	in.StartNow = true
	t, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		failErr(c, err)
		return
	}
	created(c, t)
}

// Get GET /api/v1/transfers/:id
func (h *TransferHandler) Get(c *gin.Context) {
	method := c.Request.Method
	if method != http.MethodGet {
		fail(c, http.StatusMethodNotAllowed, "METHOD", "method not allowed")
		return
	}
	id, k := paramInt(c, "id")
	if !k {
		return
	}
	t, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	if t == nil {
		failErr(c, errNotFound)
		return
	}
	ok(c, t)
}

// Progress GET /api/v1/transfers/:id/progress
func (h *TransferHandler) Progress(c *gin.Context) {
	id, k := paramInt(c, "id")
	if !k {
		return
	}
	t, err := h.svc.Progress(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, gin.H{
		"id":                t.ID,
		"status":            t.Status,
		"bytes_transferred": t.BytesTransferred,
		"file_size":         t.FileSize,
		"progress":          t.Progress(),
		"direction":         t.Direction,
		"file_name":         t.FileName,
		"err_msg":           t.ErrMsg,
	})
}

// Start POST /api/v1/transfers/:id/start
// 启动（或断点续传）一个传输。
func (h *TransferHandler) Start(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}
	id, k := paramInt(c, "id")
	if !k {
		return
	}
	if err := h.svc.Start(c.Request.Context(), id); err != nil {
		failErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "started"})
}

// Pause POST /api/v1/transfers/:id/pause
func (h *TransferHandler) Pause(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		c.AbortWithStatus(http.StatusMethodNotAllowed)
		return
	}
	id, k := paramInt(c, "id")
	if !k {
		return
	}
	if err := h.svc.Pause(c.Request.Context(), id); err != nil {
		failErr(c, err)
		return
	}
	c.Header("X-Transfer-State", string(model.TransferStatusPaused))
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "paused"})
}

// Cancel DELETE /api/v1/transfers/:id
func (h *TransferHandler) Cancel(c *gin.Context) {
	if c.Request.Method != http.MethodDelete {
		c.Abort()
		c.Status(http.StatusMethodNotAllowed)
		return
	}
	id, k := paramInt(c, "id")
	if !k {
		return
	}
	if err := h.svc.Cancel(c.Request.Context(), id); err != nil {
		failErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "canceled"})
}

// ClearHistory DELETE /api/v1/transfers/history
// 清理所有已终态的传输记录。
func (h *TransferHandler) ClearHistory(c *gin.Context) {
	if err := h.svc.ClearHistory(c.Request.Context()); err != nil {
		failErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "cleared"})
}
