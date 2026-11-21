package contact

import (
	"errors"
	"log"
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
	r.GET("/landlord/contact-settings", h.getSettings)
	r.PUT("/landlord/contact-settings", h.updateSettings)
}

func (h *Handler) getSettings(c *gin.Context) {
	userID := httpx.CurrentUserID(c)
	settings, err := h.svc.GetSettings(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrNotLandlord) {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, err.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "读取联系方式设置失败")
		return
	}
	httpx.OK(c, settings)
}

func (h *Handler) updateSettings(c *gin.Context) {
	userID := httpx.CurrentUserID(c)
	var in SettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	settings, err := h.svc.UpdateSettings(c.Request.Context(), userID, in)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotLandlord), errors.Is(err, ErrWechatRequired):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "保存联系方式设置失败")
		}
		return
	}
	httpx.OK(c, settings)
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
			log.Printf("contact request failed: %v", err)
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "联系请求失败")
		}
		return
	}
	httpx.Created(c, reveal)
}
