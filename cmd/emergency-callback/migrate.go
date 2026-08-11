package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/config"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/migrations"
)

// runMigrate applies both migration systems against the same database:
//   - goose (app schema, migrations/*.sql)
//   - River's job-queue tables, in-process — no external `river` CLI.
//
// `up` runs goose up then River up. `down`/`reset` only touch the goose
// schema (River tables are shared infrastructure; never dropped as a side
// effect). `status` prints both.
func runMigrate(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: emergency-callback migrate <up|down|status|reset|version>")
		os.Exit(2)
	}
	cmd := args[0]
	ctx := context.Background()

	cfg, err := config.LoadCore()
	if err != nil {
		slog.Error("config load", "err", err)
		os.Exit(1)
	}

	if err := migrations.GooseRun(ctx, cfg.DatabaseURL, cmd); err != nil {
		slog.Error("migrate failed", "err", err)
		os.Exit(1)
	}

	switch cmd {
	case "up", "status":
		pool, err := db.NewPool(ctx, cfg)
		if err != nil {
			slog.Error("migrate failed", "err", err)
			os.Exit(1)
		}
		defer pool.Close()
		if cmd == "up" {
			err = migrations.RiverUp(ctx, pool)
		} else {
			var applied, total int
			applied, total, err = migrations.RiverStatus(ctx, pool)
			if err == nil {
				fmt.Printf("river: %d/%d migrations applied\n", applied, total)
			}
		}
		if err != nil {
			slog.Error("migrate failed", "err", err)
			os.Exit(1)
		}
	}
}
