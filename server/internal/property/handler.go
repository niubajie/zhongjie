package property

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

func (h *Handler) Register(r *gin.RouterGroup) {
	r.GET("/properties/map", h.mapHandler)
	r.GET("/properties/:uuid", h.detailHandler)
}

func (h *Handler) RegisterAuth(r *gin.RouterGroup) {
	r.POST("/landlord/properties", h.createHandler)
}

func (h *Handler) createHandler(c *gin.Context) {
	userID := httpx.CurrentUserID(c)
	if userID == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthenticated, "请先登录")
		return
	}
	var in CreateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	created, err := h.svc.Create(c.Request.Context(), userID, in)
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			httpx.Fail(c, http.StatusForbidden, httpx.CodeForbidden, "只有房东可以发布房源")
		case errors.Is(err, ErrBuildingNotFound):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "楼宇不存在或未通过审核")
		case errors.Is(err, ErrInvalidInput):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "发布房源失败")
		}
		return
	}
	httpx.Created(c, created)
}

func (h *Handler) mapHandler(c *gin.Context) {
	q := MapQuery{CityCode: strings.TrimSpace(c.Query("cityCode"))}
	var err error
	if q.SWLat, err = parseFloat(c.Query("swLat")); err != nil {
		invalidBounds(c)
		return
	}
	if q.SWLng, err = parseFloat(c.Query("swLng")); err != nil {
		invalidBounds(c)
		return
	}
	if q.NELat, err = parseFloat(c.Query("neLat")); err != nil {
		invalidBounds(c)
		return
	}
	if q.NELng, err = parseFloat(c.Query("neLng")); err != nil {
		invalidBounds(c)
		return
	}
	zoom := 13
	if raw := strings.TrimSpace(c.Query("zoom")); raw != "" {
		zoom, err = strconv.Atoi(raw)
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "zoom 不正确")
			return
		}
	}
	q.Zoom = zoom

	result, err := h.svc.Map(c.Request.Context(), q)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeMapBoundsInvalid, err.Error())
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) detailHandler(c *gin.Context) {
	detail, err := h.svc.Detail(c.Request.Context(), c.Param("uuid"))
	if errors.Is(err, ErrNotFound) {
		httpx.Fail(c, http.StatusNotFound, httpx.CodePropertyNotFound, "房源不存在或已下架")
		return
	}
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "房源编号不正确")
		return
	}
	httpx.OK(c, detail)
}

func parseFloat(raw string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(raw), 64)
}

func invalidBounds(c *gin.Context) {
	httpx.Fail(c, http.StatusBadRequest, httpx.CodeMapBoundsInvalid, "地图边界参数不正确")
}
