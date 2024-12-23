package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/AccursedGalaxy/Orders/internal/config"
	"github.com/AccursedGalaxy/Orders/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	l := logger.InitLogger(cfg.Log)
	defer l.Sync()

	l.Info("Starting Binance Orders service...")

	// Example logging
	l.Debug("Debug message - won't show in console with info level")
	l.Info("Service initialized successfully",
		zap.String("environment", "development"),
		zap.Int("port", 8080),
	)
	l.Warn("System resources running low",
		zap.Int("memory_usage", 90),
	)

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	l.Info("Shutting down gracefully...")
} 