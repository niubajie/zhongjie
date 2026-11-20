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
	repo := freshness.NewRepository(pool)

	run := func() {
		n, err := repo.CreateDueChecks(context.Background())
		if err != nil {
			log.Printf("freshness run failed: %v", err)
			return
		}
		log.Printf("freshness run complete: created=%d", n)
	}
	run()

	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Print("worker stopped")
			return
		case <-ticker.C:
			run()
		}
	}
}
