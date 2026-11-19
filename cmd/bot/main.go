
package main

import (
	"log"
	"the-bebot/internal/platform/config"
	"the-bebot/internal/platform/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	appLogger := logger.New(cfg.LogLevel, cfg.AppEnv)

	appLogger.Info("Application started",
		"app", cfg.AppName,
		"environment", cfg.AppEnv,
	)

	appLogger.Debug("Configuration loaded",
		"database", cfg.MongoDB.Database,
		"memory_enabled", cfg.Memory.Enabled,
	)

	appLogger.Warn("Discord bot is not connected yet")
}
