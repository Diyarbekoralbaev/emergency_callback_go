package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
)

func runSeed() {
	ctx := context.Background()
	_, pool := loadCfgAndPool(ctx)
	defer pool.Close()

	q := sqlc.New(pool)

	users, err := q.ListUsers(ctx)
	if err != nil || len(users) == 0 {
		fmt.Fprintln(os.Stderr, "no users found — run 'createuser admin <pass> admin' first")
		os.Exit(1)
	}
	admin := users[0]
	for _, u := range users {
		if u.Role == "admin" {
			admin = u
			break
		}
	}

	regions := []struct {
		Name, Code string
	}{
		{"Каракалпакстан", "KK"},
		{"Ташкентская область", "TK"},
	}
	regionIDs := make([]int64, 0, len(regions))
	for _, r := range regions {
		row, err := q.CreateRegion(ctx, sqlc.CreateRegionParams{
			Name:        r.Name,
			Code:        r.Code,
			Description: "",
			IsActive:    true,
			CreatedByID: admin.ID,
		})
		if err != nil {
			slog.Warn("region exists or failed", "name", r.Name, "err", err)
			continue
		}
		regionIDs = append(regionIDs, row.ID)
	}

	teamNames := []string{"Бригада 1", "Бригада 2"}
	for _, regID := range regionIDs {
		for _, tn := range teamNames {
			_, err := q.CreateTeam(ctx, sqlc.CreateTeamParams{
				Name:        tn,
				Description: "",
				RegionID:    regID,
				IsActive:    true,
				CreatedByID: admin.ID,
			})
			if err != nil {
				slog.Warn("team failed", "team", tn, "region", regID, "err", err)
			}
		}
	}
	fmt.Println("seed complete")
}
