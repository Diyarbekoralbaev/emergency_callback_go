package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/auth"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
	"github.com/jackc/pgx/v5"
)

type loginViewData struct {
	models.BaseData
	Error string
}

// LoginGET renders the login page (only if not already logged in).
func (s *Server) LoginGET(c *gin.Context) {
	bd := s.baseData(c)
	if bd.User.Authenticated {
		c.Redirect(http.StatusFound, "/")
		return
	}
	bd.CSRFField = csrf.TemplateField(c.Request)
	s.render(c, "users/login.html", loginViewData{BaseData: bd})
}

// LoginPOST validates credentials and creates a session.
func (s *Server) LoginPOST(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	password := c.PostForm("password")

	bd := s.baseData(c)
	renderError := func(msg string) {
		bd.CSRFField = csrf.TemplateField(c.Request)
		c.Status(http.StatusBadRequest)
		s.render(c, "users/login.html", loginViewData{BaseData: bd, Error: msg})
	}

	if username == "" || password == "" {
		renderError("Введите имя пользователя и пароль")
		return
	}

	user, err := s.Q.GetUserByUsername(c.Request.Context(), username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			renderError("Неверное имя пользователя или пароль")
			return
		}
		renderError("Ошибка базы данных")
		return
	}
	if !user.IsActive {
		renderError("Аккаунт неактивен")
		return
	}
	if !auth.VerifyPassword(user.Password, password) {
		renderError("Неверное имя пользователя или пароль")
		return
	}

	// Establish session
	if err := s.Session.RenewToken(c.Request.Context()); err != nil {
		renderError("Ошибка сессии")
		return
	}
	s.Session.Put(c.Request.Context(), auth.SessionKeyUserID, user.ID)
	s.Session.Put(c.Request.Context(), auth.SessionKeyRole, user.Role)
	s.Session.Put(c.Request.Context(), auth.SessionKeyUsername, user.Username)

	_ = s.Q.UpdateUserLastLogin(c.Request.Context(), user.ID)

	c.Redirect(http.StatusFound, "/")
}

// Logout clears the session.
func (s *Server) Logout(c *gin.Context) {
	_ = s.Session.Destroy(c.Request.Context())
	c.Redirect(http.StatusFound, "/users/login/")
}
