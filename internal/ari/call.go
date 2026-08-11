package ari

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/config"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/telephony"
	"github.com/google/uuid"
)

// Bridge drives one call over ARI. Mirrors the AMI bridge's state machine
// (rating 1-5, then transfer choice 0/9) but event-driven end to end: no
// sleeps, and DTMF arrives only for our own Stasis channels, so no
// cross-leg dedupe is needed.
type Bridge struct {
	cfg   *config.Config
	saver telephony.RatingSaver
}

func New(cfg *config.Config, saver telephony.RatingSaver) *Bridge {
	return &Bridge{cfg: cfg, saver: saver}
}

type callState string

const (
	stDialing         callState = "dialing"
	stWaitingRating   callState = "waiting_rating"
	stWaitingDecision callState = "waiting_transfer_decision"
	stTransferring    callState = "transferring"
	stCompleted       callState = "completed"
	stFailed          callState = "failed"
)

// call is the per-call mutable state.
type call struct {
	id       string
	phone    string
	cbID     int64
	state    callState
	rating   *int32
	bridged  bool // operator actually connected
	errMsg   string
	answered time.Time

	mainCh       string
	opCh         string
	bridgeID     string
	playback     string // current playback id (for barge-in)
	afterPlay    func() // action to run when current playback finishes
	invalidTries int
}

