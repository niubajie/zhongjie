package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"housing/internal/admin"
	"housing/internal/assistant"
	"housing/internal/auth"
	"housing/internal/building"
	"housing/internal/config"
	"housing/internal/contact"
	"housing/internal/db"
	"housing/internal/freshness"
	"housing/internal/httpapi"
	"housing/internal/importjob"
	"housing/internal/intake"
	"housing/internal/property"
	"housing/internal/secure"
	"housing/internal/user"
	"housing/internal/wechat"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	log := newLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Error("migrate database", "err", err)
		os.Exit(1)
	}

	pii, err := secure.NewPII(cfg.PIIEncKey, cfg.PIIHashKey)
	if err != nil {
		log.Error("init pii encryption", "err", err)
		os.Exit(1)
	}
	tokens, err := auth.NewManager(cfg.JWTSecret, cfg.JWTAccessTTL)
	if err != nil {
		log.Error("init jwt", "err", err)
		os.Exit(1)
	}
	adminTokens, err := auth.NewManagerWithIssuer(cfg.AdminJWTSecret, cfg.AdminJWTAccessTTL, "housing-admin")
	if err != nil {
		log.Error("init admin jwt", "err", err)
		os.Exit(1)
	}

	userRepo := user.NewRepository(pool)
	userSvc := user.NewService(userRepo, pii, tokens)
	wxClient := wechat.NewClient(cfg.WeChatAppID, cfg.WeChatSecret)
	userHandler := user.NewHandler(userSvc, wxClient, cfg.DevLoginEnabled)

	buildingRepo := building.NewRepository(pool)
	buildingSvc := building.NewService(buildingRepo)
	buildingHandler := building.NewHandler(buildingSvc)

	propertyRepo := property.NewRepository(pool)
	propertySvc := property.NewService(propertyRepo, buildingSvc, userRepo)
	propertyHandler := property.NewHandler(propertySvc)

	contactRepo := contact.NewRepository(pool)
	contactSvc := contact.NewService(contactRepo, pii)
	contactHandler := contact.NewHandler(contactSvc)

	freshnessRepo := freshness.NewRepository(pool)
	freshnessSvc := freshness.NewService(freshnessRepo)
	freshnessHandler := freshness.NewHandler(freshnessSvc)

	assistantRepo := assistant.NewRepository(pool)
	assistantSvc := assistant.NewService(assistantRepo)
	assistantHandler := assistant.NewHandler(assistantSvc)

	importRepo := importjob.NewRepository(pool)
	importSvc := importjob.NewService(importRepo, propertySvc, buildingSvc)
	importHandler := importjob.NewHandler(importSvc)

	intakeRepo := intake.NewRepository(pool)
	intakeSvc := intake.NewService(intakeRepo, userRepo, buildingSvc, propertySvc, pii)
	intakeHandler := intake.NewHandler(intakeSvc)

	adminRepo := admin.NewRepository(pool)
	adminSvc := admin.NewService(adminRepo, pii, adminTokens)
	adminHandler := admin.NewHandler(adminSvc)

	router := httpapi.NewRouter(cfg, log, tokens, adminTokens, httpapi.Handlers{
		User:      userHandler,
		Property:  propertyHandler,
		Building:  buildingHandler,
		Contact:   contactHandler,
		Freshness: freshnessHandler,
		Assistant: assistantHandler,
		Import:    importHandler,
		Intake:    intakeHandler,
		Admin:     adminHandler,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Env, "devLogin", cfg.DevLoginEnabled)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "err", err)
	}
	log.Info("api stopped")
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
