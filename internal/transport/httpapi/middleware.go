package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func requestContext(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 为请求生成内部 ID；不信任调用方自带的 ID 作为权限信息。
		id := uuid.NewString()
		c.Set("requestId", id)
		c.Set("logger", logger)
		c.Header("X-Request-ID", id)

		// 2. 只记录方法、路由、状态和耗时，不记录密码、Token、查询参数或请求体。
		startedAt := time.Now()
		c.Next()
		fields := []any{
			"request_id", id,
			"method", c.Request.Method,
			"route", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"source_ip", c.ClientIP(),
			"user_agent", truncateLogValue(c.Request.UserAgent(), 1024),
		}
		if identity, ok := c.Get("authContext"); ok {
			if current, ok := identity.(authContext); ok {
				fields = append(fields, "actor_user_id", current.User.ID.String(), "actor_role", current.User.Role)
			}
		}
		if c.Writer.Status() >= http.StatusBadRequest {
			fields = append(fields, "result", "failure")
			if code, ok := c.Get("errorCode"); ok {
				fields = append(fields, "error_code", code)
			}
		} else {
			fields = append(fields, "result", "success")
		}
		logger.Info("http_request", fields...)
	}
}

func truncateLogValue(value string, maxLength int) string {
	if len(value) > maxLength {
		return value[:maxLength]
	}
	return value
}