// Run places the call and blocks until it ends. One WS connection + unique
// app name per call gives event isolation between concurrent calls.
func (b *Bridge) Run(ctx context.Context, phone string, brigadeID *int64, callbackRequestID int64) (telephony.CallResult, error) {
	c := &call{
		id:    uuid.NewString(),
		phone: phone,
		cbID:  callbackRequestID,
		state: stDialing,
	}
	app := "ecb-" + c.id[:8]
	client := NewClient(b.cfg.ARI.URL, b.cfg.ARI.Username, b.cfg.ARI.Password, app)

	fail := func(err error) (telephony.CallResult, error) {
		c.state = stFailed
		c.errMsg = err.Error()
		return b.result(c), err
	}

	// 1. Events first — connecting registers the app with Asterisk.
	events, err := client.ConnectEvents(ctx)
	if err != nil {
		return fail(err)
	}
	defer events.Close()

	// 2. Audio ladder — resolved before dialing so a misconfigured PBX
	// fails fast with a precise message instead of a silent call.
	audio, err := resolveAudio(ctx, client, b.cfg.AudioMediaBaseURL)
	if err != nil {
		return fail(err)
	}
	slog.Info("ari audio mode", "mode", audio.mode, "call_id", c.id)

	// 3. Originate through the PBX's own outbound routing.
	clean := telephony.FormatPhoneNumber(phone)
	endpoint := fmt.Sprintf("Local/%s@from-internal/n", clean)
	callerID := fmt.Sprintf("Ambulance <%s>", b.cfg.AMI.CallerID)
	c.mainCh, err = client.Originate(ctx, endpoint, callerID, 30)
	if err != nil {
		return fail(fmt.Errorf("originate: %w", err))
	}
	slog.Info("ari originated", "phone", clean, "call_id", c.id, "channel", c.mainCh)

	defer func() {
		// Best-effort cleanup with a fresh context (ctx may be done).
		cctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if c.bridgeID != "" {
			client.DeleteBridge(cctx, c.bridgeID)
		}
		if c.opCh != "" {
			client.Hangup(cctx, c.opCh)
		}
		client.Hangup(cctx, c.mainCh)
	}()

	// Event pump.
	evCh := make(chan Event)
	errCh := make(chan error, 1)
	go func() {
		for {
			ev, err := events.Read(ctx)
			if err != nil {
				errCh <- err
				return
			}
			select {
			case evCh <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()

	deadline := time.NewTimer(b.cfg.AMI.CallTimeout)
	defer deadline.Stop()
	// inputTimer fires when the callee stays silent after a prompt.
	inputTimer := time.NewTimer(time.Hour)
	inputTimer.Stop()
	defer inputTimer.Stop()

	play := func(key string, after func()) {
		media := audio.Media(key)
		pb, err := client.Play(ctx, c.mainCh, media)
		if err != nil {
			slog.Error("ari play", "err", err, "media", media, "call_id", c.id)
			c.afterPlay = nil
			if after != nil {
				after()
			}
			return
		}
		c.playback = pb
		c.afterPlay = after
		slog.Info("ari playing", "audio", key, "call_id", c.id)
	}
	armInput := func() {
		inputTimer.Stop()
		inputTimer.Reset(b.cfg.AMI.RatingTimeout)
	}

	for {
		select {
		case ev := <-evCh:
			done := b.handleEvent(ctx, client, c, ev, play, armInput)
			if done {
				return b.result(c), nil
			}

		case err := <-errCh:
			// WS dropped: call state is whatever we have.
			slog.Warn("ari events closed", "err", err, "call_id", c.id)
			return b.result(c), nil

		case <-inputTimer.C:
			switch c.state {
			case stWaitingRating:
				// Silence = invalid attempt (same retry budget as AMI).
				c.invalidTries++
				if c.invalidTries >= b.cfg.AMI.RatingRetryLimit {
					play("failed_rating", func() { client.Hangup(ctx, c.mainCh) })
				} else {
					play("rating_request", armInput)
				}
			case stWaitingDecision:
				c.state = stCompleted
				client.Hangup(ctx, c.mainCh)
			}

		case <-deadline.C:
			slog.Warn("ari call timeout", "call_id", c.id)
			if c.state != stTransferring {
				c.state = stFailed
				c.errMsg = "call timeout"
			}
			return b.result(c), nil

		case <-ctx.Done():
			return b.result(c), ctx.Err()
		}
	}
}

// handleEvent processes one ARI event; returns true when the call is over.
func (b *Bridge) handleEvent(ctx context.Context, client *Client, c *call, ev Event, play func(string, func()), armInput func()) bool {
	switch ev.Type {
	case "StasisStart":
		switch ev.Channel.ID {
		case c.mainCh:
			c.answered = time.Now()
			c.state = stWaitingRating
			slog.Info("ari call answered", "call_id", c.id, "channel", ev.Channel.ID)
			play("rating_request", armInput)
		case c.opCh:
			// Operator answered — bridge the two legs.
			brID, err := client.CreateBridge(ctx)
			if err == nil {
				err = client.AddToBridge(ctx, brID, c.mainCh)
			}
			if err == nil {
				err = client.AddToBridge(ctx, brID, c.opCh)
			}
			if err != nil {
				slog.Error("ari bridge", "err", err, "call_id", c.id)
				play("transfer_error", func() { client.Hangup(ctx, c.mainCh) })
				return false
			}
			c.bridgeID = brID
			c.bridged = true
			slog.Info("ari bridged to operator", "call_id", c.id)
		}

	case "ChannelDtmfReceived":
		if ev.Channel.ID != c.mainCh {
			return false
		}
		b.handleDTMF(ctx, client, c, ev.Digit, play, armInput)

	case "PlaybackFinished":
		if ev.Playback.ID == c.playback {
			c.playback = ""
			if after := c.afterPlay; after != nil {
				c.afterPlay = nil
				after()
			}
		}

	case "StasisEnd", "ChannelDestroyed":
		switch ev.Channel.ID {
		case c.mainCh:
			slog.Info("ari call ended", "call_id", c.id, "state", c.state)
			return true
		case c.opCh:
			if !c.bridged && c.state == stTransferring {
				// Operator didn't answer / failed.
				slog.Warn("ari operator unavailable", "call_id", c.id)
				c.state = stCompleted
				play("transfer_error", func() { client.Hangup(ctx, c.mainCh) })
			}
		}
	}
	return false
}

func (b *Bridge) handleDTMF(ctx context.Context, client *Client, c *call, digit string, play func(string, func()), armInput func()) {
	slog.Info("ari dtmf", "digit", digit, "state", c.state, "call_id", c.id)
	// Barge-in: a keypress cancels the current prompt.
	if c.playback != "" {
		client.StopPlayback(ctx, c.playback)
		c.playback = ""
		c.afterPlay = nil
	}

	switch c.state {
	case stWaitingRating:
		if digit >= "1" && digit <= "5" {
			n := int32(digit[0] - '0')
			c.rating = &n
			if err := b.saver.SaveRating(ctx, c.cbID, n, c.phone); err != nil {
				slog.Error("save rating", "err", err, "call_id", c.id)
			} else {
				slog.Info("rating saved", "rating", n, "call_id", c.id)
			}
			c.state = stWaitingDecision
			play("rating_thankyou", armInput)
			return
		}
		c.invalidTries++
		if c.invalidTries >= b.cfg.AMI.RatingRetryLimit {
			play("rating_invalid", func() { client.Hangup(ctx, c.mainCh) })
			return
		}
		play("rating_invalid", func() { play("rating_request", armInput) })

	case stWaitingDecision:
		if digit == "0" || digit == "9" {
			c.state = stTransferring
			operator := b.cfg.AMI.OperatorQueue
			slog.Info("ari transferring", "operator", operator, "call_id", c.id)
			play("transfer_message", func() {
				opCh, err := client.Originate(ctx,
					fmt.Sprintf("Local/%s@from-internal/n", operator),
					fmt.Sprintf("Ambulance <%s>", b.cfg.AMI.CallerID), 30)
				if err != nil {
					slog.Error("ari operator originate", "err", err, "call_id", c.id)
					c.state = stCompleted
					play("transfer_error", func() { client.Hangup(ctx, c.mainCh) })
					return
				}
				c.opCh = opCh
			})
		} else {
			c.state = stCompleted
			client.Hangup(ctx, c.mainCh)
		}
	}
}

func (b *Bridge) result(c *call) telephony.CallResult {
	r := telephony.CallResult{
		CallID:      c.id,
		Rating:      c.rating,
		Transferred: c.bridged,
	}
	switch {
	case c.state == stFailed:
		r.Success = false
		r.FinalStatus = "failed"
		r.Error = c.errMsg
	case c.bridged:
		r.Success = true
		r.FinalStatus = "transferred"
	case c.rating != nil:
		r.Success = true
		r.FinalStatus = "completed"
	default:
		r.Success = true
		r.FinalStatus = "no_rating"
	}
	if !c.answered.IsZero() {
		d := int32(time.Since(c.answered).Seconds())
		r.CallDuration = &d
	}
	return r
}
