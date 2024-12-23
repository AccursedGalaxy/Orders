package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

const (
	maxStreamsPerConnection = 200  // Binance limit per connection
	reconnectDelay         = time.Second * 5
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

func handleWebSocketConnection(ctx context.Context, symbols []string, rdb *redis.Client, wg *sync.WaitGroup) {
	defer wg.Done()

	streamURL := buildStreamURL(symbols)
	log.Printf("Connecting to stream URL for %d symbols: %s", len(symbols), streamURL)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			wsConn, resp, err := websocket.DefaultDialer.Dial(streamURL, nil)
			if err != nil {
				if resp != nil {
					log.Printf("WebSocket response status: %s", resp.Status)
				}
				log.Printf("WebSocket dial error: %v, retrying in %v...", err, reconnectDelay)
				time.Sleep(reconnectDelay)
				continue
			}

			// Set up ping handler
			go func() {
				ticker := time.NewTicker(time.Second * 5)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if err := wsConn.WriteMessage(websocket.PingMessage, nil); err != nil {
							log.Printf("Failed to send ping: %v", err)
							return
						}
					}
				}
			}()

			// Process messages
			for {
				select {
				case <-ctx.Done():
					wsConn.Close()
					return
				default:
					_, message, err := wsConn.ReadMessage()
					if err != nil {
						log.Printf("WebSocket read error: %v, reconnecting...", err)
						wsConn.Close()
						time.Sleep(reconnectDelay)
						break
					}

					var event AggTradeEvent
					if err := json.Unmarshal(message, &event); err != nil {
						log.Printf("JSON unmarshal error: %v", err)
						continue
					}

					// Store in Redis
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

					// Store in time-series list
					listKey := fmt.Sprintf("binance:aggTrade:history:%s", event.Data.Symbol)
					pipe := rdb.Pipeline()
					pipe.LPush(ctx, listKey, string(message))
					pipe.LTrim(ctx, listKey, 0, 999)
					_, err = pipe.Exec(ctx)
					if err != nil {
						log.Printf("Redis pipeline error: %v", err)
					}

					log.Printf("Processed trade for %s: price=%s, quantity=%s", 
						event.Data.Symbol, event.Data.Price, event.Data.Quantity)
				}
			}
		}
	}
}

func main() {
	// Get available symbols
	symbols, err := getSymbols()
	if err != nil {
		log.Fatalf("Failed to get symbols: %v", err)
	}

	// Initialize Redis client
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // set if Redis requires password
		DB:       0,  // use default DB
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test Redis connection
	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Redis connection error: %v", err)
	}
	log.Println("Successfully connected to Redis")

	// Split symbols into groups for multiple connections
	var wg sync.WaitGroup
	for i := 0; i < len(symbols); i += maxStreamsPerConnection {
		end := i + maxStreamsPerConnection
		if end > len(symbols) {
			end = len(symbols)
		}

		symbolGroup := symbols[i:end]
		wg.Add(1)
		go handleWebSocketConnection(ctx, symbolGroup, rdb, &wg)
	}

	log.Printf("Started streaming trades for all %d symbols", len(symbols))
	wg.Wait()
} 