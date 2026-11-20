package intake

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

func (h *Handler) RegisterAdmin(r *gin.RouterGroup) {
	r.POST("/rentals/intake/text", h.text)
}

func (h *Handler) text(c *gin.Context) {
	role := httpx.CurrentAdminRole(c)
	if role != "super_admin" && role != "operations" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeAdminForbidden, "没有录入权限")
		return
	}
	var in TextInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	result, err := h.svc.CreateFromText(c.Request.Context(), httpx.CurrentAdminID(c), in)
	if err != nil {
		switch {
		case errors.Is(err, ErrLandlordNotFound), errors.Is(err, ErrBuildingNotFound), errors.Is(err, ErrMissingFields):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "录入失败")
		}
		return
	}
	httpx.Created(c, result)
}
