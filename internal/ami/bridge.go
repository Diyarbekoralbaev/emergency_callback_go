package ami

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/config"
	"github.com/google/uuid"
	"github.com/staskobzar/goami2"
)

// RatingSaver is satisfied by anything that can persist a rating row.
// The River worker provides an implementation backed by sqlc.
type RatingSaver interface {
	SaveRating(ctx context.Context, callbackRequestID int64, rating int32, phone string) error
}

// AudioMap maps internal audio names to Asterisk extensions in the `play-audio` context.
var AudioMap = map[string]string{
	"rating_request":   "ambulance-rating-request",
	"rating_thankyou":  "ambulance-rating-thankyou",
	"rating_invalid":   "ambulance-rating-invalid",
	"transfer_message": "ambulance-transfer-message",
	"transfer_error":   "ambulance-transfer-error",
}

// Bridge owns one AMI connection used for one call.
type Bridge struct {
	cfg    config.AMIConfig
	saver  RatingSaver
	client *goami2.Client
	conn   net.Conn
	call   *Call
}

// New constructs a fresh Bridge — caller still needs to call Run.
func New(cfg config.AMIConfig, saver RatingSaver) *Bridge {
	return &Bridge{cfg: cfg, saver: saver}
}

// Run dials Asterisk, originates a call to phone, and drives the rating /
// transfer state machine. Returns when the call is hung up or the configured
// CALL_TIMEOUT elapses.
//
// Mirrors callbacks/ambulance_system.py:make_ambulance_call +
// SimpleAMIConnection.originate_call event loop, collapsed into a single
// blocking method that owns the connection lifecycle.
func (b *Bridge) Run(ctx context.Context, phone string, brigadeID *int64, callbackRequestID int64) (CallResult, error) {
	b.call = &Call{
		CallID:            uuid.NewString(),
		Phone:             phone,
		CallbackRequestID: callbackRequestID,
		BrigadeID:         brigadeID,
		State:             StateDialing,
		CreatedAt:         time.Now(),
	}

	if err := b.connect(ctx); err != nil {
		b.call.State = StateFailed
		b.call.Error = err.Error()
		return b.call.buildResult(), err
	}
	defer b.close()

	if err := b.originate(); err != nil {
		b.call.State = StateFailed
		b.call.Error = err.Error()
		return b.call.buildResult(), err
	}

	deadline := time.NewTimer(b.cfg.CallTimeout)
	defer deadline.Stop()

	for {
		select {
		case msg, ok := <-b.client.AllMessages():
			if !ok {
				return b.call.buildResult(), nil
			}
			if done := b.handleMessage(ctx, msg); done {
				return b.call.buildResult(), nil
			}
		case err := <-b.client.Err():
			if errors.Is(err, goami2.ErrEOF) {
				return b.call.buildResult(), nil
			}
			slog.Warn("ami error", "err", err, "call_id", b.call.CallID)
		case <-deadline.C:
			slog.Warn("call timeout", "call_id", b.call.CallID)
			b.call.State = StateFailed
			b.call.Error = "call timeout"
			return b.call.buildResult(), nil
		case <-ctx.Done():
			return b.call.buildResult(), ctx.Err()
		}
	}
}

func (b *Bridge) connect(ctx context.Context) error {
	addr := net.JoinHostPort(b.cfg.Host, strconv.Itoa(b.cfg.Port))
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial ami %s: %w", addr, err)
	}
	c, err := goami2.NewClientWithContext(ctx, conn, b.cfg.Username, b.cfg.Secret)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("ami login: %w", err)
	}
	b.conn = conn
	b.client = c
	slog.Info("ami connected", "host", b.cfg.Host, "call_id", b.call.CallID)
	return nil
}

func (b *Bridge) close() {
	if b.client != nil {
		b.client.Close()
	}
	if b.conn != nil {
		_ = b.conn.Close()
	}
}

func (b *Bridge) originate() error {
	cleanPhone := formatPhoneNumber(b.call.Phone)

	msg := goami2.NewAction("Originate")
	msg.AddActionID()
	// /n disables Local-channel optimization so the two legs persist after
	// they bridge — required for the app to Redirect the ;2 (ambulance-callback)
	// leg into play-audio after answer.
	msg.AddField("Channel", fmt.Sprintf("Local/%s@from-internal/n", cleanPhone))
	msg.AddField("Context", "ambulance-callback")
	msg.AddField("Exten", "s")
	msg.AddField("Priority", "1")
	msg.AddField("CallerID", fmt.Sprintf("Ambulance <%s>", b.cfg.CallerID))
	msg.AddField("Timeout", "30000")
	msg.AddField("Async", "true")
	// __ prefix => variables inherit to every leg/child channel (both Local
	// legs and the outbound trunk channel) so ${CALL_ID} resolves everywhere.
	msg.AddField("Variable", fmt.Sprintf("__CALL_ID=%s", b.call.CallID))
	msg.AddField("Variable", fmt.Sprintf("__PHONE_NUMBER=%s", b.call.Phone))
	if b.call.BrigadeID != nil {
		msg.AddField("Variable", fmt.Sprintf("__BRIGADE_ID=%d", *b.call.BrigadeID))
	}
	msg.AddField("Variable", fmt.Sprintf("__CALLBACK_REQUEST_ID=%d", b.call.CallbackRequestID))

	b.client.Send(msg.Byte())
	slog.Info("ami originated", "phone", cleanPhone, "call_id", b.call.CallID)
	return nil
}

// formatPhoneNumber mirrors callbacks/ambulance_system.py:_format_phone_number —
// strip non-digits, drop 998 country code if 12-digit local.
func formatPhoneNumber(phone string) string {
	clean := make([]byte, 0, len(phone))
	for i := 0; i < len(phone); i++ {
		if phone[i] >= '0' && phone[i] <= '9' {
			clean = append(clean, phone[i])
		}
	}
	s := string(clean)
	if len(s) == 12 && s[:3] == "998" {
		return s[3:]
	}
	return s
}
