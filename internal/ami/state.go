package ami

import "time"

// CallState mirrors callbacks/ambulance_system.py:CallState.
type CallState string

const (
	StateDialing                 CallState = "dialing"
	StateAnswered                CallState = "answered"
	StateWaitingRating           CallState = "waiting_rating"
	StateRatingReceived          CallState = "rating_received"
	StateWaitingTransferDecision CallState = "waiting_transfer_decision"
	StateTransferring            CallState = "transferring"
	StateCompleted               CallState = "completed"
	StateFailed                  CallState = "failed"
)

// Call holds the per-call mutable state during one AMI session.
type Call struct {
	CallID            string
	Phone             string
	CallbackRequestID int64
	BrigadeID         *int64
	State             CallState
	Uniqueid          string
	Channel           string
	Rating            *int32
	Transferred       bool
	Error             string
	AnsweredAt        time.Time
	CreatedAt         time.Time

	// retry counter for invalid rating attempts
	invalidRatingTries int

	// DTMF dedupe: the same physical keypress is delivered on more than one
	// leg of the Local-channel + trunk bridge (e.g. PJSIP/skyline2 leg and
	// Local ;1 leg), arriving milliseconds-to-seconds apart. Track the last
	// processed digit + time and drop an identical digit inside the window.
	lastDigit   string
	lastDigitAt time.Time
}

// CallResult is what the bridge returns once a call ends.
type CallResult struct {
	Success      bool
	CallID       string
	Error        string
	Rating       *int32
	Transferred  bool
	FinalStatus  string // "completed" | "transferred" | "no_rating" | "failed"
	CallDuration *int32
}

// FinalStatusFor returns the right final status given current call state.
func (c *Call) buildResult() CallResult {
	r := CallResult{
		CallID:      c.CallID,
		Rating:      c.Rating,
		Transferred: c.Transferred,
	}
	switch {
	case c.State == StateFailed:
		r.Success = false
		r.FinalStatus = "failed"
		r.Error = c.Error
	case c.Transferred:
		r.Success = true
		r.FinalStatus = "transferred"
	case c.Rating != nil:
		r.Success = true
		r.FinalStatus = "completed"
	default:
		r.Success = true
		r.FinalStatus = "no_rating"
	}
	if !c.AnsweredAt.IsZero() {
		d := int32(time.Since(c.AnsweredAt).Seconds())
		r.CallDuration = &d
	}
	return r
}
