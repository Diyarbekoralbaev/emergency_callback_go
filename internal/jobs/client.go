package jobs

import (
	"context"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/config"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/sms"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// Setup wires River workers and returns a started client.
// queueOnly=true (web mode) registers no workers — only inserts.
// queueOnly=false (worker mode) starts workers and the periodic scheduler.
func Setup(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool, q *sqlc.Queries, eskiz *sms.Eskiz, queueOnly bool) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()

	driver := riverpgxv5.New(pool)

	clientCfg := &river.Config{}

	if !queueOnly {
		// Worker mode: register the three workers + periodic cleanup
		clientCfg.Queues = map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: cfg.RiverMaxWorkers},
		}
		clientCfg.Workers = workers
		clientCfg.PeriodicJobs = []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(15*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) {
					return CleanupStaleCallsArgs{}, nil
				},
				&river.PeriodicJobOpts{RunOnStart: false},
			),
		}
	}

	rc, err := river.NewClient[pgx.Tx](driver, clientCfg)
	if err != nil {
		return nil, err
	}

	if !queueOnly {
		// Workers depend on the client (for SMS enqueue + cleanup enqueue),
		// so register them after construction.
		river.AddWorker(workers, &ProcessCallbackWorker{
			Pool:   pool,
			Q:      q,
			AMICfg: cfg.AMI,
			River:  rc,
		})
		river.AddWorker(workers, &SendRatingSMSWorker{
			Q:          q,
			Eskiz:      eskiz,
			SiteDomain: cfg.SiteDomain,
		})
		river.AddWorker(workers, &CleanupStaleCallsWorker{
			Q:     q,
			River: rc,
		})

		if err := rc.Start(ctx); err != nil {
			return nil, err
		}
	}

	return rc, nil
}
