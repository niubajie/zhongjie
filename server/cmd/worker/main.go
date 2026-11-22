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
	"housing/internal/modelgw"
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
	prospectRepo := prospect.NewRepository(pool)
	prospectSvc := prospect.NewService(prospectRepo, pii)
	prospectSvc.SetReminderNotifier(notifyRepo, cfg.AdminNotifyUserID)
	ingestor := prospect.NewDefaultIngestor(prospectRepo, pii, log.Printf)

	// 批次 4：LLM 意向评分。仅在配置了 LLM 时启用，失败不影响采集主流程。
	var scorer *prospect.Scorer
	if cfg.ModelLLMBaseURL != "" {
		gw := modelgw.New(modelgw.Config{
			LLM: modelgw.Provider{BaseURL: cfg.ModelLLMBaseURL, APIKey: cfg.ModelLLMApiKey, Model: cfg.ModelLLMModel},
		}, pool, nil)
		scorer = prospect.NewScorer(prospectRepo, gw, pii, log.Printf)
	}

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
		stats, err := ingestor.RunDue(context.Background(), time.Now())
		if err != nil {
			log.Printf("prospect run failed: %v", err)
			return
		}
		if stats.Halted {
			return
		}
		if stats.Sources > 0 {
			log.Printf("prospect run: due=%d fetched=%d ingested=%d duplicate=%d optedOut=%d invalid=%d failed=%d",
				stats.Sources, stats.Fetched, stats.Ingested, stats.Duplicate, stats.OptedOut, stats.Invalid, stats.Failed)
		}
		if scorer != nil {
			ss, err := scorer.ScoreBatch(context.Background(), 50)
			if err != nil {
				log.Printf("prospect scoring failed: %v", err)
				return
			}
			if ss.Total > 0 {
				log.Printf("prospect scoring: total=%d queued=%d ignored=%d failed=%d", ss.Total, ss.Queued, ss.Ignored, ss.Failed)
			}
		}
	}

	runFollowUpReminders := func() {
		stats, err := prospectSvc.ScanFollowUpReminders(context.Background(), time.Now())
		if err != nil {
			log.Printf("prospect follow-up reminder run failed: %v", err)
			return
		}
		if stats.Due+stats.Overdue+stats.Stale > 0 {
			log.Printf("prospect follow-up reminders: scanned=%d due=%d overdue=%d stale=%d",
				stats.Scanned, stats.Due, stats.Overdue, stats.Stale)
		}
	}

	runFreshness()
	runNotify()
	runFollowUpReminders()
	freshTicker := time.NewTicker(6 * time.Hour)
	notifyTicker := time.NewTicker(30 * time.Second)
	prospectTicker := time.NewTicker(time.Minute)
	followUpTicker := time.NewTicker(30 * time.Minute)
	defer freshTicker.Stop()
	defer notifyTicker.Stop()
	defer prospectTicker.Stop()
	defer followUpTicker.Stop()
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
		case <-followUpTicker.C:
			runFollowUpReminders()
		}
	}
}
