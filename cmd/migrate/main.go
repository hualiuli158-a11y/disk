package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"disk/internal/database"
	"disk/pkg/setting"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: go run ./cmd/migrate <up|down|version|baseline> [steps]")
	}
	config, err := setting.LoadDatabaseConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := database.OpenMySQL(config)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	runner, err := database.NewMigrationRunner(sqlDB)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	switch os.Args[1] {
	case "up":
		if err := runner.Up(ctx); err != nil {
			return err
		}
		version, _, err := runner.Version(ctx)
		if err == nil {
			fmt.Printf("database migrated to version %d\n", version)
		}
		return err
	case "down":
		steps := 1
		if len(os.Args) > 2 {
			steps, err = strconv.Atoi(os.Args[2])
			if err != nil || steps <= 0 {
				return fmt.Errorf("down steps must be a positive integer")
			}
		}
		if err := runner.Down(ctx, steps); err != nil {
			return err
		}
		version, _, err := runner.Version(ctx)
		if err == nil {
			fmt.Printf("database rolled back to version %d\n", version)
		}
		return err
	case "version":
		version, dirty, err := runner.Version(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("version=%d dirty=%t\n", version, dirty)
		return nil
	case "baseline":
		if err := runner.Baseline(ctx); err != nil {
			return err
		}
		version, _, err := runner.Version(ctx)
		if err == nil {
			fmt.Printf("existing database baselined at version %d\n", version)
		}
		return err
	default:
		return fmt.Errorf("unknown migration command %q", os.Args[1])
	}
}
