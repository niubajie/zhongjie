package contact

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"housing/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterAuth(r *gin.RouterGroup) {
	r.POST("/contact-requests", h.create)
}

func (h *Handler) create(c *gin.Context) {
	tenantID := httpx.CurrentUserID(c)
	if tenantID == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthenticated, "请先登录")
		return
	}
	var in CreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	reveal, err := h.svc.Create(c.Request.Context(), tenantID, in)
	if err != nil {
		switch {
		case errors.Is(err, ErrConsent):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		case errors.Is(err, ErrPhoneRequired):
			httpx.Fail(c, http.StatusForbidden, httpx.CodePhoneRequired, err.Error())
		case errors.Is(err, ErrPropertyNotFound):
			httpx.Fail(c, http.StatusNotFound, httpx.CodePropertyNotFound, err.Error())
		case errors.Is(err, ErrAlreadyContacted), errors.Is(err, ErrDailyLimit):
			httpx.Fail(c, http.StatusTooManyRequests, httpx.CodeRateLimited, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "联系请求失败")
		}
		return
	}
	httpx.Created(c, reveal)
}
