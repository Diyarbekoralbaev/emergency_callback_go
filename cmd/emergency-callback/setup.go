package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/setup"
)

func runSetup(args []string) {
	opts := setup.Options{Version: version}
	for _, a := range args {
		switch a {
		case "--non-interactive", "-n":
			opts.NonInteractive = true
		case "-h", "--help":
			fmt.Println(`emergency-callback setup [--non-interactive]

Interaktiv o'rnatish ustasi: muhitni aniqlaydi, PostgreSQL/servislarni
sozlaydi, .env'ni yangilaydi (mavjud qiymatlar saqlanadi), migratsiyalarni
qo'llaydi, admin yaratadi.

--non-interactive rejimida qiymatlar SETUP_<KEY> env o'zgaruvchilardan
olinadi (masalan SETUP_DB_NAME, SETUP_HTTP_PORT, SETUP_AMI_HOST).`)
			return
		default:
			fmt.Fprintf(os.Stderr, "noma'lum flag: %s\n", a)
			os.Exit(2)
		}
	}
	if err := setup.Run(context.Background(), opts); err != nil {
		fmt.Fprintf(os.Stderr, "\n ✗ setup: %v\n", err)
		os.Exit(1)
	}
}

func runDoctor() {
	results := setup.RunDoctor(context.Background())
	fmt.Println("Emergency Callback — doctor")
	if setup.PrintResults(results) {
		fmt.Println("\nHammasi joyida.")
	} else {
		fmt.Println("\nFAIL bor — yuqoridagi izohlarga qarang.")
		os.Exit(1)
	}
}
