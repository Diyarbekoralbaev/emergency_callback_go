package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
)

type CleanupStaleCallsArgs struct{}

func (CleanupStaleCallsArgs) Kind() string { return "cleanup_stale_calls" }

type CleanupStaleCallsWorker struct {
	river.WorkerDefaults[CleanupStaleCallsArgs]
	Q     *sqlc.Queries
	River *river.Client[pgx.Tx]
}

func (w *CleanupStaleCallsWorker) Work(ctx context.Context, job *river.Job[CleanupStaleCallsArgs]) error {
	slog.Info("cleanup_stale_calls running")

	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-30 * time.Minute), Valid: true}
	stale, err := w.Q.FindStaleCallbacks(ctx, cutoff)
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		slog.Info("cleanup_stale_calls: nothing stale")
		return nil
	}

	endedAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	for _, cb := range stale {
		var status, errMsg string
		var transferred bool
		switch cb.Status {
		case "dialing", "connecting":
			status = "failed"
			errMsg = "Call cleanup: Failed to connect"
		case "answered", "waiting_rating", "waiting_additional":
			status = "no_rating"
			errMsg = "Call cleanup: Hung up without completing"
		case "transferring":
			status = "transferred"
			transferred = true
		default:
			continue
		}

		if err := w.Q.UpdateCallbackResult(ctx, sqlc.UpdateCallbackResultParams{
			ID:           cb.ID,
			Status:       status,
			CallID:       cb.CallID,
			CallEndedAt:  endedAt,
			CallDuration: cb.CallDuration,
			Transferred:  transferred,
			ErrorMessage: nullString(errMsg),
		}); err != nil {
			slog.Error("cleanup: update failed", "id", cb.ID, "err", err)
			continue
		}

		hasRating, _ := w.Q.CallbackHasRating(ctx, cb.ID)
		if !hasRating && !cb.SmsSent {
			if _, err := w.River.Insert(ctx, SendRatingSMSArgs{CallbackID: cb.ID}, nil); err != nil {
				slog.Error("cleanup: enqueue sms failed", "id", cb.ID, "err", err)
			}
		}
		slog.Info("cleanup: marked", "id", cb.ID, "status", status)
	}
	slog.Info("cleanup_stale_calls done", "count", len(stale))
	return nil
}
