package tz

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Tashkent is Asia/Tashkent (UTC+5, no DST).
var Tashkent *time.Location

func init() {
	var err error
	Tashkent, err = time.LoadLocation("Asia/Tashkent")
	if err != nil {
		// Fallback to fixed UTC+5 if tzdata is unavailable.
		Tashkent = time.FixedZone("Asia/Tashkent", 5*60*60)
	}
}

// ToTashkent converts a UTC time to Tashkent.
func ToTashkent(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return t.In(Tashkent)
}

// FromPGTimestamp normalizes a pgtype.Timestamptz to Tashkent time. Returns
// zero time if the column was NULL.
func FromPGTimestamp(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time.In(Tashkent)
}

// Now is the current time in Tashkent.
func Now() time.Time {
	return time.Now().In(Tashkent)
}

// DayBoundsTashkentUTC returns the UTC [start, end] timestamps for the given
// calendar day in Tashkent — used to filter DB rows stored in UTC.
func DayBoundsTashkentUTC(year int, month time.Month, day int) (time.Time, time.Time) {
	start := time.Date(year, month, day, 0, 0, 0, 0, Tashkent).UTC()
	end := time.Date(year, month, day, 23, 59, 59, int(time.Second-time.Nanosecond), Tashkent).UTC()
	return start, end
}
