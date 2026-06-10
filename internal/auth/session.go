package auth

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionKeyUserID is the session key under which the authenticated user's ID is stored.
const SessionKeyUserID = "user_id"

// SessionKeyRole stores the user's role for quick gate checks without a DB hit.
const SessionKeyRole = "role"

// SessionKeyUsername stores the username for display purposes.
const SessionKeyUsername = "username"

// NewSessionManager builds an scs session manager backed by the Postgres `sessions` table.
func NewSessionManager(pool *pgxpool.Pool) *scs.SessionManager {
	sm := scs.New()
	sm.Store = pgxstore.New(pool)
	sm.Lifetime = 7 * 24 * time.Hour
	sm.IdleTimeout = 24 * time.Hour
	sm.Cookie.Name = "ecb_session"
	sm.Cookie.Path = "/"
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Secure = false // set true behind HTTPS
	return sm
}
