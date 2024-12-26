package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/storage"
)

func main() {
	var symbol string
	flag.StringVar(&symbol, "symbol", "", "Symbol to view trades for (e.g., BTCUSDT)")
	flag.Parse()

	if symbol == "" {
		fmt.Println("Please specify a symbol using -symbol flag")
		os.Exit(1)
	}

	cfg := config.DefaultConfig()
	store, err := storage.NewRedisStore(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer store.Close()

	// Get latest trade
	trade, err := store.GetLatestTrade(context.Background(), symbol)
	if err != nil {
		log.Printf("No latest trade found for %s: %v", symbol, err)
	} else {
		fmt.Printf("Latest trade for %s:\n", symbol)
		printJSON(trade)
	}

	// Get trade history
	end := time.Now()
	start := end.Add(-1 * time.Hour)
	history, err := store.GetTradeHistory(context.Background(), symbol, start, end)
	if err != nil {
		log.Printf("Failed to get trade history for %s: %v", symbol, err)
	} else {
		fmt.Printf("\nTrade history for %s (last hour):\n", symbol)
		for _, event := range history {
			printJSON(event)
		}
	}
}

func printJSON(v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Printf("Failed to marshal JSON: %v", err)
		return
	}
	fmt.Println(string(data))
}
