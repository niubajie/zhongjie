package intake

import (
	"errors"
	"io"
	"net/http"
	"strconv"

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
	r.POST("/rentals/intake/media", h.media)
	r.GET("/rentals/intake", h.list)
	r.GET("/rentals/intake/:uuid", h.detail)
	r.POST("/rentals/intake/:uuid/match", h.match)
	r.POST("/rentals/intake/:uuid/retry", h.retry)
	r.POST("/rentals/intake/:uuid/publish", h.publish)
}

func (h *Handler) allow(c *gin.Context) bool {
	role := httpx.CurrentAdminRole(c)
	if role != "super_admin" && role != "operations" {
		httpx.Fail(c, http.StatusForbidden, httpx.CodeAdminForbidden, "没有录入权限")
		return false
	}
	return true
}

func (h *Handler) media(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 201<<20)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请选择图片或视频文件")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "读取文件失败")
		return
	}
	result, err := h.svc.CreateFromMedia(c.Request.Context(), httpx.CurrentAdminID(c), header.Filename, header.Header.Get("Content-Type"), data)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnsupportedMedia):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "媒体录入失败")
		}
		return
	}
	httpx.Created(c, result)
}

func (h *Handler) list(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}
	items, err := h.svc.List(c.Request.Context(), c.Query("status"), limit)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "录入草稿加载失败")
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

func (h *Handler) retry(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	result, err := h.svc.Retry(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"))
	if err != nil {
		switch {
		case errors.Is(err, ErrJobNotFound):
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
		case errors.Is(err, ErrJobNotEditable), errors.Is(err, ErrUnsupportedMedia):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "重试失败")
		}
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) detail(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	result, err := h.svc.Detail(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"))
	if err != nil {
		if errors.Is(err, ErrJobNotFound) {
			httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, err.Error())
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "读取录入任务失败")
		return
	}
	httpx.OK(c, result)
}

func (h *Handler) match(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var in MatchInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	if err := h.svc.Match(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"), in); err != nil {
		switch {
		case errors.Is(err, ErrLandlordNotFound), errors.Is(err, ErrJobNotEditable):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "匹配房东失败")
		}
		return
	}
	httpx.OK(c, gin.H{"matched": true})
}

func (h *Handler) publish(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var in PublishInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请求参数不正确")
		return
	}
	result, err := h.svc.Publish(c.Request.Context(), httpx.CurrentAdminID(c), c.Param("uuid"), in)
	if err != nil {
		switch {
		case errors.Is(err, ErrJobNotEditable), errors.Is(err, ErrCommunityNotFound), errors.Is(err, ErrJobNotFound):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "发布失败")
		}
		return
	}
	httpx.OK(c, result)
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
		case errors.Is(err, ErrLandlordNotFound), errors.Is(err, ErrCommunityNotFound), errors.Is(err, ErrMissingFields):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "录入失败")
		}
		return
	}
	httpx.Created(c, result)
}
