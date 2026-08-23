package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"lan-share/internal/model"
	"lan-share/internal/service"
)

// DeviceHandler 负责设备发现相关接口的 HTTP 处理。
type DeviceHandler struct {
	svc *service.DiscoverService
}

// NewDeviceHandler 构造设备处理器
func NewDeviceHandler(svc *service.DiscoverService) *DeviceHandler {
	return &DeviceHandler{svc: svc}
}

// List GET /api/v1/devices
// 返回当前已发现的所有设备。
func (h *DeviceHandler) List(c *gin.Context) {
	list, err := h.svc.ListDevices(c.Request.Context())
	if err != nil {
		failErr(c, err)
		return
	}
	if list == nil {
		list = []model.Device{}
	}
	ok(c, gin.H{
		"devices": list,
		"total":   len(list),
	})
}

// Discover POST /api/v1/devices/discover
// 主动触发一次发现广播。
func (h *DeviceHandler) Discover(c *gin.Context) {
	if err := h.svc.DiscoverNow(c.Request.Context()); err != nil {
		failErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "discovery triggered",
	})
}

// Get GET /api/v1/devices/:id
// 查询单个设备。
func (h *DeviceHandler) Get(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		fail(c, http.StatusBadRequest, "MISSING_ID", "device id is required")
		return
	}
	dev, err := h.svc.GetDevice(c.Request.Context(), id)
	if err != nil {
		failErr(c, err)
		return
	}
	if dev == nil {
		failErr(c, errNotFound)
		return
	}
	ok(c, dev)
}
