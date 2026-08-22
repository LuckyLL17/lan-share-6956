// Package handler 实现各业务接口的 HTTP 处理器。
// 每个文件只负责一类资源，response.go 仅提供通用响应工具。
package handler

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// APIError 统一错误响应体
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ok 写入 200 成功响应
func ok(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    data,
	})
}

// created 写入 201 成功创建响应
func created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, gin.H{
		"code":    0,
		"message": "created",
		"data":    data,
	})
}

// fail 写入失败响应
func fail(c *gin.Context, status int, code, msg string) {
	c.JSON(status, APIError{Code: code, Message: msg})
}

// failErr 根据错误类型选择合适的 HTTP 状态码。
func failErr(c *gin.Context, err error) {
	if err == nil {
		fail(c, http.StatusInternalServerError, "UNKNOWN", "unknown error")
		return
	}
	var ce *codedError
	if errors.As(err, &ce) {
		fail(c, ce.HTTP, ce.Code, ce.Message)
		return
	}
	log.Printf("[handler] error: %v", err)
	fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
}

// codedError 带 HTTP 状态码的错误
type codedError struct {
	HTTP    int
	Code    string
	Message string
}

func (e *codedError) Error() string { return e.Message }

// NewCodedError 构造带状态码的错误
func NewCodedError(http int, code, msg string) error {
	return &codedError{HTTP: http, Code: code, Message: msg}
}

// 预定义错误
var (
	errBadRequest     = NewCodedError(http.StatusBadRequest, "BAD_REQUEST", "请求参数有误")
	errNotFound       = NewCodedError(http.StatusNotFound, "NOT_FOUND", "资源不存在")
	errForbidden      = NewCodedError(http.StatusForbidden, "FORBIDDEN", "无权限访问")
	errInternal       = NewCodedError(http.StatusInternalServerError, "INTERNAL", "服务内部错误")
)

// bindJSON 绑定 JSON 入参，失败统一返回 400。
func bindJSON(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		fail(c, http.StatusBadRequest, "BAD_BODY", err.Error())
		return false
	}
	return true
}

// bindQuery 绑定 Query 入参。
func bindQuery(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindQuery(obj); err != nil {
		fail(c, http.StatusBadRequest, "BAD_QUERY", err.Error())
		return false
	}
	return true
}

// paramInt 从路径参数解析 int。
func paramInt(c *gin.Context, name string) (int64, bool) {
	s := c.Param(name)
	if s == "" {
		fail(c, http.StatusBadRequest, "MISSING_PARAM", "missing path param "+name)
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "BAD_PARAM", "invalid int param "+name)
		return 0, false
	}
	return v, true
}

// parseInt 将字符串解析为 int，失败返回 0,false。
func parseInt(s string) (int, bool) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return v, true
}
