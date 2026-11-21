package assistant

import (
	"errors"
	"net/http"
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

func (h *Handler) Register(r *gin.RouterGroup) {
	r.POST("/assistant/sessions", h.createSession)
	r.POST("/assistant/messages", h.message)
	r.GET("/assistant/recommendations", h.recommendations)
	r.GET("/properties/:uuid/narration", h.narration)
	r.GET("/properties/:uuid/tour", h.tour)
	h.RegisterVoice(r)
}

func (h *Handler) createSession(c *gin.Context) {
	session, err := h.svc.CreateSession(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "创建会话失败")
		return
	}
	httpx.Created(c, session)
}

func (h *Handler) message(c *gin.Context) {
	var in MessageInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	if strings.TrimSpace(in.SessionID) == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "缺少 sessionId")
		return
	}
	reply, err := h.svc.Message(c.Request.Context(), in)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, "会话不存在或已过期")
			return
		}
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		return
	}
	httpx.OK(c, reply)
}

func (h *Handler) recommendations(c *gin.Context) {
	sessionID := strings.TrimSpace(c.Query("sessionId"))
	if sessionID == "" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "缺少 sessionId")
		return
	}
	result, err := h.svc.Recommendations(c.Request.Context(), sessionID)
	if err != nil {
		if errors.Is(err, ErrSessionNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, "会话不存在或已过期")
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "推荐失败")
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) narration(c *gin.Context) {
	result, err := h.svc.Narration(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodePropertyNotFound, "解说资源不存在")
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) tour(c *gin.Context) {
	result, err := h.svc.Tour(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, httpx.CodePropertyNotFound, "房源不存在")
		return
	}
	httpx.OK(c, result)
}
