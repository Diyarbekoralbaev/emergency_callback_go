package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	embedded "github.com/Diyarbekoralbaev/emergency_callback_go"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/auth"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/handlers"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/jobs"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/server"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/sms"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/templates"
)

func runWeb() {
	ctx := signalCtx()
	cfg, pool := loadCfgAndPool(ctx)
	defer pool.Close()

	q := sqlc.New(pool)

	// Disk-first: a checked-out repo keeps live template edits working;
	// a standalone binary falls back to the embedded copy.
	tmpls, err := loadTemplates()
	if err != nil {
		slog.Error("load templates", "err", err)
		os.Exit(1)
	}

	eskiz := sms.New(cfg.Eskiz)

	// In web mode, River client is queue-only — it doesn't run workers, just inserts.
	rc, err := jobs.Setup(ctx, cfg, pool, q, eskiz, true)
	if err != nil {
		slog.Error("river setup", "err", err)
		os.Exit(1)
	}
	_ = rc

	sm := auth.NewSessionManager(pool, cfg.CookieSecure)

	srv := &handlers.Server{
		Cfg:       cfg,
		Pool:      pool,
		Q:         q,
		Session:   sm,
		Templates: tmpls,
		River:     rc,
	}

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           server.Build(srv),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("web server listening", "addr", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "err", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	slog.Info("web server stopped")
}

func loadTemplates() (*templates.Registry, error) {
	if _, err := os.Stat("templates"); err == nil {
		return templates.Load("templates")
	}
	sub, err := fs.Sub(embedded.Templates, "templates")
	if err != nil {
		return nil, err
	}
	slog.Info("templates: using embedded copy (no templates/ dir on disk)")
	return templates.LoadFS(sub)
}
