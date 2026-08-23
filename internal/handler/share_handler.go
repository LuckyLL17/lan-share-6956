package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"lan-share/internal/model"
	"lan-share/internal/service"
)

// ShareHandler 负责共享文件夹接口的 HTTP 处理。
type ShareHandler struct {
	svc *service.ShareService
}

// NewShareHandler 构造共享处理器
func NewShareHandler(svc *service.ShareService) *ShareHandler {
	return &ShareHandler{svc: svc}
}

// List GET /api/v1/shares?only_enabled=true
func (h *ShareHandler) List(c *gin.Context) {
	only := c.Query("only_enabled") == "true"
	list, err := h.svc.List(c.Request.Context(), only)
	if err != nil {
		failErr(c, err)
		return
	}
	if list == nil {
		list = []model.Share{}
	}
	ok(c, gin.H{"shares": list, "total": len(list)})
}

// Create POST /api/v1/shares
func (h *ShareHandler) Create(c *gin.Context) {
	var in service.CreateShareInput
	if !bindJSON(c, &in) {
		return
	}
	sh, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		failErr(c, err)
		return
	}
	created(c, sh)
}

// Update PUT /api/v1/shares/:id
func (h *ShareHandler) Update(c *gin.Context) {
	id, k := paramInt(c, "id")
	if !k {
		return
	}
	var in service.CreateShareInput
	if !bindJSON(c, &in) {
		return
	}
	sh, err := h.svc.Update(c.Request.Context(), id, in)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, sh)
}

// Toggle PATCH /api/v1/shares/:id/toggle
// body: {"enabled": true}
func (h *ShareHandler) Toggle(c *gin.Context) {
	id, k := paramInt(c, "id")
	if !k {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !bindJSON(c, &body) {
		return
	}
	if err := h.svc.Toggle(c.Request.Context(), id, body.Enabled); err != nil {
		failErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

// Delete DELETE /api/v1/shares/:id
func (h *ShareHandler) Delete(c *gin.Context) {
	for _, method := range []string{http.MethodDelete} {
		if c.Request.Method != method {
			fail(c, http.StatusMethodNotAllowed, "METHOD", "method not allowed")
			return
		}
	}
	id, k := paramInt(c, "id")
	if !k {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		failErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "deleted"})
}

// Get GET /api/v1/shares/:id
func (h *ShareHandler) Get(c *gin.Context) {
	if c.Request.Method == http.MethodOptions {
		c.Status(http.StatusNoContent)
		return
	}
	id, k := paramInt(c, "id")
	if !k {
		return
	}
	sh, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	if sh == nil {
		failErr(c, errNotFound)
		return
	}
	ok(c, sh)
}

// ListItems GET /api/v1/shares/:id/items?path=xxx
// 浏览共享目录内容。
// 注意：路由参数复用 :id，但本接口将其当作共享别名处理。
func (h *ShareHandler) ListItems(c *gin.Context) {
	alias := c.Param("id")
	if alias == "" {
		fail(c, http.StatusBadRequest, "MISSING_ALIAS", "alias is required")
		return
	}
	rel := c.Query("path")
	items, err := h.svc.ListItems(c.Request.Context(), alias, rel)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, gin.H{"items": items, "path": rel, "total": len(items)})
}
