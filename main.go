package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

type Symbol struct {
	Symbol string `json:"symbol"`
}

type ExchangeInfo struct {
	Symbols []Symbol `json:"symbols"`
}

type AggTradeEvent struct {
	Stream string `json:"stream"`
	Data   struct {
		EventType string `json:"e"`
		EventTime int64  `json:"E"`
		Symbol    string `json:"s"`
		Price     string `json:"p"`
		Quantity  string `json:"q"`
		TradeID   int64  `json:"a"`
	} `json:"data"`
}

func getSymbols() ([]string, error) {
	log.Println("Fetching symbols from Binance...")
	resp, err := http.Get("https://fapi.binance.com/fapi/v1/exchangeInfo")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var exchangeInfo ExchangeInfo
	if err := json.NewDecoder(resp.Body).Decode(&exchangeInfo); err != nil {
		return nil, err
	}

	var symbols []string
	for _, sym := range exchangeInfo.Symbols {
		symbols = append(symbols, strings.ToLower(sym.Symbol))
	}
	log.Printf("Found %d symbols", len(symbols))
	return symbols, nil
}

func buildStreamURL(symbols []string) string {
	var streams []string
	for _, symbol := range symbols {
		streams = append(streams, fmt.Sprintf("%s@aggTrade", symbol))
	}
	return fmt.Sprintf("wss://fstream.binance.com/stream?streams=%s", strings.Join(streams, "/"))
}

func main() {
	// Get available symbols
	symbols, err := getSymbols()
	if err != nil {
		log.Fatalf("Failed to get symbols: %v", err)
	}

	// For testing, let's limit to first 3 symbols
	if len(symbols) > 3 {
		symbols = symbols[:3]
	}
	log.Printf("Selected symbols for streaming: %v", symbols)

	// Build WebSocket URL
	streamURL := buildStreamURL(symbols)
	log.Printf("Connecting to stream URL: %s", streamURL)

	// Initialize WebSocket connection
	wsConn, resp, err := websocket.DefaultDialer.Dial(streamURL, nil)
	if err != nil {
		if resp != nil {
			log.Printf("WebSocket response status: %s", resp.Status)
		}
		log.Fatalf("WebSocket dial error: %v", err)
	}
	defer wsConn.Close()

	// Initialize Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // set if Redis requires password
		DB:       0,  // use default DB
	})
	ctx := context.Background()

	// Test Redis connection
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Redis connection error: %v", err)
	}
	log.Println("Successfully connected to Redis")

	log.Println("Starting to stream trades...")

	// Set up ping handler
	go func() {
		ticker := time.NewTicker(time.Second * 5)
		defer ticker.Stop()

		for range ticker.C {
			if err := wsConn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Failed to send ping: %v", err)
				return
			}
			log.Println("Ping sent to WebSocket server")
		}
	}()

	// Main processing loop
	for {
		messageType, message, err := wsConn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			time.Sleep(time.Second)
			continue
		}

		log.Printf("Received message type: %d, length: %d bytes", messageType, len(message))

		// Print raw message for debugging
		log.Printf("Raw message: %s", string(message))

		var event AggTradeEvent
		if err := json.Unmarshal(message, &event); err != nil {
			log.Printf("JSON unmarshal error: %v", err)
			continue
		}

		// Store in Redis
		// Using a hash to store latest trade for each symbol
		key := fmt.Sprintf("binance:aggTrade:%s", event.Data.Symbol)
		err = rdb.HSet(ctx, key, map[string]interface{}{
			"price":    event.Data.Price,
			"quantity": event.Data.Quantity,
			"tradeId":  event.Data.TradeID,
			"time":     event.Data.EventTime,
		}).Err()

		if err != nil {
			log.Printf("Redis store error: %v", err)
			continue
		}

		// Also store in a time-series list (limited to last 1000 trades per symbol)
		listKey := fmt.Sprintf("binance:aggTrade:history:%s", event.Data.Symbol)
		pipe := rdb.Pipeline()
		pipe.LPush(ctx, listKey, string(message))
		pipe.LTrim(ctx, listKey, 0, 999) // Keep only last 1000 trades
		_, err = pipe.Exec(ctx)
		if err != nil {
			log.Printf("Redis pipeline error: %v", err)
		}

		log.Printf("Successfully processed trade for %s: price=%s, quantity=%s", 
			event.Data.Symbol, event.Data.Price, event.Data.Quantity)
	}
} 