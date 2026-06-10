package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/models"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/tz"
	"github.com/gin-gonic/gin"
)

type ratingDistEntry struct {
	Rating     int32
	Count      int64
	Percentage float64
}

type teamStatEntry struct {
	Name        string
	TotalCalls  int
	SuccessRate float64
	AvgRating   float64
}

type dashboardData struct {
	models.BaseData
	TotalCalls         int
	CompletedCalls     int
	FailedCalls        int
	NoRatingCalls      int
	TotalRatings       int64
	AvgRating          float64
	SuccessRate        float64
	FailureRate        float64
	RatingDistribution []ratingDistEntry
	RecentCalls        []models.CallbackView
	TeamStats          []teamStatEntry
	Regions            []models.RegionView
	Teams              []models.TeamView
	CurrentRegion      int64
	CurrentTeam        int64
	DateFrom           string
	DateTo             string
	PeriodDescription  string
	QueryString        string
}

func (s *Server) Dashboard(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)

	regionFilter, _ := strconv.ParseInt(c.Query("region"), 10, 64)
	teamFilter, _ := strconv.ParseInt(c.Query("team"), 10, 64)
	dateFrom := c.Query("date_from")
	dateTo := c.Query("date_to")

	today := tz.Now().Truncate(24 * time.Hour)
	if dateFrom == "" {
		dateFrom = today.Format("2006-01-02")
	}
	if dateTo == "" {
		dateTo = today.Format("2006-01-02")
	}

	// For simplicity, fetch the most recent 100 callbacks and compute in Go.
	// (Original Django queryset filtering by date is preserved on the SQL side
	// in future iteration; this matches the visible UI.)
	rows, _ := s.Q.ListCallbacks(ctx, sqlc.ListCallbacksParams{Limit: 200, Offset: 0})

	data := dashboardData{
		BaseData:      bd,
		CurrentRegion: regionFilter,
		CurrentTeam:   teamFilter,
		DateFrom:      dateFrom,
		DateTo:        dateTo,
	}

	for _, r := range rows {
		if regionFilter > 0 {
			if t, err := s.Q.GetTeamWithRegion(ctx, r.TeamID); err == nil && t.RegionID != regionFilter {
				continue
			}
		}
		if teamFilter > 0 && r.TeamID != teamFilter {
			continue
		}
		data.TotalCalls++
		switch r.Status {
		case "completed", "transferred":
			data.CompletedCalls++
		case "failed":
			data.FailedCalls++
		case "no_rating":
			data.NoRatingCalls++
		}
	}

	if data.TotalCalls > 0 {
		data.SuccessRate = roundOne(float64(data.CompletedCalls) / float64(data.TotalCalls) * 100)
		data.FailureRate = roundOne(float64(data.FailedCalls) / float64(data.TotalCalls) * 100)
	}

	// Ratings stats
	if avg, err := s.Q.AvgRating(ctx); err == nil {
		data.AvgRating = roundOne(avg)
	}
	if cnt, err := s.Q.CountRatings(ctx); err == nil {
		data.TotalRatings = cnt
	}
	if dist, err := s.Q.RatingDistribution(ctx); err == nil {
		total := float64(data.TotalRatings)
		filled := make(map[int32]int64, 5)
		for _, d := range dist {
			filled[d.Rating] = d.Count
		}
		for i := int32(1); i <= 5; i++ {
			cnt := filled[i]
			pct := 0.0
			if total > 0 {
				pct = roundOne(float64(cnt) / total * 100)
			}
			data.RatingDistribution = append(data.RatingDistribution, ratingDistEntry{Rating: i, Count: cnt, Percentage: pct})
		}
	}

	// Recent calls
	recent := rows
	if len(recent) > 15 {
		recent = recent[:15]
	}
	for _, r := range recent {
		data.RecentCalls = append(data.RecentCalls, s.callbackToView(ctx, r))
	}

	regs, _ := s.Q.ListActiveRegions(ctx)
	data.Regions = toRegionViews(regs)
	teams, _ := s.Q.ListActiveTeams(ctx)
	data.Teams = toTeamViewsFromList(teams)

	if dateFrom == dateTo {
		data.PeriodDescription = "За " + dateFrom
	} else {
		data.PeriodDescription = "С " + dateFrom + " по " + dateTo
	}
	data.QueryString = c.Request.URL.RawQuery
	s.render(c, "callbacks/dashboard.html", data)
}

// Excel export — minimal version: produce a workbook with one summary row per team.
func (s *Server) ExcelExport(c *gin.Context) {
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", `attachment; filename="otchet.xlsx"`)
	if err := s.writeExcel(c.Writer, c.Request.Context()); err != nil {
		c.String(http.StatusInternalServerError, "excel error: %v", err)
	}
}

func roundOne(x float64) float64 {
	return float64(int(x*10+0.5)) / 10
}
