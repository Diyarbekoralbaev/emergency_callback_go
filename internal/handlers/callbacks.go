package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/jobs"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type callbackListData struct {
	models.BaseData
	Callbacks      []models.CallbackView
	Regions        []models.RegionView
	Teams          []models.TeamView
	Statuses       []statusOpt
	Search         string
	CurrentRegion  int64
	CurrentTeam    int64
	CurrentStatus  string
	DateFrom       string
	DateTo         string
	TotalCount     int64
	ShowingCount   int
}

type statusOpt struct {
	Value string
	Label string
}

func allStatuses() []statusOpt {
	keys := []string{"pending", "dialing", "connecting", "answered", "waiting_rating", "waiting_additional", "transferring", "completed", "no_rating", "transferred", "failed"}
	out := make([]statusOpt, 0, len(keys))
	for _, k := range keys {
		out = append(out, statusOpt{Value: k, Label: models.StatusLabel(k)})
	}
	return out
}

func (s *Server) CallbackList(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)

	regionFilter, _ := strconv.ParseInt(c.Query("region"), 10, 64)
	teamFilter, _ := strconv.ParseInt(c.Query("team"), 10, 64)
	statusFilter := c.Query("status")
	search := strings.TrimSpace(c.Query("search"))

	rows, err := s.Q.ListCallbacks(ctx, sqlc.ListCallbacksParams{Limit: 100, Offset: 0})
	if err != nil {
		c.String(http.StatusInternalServerError, "%v", err)
		return
	}
	callbacks := make([]models.CallbackView, 0, len(rows))
	for _, r := range rows {
		if regionFilter > 0 || teamFilter > 0 || statusFilter != "" || search != "" {
			if statusFilter != "" && r.Status != statusFilter {
				continue
			}
			if teamFilter > 0 && r.TeamID != teamFilter {
				continue
			}
			if search != "" && !strings.Contains(r.PhoneNumber, search) {
				continue
			}
		}
		callbacks = append(callbacks, s.callbackToView(ctx, r))
	}

	total, _ := s.Q.CountCallbacks(ctx)
	regs, _ := s.Q.ListActiveRegions(ctx)
	teams, _ := s.Q.ListActiveTeams(ctx)

	data := callbackListData{
		BaseData:      bd,
		Callbacks:     callbacks,
		Regions:       toRegionViews(regs),
		Teams:         toTeamViewsFromList(teams),
		Statuses:      allStatuses(),
		Search:        search,
		CurrentRegion: regionFilter,
		CurrentTeam:   teamFilter,
		CurrentStatus: statusFilter,
		DateFrom:      c.Query("date_from"),
		DateTo:        c.Query("date_to"),
		TotalCount:    total,
		ShowingCount:  len(callbacks),
	}
	s.render(c, "callbacks/list.html", data)
}

type callbackCreateData struct {
	models.BaseData
	Regions          []models.RegionView
	Teams            []models.TeamView
	SelectedRegion   int64
	SelectedTeam     int64
	PhoneNumber      string
	FormErrors       []string
	TodayCalls       int64
	TodayCompleted   int64
	TodayFailed      int64
	TodayRatings     int64
	TodaySuccessRate int
}

func (s *Server) CallbackCreateGET(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	regs, _ := s.Q.ListActiveRegions(ctx)
	teams, _ := s.Q.ListActiveTeams(ctx)
	s.render(c, "callbacks/form.html", callbackCreateData{
		BaseData: bd,
		Regions:  toRegionViews(regs),
		Teams:    toTeamViewsFromList(teams),
	})
}

func (s *Server) CallbackCreatePOST(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)

	teamID, _ := strconv.ParseInt(c.PostForm("team"), 10, 64)
	regionID, _ := strconv.ParseInt(c.PostForm("region"), 10, 64)
	phone := strings.TrimSpace(c.PostForm("phone_number"))

	var errs []string
	if phone == "" {
		errs = append(errs, "Номер телефона обязателен")
	}
	if teamID == 0 {
		errs = append(errs, "Выберите бригаду")
	}
	if len(errs) > 0 {
		regs, _ := s.Q.ListActiveRegions(ctx)
		teams, _ := s.Q.ListActiveTeams(ctx)
		s.render(c, "callbacks/form.html", callbackCreateData{
			BaseData:       bd,
			Regions:        toRegionViews(regs),
			Teams:          toTeamViewsFromList(teams),
			SelectedRegion: regionID,
			SelectedTeam:   teamID,
			PhoneNumber:    phone,
			FormErrors:     errs,
		})
		return
	}

	// Insert callback inside a transaction, enqueue River job atomically.
	var newID int64
	err := s.withTx(ctx, func(tx pgx.Tx, q *sqlc.Queries) error {
		row, err := q.CreateCallback(ctx, sqlc.CreateCallbackParams{
			PhoneNumber:   phone,
			TeamID:        teamID,
			Status:        "pending",
			RequestedByID: bd.User.ID,
		})
		if err != nil {
			return err
		}
		newID = row.ID
		_, err = s.River.InsertTx(ctx, tx, jobs.ProcessCallbackArgs{CallbackID: newID}, nil)
		return err
	})
	if err != nil {
		s.pushFlash(c, "danger", "Ошибка создания вызова: "+err.Error())
		c.Redirect(http.StatusFound, "/callbacks/create/")
		return
	}
	s.pushFlash(c, "success", "Экстренный вызов создан! Звоним на номер "+phone+"...")
	c.Redirect(http.StatusFound, "/callbacks/")
}

