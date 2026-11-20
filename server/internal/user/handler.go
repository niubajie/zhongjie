package user

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"housing/internal/httpx"
	"housing/internal/wechat"
)

type Handler struct {
	svc      *Service
	wx       *wechat.Client
	devLogin bool
}

func NewHandler(svc *Service, wx *wechat.Client, devLogin bool) *Handler {
	return &Handler{svc: svc, wx: wx, devLogin: devLogin}
}

func (h *Handler) RegisterPublic(r *gin.RouterGroup) {
	r.POST("/auth/wechat/login", h.login)
}

func (h *Handler) RegisterAuth(r *gin.RouterGroup) {
	r.POST("/auth/wechat/phone", h.phone)
	r.GET("/me", h.me)
	r.POST("/profile", h.profile)
}

type loginRequest struct {
	Code      string `json:"code"`
	DevOpenID string `json:"devOpenid"`
}

func (h *Handler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}

	var openID string
	if h.devLogin && strings.TrimSpace(req.DevOpenID) != "" {
		openID = strings.TrimSpace(req.DevOpenID)
	} else {
		if strings.TrimSpace(req.Code) == "" {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "缺少微信登录 code")
			return
		}
		var err error
		openID, _, err = h.wx.Code2Session(c.Request.Context(), req.Code)
		if err != nil {
			httpx.Fail(c, http.StatusBadGateway, httpx.CodeInternal, "微信登录失败，请稍后重试")
			return
		}
	}

	result, err := h.svc.LoginByOpenID(c.Request.Context(), openID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "登录失败")
		return
	}
	httpx.OK(c, result)
}

type phoneRequest struct {
	Code     string `json:"code"`
	DevPhone string `json:"devPhone"`
}

func (h *Handler) phone(c *gin.Context) {
	userID := httpx.CurrentUserID(c)
	if userID == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthenticated, "请先登录")
		return
	}
	var req phoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}

	rawPhone := strings.TrimSpace(req.DevPhone)
	if !(h.devLogin && rawPhone != "") {
		if strings.TrimSpace(req.Code) == "" {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "缺少手机号授权 code")
			return
		}
		var err error
		rawPhone, err = h.wx.GetPhoneNumber(c.Request.Context(), req.Code)
		if err != nil {
			httpx.Fail(c, http.StatusBadGateway, httpx.CodeInternal, "获取手机号失败，请稍后重试")
			return
		}
	}

	u, err := h.svc.SetPhone(c.Request.Context(), userID, rawPhone)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidPhone):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		case errors.Is(err, ErrPhoneTaken):
			httpx.Fail(c, http.StatusConflict, httpx.CodeInvalidArgument, "该手机号已绑定其他账号")
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "保存手机号失败")
		}
		return
	}
	httpx.OK(c, u.Public())
}

func (h *Handler) me(c *gin.Context) {
	userID := httpx.CurrentUserID(c)
	if userID == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthenticated, "请先登录")
		return
	}
	u, err := h.svc.Get(c.Request.Context(), userID)
	if err != nil {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthenticated, "登录态无效")
		return
	}
	httpx.OK(c, u.Public())
}

type profileRequest struct {
	Surname string `json:"surname"`
	Role    string `json:"role"`
}

func (h *Handler) profile(c *gin.Context) {
	userID := httpx.CurrentUserID(c)
	if userID == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthenticated, "请先登录")
		return
	}
	var req profileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	u, err := h.svc.CompleteProfile(c.Request.Context(), userID, req.Surname, req.Role)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidSurname), errors.Is(err, ErrInvalidRole):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "保存资料失败")
		}
		return
	}
	httpx.OK(c, u.Public())
}
