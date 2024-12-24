package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/joho/godotenv"
)

const (
	maxRetries = 3
	retryDelay = 2 * time.Second
	timeout    = 10 * time.Second
)

func loadEnv() error {
	// Try to find .env file in current directory or parent directories
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	for {
		envFile := filepath.Join(dir, ".env")
		if _, err := os.Stat(envFile); err == nil {
			if err := godotenv.Load(envFile); err != nil {
				return fmt.Errorf("failed to load .env file: %w", err)
			}
			log.Printf("Loaded environment from: %s", envFile)
			return nil
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break // We've reached the root directory
		}
		dir = parent
	}

	return fmt.Errorf(".env file not found in current or parent directories")
}

func connectToRedis(url string) (*redis.Client, error) {
	// Parse Redis URL and configure TLS
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	// Configure TLS for Heroku Redis
	if opt.TLSConfig != nil {
		opt.TLSConfig.InsecureSkipVerify = true
	}

	// Set reasonable defaults for timeouts
	opt.DialTimeout = timeout
	opt.ReadTimeout = timeout
	opt.WriteTimeout = timeout

	// Create Redis client
	client := redis.NewClient(opt)

	// Test connection with retries
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if err := client.Ping(ctx).Err(); err != nil {
			lastErr = err
			log.Printf("Failed to connect to Redis (attempt %d/%d): %v", i+1, maxRetries, err)
			if i < maxRetries-1 {
				time.Sleep(retryDelay)
				continue
			}
			client.Close()
			return nil, fmt.Errorf("failed to connect after %d attempts: %w", maxRetries, lastErr)
		}
		log.Printf("Successfully connected to Redis at %s", opt.Addr)
		return client, nil
	}

	return nil, lastErr
}

func main() {
	// Load .env file
	if err := loadEnv(); err != nil {
		log.Printf("Warning: %v", err)
	}

	// Parse command line flags
	cmd := flag.String("cmd", "list", "Command to run (list, symbols, latest, history)")
	symbol := flag.String("symbol", "", "Symbol to query (e.g., btcusdt)")
	flag.Parse()

	// Get Redis URL from environment
	url := os.Getenv("REDIS_URL")
	if url == "" {
		log.Fatal("REDIS_URL environment variable is required")
	}

	// Connect to Redis with enhanced error handling
	client, err := connectToRedis(url)
	if err != nil {
		log.Fatalf("Failed to initialize Redis connection: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	prefix := "binance:"

	switch *cmd {
	case "list":
		// List all keys
		keys, err := client.Keys(ctx, prefix+"*").Result()
		if err != nil {
			log.Fatalf("Failed to get keys: %v", err)
		}
		fmt.Println("All keys:")
		for _, key := range keys {
			fmt.Println("-", key)
		}

	case "symbols":
		// List all symbols
		symbols, err := client.SMembers(ctx, prefix+"symbols").Result()
		if err != nil {
			log.Fatalf("Failed to get symbols: %v", err)
		}
		if len(symbols) == 0 {
			fmt.Println("No symbols found in Redis")
			return
		}
		fmt.Println("Available symbols:")
		for _, symbol := range symbols {
			fmt.Println("-", symbol)
		}

	case "latest":
		// Get latest trade for symbol
		if *symbol == "" {
			log.Fatal("Symbol is required for latest command")
		}
		key := fmt.Sprintf("%saggTrade:%s:latest", prefix, *symbol)
		data, err := client.Get(ctx, key).Result()
		if err == redis.Nil {
			log.Fatalf("No latest trade found for symbol %s", *symbol)
		}
		if err != nil {
			log.Fatalf("Failed to get latest trade: %v", err)
		}
		var trade map[string]interface{}
		if err := json.Unmarshal([]byte(data), &trade); err != nil {
			log.Fatalf("Failed to parse trade data: %v", err)
		}
		prettyPrint(trade)

	case "history":
		// Get trade history for symbol
		if *symbol == "" {
			log.Fatal("Symbol is required for history command")
		}
		key := fmt.Sprintf("%saggTrade:%s:history", prefix, *symbol)
		// Get last 10 trades
		trades, err := client.ZRevRange(ctx, key, 0, 9).Result()
		if err != nil {
			log.Fatalf("Failed to get trade history: %v", err)
		}
		if len(trades) == 0 {
			fmt.Printf("No trade history found for %s\n", *symbol)
			return
		}
		fmt.Printf("Last 10 trades for %s:\n", *symbol)
		for i, trade := range trades {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(trade), &data); err != nil {
				log.Printf("Warning: failed to parse trade %d: %v", i+1, err)
				continue
			}
			fmt.Printf("\nTrade %d:\n", i+1)
			prettyPrint(data)
		}

	default:
		log.Fatalf("Unknown command: %s", *cmd)
	}
}

func prettyPrint(v interface{}) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
} 