package prospect

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"housing/internal/httpx"
)

// Handler 只做参数解析、调用 service、写响应；不写业务判断、不直接访问数据库。
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterAdmin 挂在 /v1/admin 组下（由 httpapi 注入鉴权中间件）。
// 批次 1 先提供信息源与线索的只读 / 基础写入，复核、联系、跟进在后续批次补齐。
func (h *Handler) RegisterAdmin(r *gin.RouterGroup) {
	r.GET("/lead-sources", h.listSources)
	r.POST("/lead-sources", h.createSource)
	r.PATCH("/lead-sources/:uuid/enabled", h.setSourceEnabled)
	r.GET("/prospects", h.listLeads)
	r.GET("/prospects/kill-switch", h.getKillSwitch)
	r.PUT("/prospects/kill-switch", h.setKillSwitch)
	r.GET("/prospects/:uuid", h.getLead)
	r.PATCH("/lead-sources/:uuid", h.updateSource)
	r.DELETE("/lead-sources/:uuid", h.deleteSource)
}

func (h *Handler) allow(c *gin.Context) bool {
	role := httpx.CurrentAdminRole(c)
	if role != "super_admin" && role != "operations" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeAdminForbidden, "没有自主获客权限")
		return false
	}
	return true
}

func (h *Handler) listSources(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	items, err := h.svc.ListSources(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "加载信息源失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) createSource(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var in CreateSourceInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	result, err := h.svc.CreateSource(c.Request.Context(), httpx.CurrentAdminID(c), in)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		return
	}
	httpx.Created(c, result)
}

func (h *Handler) setSourceEnabled(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	if err := h.svc.SetSourceEnabled(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"), body.Enabled); err != nil {
		switch {
		case errors.Is(err, ErrSourceNotFound):
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新信息源失败")
		}
		return
	}
	httpx.OK(c, gin.H{"enabled": body.Enabled})
}

func (h *Handler) listLeads(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	items, err := h.svc.ListLeads(c.Request.Context(), c.Query("leadType"), c.Query("status"), limit)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidStatus):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "加载线索失败")
		}
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) getLead(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	result, err := h.svc.GetLead(c.Request.Context(), c.Param("uuid"))
	if err != nil {
		if errors.Is(err, ErrLeadNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "读取线索失败")
		return
	}
	httpx.OK(c, result)
}

// ---- 批次 2：全局停机开关 ----

func (h *Handler) getKillSwitch(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	httpx.OK(c, h.svc.GetKillSwitch())
}

func (h *Handler) setKillSwitch(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var body KillSwitchState
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	if err := h.svc.SetHalted(c.Request.Context(), httpx.CurrentAdminID(c), body.Halted); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "切换停机开关失败")
		return
	}
	httpx.OK(c, h.svc.GetKillSwitch())
}

// ---- 批次 2：信息源更新与删除 ----

func (h *Handler) updateSource(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var in UpdateSourceInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	if err := h.svc.UpdateSource(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"), in); err != nil {
		switch {
		case errors.Is(err, ErrSourceNotFound):
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
		case errors.Is(err, ErrDuplicateSource), errors.Is(err, ErrInvalidSourceInput):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "更新信息源失败")
		}
		return
	}
	httpx.OK(c, gin.H{"updated": true})
}

func (h *Handler) deleteSource(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	if err := h.svc.DeleteSource(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid")); err != nil {
		if errors.Is(err, ErrSourceNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "删除信息源失败")
		return
	}
	httpx.OK(c, gin.H{"deleted": true})
}
