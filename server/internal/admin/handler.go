package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"housing/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) RegisterPublic(r *gin.RouterGroup) {
	r.POST("/admin/auth/login", h.login)
}

func (h *Handler) RegisterAuth(r *gin.RouterGroup) {
	r.GET("/me", h.me)
	r.GET("/stats", h.stats)
	r.GET("/properties", h.properties)
	r.POST("/properties/:uuid/moderate", h.moderate)
	r.POST("/auth/totp/setup", h.setupTOTP)
	r.POST("/auth/totp/enable", h.enableTOTP)
	r.POST("/auth/totp/disable", h.disableTOTP)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTPCode string `json:"totpCode"`
}

func (h *Handler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	result, err := h.svc.Login(c.Request.Context(), req.Username, req.Password, req.TOTPCode)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeAdminCredentials, err.Error())
		case errors.Is(err, ErrDisabled):
			httpx.Fail(c, http.StatusForbidden, httpx.CodeAdminForbidden, err.Error())
		case errors.Is(err, ErrTOTPRequired):
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeAdminTOTPRequired, err.Error())
		case errors.Is(err, ErrTOTPInvalid):
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeAdminCredentials, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "登录失败")
		}
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) me(c *gin.Context) {
	id := httpx.CurrentAdminID(c)
	if id == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeAdminUnauthorized, "请先登录管理站")
		return
	}
	a, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeAdminUnauthorized, "登录态无效")
		return
	}
	httpx.OK(c, a.Public())
}

func (h *Handler) stats(c *gin.Context) {
	stats, err := h.svc.Stats(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "统计失败")
		return
	}
	httpx.OK(c, stats)
}

func (h *Handler) setupTOTP(c *gin.Context) {
	id := httpx.CurrentAdminID(c)
	if id == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeAdminUnauthorized, "请先登录")
		return
	}
	secret, url, err := h.svc.SetupTOTP(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "生成动态验证码失败")
		return
	}
	httpx.OK(c, gin.H{"secret": secret, "otpauthUrl": url})
}

type totpRequest struct {
	Code string `json:"code"`
}

func (h *Handler) enableTOTP(c *gin.Context) {
	id := httpx.CurrentAdminID(c)
	var req totpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	if err := h.svc.EnableTOTP(c.Request.Context(), id, req.Code); err != nil {
		if errors.Is(err, ErrTOTPInvalid) {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeAdminCredentials, "动态验证码不正确")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "启用 TOTP 失败")
		return
	}
	httpx.OK(c, gin.H{"enabled": true})
}

func (h *Handler) disableTOTP(c *gin.Context) {
	id := httpx.CurrentAdminID(c)
	var req totpRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	if err := h.svc.DisableTOTP(c.Request.Context(), id, req.Code); err != nil {
		if errors.Is(err, ErrTOTPInvalid) {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeAdminCredentials, "动态验证码不正确")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "关闭 TOTP 失败")
		return
	}
	httpx.OK(c, gin.H{"enabled": false})
}

type moderateRequest struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func (h *Handler) moderate(c *gin.Context) {
	role := httpx.CurrentAdminRole(c)
	if role != "super_admin" && role != "operations" && role != "reviewer" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeAdminForbidden, "没有审核权限")
		return
	}
	var req moderateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	status, err := h.svc.ModerateProperty(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"), req.Action, req.Reason)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodePropertyNotFound, "房源不存在")
			return
		}
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "审核操作失败")
		return
	}
	httpx.OK(c, gin.H{"status": status})
}

func (h *Handler) properties(c *gin.Context) {
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			offset = n
		}
	}
	items, err := h.svc.ListProperties(c.Request.Context(), c.Query("status"), limit, offset)
	if err != nil {
		if errors.Is(err, ErrInvalidStatus) {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "房源加载失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}
