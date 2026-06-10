package handlers

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/auth"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/config"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/models"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/templates"
	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// Server bundles dependencies for HTTP handlers.
type Server struct {
	Cfg       *config.Config
	Pool      *pgxpool.Pool
	Q         *sqlc.Queries
	Session   *scs.SessionManager
	Templates *templates.Registry
	River     *river.Client[pgx.Tx]
}

// baseData returns a *populated* BaseData for the given request — pulls user
// info from the session and CSRF field from gorilla/csrf.
func (s *Server) baseData(c *gin.Context) models.BaseData {
	bd := models.BaseData{
		CSRFField: csrf.TemplateField(c.Request),
	}
	id, role, username, ok := auth.CurrentUser(s.Session, c)
	if ok {
		bd.User = models.SessionUser{
			ID:            id,
			Username:      username,
			Role:          role,
			Authenticated: true,
		}
	}
	// Flash messages from session
	if msgs := s.Session.Pop(c.Request.Context(), "flash"); msgs != nil {
		if list, ok := msgs.([]models.FlashMessage); ok {
			bd.Messages = list
		}
	}
	return bd
}

// pushFlash adds a flash message to the session for the next request.
func (s *Server) pushFlash(c *gin.Context, level, text string) {
	existing := s.Session.Get(c.Request.Context(), "flash")
	var list []models.FlashMessage
	if existing != nil {
		if l, ok := existing.([]models.FlashMessage); ok {
			list = l
		}
	}
	list = append(list, models.FlashMessage{Level: level, Text: text})
	s.Session.Put(c.Request.Context(), "flash", list)
}

// render writes a layout template (or returns a 500 on error).
func (s *Server) render(c *gin.Context, name string, data any) {
	var buf bytes.Buffer
	if err := s.Templates.Render(&buf, name, data); err != nil {
		slog.Error("render", "name", name, "err", err)
		c.String(http.StatusInternalServerError, "template error: %v", err)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", buf.Bytes())
}

// renderStandalone writes a vote/login page (no base layout).
func (s *Server) renderStandalone(c *gin.Context, name string, data any) {
	var buf bytes.Buffer
	if err := s.Templates.RenderStandalone(&buf, name, data); err != nil {
		slog.Error("render standalone", "name", name, "err", err)
		c.String(http.StatusInternalServerError, "template error: %v", err)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", buf.Bytes())
}

// withTx runs fn inside a pgx transaction; commits on nil error.
func (s *Server) withTx(ctx context.Context, fn func(tx pgx.Tx, q *sqlc.Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx, s.Q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
