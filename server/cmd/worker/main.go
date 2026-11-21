package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"housing/internal/config"
	"housing/internal/db"
	"housing/internal/freshness"
	"housing/internal/notify"
	"housing/internal/prospect"
	"housing/internal/secure"
	"housing/internal/wechat"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}
	pii, err := secure.NewPII(cfg.PIIEncKey, cfg.PIIHashKey)
	if err != nil {
		log.Fatal(err)
	}
	freshRepo := freshness.NewRepository(pool)
	notifyRepo := notify.NewRepository(pool)
	notifySvc := notify.NewService(notifyRepo, pii, wechat.NewClient(cfg.WeChatAppID, cfg.WeChatSecret),
		cfg.WeChatSubscribeTemplateID, cfg.WeChatSubscribePage, cfg.NotifyFields)
	ingestor := prospect.NewDefaultIngestor(prospect.NewRepository(pool), pii, log.Printf)

	runFreshness := func() {
		n, err := freshRepo.CreateDueChecks(context.Background())
		if err != nil {
			log.Printf("freshness run failed: %v", err)
			return
		}
		log.Printf("freshness run complete: created=%d", n)
	}
	runNotify := func() {
		result, err := notifySvc.Process(context.Background(), 20)
		if err != nil {
			log.Printf("notification run failed: %v", err)
			return
		}
		if result.Sent+result.Failed+result.NotConfigured > 0 {
			log.Printf("notification run: sent=%d failed=%d notConfigured=%d",
				result.Sent, result.Failed, result.NotConfigured)
		}
	}

	runProspect := func() {
		stats, err := ingestor.RunOnce(context.Background())
		if err != nil {
			log.Printf("prospect run failed: %v", err)
			return
		}
		if stats.Halted {
			return
		}
		if stats.Fetched > 0 || stats.Ingested > 0 || stats.Failed > 0 {
			log.Printf("prospect run: sources=%d fetched=%d ingested=%d duplicate=%d optedOut=%d invalid=%d failed=%d",
				stats.Sources, stats.Fetched, stats.Ingested, stats.Duplicate, stats.OptedOut, stats.Invalid, stats.Failed)
		}
	}

	runFreshness()
	runNotify()
	freshTicker := time.NewTicker(6 * time.Hour)
	notifyTicker := time.NewTicker(30 * time.Second)
	prospectTicker := time.NewTicker(10 * time.Minute)
	defer freshTicker.Stop()
	defer notifyTicker.Stop()
	defer prospectTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Print("worker stopped")
			return
		case <-freshTicker.C:
			runFreshness()
		case <-notifyTicker.C:
			runNotify()
		case <-prospectTicker.C:
			runProspect()
		}
	}
}
