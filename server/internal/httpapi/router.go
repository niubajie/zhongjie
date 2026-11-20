package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"housing/internal/admin"
	"housing/internal/auth"
	"housing/internal/building"
	"housing/internal/config"
	"housing/internal/contact"
	"housing/internal/httpx"
	"housing/internal/property"
	"housing/internal/user"
)

type Handlers struct {
	User     *user.Handler
	Property *property.Handler
	Building *building.Handler
	Contact  *contact.Handler
	Admin    *admin.Handler
}

func NewRouter(cfg config.Config, log *slog.Logger, mgr *auth.Manager, adminMgr *auth.Manager, handlers Handlers) *gin.Engine {
	if cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(RequestID(), AccessLog(log), Recovery(log), CORS(cfg.AllowedAdminOrigin))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/v1")
	handlers.User.RegisterPublic(v1)
	handlers.Property.Register(v1)
	if handlers.Building != nil {
		handlers.Building.Register(v1)
	}
	if handlers.Admin != nil {
		handlers.Admin.RegisterPublic(v1)
		adminGroup := v1.Group("/admin")
		adminGroup.Use(RequireAdmin(adminMgr))
		handlers.Admin.RegisterAuth(adminGroup)
	}

	protected := v1.Group("")
	protected.Use(RequireAuth(mgr))
	handlers.User.RegisterAuth(protected)
	handlers.Property.RegisterAuth(protected)
	if handlers.Contact != nil {
		handlers.Contact.RegisterAuth(protected)
	}

	r.NoRoute(func(c *gin.Context) {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, "接口不存在")
	})
	r.NoMethod(func(c *gin.Context) {
		httpx.Fail(c, http.StatusMethodNotAllowed, httpx.CodeInvalidArgument, "请求方法不支持")
	})

	return r
}
