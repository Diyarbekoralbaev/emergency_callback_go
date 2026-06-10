package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/auth"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/handlers"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/templates"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
)

// Build constructs the Gin engine with all routes mounted, then wraps it with
// session management and CSRF protection at the http.Handler level.
func Build(s *handlers.Server) http.Handler {
	registerURLs()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(loggingMiddleware())

	// Public routes
	r.GET("/users/login/", s.LoginGET)
	r.POST("/users/login/", s.LoginPOST)
	r.GET("/users/logout/", auth.LoginRequired(s.Session), s.Logout)
	r.POST("/users/logout/", auth.LoginRequired(s.Session), s.Logout)

	r.GET("/vote/:uuid/", s.VotePage)
	r.POST("/vote/:uuid/submit/", s.SubmitVote)
	r.GET("/vote/:uuid/thanks/", s.VoteThanks)

	r.POST("/api/create/", s.APICallbackCreate)

	// Admin + operator routes
	rOp := r.Group("/", auth.HasRole(s.Session, auth.RoleAdmin, auth.RoleOperator))
	rOp.GET("/callbacks/", s.CallbackList)
	rOp.GET("/callbacks/create/", s.CallbackCreateGET)
	rOp.POST("/callbacks/create/", s.CallbackCreatePOST)
	rOp.GET("/callbacks/:id/", s.CallbackDetail)
	rOp.GET("/get-teams-by-region/", s.TeamsByRegion)

	// Admin-only routes
	rAdmin := r.Group("/", auth.AdminRequired(s.Session))
	rAdmin.GET("/", s.Dashboard)
	rAdmin.GET("/ratings/", s.RatingsList)
	rAdmin.GET("/export-excel/", s.ExcelExport)

	rAdmin.GET("/teams/", s.TeamList)
	rAdmin.GET("/teams/create/", s.TeamCreateGET)
	rAdmin.POST("/teams/create/", s.TeamCreatePOST)
	rAdmin.GET("/teams/:id/", s.TeamDetail)
	rAdmin.GET("/teams/:id/edit/", s.TeamEditGET)
	rAdmin.POST("/teams/:id/edit/", s.TeamEditPOST)
	rAdmin.GET("/teams/:id/delete/", s.TeamDeleteGET)
	rAdmin.POST("/teams/:id/delete/", s.TeamDeletePOST)
	rAdmin.GET("/teams/stats-api/", s.TeamStatsAPI)

	rAdmin.GET("/teams/regions/", s.RegionList)
	rAdmin.GET("/teams/regions/create/", s.RegionCreateGET)
	rAdmin.POST("/teams/regions/create/", s.RegionCreatePOST)
	rAdmin.GET("/teams/regions/:id/", s.RegionDetail)
	rAdmin.GET("/teams/regions/:id/edit/", s.RegionEditGET)
	rAdmin.POST("/teams/regions/:id/edit/", s.RegionEditPOST)
	rAdmin.GET("/teams/regions/:id/delete/", s.RegionDeleteGET)
	rAdmin.POST("/teams/regions/:id/delete/", s.RegionDeletePOST)

	// Wrap with CSRF protection — exempt /api/create/ and /vote/*/submit/.
	csrfMW := csrf.Protect(
		s.Cfg.CSRFKey,
		csrf.Secure(false),
		csrf.SameSite(csrf.SameSiteLaxMode),
		csrf.Path("/"),
		csrf.TrustedOrigins([]string{"127.0.0.1:9999", "localhost:9999", "103tezjardem.uz", "callback.diyarbek.uz"}),
	)
	csrfWrapped := csrfMW(r)

	// Route exempt paths around CSRF entirely. Mark all non-exempt requests as
	// plaintext HTTP so gorilla/csrf doesn't enforce HTTPS-only origin/referer
	// checks (this is a TLS-terminated-at-the-proxy or pure-HTTP deployment).
	dispatcher := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if isCSRFExempt(req) {
			r.ServeHTTP(w, req)
			return
		}
		csrfWrapped.ServeHTTP(w, csrf.PlaintextHTTPRequest(req))
	})

	// Wrap with scs LoadAndSave so sessions are loaded before any handler.
	return s.Session.LoadAndSave(dispatcher)
}

func isCSRFExempt(r *http.Request) bool {
	if r.URL.Path == "/api/create/" {
		return true
	}
	p := r.URL.Path
	if r.Method == http.MethodPost && len(p) > 14 && p[:6] == "/vote/" && p[len(p)-8:] == "/submit/" {
		return true
	}
	return false
}

// registerURLs populates the urlFor template registry.
func registerURLs() {
	for k, v := range map[string]string{
		"callbacks:dashboard":           "/",
		"callbacks:list":                "/callbacks/",
		"callbacks:create":              "/callbacks/create/",
		"callbacks:detail":              "/callbacks/:id/",
		"callbacks:ratings":             "/ratings/",
		"callbacks:get_teams_by_region": "/get-teams-by-region/",
		"callbacks:api_callback_create": "/api/create/",
		"callbacks:export_excel":        "/export-excel/",
		"callbacks:vote_page":           "/vote/:uuid/",
		"callbacks:submit_vote":         "/vote/:uuid/submit/",
		"callbacks:vote_thanks":         "/vote/:uuid/thanks/",
		"users:login":                   "/users/login/",
		"users:logout":                  "/users/logout/",
		"teams:list":                    "/teams/",
		"teams:create":                  "/teams/create/",
		"teams:detail":                  "/teams/:id/",
		"teams:edit":                    "/teams/:id/edit/",
		"teams:delete":                  "/teams/:id/delete/",
		"teams:stats_api":               "/teams/stats-api/",
		"teams:region_list":             "/teams/regions/",
		"teams:region_create":           "/teams/regions/create/",
		"teams:region_detail":           "/teams/regions/:id/",
		"teams:region_edit":             "/teams/regions/:id/edit/",
		"teams:region_delete":           "/teams/regions/:id/delete/",
	} {
		templates.RegisterURL(k, v)
	}
}

func loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"dur", time.Since(start),
		)
	}
}
