// Package api 负责路由注册与各层依赖装配。
package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"lan-share/internal/handler"
	"lan-share/internal/middleware"
)

// Router 依赖容器，聚合所有 handler。
type Router struct {
	Device    *handler.DeviceHandler
	Share     *handler.ShareHandler
	Transfer  *handler.TransferHandler
	File      *handler.FileHandler
	Message   *handler.MessageHandler
	Settings  *handler.SettingsHandler
	WebDir    string // 静态前端目录
}

// NewRouter 构造路由容器。
func NewRouter(
	dev *handler.DeviceHandler,
	share *handler.ShareHandler,
	transfer *handler.TransferHandler,
	file *handler.FileHandler,
	msg *handler.MessageHandler,
	settings *handler.SettingsHandler,
	webDir string,
) *Router {
	return &Router{
		Device:   dev,
		Share:    share,
		Transfer: transfer,
		File:     file,
		Message:  msg,
		Settings: settings,
		WebDir:   webDir,
	}
}

// Setup 配置 gin 引擎与路由表。
func (r *Router) Setup() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.Use(middleware.Recovery())
	e.Use(middleware.CORS())
	e.Use(middleware.Logger())

	e.GET("/api/v1/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"status": "up"}})
	})
	e.GET("/api/v1/ping", r.File.Ping)

	v1 := e.Group("/api/v1")
	{
		// 设备
		dev := v1.Group("/devices")
		dev.GET("", r.Device.List)
		dev.POST("/discover", r.Device.Discover)
		dev.GET("/:id", r.Device.Get)

		// 共享
		shares := v1.Group("/shares")
		shares.GET("", r.Share.List)
		shares.POST("", r.Share.Create)
		shares.GET("/:id", r.Share.Get)
		shares.PUT("/:id", r.Share.Update)
		shares.PATCH("/:id/toggle", r.Share.Toggle)
		shares.DELETE("/:id", r.Share.Delete)
		// 注意：gin 不允许同一层使用不同参数名，故 items 路由复用 :id
		// ListItems 会把该参数当作共享别名处理
		shares.GET("/:id/items", r.Share.ListItems)

		// 传输
		transfer := v1.Group("/transfers")
		transfer.GET("", r.Transfer.List)
		transfer.POST("", r.Transfer.Create)
		transfer.GET("/:id", r.Transfer.Get)
		transfer.GET("/:id/progress", r.Transfer.Progress)
		transfer.POST("/:id/start", r.Transfer.Start)
		transfer.POST("/:id/pause", r.Transfer.Pause)
		transfer.DELETE("/:id", r.Transfer.Cancel)
		transfer.DELETE("/history", r.Transfer.ClearHistory)

		// 消息
		msg := v1.Group("/messages")
		msg.GET("", r.Message.List)
		msg.POST("", r.Message.Create)
		msg.POST("/:id/read", r.Message.MarkRead)
		msg.POST("/read-all", r.Message.MarkAllRead)
		msg.GET("/unread", r.Message.UnreadCount)
		msg.DELETE("/:id", r.Message.Delete)

		// 设置
		settings := v1.Group("/settings")
		settings.GET("", r.Settings.Get)
		settings.PATCH("", r.Settings.Update)
		v1.GET("/me", r.Settings.Me)

		// 对外暴露给其他设备的资源接口
		remote := v1.Group("/remote")
		remote.GET("/shares", r.File.ShareList)
		remote.GET("/shares/:alias/items", r.File.RemoteListItems)

		// 上传
		v1.POST("/uploads/:alias", r.File.Upload)
	}

	// 文件浏览/下载（含缩略图）
	files := e.Group("/files")
	files.GET("/:alias/*path", r.File.Browse)

	// 静态前端
	r.serveWeb(e)

	return e
}

// serveWeb 注册静态前端文件，若 WebDir 不存在则跳过。
func (r *Router) serveWeb(e *gin.Engine) {
	if r.WebDir == "" {
		return
	}
	if info, err := os.Stat(r.WebDir); err != nil || !info.IsDir() {
		return
	}
	// 直接提供 index.html
	index := filepath.Join(r.WebDir, "index.html")
	if _, err := os.Stat(index); err == nil {
		e.StaticFile("/", index)
		e.StaticFile("/index.html", index)
	}
	// 其他静态资源
	e.Static("/static", r.WebDir)
	// SPA 兜底：未匹配的 GET 返回 index.html
	e.NoRoute(func(c *gin.Context) {
		if c.Request.Method == http.MethodGet {
			if _, err := os.Stat(index); err == nil {
				c.File(index)
				return
			}
		}
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "not found"})
	})
}
