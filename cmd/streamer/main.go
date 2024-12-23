package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"binance-redis-streamer/pkg/binance"
	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/storage"
)

func main() {
	// Load configuration
	cfg := config.DefaultConfig()

	// Create storage
	store, err := storage.NewRedisStore(cfg)
	if err != nil {
		log.Fatalf("Failed to create Redis store: %v", err)
	}
	defer store.Close()

	// Create Binance client
	client := binance.NewClient(cfg, store)

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	// Start streaming
	log.Println("Starting trade streamer...")
	if err := client.StreamTrades(ctx); err != nil {
		log.Printf("Streaming ended with error: %v", err)
		os.Exit(1)
	}
} 