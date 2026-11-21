package community

import (
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
	r.GET("/communities/suggest", h.suggest)
}

func (h *Handler) RegisterAdmin(r *gin.RouterGroup) {
	r.POST("/communities", h.create)
}

func (h *Handler) suggest(c *gin.Context) {
	items, err := h.svc.Suggest(c.Request.Context(), c.Query("cityCode"), c.Query("keyword"))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) create(c *gin.Context) {
	var in CreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	created, err := h.svc.Create(c.Request.Context(), httpx.CurrentAdminID(c), in)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "duplicate") {
			status = http.StatusConflict
		}
		httpx.Fail(c, status, httpx.CodeInvalidArgument, err.Error())
		return
	}
	httpx.Created(c, created.Suggest())
}
