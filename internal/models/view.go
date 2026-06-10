package models

import (
	"encoding/gob"
	"html/template"
	"time"
)

func init() {
	// scs uses encoding/gob for session payloads — register the types we
	// stuff into session.Put(...) so encoding doesn't fail at write time.
	gob.Register([]FlashMessage{})
	gob.Register(FlashMessage{})
}

// SessionUser is the lightweight User struct passed to every template.
type SessionUser struct {
	ID            int64
	Username      string
	Email         string
	Role          string // "admin" | "operator"
	IsStaff       bool
	Authenticated bool
}

// FlashMessage shows up in the alert area at the top of every page.
type FlashMessage struct {
	Level string // "success" | "danger" | "warning" | "info"
	Text  string
}

// BaseData is embedded in every template context.
type BaseData struct {
	User      SessionUser
	Messages  []FlashMessage
	CSRFField template.HTML
}

// CallbackView is a template-friendly projection of a CallbackRequest row,
// computing the properties Django exposed via @property (has_rating,
// status_color, duration_formatted, etc.)
type CallbackView struct {
	ID                  int64
	PhoneNumber         string
	TeamID              int64
	TeamName            string
	RegionName          string
	Status              string
	StatusLabel         string
	StatusColor         string
	CallID              string
	Channel             string
	CreatedAt           time.Time
	CallStartedAt       time.Time
	CallEndedAt         time.Time
	CallDuration        *int32
	DurationFormatted   string
	ErrorMessage        string
	Transferred         bool
	RequestedByUsername string
	VoteUUID            string
	VoteURL             string
	SmsSent             bool
	SmsSentAt           time.Time
	VotedViaSMS         bool

	HasRating    bool
	Rating       *int32
	RatingStars  string
	RatingText   string
	RatingColor  string
	RatingDate   time.Time
}

// RatingView is the projection used on the ratings list page.
type RatingView struct {
	ID                int64
	Rating            int32
	Stars             string
	Color             string
	Comment           string
	PhoneNumber       string
	TeamName          string
	RegionName        string
	Timestamp         time.Time
	CallbackRequestID int64
}

// RegionView for region list/detail pages.
type RegionView struct {
	ID             int64
	Name           string
	Code           string
	Description    string
	IsActive       bool
	CreatedAt      time.Time
	TeamCount      int64
	TotalTeamCount int64
}

// TeamView for team list/detail pages.
type TeamView struct {
	ID          int64
	Name        string
	Description string
	RegionID    int64
	RegionName  string
	IsActive    bool
	CreatedAt   time.Time
}

// StatusLabel maps a status enum value to its Russian display string.
func StatusLabel(status string) string {
	return statusLabels[status]
}

// StatusColor maps a status to a Bootstrap alert color class.
func StatusColor(status string) string {
	if c, ok := statusColors[status]; ok {
		return c
	}
	return "secondary"
}

var statusLabels = map[string]string{
	"pending":            "В ожидании",
	"dialing":            "Набирается",
	"connecting":         "Соединение",
	"answered":           "Отвечен",
	"waiting_rating":     "Ожидание оценки",
	"waiting_additional": "Ожидание доп. информации",
	"transferring":       "Перевод",
	"completed":          "Завершен успешно",
	"no_rating":          "Без оценки",
	"transferred":        "Переведен оператору",
	"failed":             "Неудачный вызов",
}

var statusColors = map[string]string{
	"pending":            "warning",
	"dialing":            "info",
	"connecting":         "info",
	"answered":           "info",
	"waiting_rating":     "info",
	"waiting_additional": "info",
	"transferring":       "info",
	"completed":          "success",
	"transferred":        "success",
	"no_rating":          "warning",
	"failed":             "danger",
}

// RatingStars renders an N-of-5 stars string.
func RatingStars(rating int32) string {
	full := ""
	empty := ""
	for i := int32(0); i < rating; i++ {
		full += "★"
	}
	for i := rating; i < 5; i++ {
		empty += "☆"
	}
	return full + empty
}

// RatingText renders the Russian rating word.
func RatingText(rating int32) string {
	switch rating {
	case 1:
		return "Очень плохо"
	case 2:
		return "Плохо"
	case 3:
		return "Удовлетворительно"
	case 4:
		return "Хорошо"
	case 5:
		return "Отлично"
	default:
		return "Неизвестно"
	}
}

// RatingColor returns the Bootstrap color for a rating.
func RatingColor(rating int32) string {
	switch {
	case rating >= 4:
		return "success"
	case rating == 3:
		return "warning"
	default:
		return "danger"
	}
}

// DurationFormatted is the human-readable duration string.
func DurationFormatted(seconds *int32) string {
	if seconds == nil || *seconds == 0 {
		return "—"
	}
	s := int(*seconds)
	h := s / 3600
	m := (s % 3600) / 60
	sec := s % 60
	if h > 0 {
		return durString(h, "ч", m, "м", sec, "с")
	}
	if m > 0 {
		return durString2(m, "м", sec, "с")
	}
	return durString1(sec, "с")
}

func durString(a int, au string, b int, bu string, c int, cu string) string {
	return itoaSp(a) + au + " " + itoaSp(b) + bu + " " + itoaSp(c) + cu
}
func durString2(b int, bu string, c int, cu string) string {
	return itoaSp(b) + bu + " " + itoaSp(c) + cu
}
func durString1(c int, cu string) string {
	return itoaSp(c) + cu
}
func itoaSp(n int) string {
	// tiny zero-alloc int → string for the duration formatter
	if n == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
