// Package telephony defines the backend-neutral contract for driving one
// outbound rating call. Two implementations exist: internal/ami (classic
// AMI + dialplan) and internal/ari (ARI/Stasis, no dialplan). The backend
// is chosen per deployment via TELEPHONY_BACKEND.
package telephony

import "context"

// Caller places one call and drives the rating/transfer state machine.
// Run blocks until the call ends or ctx is cancelled.
type Caller interface {
	Run(ctx context.Context, phone string, brigadeID *int64, callbackRequestID int64) (CallResult, error)
}

// RatingSaver is satisfied by anything that can persist a rating row.
// The River worker provides an implementation backed by sqlc.
type RatingSaver interface {
	SaveRating(ctx context.Context, callbackRequestID int64, rating int32, phone string) error
}

// CallResult is what a backend returns once a call ends.
type CallResult struct {
	Success      bool
	CallID       string
	Error        string
	Rating       *int32
	Transferred  bool
	FinalStatus  string // "completed" | "transferred" | "no_rating" | "failed"
	CallDuration *int32
}

// FormatPhoneNumber strips non-digits and drops the 998 country code from
// 12-digit local numbers — both backends dial the resulting 9-digit form
// into from-internal, where the PBX outbound route matches it.
func FormatPhoneNumber(phone string) string {
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
