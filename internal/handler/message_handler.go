package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"lan-share/internal/model"
	"lan-share/internal/repository"
	"lan-share/internal/service"
)

// MessageHandler 负责设备消息接口的 HTTP 处理。
type MessageHandler struct {
	msgRepo *repository.MessageRepository
	// svc 用于发送（通过 UDP 广播）消息
	svc *service.DiscoverService
}

// NewMessageHandler 构造消息处理器
func NewMessageHandler(msgRepo *repository.MessageRepository, svc *service.DiscoverService) *MessageHandler {
	return &MessageHandler{msgRepo: msgRepo, svc: svc}
}

// List GET /api/v1/messages?limit=100
func (h *MessageHandler) List(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		if n, ok := parseInt(l); ok && n > 0 {
			limit = n
		}
	}
	list, err := h.msgRepo.List(c.Request.Context(), limit)
	if err != nil {
		failErr(c, err)
		return
	}
	if list == nil {
		list = []model.Message{}
	}
	ok(c, gin.H{"messages": list, "total": len(list)})
}

// Create POST /api/v1/messages
// 通过 UDP 广播一条消息给局域网内其他设备。
type createMessageInput struct {
	ToIP    string `json:"to_ip"`
	Content string `json:"content"`
}

func (h *MessageHandler) Create(c *gin.Context) {
	var in createMessageInput
	if !bindJSON(c, &in) {
		return
	}
	if in.Content == "" {
		fail(c, http.StatusBadRequest, "EMPTY", "content is empty")
		return
	}
	// 由 discover 服务负责广播
	_ = h.svc.SendMessage(c.Request.Context(), in.ToIP, in.Content)
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "sent"})
}

// MarkRead POST /api/v1/messages/:id/read
func (h *MessageHandler) MarkRead(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		fail(c, http.StatusMethodNotAllowed, "METHOD", "method not allowed")
		return
	}
	id, k := paramInt(c, "id")
	if !k {
		return
	}
	if err := h.msgRepo.MarkRead(c.Request.Context(), id); err != nil {
		failErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "read"})
}

// MarkAllRead POST /api/v1/messages/read-all
func (h *MessageHandler) MarkAllRead(c *gin.Context) {
	if err := h.msgRepo.MarkAllRead(c.Request.Context()); err != nil {
		failErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

// UnreadCount GET /api/v1/messages/unread
func (h *MessageHandler) UnreadCount(c *gin.Context) {
	n, err := h.msgRepo.CountUnread(c.Request.Context())
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, gin.H{"unread": n})
}

// Delete DELETE /api/v1/messages/:id
func (h *MessageHandler) Delete(c *gin.Context) {
	switch c.Request.Method {
	case http.MethodDelete:
	default:
		fail(c, http.StatusMethodNotAllowed, "METHOD", "method not allowed")
		return
	}
	id, k := paramInt(c, "id")
	if !k {
		return
	}
	if err := h.msgRepo.Delete(c.Request.Context(), id); err != nil {
		failErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "deleted"})
}
