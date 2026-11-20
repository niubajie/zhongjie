package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"housing/internal/auth"
	"housing/internal/httpx"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := strings.TrimSpace(c.GetHeader("X-Request-Id"))
		if id == "" || len(id) > 64 {
			id = newTraceID()
		}
		c.Set(httpx.TraceIDKey, id)
		c.Header("X-Request-Id", id)
		c.Next()
	}
}

func Recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		log.Error("panic recovered", "traceId", httpx.TraceID(c), "panic", recovered)
		httpx.Fail(c, http.StatusInternalServerError, httpx.CodeInternal, "internal error")
	})
}

func CORS(origins string) gin.HandlerFunc {
	allowed := make(map[string]struct{})
	for _, origin := range strings.Split(origins, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		requestOrigin := c.GetHeader("Origin")
		if _, ok := allowed[requestOrigin]; ok {
			c.Header("Access-Control-Allow-Origin", requestOrigin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func AccessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http request",
			"traceId", httpx.TraceID(c),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"durationMs", time.Since(start).Milliseconds(),
		)
	}
}

func RequireAuth(mgr *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(raw, "Bearer ") {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthenticated, "请先登录")
			return
		}
		claims, err := mgr.Parse(strings.TrimSpace(strings.TrimPrefix(raw, "Bearer ")))
		if err != nil {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthenticated, "登录已过期")
			return
		}
		userID, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil || userID <= 0 {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeUnauthenticated, "登录态无效")
			return
		}
		httpx.SetUser(c, userID, claims.UserUUID, claims.Role)
		c.Next()
	}
}

func RequireAdmin(mgr *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(raw, "Bearer ") {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeAdminUnauthorized, "请先登录管理站")
			return
		}
		claims, err := mgr.Parse(strings.TrimSpace(strings.TrimPrefix(raw, "Bearer ")))
		if err != nil {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeAdminUnauthorized, "管理登录已过期")
			return
		}
		adminID, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil || adminID <= 0 {
			httpx.Fail(c, http.StatusUnauthorized, httpx.CodeAdminUnauthorized, "管理登录态无效")
			return
		}
		httpx.SetAdmin(c, adminID, claims.UserUUID, claims.Role)
		c.Next()
	}
}

func RequireAdminRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(c *gin.Context) {
		role := httpx.CurrentAdminRole(c)
		if _, ok := allowed[role]; !ok {
			httpx.Fail(c, http.StatusForbidden, httpx.CodeAdminForbidden, "没有该操作权限")
			return
		}
		c.Next()
	}
}

func newTraceID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}
