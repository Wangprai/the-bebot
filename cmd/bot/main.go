
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"the-bebot/internal/platform/config"
	"the-bebot/internal/platform/database"
	"the-bebot/internal/platform/logger"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	// Initialize logger
	appLogger := logger.New(cfg.LogLevel, cfg.AppEnv)

	appLogger.Info("Starting The Bebot")

	// Create application context
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// Connect MongoDB
	connectCtx, connectCancel := context.WithTimeout(
		ctx,
		10*time.Second,
	)
	defer connectCancel()

	db, err := database.NewMongoDB(
		connectCtx,
		cfg.MongoDB.URI,
		cfg.MongoDB.Database,
	)
	if err != nil {
		appLogger.Error("Failed to connect MongoDB", "error", err)
		return
	}

	appLogger.Info("MongoDB connected",
		"database", cfg.MongoDB.Database,
	)

	// Wait for shutdown signal
	<-ctx.Done()

	appLogger.Info("Shutdown signal received")

	// Shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer shutdownCancel()

	// Disconnect MongoDB
	if err := db.Close(shutdownCtx); err != nil {
		appLogger.Error("Failed to disconnect MongoDB", "error", err)
	} else {
		appLogger.Info("MongoDB disconnected")
	}

	appLogger.Info("The Bebot stopped")
}
