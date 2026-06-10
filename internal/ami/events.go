package ami

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/staskobzar/goami2"
)

// handleMessage dispatches one inbound AMI message. Returns true when the call
// is over (Hangup on our channel, or CallEnded UserEvent).
func (b *Bridge) handleMessage(ctx context.Context, msg *goami2.Message) bool {
	if !msg.IsEvent() {
		return false
	}
	switch msg.Field("Event") {
	case "Newchannel":
		b.onNewchannel(msg)
	case "UserEvent":
		return b.onUserEvent(ctx, msg)
	case "DTMFEnd":
		b.onDTMFEnd(ctx, msg)
	case "Hangup":
		return b.onHangup(msg)
	}
	return false
}

func (b *Bridge) onNewchannel(msg *goami2.Message) {
	if b.call.Channel != "" {
		return
	}
	channel := msg.Field("Channel")
	exten := msg.Field("Exten")
	cleanPhone := formatPhoneNumber(b.call.Phone)

	if exten == cleanPhone || contains(channel, cleanPhone) {
		b.call.Channel = channel
		b.call.Uniqueid = msg.Field("Uniqueid")
		slog.Info("ami channel captured", "channel", channel, "call_id", b.call.CallID)
	}
}

func (b *Bridge) onUserEvent(ctx context.Context, msg *goami2.Message) bool {
	userevent := msg.Field("UserEvent")
	callID := msg.Field("CallID")
	if callID != b.call.CallID {
		return false
	}

	switch userevent {
	case "CallAnswered":
		b.call.State = StateAnswered
		b.call.AnsweredAt = time.Now()
		// The channel that executed UserEvent(CallAnswered) IS the
		// ambulance-callback application leg — this is the channel we must
		// Redirect into play-audio. It's authoritative; override whatever
		// Newchannel guessed (which can race between the two Local-channel legs).
		if ch := msg.Field("Channel"); ch != "" {
			b.call.Channel = ch
		}
		if uid := msg.Field("Uniqueid"); uid != "" {
			b.call.Uniqueid = uid
		}
		slog.Info("ami call answered", "call_id", callID, "channel", b.call.Channel)
		b.playAudio("rating_request")

	case "CallEnded":
		slog.Info("ami call ended event", "call_id", callID)
		return true

	case "DTMFReceived":
		digit := msg.Field("Digit")
		b.handleDTMF(ctx, digit)
	}
	return false
}

func (b *Bridge) onDTMFEnd(ctx context.Context, msg *goami2.Message) {
	if msg.Field("Direction") == "Sent" {
		return
	}
	uniqueid := msg.Field("Uniqueid")
	channel := msg.Field("Channel")
	linkedid := msg.Field("Linkedid")
	digit := msg.Field("Digit")
	cleanPhone := formatPhoneNumber(b.call.Phone)

	// DTMF can surface on any leg of the Local-channel + trunk bridge (the
	// callee's PJSIP leg, the Local ;1, or the app's Local ;2). Accept the
	// digit if it belongs to this call by any of these signals.
	match := false
	switch {
	case b.call.Uniqueid != "" && uniqueid == b.call.Uniqueid:
		match = true
	case b.call.Uniqueid != "" && linkedid == b.call.Uniqueid:
		match = true
	case b.call.Channel != "" && channel == b.call.Channel:
		match = true
	case cleanPhone != "" && contains(channel, cleanPhone):
		match = true
	}
	if !match {
		return
	}
	slog.Info("ami dtmf", "digit", digit, "channel", channel, "call_id", b.call.CallID)
	b.handleDTMF(ctx, digit)
}

func (b *Bridge) onHangup(msg *goami2.Message) bool {
	uniqueid := msg.Field("Uniqueid")
	if uniqueid == b.call.Uniqueid {
		slog.Info("ami hangup", "call_id", b.call.CallID)
		return true
	}
	return false
}

// dtmfDedupeWindow drops an identical digit re-delivered on another bridge leg
// within this window of the previously processed one.
const dtmfDedupeWindow = 4 * time.Second

