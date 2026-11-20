package building

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
	r.GET("/buildings/suggest", h.suggest)
}

func (h *Handler) suggest(c *gin.Context) {
	cityCode := strings.TrimSpace(c.Query("cityCode"))
	keyword := strings.TrimSpace(c.Query("keyword"))
	items, err := h.svc.Suggest(c.Request.Context(), cityCode, keyword)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		return
	}
	httpx.OK(c, gin.H{"items": items})
}