type callbackDetailData struct {
	models.BaseData
	Callback models.CallbackView
}

func (s *Server) CallbackDetail(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	row, err := s.Q.GetCallback(ctx, id)
	if err != nil {
		c.String(http.StatusNotFound, "вызов не найден")
		return
	}
	s.render(c, "callbacks/detail.html", callbackDetailData{
		BaseData: bd,
		Callback: s.callbackToView(ctx, row),
	})
}

// TeamsByRegion returns JSON list of teams in the given region.
func (s *Server) TeamsByRegion(c *gin.Context) {
	regionID, _ := strconv.ParseInt(c.Query("region_id"), 10, 64)
	if regionID == 0 {
		c.JSON(http.StatusOK, gin.H{"teams": []any{}})
		return
	}
	teams, _ := s.Q.ListTeamsByRegion(c.Request.Context(), regionID)
	out := make([]gin.H, 0, len(teams))
	for _, t := range teams {
		out = append(out, gin.H{"id": t.ID, "name": t.Name})
	}
	c.JSON(http.StatusOK, gin.H{"teams": out})
}

// callbackToView converts a sqlc CallbackRequest row into a template-friendly view.
func (s *Server) callbackToView(ctx context.Context, r sqlc.CallbacksCallbackrequest) models.CallbackView {
	v := models.CallbackView{
		ID:                  r.ID,
		PhoneNumber:         r.PhoneNumber,
		TeamID:              r.TeamID,
		Status:              r.Status,
		StatusLabel:         models.StatusLabel(r.Status),
		StatusColor:         models.StatusColor(r.Status),
		CreatedAt:           r.CreatedAt.Time,
		CallStartedAt:       r.CallStartedAt.Time,
		CallEndedAt:         r.CallEndedAt.Time,
		CallDuration:        r.CallDuration,
		DurationFormatted:   models.DurationFormatted(r.CallDuration),
		Transferred:         r.Transferred,
		SmsSent:             r.SmsSent,
		SmsSentAt:           r.SmsSentAt.Time,
		VotedViaSMS:         r.VotedViaSms,
	}
	if r.ErrorMessage != nil {
		v.ErrorMessage = *r.ErrorMessage
	}
	if r.CallID.Valid {
		v.CallID = uuid.UUID(r.CallID.Bytes).String()
	}
	if r.VoteUuid.Valid {
		v.VoteUUID = uuid.UUID(r.VoteUuid.Bytes).String()
		v.VoteURL = s.Cfg.SiteDomain + "/vote/" + v.VoteUUID
	}
	// Decorate with team + region names
	if team, err := s.Q.GetTeamWithRegion(ctx, r.TeamID); err == nil {
		v.TeamName = team.Name
		v.RegionName = team.RegionName
	}
	// Rating
	if rating, err := s.Q.GetRatingByCallback(ctx, r.ID); err == nil {
		v.HasRating = true
		v.Rating = &rating.Rating
		v.RatingStars = models.RatingStars(rating.Rating)
		v.RatingText = models.RatingText(rating.Rating)
		v.RatingColor = models.RatingColor(rating.Rating)
		v.RatingDate = rating.Timestamp.Time
	}
	// Requested by username
	if user, err := s.Q.GetUser(ctx, r.RequestedByID); err == nil {
		v.RequestedByUsername = user.Username
	}
	return v
}

func toRegionViews(rows []sqlc.TeamsRegion) []models.RegionView {
	out := make([]models.RegionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, models.RegionView{
			ID: r.ID, Name: r.Name, Code: r.Code, Description: r.Description,
			IsActive: r.IsActive, CreatedAt: r.CreatedAt.Time,
		})
	}
	return out
}

func toTeamViewsFromList(rows []sqlc.ListActiveTeamsRow) []models.TeamView {
	out := make([]models.TeamView, 0, len(rows))
	for _, r := range rows {
		out = append(out, models.TeamView{
			ID: r.ID, Name: r.Name, Description: r.Description,
			RegionID: r.RegionID, RegionName: r.RegionName,
			IsActive: r.IsActive, CreatedAt: r.CreatedAt.Time,
		})
	}
	return out
}
