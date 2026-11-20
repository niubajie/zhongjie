package freshness

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

func (h *Handler) RegisterLandlord(r *gin.RouterGroup) {
	r.POST("/landlord/properties/:uuid/freshness", h.respond)
}

func (h *Handler) RegisterAdmin(r *gin.RouterGroup) {
	r.POST("/freshness/run", h.run)
}

func (h *Handler) respond(c *gin.Context) {
	userID := httpx.CurrentUserID(c)
	var in RespondInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	status, err := h.svc.Respond(c.Request.Context(), userID, c.Param("uuid"), in.Status)
	if err != nil {
		switch {
		case errors.Is(err, ErrNotFound):
			httpx.Fail(c, http.StatusNotFound, httpx.CodePropertyNotFound, "房源不存在")
		case errors.Is(err, ErrNotOwner):
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "不能操作他人房源")
		case errors.Is(err, ErrInvalidStatus):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新在租状态失败")
		}
		return
	}
	httpx.OK(c, gin.H{"status": status})
}

func (h *Handler) run(c *gin.Context) {
	role := httpx.CurrentAdminRole(c)
	if role != "super_admin" && role != "operations" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeAdminForbidden, "没有巡检权限")
		return
	}
	result, err := h.svc.Run(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "生成巡检任务失败")
		return
	}
	httpx.OK(c, result)
}
