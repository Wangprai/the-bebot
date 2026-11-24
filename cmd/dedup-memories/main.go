package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"

	memorymongodb "the-bebot/internal/memory/adapter/mongodb"
	"the-bebot/internal/platform/database"
)

func main() {
	var (
		guildID string
		apply   bool
		timeout time.Duration
	)

	flag.StringVar(&guildID, "guild", "", "optional guild ID to scope deduplication (empty = all guilds)")
	flag.BoolVar(&apply, "apply", false, "apply changes to MongoDB (default is dry-run)")
	flag.DurationVar(&timeout, "timeout", 2*time.Minute, "operation timeout")
	flag.Parse()

	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("load .env: %v", err)
	}

	uri := os.Getenv("MONGODB_URI")
	if uri == "" {
		uri = "mongodb://127.0.0.1:27017"
	}
	dbName := os.Getenv("MONGODB_DATABASE")
	if dbName == "" {
		dbName = "the_bebot"
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	db, err := database.NewMongoDB(ctx, uri, dbName)
	if err != nil {
		log.Fatalf("connect MongoDB: %v", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = db.Close(closeCtx)
	}()

	repo, err := memorymongodb.NewMemoryRepository(db.Database)
	if err != nil {
		log.Fatalf("init memory repository: %v", err)
	}

	report, err := repo.DeduplicateMemories(ctx, guildID, !apply)
	if err != nil {
		log.Fatalf("deduplicate memories: %v", err)
	}

	fmt.Printf(
		"dedup complete (dry_run=%v): scanned=%d groups_merged=%d duplicates_deleted=%d backfilled=%d\n",
		report.DryRun,
		report.Scanned,
		report.GroupsMerged,
		report.DuplicatesDeleted,
		report.Backfilled,
	)
}
