package handlers

import (
	"strconv"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/models"
	"github.com/gin-gonic/gin"
)

type ratingsListData struct {
	models.BaseData
	Ratings        []models.RatingView
	Regions        []models.RegionView
	Teams          []models.TeamView
	CurrentRegion  int64
	CurrentTeam    int64
	CurrentRating  string
	DateFrom       string
	DateTo         string
	TotalRatings   int64
	AvgRating      float64
	GoodPercentage float64
}

func (s *Server) RatingsList(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)

	regionFilter, _ := strconv.ParseInt(c.Query("region"), 10, 64)
	teamFilter, _ := strconv.ParseInt(c.Query("team"), 10, 64)
	ratingFilter := c.Query("rating")

	rows, _ := s.Q.ListRatings(ctx, sqlc.ListRatingsParams{Limit: 100, Offset: 0})
	out := make([]models.RatingView, 0, len(rows))
	goodCount := 0
	sum := 0
	teamCache := map[int64]struct {
		name, region string
	}{}

	for _, r := range rows {
		if teamFilter > 0 && r.TeamID != teamFilter {
			continue
		}
		if ratingFilter != "" {
			if v, err := strconv.Atoi(ratingFilter); err == nil && int(r.Rating) != v {
				continue
			}
		}
		// Lookup team + region (cached)
		var teamName, regionName string
		if cached, ok := teamCache[r.TeamID]; ok {
			teamName, regionName = cached.name, cached.region
		} else {
			if t, err := s.Q.GetTeamWithRegion(ctx, r.TeamID); err == nil {
				teamName = t.Name
				regionName = t.RegionName
				if regionFilter > 0 && t.RegionID != regionFilter {
					teamCache[r.TeamID] = struct{ name, region string }{teamName, regionName}
					continue
				}
				teamCache[r.TeamID] = struct{ name, region string }{teamName, regionName}
			}
		}

		out = append(out, models.RatingView{
			ID:                r.ID,
			Rating:            r.Rating,
			Stars:             models.RatingStars(r.Rating),
			Color:             models.RatingColor(r.Rating),
			PhoneNumber:       r.PhoneNumber,
			TeamName:          teamName,
			RegionName:        regionName,
			Timestamp:         r.Timestamp.Time,
			CallbackRequestID: r.CallbackRequestID,
		})
		sum += int(r.Rating)
		if r.Rating >= 4 {
			goodCount++
		}
	}

	totalRatings, _ := s.Q.CountRatings(ctx)
	avg, _ := s.Q.AvgRating(ctx)
	good := 0.0
	if len(out) > 0 {
		good = roundOne(float64(goodCount) / float64(len(out)) * 100)
	}

	regs, _ := s.Q.ListActiveRegions(ctx)
	teams, _ := s.Q.ListActiveTeams(ctx)

	s.render(c, "callbacks/ratings.html", ratingsListData{
		BaseData:       bd,
		Ratings:        out,
		Regions:        toRegionViews(regs),
		Teams:          toTeamViewsFromList(teams),
		CurrentRegion:  regionFilter,
		CurrentTeam:    teamFilter,
		CurrentRating:  ratingFilter,
		DateFrom:       c.Query("date_from"),
		DateTo:         c.Query("date_to"),
		TotalRatings:   totalRatings,
		AvgRating:      roundOne(avg),
		GoodPercentage: good,
	})
}
