package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"housing/internal/admin"
	"housing/internal/assistant"
	"housing/internal/auth"
	"housing/internal/building"
	"housing/internal/community"
	"housing/internal/config"
	"housing/internal/contact"
	"housing/internal/freshness"
	"housing/internal/httpx"
	"housing/internal/importjob"
	"housing/internal/intake"
	"housing/internal/property"
	"housing/internal/storage"
	"housing/internal/user"
)

type Handlers struct {
	User            *user.Handler
	Property        *property.Handler
	Building        *building.Handler
	Community       *community.Handler
	Contact         *contact.Handler
	Freshness       *freshness.Handler
	Assistant       *assistant.Handler
	Import          *importjob.Handler
	Intake          *intake.Handler
	Admin           *admin.Handler
	Media           *storage.Service
	AllowLocalMedia bool
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

	// Local development stores uploads outside COS; production never mounts this.
	if handlers.AllowLocalMedia && handlers.Media != nil && handlers.Media.Local() {
		r.Static("/media", handlers.Media.LocalDir())
	}

	v1 := r.Group("/v1")
	handlers.User.RegisterPublic(v1)
	handlers.Property.Register(v1)
	if handlers.Assistant != nil {
		handlers.Assistant.Register(v1)
	}
	if handlers.Building != nil {
		handlers.Building.Register(v1)
	}
	if handlers.Community != nil {
		handlers.Community.Register(v1)
	}
	if handlers.Admin != nil {
		handlers.Admin.RegisterPublic(v1)
		adminGroup := v1.Group("/admin")
		adminGroup.Use(RequireAdmin(adminMgr))
		handlers.Admin.RegisterAuth(adminGroup)
		if handlers.Freshness != nil {
			handlers.Freshness.RegisterAdmin(adminGroup)
		}
		if handlers.Intake != nil {
			handlers.Intake.RegisterAdmin(adminGroup)
		}
		if handlers.Community != nil {
			handlers.Community.RegisterAdmin(adminGroup)
		}
	}

	protected := v1.Group("")
	protected.Use(RequireAuth(mgr))
	handlers.User.RegisterAuth(protected)
	handlers.Property.RegisterAuth(protected)
	if handlers.Contact != nil {
		handlers.Contact.RegisterAuth(protected)
	}
	if handlers.Freshness != nil {
		handlers.Freshness.RegisterLandlord(protected)
	}
	if handlers.Import != nil {
		handlers.Import.RegisterAuth(protected)
	}

	r.NoRoute(func(c *gin.Context) {
		httpx.Fail(c, http.StatusNotFound, httpx.CodeInvalidArgument, "接口不存在")
	})
	r.NoMethod(func(c *gin.Context) {
		httpx.Fail(c, http.StatusMethodNotAllowed, httpx.CodeInvalidArgument, "请求方法不支持")
	})

	return r
}
