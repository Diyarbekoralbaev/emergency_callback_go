package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/models"
	"github.com/gin-gonic/gin"
)

type teamListData struct {
	models.BaseData
	Teams         []models.TeamView
	Regions       []models.RegionView
	Search        string
	CurrentRegion int64
	ShowInactive  bool
}

func (s *Server) TeamList(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	search := strings.TrimSpace(c.Query("search"))
	regionFilter, _ := strconv.ParseInt(c.Query("region"), 10, 64)
	showInactive := c.Query("show_inactive") == "1"

	rows, _ := s.Q.ListTeams(ctx)
	out := make([]models.TeamView, 0, len(rows))
	for _, r := range rows {
		if !showInactive && !r.IsActive {
			continue
		}
		if regionFilter > 0 && r.RegionID != regionFilter {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(r.Name), strings.ToLower(search)) &&
			!strings.Contains(strings.ToLower(r.RegionName), strings.ToLower(search)) {
			continue
		}
		out = append(out, models.TeamView{
			ID: r.ID, Name: r.Name, Description: r.Description,
			RegionID: r.RegionID, RegionName: r.RegionName,
			IsActive: r.IsActive, CreatedAt: r.CreatedAt.Time,
		})
	}

	regs, _ := s.Q.ListActiveRegions(ctx)
	s.render(c, "teams/list.html", teamListData{
		BaseData:      bd,
		Teams:         out,
		Regions:       toRegionViews(regs),
		Search:        search,
		CurrentRegion: regionFilter,
		ShowInactive:  showInactive,
	})
}

type teamFormData struct {
	models.BaseData
	Team       models.TeamView
	Regions    []models.RegionView
	IsEdit     bool
	FormErrors []string
}

func (s *Server) TeamCreateGET(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	regs, _ := s.Q.ListActiveRegions(ctx)
	s.render(c, "teams/form.html", teamFormData{
		BaseData: bd,
		Team:     models.TeamView{IsActive: true},
		Regions:  toRegionViews(regs),
		IsEdit:   false,
	})
}

func (s *Server) TeamCreatePOST(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	name := strings.TrimSpace(c.PostForm("name"))
	regionID, _ := strconv.ParseInt(c.PostForm("region"), 10, 64)
	description := c.PostForm("description")
	isActive := c.PostForm("is_active") == "1"

	var errs []string
	if name == "" {
		errs = append(errs, "Название обязательно")
	}
	if regionID == 0 {
		errs = append(errs, "Регион обязателен")
	}
	if len(errs) > 0 {
		regs, _ := s.Q.ListActiveRegions(ctx)
		s.render(c, "teams/form.html", teamFormData{
			BaseData:   bd,
			Team:       models.TeamView{Name: name, RegionID: regionID, Description: description, IsActive: isActive},
			Regions:    toRegionViews(regs),
			FormErrors: errs,
		})
		return
	}

	_, err := s.Q.CreateTeam(ctx, sqlc.CreateTeamParams{
		Name:        name,
		Description: description,
		RegionID:    regionID,
		IsActive:    isActive,
		CreatedByID: bd.User.ID,
	})
	if err != nil {
		s.pushFlash(c, "danger", "Ошибка создания бригады: "+err.Error())
		c.Redirect(http.StatusFound, "/teams/create/")
		return
	}
	s.pushFlash(c, "success", "Бригада создана")
	c.Redirect(http.StatusFound, "/teams/")
}

func (s *Server) TeamDetail(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	t, err := s.Q.GetTeamWithRegion(ctx, id)
	if err != nil {
		c.String(http.StatusNotFound, "не найдено")
		return
	}
	s.render(c, "teams/detail.html", struct {
		models.BaseData
		Team models.TeamView
	}{
		BaseData: bd,
		Team: models.TeamView{
			ID: t.ID, Name: t.Name, Description: t.Description,
			RegionID: t.RegionID, RegionName: t.RegionName,
			IsActive: t.IsActive, CreatedAt: t.CreatedAt.Time,
		},
	})
}

func (s *Server) TeamEditGET(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	t, err := s.Q.GetTeamWithRegion(ctx, id)
	if err != nil {
		c.String(http.StatusNotFound, "не найдено")
		return
	}
	regs, _ := s.Q.ListActiveRegions(ctx)
	s.render(c, "teams/form.html", teamFormData{
		BaseData: bd,
		Team: models.TeamView{
			ID: t.ID, Name: t.Name, Description: t.Description,
			RegionID: t.RegionID, RegionName: t.RegionName,
			IsActive: t.IsActive,
		},
		Regions: toRegionViews(regs),
		IsEdit:  true,
	})
}

func (s *Server) TeamEditPOST(c *gin.Context) {
	ctx := c.Request.Context()
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	_, err := s.Q.UpdateTeam(ctx, sqlc.UpdateTeamParams{
		ID:          id,
		Name:        strings.TrimSpace(c.PostForm("name")),
		Description: c.PostForm("description"),
		RegionID:    parseInt64(c.PostForm("region")),
		IsActive:    c.PostForm("is_active") == "1",
	})
	if err != nil {
		s.pushFlash(c, "danger", "Ошибка обновления: "+err.Error())
		c.Redirect(http.StatusFound, c.Request.URL.Path)
		return
	}
	s.pushFlash(c, "success", "Бригада обновлена")
	c.Redirect(http.StatusFound, "/teams/")
}

func (s *Server) TeamDeleteGET(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	t, err := s.Q.GetTeamWithRegion(ctx, id)
	if err != nil {
		c.String(http.StatusNotFound, "не найдено")
		return
	}
	s.render(c, "teams/delete.html", struct {
		models.BaseData
		Team models.TeamView
	}{
		BaseData: bd,
		Team: models.TeamView{
			ID: t.ID, Name: t.Name, RegionName: t.RegionName,
		},
	})
}

func (s *Server) TeamDeletePOST(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := s.Q.DeleteTeam(c.Request.Context(), id); err != nil {
		s.pushFlash(c, "danger", "Ошибка удаления: "+err.Error())
		c.Redirect(http.StatusFound, "/teams/")
		return
	}
	s.pushFlash(c, "success", "Бригада удалена")
	c.Redirect(http.StatusFound, "/teams/")
}

// TeamStatsAPI — JSON endpoint for team-level stats (legacy).
func (s *Server) TeamStatsAPI(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}
