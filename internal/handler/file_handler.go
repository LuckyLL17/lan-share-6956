package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"lan-share/internal/service"
)

// FileHandler 负责文件下载、缩略图、预览相关接口的 HTTP 处理。
// 它是 /files/* 路由与上传接口的入口。
type FileHandler struct {
	shareSvc   *service.ShareService
	previewSvc *service.PreviewService
}

// NewFileHandler 构造文件处理器
func NewFileHandler(shareSvc *service.ShareService, previewSvc *service.PreviewService) *FileHandler {
	return &FileHandler{shareSvc: shareSvc, previewSvc: previewSvc}
}

// Browse GET /files/:alias/*path
// 浏览/下载共享内文件。带 ?download=true 时强制下载，否则按类型预览。
func (h *FileHandler) Browse(c *gin.Context) {
	alias := c.Param("alias")
	rel := c.Param("path")
	// 去掉前导斜杠
	rel = strings.TrimPrefix(rel, "/")

	sh, full, err := h.shareSvc.ResolveFile(c.Request.Context(), alias, rel)
	if err != nil {
		failErr(c, err)
		return
	}
	info, err := os.Stat(full)
	if err != nil {
		if os.IsNotExist(err) {
			failErr(c, errNotFound)
			return
		}
		failErr(c, err)
		return
	}
	// 在响应头暴露共享权限，便于前端判断
	c.Header("X-Share-Permission", string(sh.Permission))
	if info.IsDir() {
		h.serveDirListing(c, alias, rel)
		return
	}

	forceDownload := c.Query("download") == "true"
	if forceDownload {
		c.FileAttachment(full, info.Name())
		return
	}

	// 预览判定
	pi, err := h.previewSvc.Inspect(full)
	if err != nil {
		failErr(c, err)
		return
	}
	switch pi.Type {
	case service.PreviewTypeText:
		h.serveTextPreview(c, full)
	case service.PreviewTypePDF:
		c.Header("Content-Type", "application/pdf")
		c.File(full)
	case service.PreviewTypeImage, service.PreviewTypeVideo, service.PreviewTypeAudio:
		c.Header("Content-Type", pi.MimeType)
		c.File(full)
	default:
		c.FileAttachment(full, info.Name())
	}
}

// serveDirListing 当目标是目录时，返回 JSON 列表。
func (h *FileHandler) serveDirListing(c *gin.Context, alias, rel string) {
	items, err := h.shareSvc.ListItems(c.Request.Context(), alias, rel)
	if err != nil {
		failErr(c, err)
		return
	}
	ok(c, gin.H{
		"alias": alias,
		"path":  rel,
		"items": items,
	})
}

// serveTextPreview 文本预览：返回 JSON，包含内容与摘要。
func (h *FileHandler) serveTextPreview(c *gin.Context, full string) {
	max := 256 * 1024
	if v := c.Query("max"); v != "" {
		if n, ok := parseInt(v); ok && n > 0 {
			max = n
		}
	}
	content, err := h.previewSvc.ReadTextContent(full, max)
	if err != nil {
		failErr(c, err)
		return
	}
	info, _ := os.Stat(full)
	ok(c, gin.H{
		"type":     service.PreviewTypeText,
		"content":  content,
		"truncated": info != nil && info.Size() > int64(len(content)),
		"size":     fileSize(info),
	})
}

// Thumbnail GET /files/:alias/*path?thumb=1
// 生成图片缩略图：直接返回原图，前端用 CSS 缩放（简化实现）。
func (h *FileHandler) Thumbnail(c *gin.Context) {
	alias := c.Param("alias")
	rel := strings.TrimPrefix(c.Param("path"), "/")
	_, full, err := h.shareSvc.ResolveFile(c.Request.Context(), alias, rel)
	if err != nil {
		failErr(c, err)
		return
	}
	pi, err := h.previewSvc.Inspect(full)
	if err != nil {
		failErr(c, err)
		return
	}
	if pi.Type != service.PreviewTypeImage {
		fail(c, http.StatusBadRequest, "NOT_IMAGE", "file is not an image")
		return
	}
	c.Header("Content-Type", pi.MimeType)
	c.File(full)
}

// Upload POST /api/v1/uploads/:alias
// 接收上传文件到指定共享目录（共享需为 readwrite 权限）。
// 支持 Range 风格的断点续传：header X-Start-Offset 指定追加写入位置。
func (h *FileHandler) Upload(c *gin.Context) {
	alias := c.Param("alias")
	sh, err := h.shareSvc.GetByAlias(c.Request.Context(), alias)
	if err != nil {
		failErr(c, err)
		return
	}
	if sh == nil {
		failErr(c, errNotFound)
		return
	}
	if !sh.IsWritable() {
		failErr(c, errForbidden)
		return
	}

	rel := c.PostForm("path")
	rel = strings.TrimPrefix(rel, "/")
	// 防穿越 + 符号链接逃逸：父目录必须已在共享根内
	full, ok := h.shareSvc.ResolveUploadPath(sh, rel)
	if !ok {
		fail(c, http.StatusBadRequest, "INVALID_PATH", "invalid upload path")
		return
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		failErr(c, err)
		return
	}

	// 断点续传起始偏移
	start := int64(0)
	if s := c.GetHeader("X-Start-Offset"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil && v >= 0 {
			start = v
		}
	}

	flag := os.O_CREATE | os.O_WRONLY
	if start > 0 {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(full, flag, 0o644)
	if err != nil {
		failErr(c, err)
		return
	}
	defer f.Close()

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		fail(c, http.StatusBadRequest, "NO_FILE", "missing file in form: "+err.Error())
		return
	}
	defer file.Close()

	n, err := io.Copy(f, file)
	if err != nil {
		failErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "uploaded",
		"data": gin.H{
			"path":   rel,
			"start":  start,
			"bytes":  n,
		},
	})
}

// fileSize 安全获取文件大小
func fileSize(info os.FileInfo) int64 {
	if info == nil {
		return 0
	}
	return info.Size()
}

// Ping GET /api/v1/ping
// 简单心跳接口，供其他设备探测本机文件服务是否在线。
func (h *FileHandler) Ping(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "pong",
		"data":    gin.H{"name": "lan-share", "version": "1.0.0"},
	})
}

// ShareList GET /api/v1/remote/shares
// 对外暴露本机已开启的共享列表（供其他设备浏览）。
func (h *FileHandler) ShareList(c *gin.Context) {
	list, err := h.shareSvc.List(c.Request.Context(), true)
	if err != nil {
		failErr(c, err)
		return
	}
	// 不暴露真实路径
	out := make([]map[string]interface{}, 0, len(list))
	for _, s := range list {
		out = append(out, map[string]interface{}{
			"id":         s.ID,
			"alias":      s.Alias,
			"permission": s.Permission,
			"writable":   s.IsWritable(),
			"created_at": s.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"shares": out, "total": len(out)}})
}

// RemoteListItems GET /api/v1/remote/shares/:alias/items?path=xxx
// 对外提供共享内目录浏览。
func (h *FileHandler) RemoteListItems(c *gin.Context) {
	alias := c.Param("alias")
	rel := c.Query("path")
	items, err := h.shareSvc.ListItems(c.Request.Context(), alias, rel)
	if err != nil {
		failErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{"alias": alias, "path": rel, "items": items, "total": len(items)},
	})
}

// RemoteFileURL 构造远端文件下载 URL（供前端拼装）。
func RemoteFileURL(ip string, port int, alias, rel string) string {
	return fmt.Sprintf("http://%s:%d/files/%s/%s", ip, port, alias, strings.TrimPrefix(rel, "/"))
}
