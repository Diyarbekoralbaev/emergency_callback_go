// Package migrations runs both migration systems (goose app schema +
// River job-queue tables) in-process, shared by the `migrate` subcommand
// and the setup wizard.
package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	embedded "github.com/Diyarbekoralbaev/emergency_callback_go"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// openGoose prepares a database/sql handle and the goose dir, using the
// disk migrations/ when present (MIGRATIONS_DIR override wins) and the
// embedded copy otherwise.
func openGoose(ctx context.Context, databaseURL string) (*sql.DB, string, error) {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, "", fmt.Errorf("open db: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, "", fmt.Errorf("ping db: %w", err)
	}
	if err := goose.SetDialect("postgres"); err != nil {
		sqlDB.Close()
		return nil, "", err
	}

	dir := "migrations"
	if env := os.Getenv("MIGRATIONS_DIR"); env != "" {
		dir = env
		goose.SetBaseFS(nil)
	} else if _, statErr := os.Stat(dir); statErr != nil {
		goose.SetBaseFS(embedded.Migrations)
		slog.Info("migrations: using embedded copy (no migrations/ dir on disk)")
	} else {
		goose.SetBaseFS(nil)
	}
	return sqlDB, dir, nil
}

// GooseRun executes one goose command (up|down|status|reset|version).
func GooseRun(ctx context.Context, databaseURL, cmd string) error {
	sqlDB, dir, err := openGoose(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	switch cmd {
	case "up":
		return goose.Up(sqlDB, dir)
	case "down":
		return goose.Down(sqlDB, dir)
	case "status":
		return goose.Status(sqlDB, dir)
	case "reset":
		return goose.Reset(sqlDB, dir)
	case "version":
		return goose.Version(sqlDB, dir)
	default:
		return fmt.Errorf("unknown migrate cmd: %s", cmd)
	}
}

// RiverUp brings River's internal tables up to date (idempotent).
func RiverUp(ctx context.Context, pool *pgxpool.Pool) error {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("river migrate: %w", err)
	}
	res, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, &rivermigrate.MigrateOpts{})
	if err != nil {
		return fmt.Errorf("river migrate: %w", err)
	}
	if len(res.Versions) == 0 {
		slog.Info("river migrations already up to date")
	} else {
		for _, v := range res.Versions {
			slog.Info("river migration applied", "version", v.Version)
		}
	}
	return nil
}

// RiverStatus returns "applied/total" for River's migrations.
func RiverStatus(ctx context.Context, pool *pgxpool.Pool) (applied, total int, err error) {
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return 0, 0, err
	}
	existing, err := migrator.ExistingVersions(ctx)
	if err != nil {
		return 0, 0, err
	}
	return len(existing), len(migrator.AllVersions()), nil
}
