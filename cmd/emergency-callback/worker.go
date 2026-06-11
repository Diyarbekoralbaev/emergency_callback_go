package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/jobs"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/sms"
)

func runWorker() {
	ctx := signalCtx()
	cfg, pool := loadCfgAndPool(ctx)
	defer pool.Close()

	q := sqlc.New(pool)
	eskiz := sms.New(cfg.Eskiz)

	rc, err := jobs.Setup(ctx, cfg, pool, q, eskiz, false)
	if err != nil {
		slog.Error("river setup", "err", err)
		return
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = rc.Stop(stopCtx)
	}()

	slog.Info("worker started", "max_workers", cfg.RiverMaxWorkers)
	<-ctx.Done()
	slog.Info("worker stopping")
}
