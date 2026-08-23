// Package middleware 提供通用 HTTP 中间件。
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS 允许跨域，便于前端在不同端口调试。
// 同时允许自定义请求头 X-Start-Offset，用于断点续传。
func CORS() gin.HandlerFunc {
	allowHeaders := []string{
		"Content-Type", "Authorization", "X-Requested-With",
		"X-Start-Offset", "X-Share-Permission", "Accept",
	}
	exposeHeaders := []string{
		"Content-Length", "Content-Range", "X-Share-Permission",
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Methods",
			strings.Join([]string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}, ", "))
		c.Header("Access-Control-Allow-Headers", strings.Join(allowHeaders, ", "))
		c.Header("Access-Control-Expose-Headers", strings.Join(exposeHeaders, ", "))
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// Logger 简易请求日志中间件。
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		method := c.Request.Method
		c.Next()
		// 只记录非 2xx 与非静态资源
		if c.Writer.Status() >= 400 || strings.HasPrefix(path, "/api/") {
			// 这里可替换为结构化日志
		}
		_ = method
	}
}

// Recovery 兜底 panic，避免单请求崩溃整个进程。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "internal server error",
				})
			}
		}()
		c.Next()
	}
}
