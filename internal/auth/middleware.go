package auth

import (
	"net/http"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
)

const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
)

type ctxKey int

const (
	ctxUserID ctxKey = iota
	ctxRole
	ctxUsername
)

// CurrentUser fetches the logged-in user id and role from the session.
// Returns (0, "", false) if not logged in.
func CurrentUser(sm *scs.SessionManager, c *gin.Context) (int64, string, string, bool) {
	id := sm.GetInt64(c.Request.Context(), SessionKeyUserID)
	role := sm.GetString(c.Request.Context(), SessionKeyRole)
	username := sm.GetString(c.Request.Context(), SessionKeyUsername)
	if id == 0 {
		return 0, "", "", false
	}
	return id, role, username, true
}

// LoginRequired aborts the request if no user is logged in.
func LoginRequired(sm *scs.SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, _, _, ok := CurrentUser(sm, c)
		if !ok {
			c.Redirect(http.StatusFound, "/users/login/")
			c.Abort()
			return
		}
		c.Next()
	}
}

// HasRole aborts the request if the user is not logged in or has none of the allowed roles.
func HasRole(sm *scs.SessionManager, allowed ...string) gin.HandlerFunc {
	allowSet := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		allowSet[r] = struct{}{}
	}
	return func(c *gin.Context) {
		_, role, _, ok := CurrentUser(sm, c)
		if !ok {
			c.Redirect(http.StatusFound, "/users/login/")
			c.Abort()
			return
		}
		if _, allowed := allowSet[role]; !allowed {
			c.Redirect(http.StatusFound, "/callbacks/")
			c.Abort()
			return
		}
		c.Next()
	}
}

// AdminRequired is shorthand for HasRole(RoleAdmin).
func AdminRequired(sm *scs.SessionManager) gin.HandlerFunc {
	return HasRole(sm, RoleAdmin)
}
