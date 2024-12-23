package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/storage"
)

// Client represents a Binance client
type Client struct {
	config *config.Config
	store  storage.TradeStore
}

// NewClient creates a new Binance client
func NewClient(cfg *config.Config, store storage.TradeStore) *Client {
	return &Client{
		config: cfg,
		store:  store,
	}
}

// GetSymbols fetches all available symbols from Binance
func (c *Client) GetSymbols(ctx context.Context) ([]string, error) {
	log.Println("Fetching symbols from Binance...")
	url := fmt.Sprintf("%s/fapi/v1/exchangeInfo", c.config.Binance.BaseURL)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch symbols: %w", err)
	}
	defer resp.Body.Close()

	var exchangeInfo models.ExchangeInfo
	if err := json.NewDecoder(resp.Body).Decode(&exchangeInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var symbols []string
	for _, sym := range exchangeInfo.Symbols {
		symbols = append(symbols, strings.ToLower(sym.Symbol))
	}
	
	log.Printf("Found %d symbols", len(symbols))
	return symbols, nil
}

// StreamTrades starts streaming trades for the given symbols
func (c *Client) StreamTrades(ctx context.Context) error {
	symbols, err := c.GetSymbols(ctx)
	if err != nil {
		return fmt.Errorf("failed to get symbols: %w", err)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	// Split symbols into groups
	for i := 0; i < len(symbols); i += c.config.Binance.MaxStreamsPerConn {
		end := i + c.config.Binance.MaxStreamsPerConn
		if end > len(symbols) {
			end = len(symbols)
		}

		symbolGroup := symbols[i:end]
		wg.Add(1)
		go func(symbols []string) {
			defer wg.Done()
			if err := c.handleSymbolGroup(ctx, symbols); err != nil {
				select {
				case errChan <- err:
				default:
				}
			}
		}(symbolGroup)
	}

	// Wait for error or context cancellation
	go func() {
		wg.Wait()
		close(errChan)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("streaming error: %w", err)
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (c *Client) handleSymbolGroup(ctx context.Context, symbols []string) error {
	url := c.buildStreamURL(symbols)
	log.Printf("Connecting to stream URL for %d symbols", len(symbols))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := c.connectAndStream(ctx, url); err != nil {
				log.Printf("Stream error: %v, reconnecting...", err)
				continue
			}
		}
	}
}

func (c *Client) buildStreamURL(symbols []string) string {
	var streams []string
	for _, symbol := range symbols {
		streams = append(streams, fmt.Sprintf("%s@aggTrade", symbol))
	}
	return fmt.Sprintf("wss://fstream.binance.com/stream?streams=%s", strings.Join(streams, "/"))
}

func (c *Client) connectAndStream(ctx context.Context, url string) error {
	wsConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial error: %w", err)
	}
	defer wsConn.Close()

	// Set up ping handler
	go c.handlePing(ctx, wsConn)

	// Process messages
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				return fmt.Errorf("websocket read error: %w", err)
			}

			if err := c.processMessage(ctx, message); err != nil {
				log.Printf("Failed to process message: %v", err)
			}
		}
	}
}

func (c *Client) handlePing(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(c.config.WebSocket.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Failed to send ping: %v", err)
				return
			}
		}
	}
}

func (c *Client) processMessage(ctx context.Context, message []byte) error {
	var event models.AggTradeEvent
	if err := json.Unmarshal(message, &event); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	trade := event.ToTrade()
	
	// Store processed trade
	if err := c.store.StoreTrade(ctx, trade); err != nil {
		return fmt.Errorf("failed to store trade: %w", err)
	}

	// Store raw message
	if err := c.store.StoreRawTrade(ctx, trade.Symbol, message); err != nil {
		return fmt.Errorf("failed to store raw trade: %w", err)
	}

	log.Printf("Processed trade for %s: price=%s, quantity=%s",
		trade.Symbol, trade.Price, trade.Quantity)

	return nil
} 