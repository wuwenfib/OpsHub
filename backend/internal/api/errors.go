package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/fh/opshub/backend/internal/apperr"
)

// writeError 是所有 handler 统一的错误出口：业务错误映射成 4xx 并把原因返回给调用方；
// 其他错误一律 500，详情只写日志，不暴露给客户端。
func writeError(c *gin.Context, err error) {
	status, code := statusOf(err)
	if status == http.StatusInternalServerError {
		slog.Error("internal error", "method", c.Request.Method, "path", c.Request.URL.Path, "err", err)
		c.JSON(status, gin.H{"error": "internal server error", "code": code})
		return
	}
	c.JSON(status, gin.H{"error": err.Error(), "code": code})
}

func statusOf(err error) (int, string) {
	switch {
	case errors.Is(err, apperr.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, apperr.ErrValidation):
		return http.StatusBadRequest, "validation"
	case errors.Is(err, apperr.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, apperr.ErrInvalidTransition):
		return http.StatusConflict, "invalid_transition"
	default:
		return http.StatusInternalServerError, "internal"
	}
}

// bindJSON 解析请求体；失败时已写好 400 响应，调用方直接 return。
func bindJSON(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		writeError(c, fmt.Errorf("%w: 请求体不是合法的 JSON: %v", apperr.ErrValidation, err))
		return false
	}
	return true
}

// pathID 解析路径里的 :id；失败时已写好 400 响应。
func pathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(c, fmt.Errorf("%w: id %q 不合法", apperr.ErrValidation, c.Param("id")))
		return 0, false
	}
	return id, true
}
