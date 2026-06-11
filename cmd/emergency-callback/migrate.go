package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func runMigrate(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: emergency-callback migrate <up|down|status|reset|version>")
		os.Exit(2)
	}
	cmd := args[0]

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load", "err", err)
		os.Exit(1)
	}

	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		slog.Error("open db", "err", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	if err := sqlDB.PingContext(context.Background()); err != nil {
		slog.Error("ping db", "err", err)
		os.Exit(1)
	}

	if err := goose.SetDialect("postgres"); err != nil {
		slog.Error("set dialect", "err", err)
		os.Exit(1)
	}

	dir := "migrations"
	if env := os.Getenv("MIGRATIONS_DIR"); env != "" {
		dir = env
	}

	switch cmd {
	case "up":
		err = goose.Up(sqlDB, dir)
	case "down":
		err = goose.Down(sqlDB, dir)
	case "status":
		err = goose.Status(sqlDB, dir)
	case "reset":
		err = goose.Reset(sqlDB, dir)
	case "version":
		err = goose.Version(sqlDB, dir)
	default:
		fmt.Fprintf(os.Stderr, "unknown migrate cmd: %s\n", cmd)
		os.Exit(2)
	}
	if err != nil {
		slog.Error("migrate failed", "err", err)
		os.Exit(1)
	}
}
