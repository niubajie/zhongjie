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
	r.GET("/prospects/review-queue", h.listReviewQueue)
	r.GET("/prospects/opt-outs", h.listOptOuts)
	r.GET("/prospects/reminders", h.listReminders)
	r.GET("/prospects/notifications", h.listNotifications)
	r.POST("/prospects/notifications/:id/read", h.markNotificationRead)
	r.GET("/prospects/stats", h.getStats)
	r.GET("/prospects/kill-switch", h.getKillSwitch)
	r.PUT("/prospects/kill-switch", h.setKillSwitch)
	r.GET("/prospects/:uuid", h.getLead)
	r.POST("/prospects/:uuid/review", h.reviewLead)
	r.POST("/prospects/:uuid/contact", h.contactLead)
	r.POST("/prospects/:uuid/follow-ups", h.addFollowUp)
	r.POST("/prospects/:uuid/status", h.transitionLead)
	r.POST("/prospects/:uuid/opt-out", h.optOutLead)
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
	result, err := h.svc.GetLeadDetail(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"))
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

// ---- 批次 5：待复核队列 ----

func (h *Handler) listReviewQueue(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	items, err := h.svc.ListReviewQueue(c.Request.Context(), c.Query("leadType"), limit)
	if err != nil {
		if errors.Is(err, ErrInvalidStatus) {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "加载待复核队列失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ---- 批次 5：复核 ----

func (h *Handler) reviewLead(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var in ReviewLeadInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	result, err := h.svc.ReviewLead(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"), in.Decision)
	if err != nil {
		switch {
		case errors.Is(err, ErrLeadNotFound):
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
		case errors.Is(err, ErrInvalidReview), errors.Is(err, ErrInvalidTransition):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "复核线索失败")
		}
		return
	}
	httpx.OK(c, result)
}

// ---- 批次 5：首条联系（100% 人工，禁止自动发送）----

func (h *Handler) contactLead(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var in ContactLeadInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	result, err := h.svc.ContactLead(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"), in)
	if err != nil {
		switch {
		case errors.Is(err, ErrLeadNotFound):
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
		case errors.Is(err, ErrHalted), errors.Is(err, ErrOptOutExists),
			errors.Is(err, ErrInvalidContact), errors.Is(err, ErrInvalidTransition):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "发起联系失败")
		}
		return
	}
	httpx.OK(c, result)
}

// ---- 批次 6：状态流转与拒收 ----

func (h *Handler) transitionLead(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var in TransitionLeadInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	result, err := h.svc.TransitionLead(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"), in.Status)
	if err != nil {
		switch {
		case errors.Is(err, ErrLeadNotFound):
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
		case errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrInvalidTransition):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "流转线索状态失败")
		}
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) optOutLead(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var in OptOutLeadInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	result, err := h.svc.OptOutLead(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"), in.Reason)
	if err != nil {
		switch {
		case errors.Is(err, ErrLeadNotFound):
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
		case errors.Is(err, ErrInvalidTransition):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "拒收线索失败")
		}
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) listOptOuts(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	items, err := h.svc.ListOptOuts(c.Request.Context(), c.Query("leadType"), limit)
	if err != nil {
		if errors.Is(err, ErrInvalidStatus) {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "加载拒收名单失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ---- 批次 8：后续跟进与提醒 ----

func (h *Handler) addFollowUp(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var in AddFollowUpInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	result, err := h.svc.AddFollowUp(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"), in)
	if err != nil {
		switch {
		case errors.Is(err, ErrLeadNotFound):
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
		case errors.Is(err, ErrInvalidFollowUp), errors.Is(err, ErrInvalidTransition):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "记录跟进失败")
		}
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) listReminders(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	items, err := h.svc.ListReminders(c.Request.Context(), limit)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "加载跟进提醒失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

// ---- 批次 11：站内通知 ----

func (h *Handler) listNotifications(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	unreadOnly := c.Query("unread") == "true"
	result, err := h.svc.ListNotifications(c.Request.Context(), httpx.CurrentAdminID(c), unreadOnly, limit)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "加载站内通知失败")
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) markNotificationRead(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "通知 ID 不合法")
		return
	}
	if err := h.svc.MarkNotificationRead(c.Request.Context(), httpx.CurrentAdminID(c), id); err != nil {
		if errors.Is(err, ErrNotificationNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "标记通知已读失败")
		return
	}
	httpx.OK(c, gin.H{"read": true})
}

// ---- 批次 10：获客效果统计 ----

func (h *Handler) getStats(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	stats, err := h.svc.GetStats(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "加载获客统计失败")
		return
	}
	httpx.OK(c, stats)
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
