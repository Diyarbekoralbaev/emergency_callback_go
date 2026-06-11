package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/config"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "web":
		runWeb()
	case "worker":
		runWorker()
	case "createuser":
		runCreateUser(args)
	case "seed":
		runSeed()
	case "migrate":
		runMigrate(args)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `emergency-callback — emergency callback system

Usage:
  emergency-callback web                       Run HTTP server
  emergency-callback worker                    Run River background worker
  emergency-callback createuser <user> <pass> [admin|operator]   Create user
  emergency-callback seed                      Seed demo regions/teams
  emergency-callback migrate <up|down|status>  Run goose migrations

Env: load from .env (see .env.example)
`)
}

// signalCtx returns a context that's cancelled on SIGINT/SIGTERM.
func signalCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		slog.Info("signal received, shutting down")
		cancel()
	}()
	return ctx
}

func loadCfgAndPool(ctx context.Context) (*config.Config, *pgxpool.Pool) {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load", "err", err)
		os.Exit(1)
	}
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		slog.Error("db connect", "err", err)
		os.Exit(1)
	}
	return cfg, pool
}
