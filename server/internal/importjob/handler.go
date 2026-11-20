package importjob

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
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

func (h *Handler) RegisterAuth(r *gin.RouterGroup) {
	r.POST("/landlord/properties/imports", h.importFile)
}

func (h *Handler) importFile(c *gin.Context) {
	landlordID := httpx.CurrentUserID(c)
	if landlordID == 0 {
		httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthenticated, "请先登录")
		return
	}
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "请选择 docx 或 xlsx 文件")
		return
	}
	defer file.Close()
	if header.Size > 20<<20 {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "文件不能超过 20MB")
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, 20<<20))
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "读取文件失败")
		return
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), ".")
	if ext != "docx" && ext != "xlsx" {
		httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, "只支持 .docx / .xlsx")
		return
	}

	result, err := h.svc.Import(c.Request.Context(), landlordID, FileInput{
		Name: header.Filename,
		Ext:  ext,
		Data: data,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrUnsupportedType), errors.Is(err, ErrNoRows):
			httpx.Fail(c, http.StatusBadRequest, httpx.CodeInvalidArgument, err.Error())
		default:
			httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "导入失败")
		}
		return
	}
	httpx.Created(c, result)
}
