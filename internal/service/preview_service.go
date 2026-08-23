package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PreviewType 预览类型
type PreviewType string

const (
	PreviewTypeImage  PreviewType = "image"
	PreviewTypeVideo  PreviewType = "video"
	PreviewTypeAudio  PreviewType = "audio"
	PreviewTypePDF    PreviewType = "pdf"
	PreviewTypeText   PreviewType = "text"
	PreviewTypeBinary PreviewType = "binary"
	PreviewTypeNone   PreviewType = "none"
)

// PreviewInfo 预览所需信息
type PreviewInfo struct {
	Type     PreviewType `json:"type"`
	MimeType string      `json:"mime_type"`
	// Text 用于文本预览返回内容
	Text string `json:"text,omitempty"`
	// ThumbnailBase64 缩略图（仅图片），前端按需实现
	ThumbnailBase64 string `json:"thumbnail,omitempty"`
	// Size 文件大小
	Size int64 `json:"size"`
	// MaxTextBytes 文本预览的最大字节数
	MaxTextBytes int `json:"max_text_bytes,omitempty"`
}

// maxTextPreviewBytes 文本预览最大字节数
const maxTextPreviewBytes = 256 * 1024

// PreviewService 负责根据文件扩展名判定预览类型，并为文本生成预览内容。
// 它不直接发送文件，仅返回元信息，由 handler 负责 IO。
type PreviewService struct {
	// AllowedTextExt 允许作为文本预览的扩展名集合（小写，不含点）
	AllowedTextExt map[string]bool
}

// NewPreviewService 构造预览服务
func NewPreviewService() *PreviewService {
	return &PreviewService{AllowedTextExt: defaultTextExts()}
}

// defaultTextExt 默认支持文本预览的扩展名
func defaultTextExts() map[string]bool {
	exts := []string{
		"txt", "md", "log", "ini", "conf", "cfg", "yaml", "yml", "json", "xml", "csv", "tsv",
		"go", "py", "js", "ts", "tsx", "jsx", "java", "c", "h", "cpp", "hpp", "cc", "rs",
		"sh", "bash", "zsh", "rb", "php", "pl", "sql", "html", "htm", "css", "scss", "less",
		"toml", "makefile", "dockerfile", "gradle", "kt", "swift", "lua", "vim",
	}
	m := make(map[string]bool, len(exts))
	for _, e := range exts {
		m[e] = true
	}
	return m
}

// Inspect 根据文件路径生成预览信息。
func (s *PreviewService) Inspect(path string) (PreviewInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return PreviewInfo{}, fmt.Errorf("stat: %w", err)
	}
	if info.IsDir() {
		return PreviewInfo{Type: PreviewTypeNone, Size: 0}, nil
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	mime := guessMime(ext)
	pi := PreviewInfo{
		Type:     PreviewTypeBinary,
		MimeType: mime,
		Size:     info.Size(),
	}
	switch {
	case isImageExt(ext):
		pi.Type = PreviewTypeImage
	case isVideoExt(ext):
		pi.Type = PreviewTypeVideo
	case isAudioExt(ext):
		pi.Type = PreviewTypeAudio
	case ext == "pdf":
		pi.Type = PreviewTypePDF
	case s.AllowedTextExt[ext] || ext == "":
		pi.Type = PreviewTypeText
		pi.MaxTextBytes = maxTextPreviewBytes
	}
	return pi, nil
}

// ReadTextContent 读取文本内容用于预览，最多读取 MaxTextBytes 字节。
func (s *PreviewService) ReadTextContent(path string, max int) (string, error) {
	if max <= 0 {
		max = maxTextPreviewBytes
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, max)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	return string(buf[:n]), nil
}

// isImageExt 是否为常见图片扩展名
func isImageExt(ext string) bool {
	switch ext {
	case "jpg", "jpeg", "png", "gif", "bmp", "webp", "svg", "ico", "tiff", "tif":
		return true
	}
	return false
}

// isVideoExt 是否为常见视频扩展名
func isVideoExt(ext string) bool {
	switch ext {
	case "mp4", "mkv", "mov", "avi", "webm", "flv", "wmv", "m4v", "mpg", "mpeg", "ts":
		return true
	}
	return false
}

// isAudioExt 是否为常见音频扩展名
func isAudioExt(ext string) bool {
	switch ext {
	case "mp3", "wav", "flac", "aac", "ogg", "m4a", "wma", "opus":
		return true
	}
	return false
}

// guessMime 简易 MIME 推断，避免引入额外依赖。
func guessMime(ext string) string {
	switch ext {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "mp4":
		return "video/mp4"
	case "webm":
		return "video/webm"
	case "mov":
		return "video/quicktime"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	case "pdf":
		return "application/pdf"
	case "html", "htm":
		return "text/html"
	case "css":
		return "text/css"
	case "js":
		return "application/javascript"
	case "json":
		return "application/json"
	case "xml":
		return "application/xml"
	case "txt", "md", "log":
		return "text/plain"
	}
	return "application/octet-stream"
}
