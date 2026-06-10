package handlers

import (
	"context"
	"fmt"
	"io"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/xuri/excelize/v2"
)

// writeExcel builds a per-team summary spreadsheet and streams it to w.
// One row per active team with totals + per-star rating counts (matches
// callbacks/views.py:export_excel).
func (s *Server) writeExcel(w io.Writer, ctx context.Context) error {
	f := excelize.NewFile()
	defer f.Close()

	const sheet = "Отчет"
	idx, _ := f.NewSheet(sheet)
	f.SetActiveSheet(idx)
	_ = f.DeleteSheet("Sheet1")

	headers := []string{"№", "Регион - Бригада", "Всего вызовов", "Успешных", "Неудачных",
		"5★", "4★", "3★", "2★", "1★", "Средняя оценка"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}

	teams, err := s.Q.ListActiveTeams(ctx)
	if err != nil {
		return err
	}

	rowNum := 2
	for i, t := range teams {
		ratings, _ := s.Q.RatingsByTeam(ctx, t.ID)
		ratingCounts := map[int32]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
		sum := 0
		for _, r := range ratings {
			ratingCounts[r.Rating]++
			sum += int(r.Rating)
		}
		avg := 0.0
		if len(ratings) > 0 {
			avg = float64(sum) / float64(len(ratings))
		}

		// counts per team
		var total, success, failed int
		cbs, _ := s.Q.ListCallbacksByTeam(ctx, sqlc.ListCallbacksByTeamParams{TeamID: t.ID, Limit: 10000, Offset: 0})
		for _, c := range cbs {
			total++
			switch c.Status {
			case "completed", "transferred":
				success++
			case "failed":
				failed++
			}
		}

		row := []any{
			i + 1,
			fmt.Sprintf("%s - %s", t.RegionName, t.Name),
			total, success, failed,
			ratingCounts[5], ratingCounts[4], ratingCounts[3], ratingCounts[2], ratingCounts[1],
			fmt.Sprintf("%.2f", avg),
		}
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, rowNum)
			_ = f.SetCellValue(sheet, cell, v)
		}
		rowNum++
	}

	return f.Write(w)
}
