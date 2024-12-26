package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"binance-redis-streamer/pkg/binance"
	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/ingestion"
	"binance-redis-streamer/pkg/metrics"
	"binance-redis-streamer/pkg/processor"
	"binance-redis-streamer/pkg/storage"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
	}

	// Load configuration
	cfg := loadConfig()

	// Create Redis store
	redisStore, err := storage.NewRedisStore(cfg)
	if err != nil {
		log.Fatalf("Failed to create Redis store: %v", err)
	}
	defer redisStore.Close()

	// Create PostgreSQL store
	postgresStore, err := storage.NewPostgresStore()
	if err != nil {
		log.Fatalf("Failed to create PostgreSQL store: %v", err)
	}
	defer postgresStore.Close()

	// Create trade aggregator
	aggregator := storage.NewTradeAggregator(redisStore, postgresStore)

	// Create metrics exporter
	exporter := metrics.NewMetricsExporter(cfg, redisStore.GetRedisClient())

	// Create Binance client
	client := binance.NewClient(cfg, redisStore)

	// Create ingestion service
	ingestService := ingestion.NewService(cfg, client, redisStore)

	// Create processor service
	processService := processor.NewService(cfg, redisStore, aggregator)

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start metrics collection
	go exporter.Start(ctx)

	// Start trade aggregator
	go aggregator.Start(ctx)

	// Start processor service
	go func() {
		if err := processService.Start(ctx); err != nil {
			log.Printf("Processor service error: %v", err)
			cancel()
		}
	}()

	// Start ingestion service
	go func() {
		if err := ingestService.Start(ctx); err != nil {
			log.Printf("Ingestion service error: %v", err)
			cancel()
		}
	}()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("Received signal %v, shutting down...", sig)
	cancel()

	// Stop services
	processService.Stop()
	ingestService.Stop()

	// Allow some time for cleanup
	time.Sleep(5 * time.Second)
}

func loadConfig() *config.Config {
	cfg := config.DefaultConfig()

	// Override configuration from environment variables
	if maxSymbols := os.Getenv("MAX_SYMBOLS"); maxSymbols != "" {
		if val, err := strconv.Atoi(maxSymbols); err == nil {
			cfg.Binance.MaxSymbols = val
		}
	}

	if retentionDays := os.Getenv("RETENTION_DAYS"); retentionDays != "" {
		if val, err := strconv.Atoi(retentionDays); err == nil {
			cfg.Redis.RetentionPeriod = time.Duration(val) * 24 * time.Hour
		}
	}

	return cfg
}
