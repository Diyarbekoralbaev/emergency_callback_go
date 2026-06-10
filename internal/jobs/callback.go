package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/ami"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/config"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// ProcessCallbackArgs is the River job that drives one Asterisk call. Mirrors
// callbacks/tasks.py:process_callback_call.
type ProcessCallbackArgs struct {
	CallbackID int64 `json:"callback_id"`
}

func (ProcessCallbackArgs) Kind() string { return "process_callback_call" }

type ProcessCallbackWorker struct {
	river.WorkerDefaults[ProcessCallbackArgs]
	Pool   *pgxpool.Pool
	Q      *sqlc.Queries
	AMICfg config.AMIConfig
	River  *river.Client[pgx.Tx]
}

// ratingSaverDB persists ratings using sqlc inside the AMI bridge.
type ratingSaverDB struct {
	q *sqlc.Queries
}

func (s *ratingSaverDB) SaveRating(ctx context.Context, callbackID int64, rating int32, phone string) error {
	cb, err := s.q.GetCallback(ctx, callbackID)
	if err != nil {
		return err
	}
	_, err = s.q.CreateRating(ctx, sqlc.CreateRatingParams{
		CallbackRequestID: callbackID,
		Rating:            rating,
		Comment:           nil,
		PhoneNumber:       phone,
		TeamID:            cb.TeamID,
	})
	if err != nil {
		return err
	}
	return s.q.UpdateCallbackStatus(ctx, sqlc.UpdateCallbackStatusParams{
		ID:     callbackID,
		Status: "completed",
	})
}

func (w *ProcessCallbackWorker) Work(ctx context.Context, job *river.Job[ProcessCallbackArgs]) error {
	id := job.Args.CallbackID
	slog.Info("process_callback start", "callback_id", id)

	cb, err := w.Q.GetCallback(ctx, id)
	if err != nil {
		slog.Error("process_callback: callback not found", "id", id, "err", err)
		return err
	}

	// mark dialing
	now := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	if err := w.Q.UpdateCallbackStatus(ctx, sqlc.UpdateCallbackStatusParams{
		ID:            id,
		Status:        "dialing",
		CallStartedAt: now,
	}); err != nil {
		slog.Error("process_callback: status->dialing failed", "id", id, "err", err)
	}

	// Run the AMI bridge.
	bridge := ami.New(w.AMICfg, &ratingSaverDB{q: w.Q})
	brigadeID := cb.TeamID
	result, _ := bridge.Run(ctx, cb.PhoneNumber, &brigadeID, id)

	// Save final state.
	mapStatus := map[string]string{
		"transferred": "transferred",
		"completed":   "completed",
		"no_rating":   "no_rating",
		"failed":      "failed",
	}
	status := mapStatus[result.FinalStatus]
	if status == "" {
		status = "completed"
	}

	endedAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	callIDPG := pgtype.UUID{Valid: false}
	if result.CallID != "" {
		_ = callIDPG.Scan(result.CallID)
	}

	if err := w.Q.UpdateCallbackResult(ctx, sqlc.UpdateCallbackResultParams{
		ID:           id,
		Status:       status,
		CallID:       callIDPG,
		CallEndedAt:  endedAt,
		CallDuration: result.CallDuration,
		Transferred:  result.Transferred,
		ErrorMessage: nullString(result.Error),
	}); err != nil {
		slog.Error("update result", "id", id, "err", err)
	}

	// SMS fallback if no rating.
	if result.Rating == nil {
		if _, err := w.River.Insert(ctx, SendRatingSMSArgs{CallbackID: id}, nil); err != nil {
			slog.Error("enqueue sms", "id", id, "err", err)
		}
	}
	slog.Info("process_callback done", "id", id, "status", status, "rating", result.Rating)
	return nil
}

func nullString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
