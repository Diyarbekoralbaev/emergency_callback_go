package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/auth"
	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
)

func runCreateUser(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: createuser <username> <password> [admin|operator]")
		os.Exit(2)
	}
	username := args[0]
	password := args[1]
	role := "operator"
	if len(args) >= 3 {
		role = args[2]
	}
	if role != auth.RoleAdmin && role != auth.RoleOperator {
		fmt.Fprintln(os.Stderr, "role must be 'admin' or 'operator'")
		os.Exit(2)
	}

	ctx := context.Background()
	_, pool := loadCfgAndPool(ctx)
	defer pool.Close()

	q := sqlc.New(pool)

	hash, err := auth.HashPassword(password)
	if err != nil {
		slog.Error("hash password", "err", err)
		os.Exit(1)
	}

	user, err := q.CreateUser(ctx, sqlc.CreateUserParams{
		Username:    username,
		Password:    hash,
		Email:       "",
		FirstName:   "",
		LastName:    "",
		IsActive:    true,
		IsStaff:     role == auth.RoleAdmin,
		IsSuperuser: role == auth.RoleAdmin,
		Role:        role,
	})
	if err != nil {
		slog.Error("create user", "err", err)
		os.Exit(1)
	}
	fmt.Printf("Created user id=%d username=%q role=%s\n", user.ID, user.Username, user.Role)
}
