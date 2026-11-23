package notify

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"housing/internal/httpx"
)

// Handler 只做参数解析、调用 service、写响应。
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterAdmin 挂在 /v1/admin 组下，供管理站查看外发记录并手动重试。
func (h *Handler) RegisterAdmin(r *gin.RouterGroup) {
	r.GET("/prospects/notification-outbox", h.listOutbox)
	r.POST("/prospects/notification-outbox/:id/retry", h.retryOutbox)
}

func (h *Handler) allow(c *gin.Context) bool {
	role := httpx.CurrentAdminRole(c)
	if role != "super_admin" && role != "operations" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeAdminForbidden, "没有自主获客权限")
		return false
	}
	return true
}

func (h *Handler) listOutbox(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	result, err := h.svc.ListOutbox(c.Request.Context(), c.Query("status"), limit)
	if err != nil {
		if errors.Is(err, ErrInvalidOutboxQuery) {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "加载外发记录失败")
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) retryOutbox(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "外发记录 ID 不合法")
		return
	}
	if err := h.svc.RetryOutbox(c.Request.Context(), httpx.CurrentAdminID(c), id); err != nil {
		switch {
		case errors.Is(err, ErrOutboxNotFound):
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
		case errors.Is(err, ErrOutboxNotRetryable):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重试外发记录失败")
		}
		return
	}
	httpx.OK(c, gin.H{"retried": true})
}
