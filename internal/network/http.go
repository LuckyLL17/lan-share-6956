package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"lan-share/config"
)

// HTTPFileClient 负责与其他设备的 HTTP 文件交互：下载与上传。
type HTTPFileClient struct {
	cfg *config.Config
	hc  *http.Client
}

// NewHTTPFileClient 构造 HTTP 文件客户端。
func NewHTTPFileClient(cfg *config.Config) *HTTPFileClient {
	return &HTTPFileClient{
		cfg: cfg,
		hc: &http.Client{
			Timeout: 0,
			// 启用连接复用，提升多次小文件传输效率
			Transport: &http.Transport{
				MaxIdleConns:        8,
				MaxIdleConnsPerHost: 4,
				IdleConnTimeout:     60 * time.Second,
			},
		},
	}
}

// Download 向对端发起 GET 请求下载文件。
// start > 0 时附带 Range 头，支持断点续传。
func (c *HTTPFileClient) Download(ctx context.Context, ip string, port int, alias, rel string, start int64) (*http.Response, error) {
	if ip == "" {
		return nil, errors.New("peer ip empty")
	}
	if port == 0 {
		port = c.cfg.Network.FileServerPort
	}
	u := fmt.Sprintf("http://%s:%d/files/%s/%s?download=true", ip, port, url.PathEscape(alias), url.PathEscape(rel))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if start > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("download status %d: %s", resp.StatusCode, string(body))
	}
	return resp, nil
}

// ProgressFunc 上传进度回调，参数为已上传字节
type ProgressFunc func(written int64)

// Upload 向对端 POST 上传文件（multipart）。
// start > 0 时通过 X-Start-Offset 请求对端追加写入，支持断点续传。
func (c *HTTPFileClient) Upload(ctx context.Context, ip string, port int, alias, rel string, body io.Reader, start int64, onProgress ProgressFunc) (int64, error) {
	if ip == "" {
		return 0, errors.New("peer ip empty")
	}
	if port == 0 {
		port = c.cfg.Network.FileServerPort
	}
	u := fmt.Sprintf("http://%s:%d/api/v1/uploads/%s", ip, port, url.PathEscape(alias))

	// 使用 Pipe 流式构建 multipart，避免一次性占用大量内存
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer writer.Close()
		// 先写 path 字段
		if err := writer.WriteField("path", rel); err != nil {
			pw.CloseWithError(err)
			return
		}
		part, err := writer.CreateFormFile("file", rel)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		// 用计数 reader 跟踪已写字节
		pr := &countingReader{r: body, on: onProgress}
		if _, err := io.Copy(part, pr); err != nil {
			pw.CloseWithError(err)
			return
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, pr)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if start > 0 {
		req.Header.Set("X-Start-Offset", strconv.FormatInt(start, 10))
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("upload status %d: %s", resp.StatusCode, string(rb))
	}
	// 实际字节数由 countingReader 在 onProgress 回调中维护
	return 0, nil
}

// countingReader 包装 reader 并触发进度回调。
type countingReader struct {
	r  io.Reader
	n  int64
	on ProgressFunc
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.on != nil && n > 0 {
		c.on(c.n)
	}
	return n, err
}

// PingFileServer 探测对端文件服务是否在线。
func (c *HTTPFileClient) PingFileServer(ctx context.Context, ip string, port int) error {
	if ip == "" {
		return errors.New("peer ip empty")
	}
	if port == 0 {
		port = c.cfg.Network.FileServerPort
	}
	u := fmt.Sprintf("http://%s:%d/api/v1/ping", ip, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping status %d", resp.StatusCode)
	}
	return nil
}

// ListRemoteShares 拉取对端共享列表。
func (c *HTTPFileClient) ListRemoteShares(ctx context.Context, ip string, port int) ([]byte, error) {
	if ip == "" {
		return nil, errors.New("peer ip empty")
	}
	if port == 0 {
		port = c.cfg.Network.FileServerPort
	}
	u := fmt.Sprintf("http://%s:%d/api/v1/remote/shares", ip, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote shares status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// RemoteListItems 拉取对端共享内某目录条目。
func (c *HTTPFileClient) RemoteListItems(ctx context.Context, ip string, port int, alias, rel string) ([]byte, error) {
	if ip == "" {
		return nil, errors.New("peer ip empty")
	}
	if port == 0 {
		port = c.cfg.Network.FileServerPort
	}
	q := url.Values{}
	if rel != "" {
		q.Set("path", rel)
	}
	u := fmt.Sprintf("http://%s:%d/api/v1/remote/shares/%s/items?%s", ip, port, url.PathEscape(alias), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote items status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// EnsureCtx 提供一个非空上下文，避免误用 nil。
func EnsureCtx(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// discardBuffer 复用的临时缓冲，避免在 hot path 反复分配
var discardBuffer = &bytes.Buffer{}

// ResetBuf 重置复用缓冲
func ResetBuf() { discardBuffer.Reset() }
