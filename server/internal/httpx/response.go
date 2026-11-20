package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	CodeOK                = 0
	CodeInvalidArgument   = 10001
	CodeUnauthenticated   = 10002
	CodeForbidden         = 10003
	CodeRateLimited       = 10004
	CodeInternal          = 19999
	CodePhoneRequired     = 20001
	CodeProfileIncomplete = 20002
	CodePropertyNotFound  = 70001
	CodePropertyOffline   = 70002
	CodeMapBoundsInvalid  = 70003
	CodeAdminUnauthorized = 90001
	CodeAdminCredentials  = 90002
	CodeAdminTOTPRequired = 90003
	CodeAdminForbidden    = 90004
)

const TraceIDKey = "traceId"

type Envelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	TraceID string `json:"traceId,omitempty"`
}

func TraceID(c *gin.Context) string {
	if v, ok := c.Get(TraceIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Code: CodeOK, Message: "ok", Data: data, TraceID: TraceID(c)})
}

func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Envelope{Code: CodeOK, Message: "ok", Data: data, TraceID: TraceID(c)})
}

func Fail(c *gin.Context, status, code int, message string) {
	c.AbortWithStatusJSON(status, Envelope{Code: code, Message: message, TraceID: TraceID(c)})
}
