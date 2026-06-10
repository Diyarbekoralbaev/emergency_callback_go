package templates

import (
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/tz"
	"github.com/jackc/pgx/v5/pgtype"
)

// URL registry — populated by server.routes.RegisterURLs at startup so
// templates can resolve named URLs (Django's {% url 'foo' %} equivalent).
var urlRegistry = map[string]string{}

// RegisterURL records a name → URL pattern. Patterns use Gin syntax (:id, *path).
func RegisterURL(name, pattern string) {
	urlRegistry[name] = pattern
}

// urlFor substitutes positional :param markers in the pattern with the args.
// Falls back to "#" if the name is unknown so a typo doesn't break rendering.
func urlFor(name string, args ...any) string {
	pat, ok := urlRegistry[name]
	if !ok {
		return "#"
	}
	for _, a := range args {
		idx := strings.Index(pat, ":")
		if idx < 0 {
			break
		}
		end := idx + 1
		for end < len(pat) && (isAlnum(pat[end]) || pat[end] == '_') {
			end++
		}
		pat = pat[:idx] + fmt.Sprint(a) + pat[end:]
	}
	return pat
}

func isAlnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func tashkentTime(t any) string {
	tm := coerceTime(t)
	if tm.IsZero() {
		return "—"
	}
	return tz.ToTashkent(tm).Format("15:04")
}

func tashkentDate(t any) string {
	tm := coerceTime(t)
	if tm.IsZero() {
		return "—"
	}
	return tz.ToTashkent(tm).Format("02.01.2006")
}

func tashkentDateTime(t any) string {
	tm := coerceTime(t)
	if tm.IsZero() {
		return "—"
	}
	return tz.ToTashkent(tm).Format("02.01.2006 15:04")
}

func currentTashkentTime() time.Time {
	return tz.Now()
}

func currentTashkentHM() string {
	return tz.Now().Format("15:04")
}

func currentTashkentFull() string {
	return tz.Now().Format("02.01.2006 15:04")
}

func durationFormat(seconds any) string {
	var s int
	switch v := seconds.(type) {
	case int:
		s = v
	case int32:
		s = int(v)
	case int64:
		s = int(v)
	case *int32:
		if v == nil {
			return "—"
		}
		s = int(*v)
	case nil:
		return "—"
	default:
		return "—"
	}
	if s <= 0 {
		return "—"
	}
	h := s / 3600
	m := (s % 3600) / 60
	sec := s % 60
	if h > 0 {
		return fmt.Sprintf("%dч %dм %dс", h, m, sec)
	}
	if m > 0 {
		return fmt.Sprintf("%dм %dс", m, sec)
	}
	return fmt.Sprintf("%dс", sec)
}

func coerceTime(t any) time.Time {
	switch v := t.(type) {
	case time.Time:
		return v
	case *time.Time:
		if v == nil {
			return time.Time{}
		}
		return *v
	case pgtype.Timestamptz:
		if !v.Valid {
			return time.Time{}
		}
		return v.Time
	case pgtype.Date:
		if !v.Valid {
			return time.Time{}
		}
		return v.Time
	default:
		return time.Time{}
	}
}

// FuncMap returns the html/template FuncMap used by every template.
func FuncMap() template.FuncMap {
	return template.FuncMap{
		"urlFor":              urlFor,
		"tashkentTime":        tashkentTime,
		"tashkentDate":        tashkentDate,
		"tashkentDateTime":    tashkentDateTime,
		"currentTashkentTime": currentTashkentTime,
		"currentTashkentHM":   currentTashkentHM,
		"currentTashkentFull": currentTashkentFull,
		"durationFormat":      durationFormat,
		"add":                 func(a, b int) int { return a + b },
		"sub":                 func(a, b int) int { return a - b },
		"mul":                 func(a, b int) int { return a * b },
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
		"upper":    strings.ToUpper,
		"lower":    strings.ToLower,
	}
}
