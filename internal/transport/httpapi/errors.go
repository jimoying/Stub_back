package httpapi

import "github.com/gin-gonic/gin"

type apiErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

type apiErrorResponse struct {
	Error apiErrorDetail `json:"error"`
}

func writeError(c *gin.Context, status int, code, message string) {
	// 1. 所有 API 错误使用统一信封，并关联当前请求追踪 ID。
	c.Set("errorCode", code)
	c.AbortWithStatusJSON(status, apiErrorResponse{
		Error: apiErrorDetail{Code: code, Message: message, RequestID: requestID(c)},
	})
}

func requestID(c *gin.Context) string {
	value, _ := c.Get("requestId")
	id, _ := value.(string)
	return id
}
