package handlers

import (
	"net/http"
	"strings"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/jobs"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// APICallbackCreate is the CSRF-exempt JSON endpoint. Replaces api_callback_create.
//   POST /api/create/  {"phone_number": "+998..."}
func (s *Server) APICallbackCreate(c *gin.Context) {
	ctx := c.Request.Context()
	var body struct {
		PhoneNumber string `json:"phone_number"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректные данные"})
		return
	}
	phone := strings.TrimSpace(body.PhoneNumber)
	if phone == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Некорректный номер телефона"})
		return
	}

	// Pick any user as requested_by (matches Django behaviour of `User.objects.first()`)
	users, err := s.Q.ListUsers(ctx)
	if err != nil || len(users) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Произошла системная ошибка"})
		return
	}
	user := users[0]

	team, err := s.Q.RandomActiveTeam(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Нет активных бригад"})
		return
	}

	var newID int64
	err = s.withTx(ctx, func(tx pgx.Tx, q *sqlc.Queries) error {
		row, err := q.CreateCallback(ctx, sqlc.CreateCallbackParams{
			PhoneNumber:   phone,
			TeamID:        team.ID,
			Status:        "pending",
			RequestedByID: user.ID,
		})
		if err != nil {
			return err
		}
		newID = row.ID
		_, err = s.River.InsertTx(ctx, tx, jobs.ProcessCallbackArgs{CallbackID: newID}, nil)
		return err
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Region name lookup
	regionName := ""
	if r, err := s.Q.GetRegion(ctx, team.RegionID); err == nil {
		regionName = r.Name
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"callback_id":  newID,
		"phone_number": phone,
		"team":         team.Name,
		"region":       regionName,
		"status":       "pending",
		"message":      "Экстренный вызов создан! Звоним на номер " + phone + "...",
	})
}