// handleDTMF is the state machine: rating 1-5, then transfer choice 0/9.
//
// IMPORTANT: this runs synchronously on the AMI message loop — it must NOT
// block (no time.Sleep), otherwise hangup/DTMF events queue up behind it and
// get processed late (which previously cut the thank-you audio short).
func (b *Bridge) handleDTMF(ctx context.Context, digit string) {
	// Drop duplicate keypress echoed from another leg.
	if digit == b.call.lastDigit && !b.call.lastDigitAt.IsZero() &&
		time.Since(b.call.lastDigitAt) < dtmfDedupeWindow {
		slog.Info("dtmf duplicate ignored", "digit", digit, "call_id", b.call.CallID)
		return
	}
	b.call.lastDigit = digit
	b.call.lastDigitAt = time.Now()

	switch b.call.State {
	case StateWaitingRating:
		if digit >= "1" && digit <= "5" {
			n, _ := strconv.Atoi(digit)
			r := int32(n)
			b.call.Rating = &r
			b.call.State = StateRatingReceived

			if err := b.saver.SaveRating(ctx, b.call.CallbackRequestID, r, b.call.Phone); err != nil {
				slog.Error("save rating", "err", err, "call_id", b.call.CallID)
			} else {
				slog.Info("rating saved", "rating", r, "call_id", b.call.CallID)
			}

			// Play the thank-you prompt and move straight to waiting for the
			// transfer decision. The audio plays to completion in the dialplan;
			// we do NOT sleep here (that would block the message loop). The
			// dedupe above prevents the echoed rating digit from being read as
			// a transfer choice.
			b.playAudio("rating_thankyou")
			b.call.State = StateWaitingTransferDecision
			return
		}
		b.handleInvalidRating()

	case StateWaitingTransferDecision:
		if digit == "0" || digit == "9" {
			b.call.Transferred = true
			b.call.State = StateTransferring
			slog.Info("transferring", "call_id", b.call.CallID)
			b.transferToOperator()
		} else {
			b.call.State = StateCompleted
			b.hangup()
		}
	}
}

func (b *Bridge) handleInvalidRating() {
	limit := b.cfg.RatingRetryLimit
	b.call.invalidRatingTries++
	if b.call.invalidRatingTries >= limit {
		b.playAudio("rating_invalid")
		time.Sleep(3 * time.Second)
		b.hangup()
		return
	}
	b.playAudio("rating_invalid")
	time.Sleep(2 * time.Second)
	b.playAudio("rating_request")
}

// playAudio routes the call into the play-audio context at the named extension.
func (b *Bridge) playAudio(name string) {
	if b.call.Channel == "" {
		slog.Warn("playAudio: no channel", "call_id", b.call.CallID)
		return
	}
	exten, ok := AudioMap[name]
	if !ok {
		exten = name
	}
	if name == "rating_request" {
		b.call.State = StateWaitingRating
	}
	msg := goami2.NewAction("Redirect")
	msg.AddActionID()
	msg.AddField("Channel", b.call.Channel)
	msg.AddField("Context", "play-audio")
	msg.AddField("Exten", exten)
	msg.AddField("Priority", "1")
	b.client.Send(msg.Byte())
	slog.Info("playing audio", "audio", name, "call_id", b.call.CallID)
}

func (b *Bridge) transferToOperator() {
	if b.call.Channel == "" {
		return
	}
	msg := goami2.NewAction("Redirect")
	msg.AddActionID()
	msg.AddField("Channel", b.call.Channel)
	msg.AddField("Context", "transfer-to-337")
	msg.AddField("Exten", "s")
	msg.AddField("Priority", "1")
	b.client.Send(msg.Byte())
}

func (b *Bridge) hangup() {
	if b.call.Channel == "" {
		return
	}
	msg := goami2.NewAction("Hangup")
	msg.AddActionID()
	msg.AddField("Channel", b.call.Channel)
	b.client.Send(msg.Byte())
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// formatDebugMsg is unused but handy when investigating new events; keep it for now.
var _ = fmt.Sprintf
