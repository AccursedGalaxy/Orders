package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/go-redis/redis/v8"
)

func main() {
	// Parse command line flags
	cmd := flag.String("cmd", "list", "Command to run (list, symbols, latest, history)")
	symbol := flag.String("symbol", "", "Symbol to query (e.g., btcusdt)")
	flag.Parse()

	// Get Redis URL from environment
	url := os.Getenv("REDIS_URL")
	if url == "" {
		log.Fatal("REDIS_URL environment variable is required")
	}

	// Parse Redis URL and configure TLS
	opt, err := redis.ParseURL(url)
	if err != nil {
		log.Fatalf("Failed to parse Redis URL: %v", err)
	}
	if opt.TLSConfig != nil {
		opt.TLSConfig.InsecureSkipVerify = true
	}

	// Create Redis client
	client := redis.NewClient(opt)
	defer client.Close()

	ctx := context.Background()

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

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
		fmt.Printf("Last 10 trades for %s:\n", *symbol)
		for i, trade := range trades {
			var data map[string]interface{}
			if err := json.Unmarshal([]byte(trade), &data); err != nil {
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