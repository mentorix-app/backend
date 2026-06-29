package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"

	"mentorix-backend/internal/health"
	"mentorix-backend/internal/seed"
)

func main() {
	_ = godotenv.Load()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is not set. Add it to .env or export it.")
		os.Exit(1)
	}

	target := seed.DatabaseTargetLabel(databaseURL)
	fmt.Printf("Seed target: %s\n", target)

	if !seed.IsLocalDatabaseURL(databaseURL) {
		confirm := strings.TrimSpace(os.Getenv("SEED_CONFIRM"))
		if !strings.EqualFold(confirm, "yes") {
			fmt.Fprintln(os.Stderr, "Remote database detected. Re-run with SEED_CONFIRM=yes to proceed.")
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := health.NewPool(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "database pool: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "database ping: %v\n", err)
		os.Exit(1)
	}

	result, err := seed.Run(ctx, pool)
	if err != nil {
		fmt.Fprintf(os.Stderr, "seed failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Seed complete.")
	if result.UserCreated {
		fmt.Printf("  user: created %s\n", seed.DevEmail)
	} else {
		fmt.Printf("  user: already exists %s\n", seed.DevEmail)
	}
	fmt.Printf("  exercises: +%d (total %d)\n", result.ExercisesCreated, result.ExerciseTotal)
	fmt.Printf("  programs: +%d (total %d)\n", result.ProgramsCreated, result.ProgramTotal)
	fmt.Printf("  login: %s / %s\n", seed.DevEmail, seed.DevPassword)
	fmt.Println("  Postman: Login → List exercises / programs")
}
