package jobs

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/sms"
	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

type SendRatingSMSArgs struct {
	CallbackID int64 `json:"callback_id"`
}

func (SendRatingSMSArgs) Kind() string { return "send_rating_sms" }

type SendRatingSMSWorker struct {
	river.WorkerDefaults[SendRatingSMSArgs]
	Q          *sqlc.Queries
	Eskiz      *sms.Eskiz
	SiteDomain string
}

func (w *SendRatingSMSWorker) Work(ctx context.Context, job *river.Job[SendRatingSMSArgs]) error {
	id := job.Args.CallbackID
	slog.Info("send_rating_sms start", "id", id)

	cb, err := w.Q.GetCallback(ctx, id)
	if err != nil {
		return err
	}
	if cb.SmsSent {
		slog.Info("sms already sent", "id", id)
		return nil
	}
	// Check if already rated
	hasRating, err := w.Q.CallbackHasRating(ctx, id)
	if err == nil && hasRating {
		slog.Info("already has rating, no sms needed", "id", id)
		return nil
	}

	voteUUID := uuid.UUID(cb.VoteUuid.Bytes).String()
	voteURL := fmt.Sprintf("%s/vote/%s", w.SiteDomain, voteUUID)
	body := fmt.Sprintf(
		"Assalawma aleykum. Sizge ko'rsetilgen tez medecinaliq xizmetin bahalaw ushin "+
			"to'mende ko'rsetilgen siltemege o'tip bahalawinizdi soranamiz. %s",
		voteURL,
	)

	if err := w.Eskiz.Send(ctx, cb.PhoneNumber, body); err != nil {
		slog.Error("eskiz send failed", "id", id, "err", err)
		return err
	}

	if err := w.Q.UpdateCallbackSMSSent(ctx, sqlc.UpdateCallbackSMSSentParams{
		ID:     id,
		Status: "waiting_rating",
	}); err != nil {
		slog.Error("mark sms_sent failed", "id", id, "err", err)
	}

	slog.Info("send_rating_sms done", "id", id, "phone", cb.PhoneNumber)
	return nil
}
