package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"lan-share/config"
)

// SettingsHandler 负责本机设置接口的 HTTP 处理。
type SettingsHandler struct {
	cfg *config.Config
}

// NewSettingsHandler 构造设置处理器
func NewSettingsHandler(cfg *config.Config) *SettingsHandler {
	return &SettingsHandler{cfg: cfg}
}

// Get GET /api/v1/settings
func (h *SettingsHandler) Get(c *gin.Context) {
	ok(c, gin.H{
		"device": gin.H{
			"name":         h.cfg.Device.Name,
			"auto_receive": h.cfg.Device.AutoReceive,
		},
		"server": gin.H{
			"host":          h.cfg.Server.Host,
			"port":          h.cfg.Server.Port,
			"max_upload_mb": h.cfg.Server.MaxUploadMB,
		},
		"network": gin.H{
			"udp_discover_port": h.cfg.Network.UDPDiscoverPort,
			"file_server_port":  h.cfg.Network.FileServerPort,
			"discover_interval": h.cfg.Network.DiscoverInterval,
			"device_ttl":        h.cfg.Network.DeviceTTL,
		},
		"storage": gin.H{
			"data_dir":     h.cfg.Storage.DataDir,
			"share_dir":    h.cfg.Storage.ShareDir,
			"download_dir": h.cfg.Storage.DownloadDir,
		},
	})
}

type updateSettingsInput struct {
	DeviceName  *string `json:"device_name"`
	AutoReceive *bool   `json:"auto_receive"`
}

// Update PATCH /api/v1/settings
// 当前仅支持运行期修改设备名与自动接收开关。
func (h *SettingsHandler) Update(c *gin.Context) {
	var in updateSettingsInput
	if !bindJSON(c, &in) {
		return
	}
	if in.DeviceName != nil && *in.DeviceName != "" {
		config.SetDeviceName(*in.DeviceName)
	}
	if in.AutoReceive != nil {
		config.SetAutoReceive(*in.AutoReceive)
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "updated"})
}

// Me GET /api/v1/me
// 返回本机设备简表，前端用于显示自身身份。
func (h *SettingsHandler) Me(c *gin.Context) {
	ok(c, gin.H{
		"name": h.cfg.Device.Name,
		"ip":   "",
		"port": h.cfg.Network.FileServerPort,
	})
}
