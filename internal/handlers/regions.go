package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/models"
	"github.com/gin-gonic/gin"
)

type regionListData struct {
	models.BaseData
	Regions []models.RegionView
	Search  string
}

func (s *Server) RegionList(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	rows, _ := s.Q.ListRegions(ctx)
	out := make([]models.RegionView, 0, len(rows))
	for _, r := range rows {
		count, _ := s.Q.RegionTeamCount(ctx, r.ID)
		total, _ := s.Q.RegionTotalTeamCount(ctx, r.ID)
		out = append(out, models.RegionView{
			ID: r.ID, Name: r.Name, Code: r.Code,
			Description: r.Description, IsActive: r.IsActive,
			CreatedAt: r.CreatedAt.Time,
			TeamCount: count, TotalTeamCount: total,
		})
	}
	s.render(c, "teams/regions/list.html", regionListData{
		BaseData: bd,
		Regions:  out,
	})
}

type regionFormData struct {
	models.BaseData
	Region     models.RegionView
	IsEdit     bool
	FormErrors []string
}

func (s *Server) RegionCreateGET(c *gin.Context) {
	bd := s.baseData(c)
	s.render(c, "teams/regions/form.html", regionFormData{
		BaseData: bd,
		Region:   models.RegionView{IsActive: true},
	})
}

func (s *Server) RegionCreatePOST(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	name := strings.TrimSpace(c.PostForm("name"))
	code := strings.TrimSpace(c.PostForm("code"))
	description := c.PostForm("description")
	isActive := c.PostForm("is_active") == "1"

	var errs []string
	if name == "" {
		errs = append(errs, "Название обязательно")
	}
	if code == "" {
		errs = append(errs, "Код обязателен")
	}
	if len(errs) > 0 {
		s.render(c, "teams/regions/form.html", regionFormData{
			BaseData:   bd,
			Region:     models.RegionView{Name: name, Code: code, Description: description, IsActive: isActive},
			FormErrors: errs,
		})
		return
	}

	_, err := s.Q.CreateRegion(ctx, sqlc.CreateRegionParams{
		Name:        name,
		Code:        code,
		Description: description,
		IsActive:    isActive,
		CreatedByID: bd.User.ID,
	})
	if err != nil {
		s.pushFlash(c, "danger", "Ошибка создания региона: "+err.Error())
		c.Redirect(http.StatusFound, "/teams/regions/create/")
		return
	}
	s.pushFlash(c, "success", "Регион создан")
	c.Redirect(http.StatusFound, "/teams/regions/")
}

func (s *Server) RegionDetail(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	r, err := s.Q.GetRegion(ctx, id)
	if err != nil {
		c.String(http.StatusNotFound, "не найдено")
		return
	}
	teams, _ := s.Q.ListTeamsByRegion(ctx, id)
	teamViews := make([]models.TeamView, 0, len(teams))
	for _, t := range teams {
		teamViews = append(teamViews, models.TeamView{
			ID: t.ID, Name: t.Name, IsActive: t.IsActive,
			RegionID: t.RegionID, RegionName: t.RegionName,
		})
	}
	count, _ := s.Q.RegionTeamCount(ctx, id)
	total, _ := s.Q.RegionTotalTeamCount(ctx, id)
	s.render(c, "teams/regions/detail.html", struct {
		models.BaseData
		Region models.RegionView
		Teams  []models.TeamView
	}{
		BaseData: bd,
		Region: models.RegionView{
			ID: r.ID, Name: r.Name, Code: r.Code,
			Description: r.Description, IsActive: r.IsActive,
			CreatedAt: r.CreatedAt.Time,
			TeamCount: count, TotalTeamCount: total,
		},
		Teams: teamViews,
	})
}

func (s *Server) RegionEditGET(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	r, err := s.Q.GetRegion(ctx, id)
	if err != nil {
		c.String(http.StatusNotFound, "не найдено")
		return
	}
	s.render(c, "teams/regions/form.html", regionFormData{
		BaseData: bd,
		Region: models.RegionView{
			ID: r.ID, Name: r.Name, Code: r.Code,
			Description: r.Description, IsActive: r.IsActive,
		},
		IsEdit: true,
	})
}

func (s *Server) RegionEditPOST(c *gin.Context) {
	ctx := c.Request.Context()
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	_, err := s.Q.UpdateRegion(ctx, sqlc.UpdateRegionParams{
		ID:          id,
		Name:        strings.TrimSpace(c.PostForm("name")),
		Code:        strings.TrimSpace(c.PostForm("code")),
		Description: c.PostForm("description"),
		IsActive:    c.PostForm("is_active") == "1",
	})
	if err != nil {
		s.pushFlash(c, "danger", "Ошибка обновления: "+err.Error())
		c.Redirect(http.StatusFound, c.Request.URL.Path)
		return
	}
	s.pushFlash(c, "success", "Регион обновлен")
	c.Redirect(http.StatusFound, "/teams/regions/")
}

func (s *Server) RegionDeleteGET(c *gin.Context) {
	ctx := c.Request.Context()
	bd := s.baseData(c)
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	r, err := s.Q.GetRegion(ctx, id)
	if err != nil {
		c.String(http.StatusNotFound, "не найдено")
		return
	}
	total, _ := s.Q.RegionTotalTeamCount(ctx, id)
	s.render(c, "teams/regions/delete.html", struct {
		models.BaseData
		Region models.RegionView
	}{
		BaseData: bd,
		Region: models.RegionView{
			ID: r.ID, Name: r.Name, TotalTeamCount: total,
		},
	})
}

func (s *Server) RegionDeletePOST(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := s.Q.DeleteRegion(c.Request.Context(), id); err != nil {
		s.pushFlash(c, "danger", "Ошибка удаления: "+err.Error())
		c.Redirect(http.StatusFound, "/teams/regions/")
		return
	}
	s.pushFlash(c, "success", "Регион удален")
	c.Redirect(http.StatusFound, "/teams/regions/")
}
