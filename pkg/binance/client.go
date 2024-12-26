package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/storage"
)

// Client represents a Binance WebSocket client
type Client struct {
	config  *config.Config
	store   storage.TradeStore
	baseURL string
	wsConn  *websocket.Conn
	mu      sync.RWMutex
	isTest  bool
	debug   bool
}

// NewClient creates a new Binance client
func NewClient(cfg *config.Config, store storage.TradeStore) *Client {
	return &Client{
		config:  cfg,
		store:   store,
		baseURL: cfg.Binance.BaseURL,
		debug:   cfg.Debug,
	}
}

// NewTestClient creates a new Binance client for testing
func NewTestClient(cfg *config.Config, store storage.TradeStore) *Client {
	return &Client{
		config:  cfg,
		store:   store,
		baseURL: cfg.Binance.BaseURL,
		isTest:  true,
		debug:   cfg.Debug,
	}
}

// GetSymbols fetches all available symbols from Binance
func (c *Client) GetSymbols(ctx context.Context) ([]string, error) {
	if c.debug {
		log.Println("Fetching symbols from Binance...")
	}

	// If main symbols are configured and no additional symbols are allowed
	if len(c.config.Binance.MainSymbols) > 0 && c.config.Binance.MaxSymbols <= len(c.config.Binance.MainSymbols) {
		symbols := make([]string, len(c.config.Binance.MainSymbols))
		for i, s := range c.config.Binance.MainSymbols {
			symbols[i] = strings.ToLower(s)
		}
		if c.debug {
			log.Printf("Using configured main symbols only: %v", symbols)
		}
		return symbols, nil
	}

	// First get exchange info
	url := fmt.Sprintf("%s/api/v3/exchangeInfo", c.config.Binance.BaseURL)
	exchangeInfo, err := c.fetchExchangeInfo(ctx, url)
	if err != nil {
		return nil, err
	}

	// Get 24hr ticker data if volume filtering is enabled
	var volumeData map[string]float64
	if c.config.Binance.MinDailyVolume > 0 {
		volumeData, err = c.fetch24hVolume(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch volume data: %w", err)
		}
	}

	// First, add main symbols
	symbolMap := make(map[string]bool)
	for _, s := range c.config.Binance.MainSymbols {
		symbolMap[strings.ToLower(s)] = true
	}

	// Then add additional symbols up to MaxSymbols
	remainingSlots := c.config.Binance.MaxSymbols - len(symbolMap)
	if remainingSlots > 0 {
		for _, sym := range exchangeInfo.Symbols {
			symbol := strings.ToLower(sym.Symbol)
			// Skip if already added or not a USDT pair or not trading
			if symbolMap[symbol] || !strings.HasSuffix(symbol, "usdt") || sym.Status != "TRADING" {
				continue
			}

			// Check volume if filtering is enabled
			if c.config.Binance.MinDailyVolume > 0 {
				volume, exists := volumeData[symbol]
				if !exists || volume < c.config.Binance.MinDailyVolume {
					continue
				}
			}

			// Add symbol if we haven't reached the limit
			if len(symbolMap) < c.config.Binance.MaxSymbols {
				symbolMap[symbol] = true
			} else {
				break
			}
		}
	}

	// Convert map to slice
	symbols := make([]string, 0, len(symbolMap))
	for symbol := range symbolMap {
		symbols = append(symbols, symbol)
	}

	if len(symbols) == 0 {
		return nil, fmt.Errorf("no trading pairs found")
	}

	if c.debug {
		log.Printf("Selected %d trading pairs", len(symbols))
	}
	return symbols, nil
}

// fetchExchangeInfo fetches exchange information from Binance
func (c *Client) fetchExchangeInfo(ctx context.Context, url string) (*models.ExchangeInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch symbols: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var exchangeInfo models.ExchangeInfo
	if err := json.Unmarshal(body, &exchangeInfo); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &exchangeInfo, nil
}

// fetch24hVolume fetches 24h volume data for all symbols
func (c *Client) fetch24hVolume(ctx context.Context) (map[string]float64, error) {
	url := fmt.Sprintf("%s/api/v3/ticker/24hr", c.config.Binance.BaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch volume data: %w", err)
	}
	defer resp.Body.Close()

	var tickers []struct {
		Symbol      string `json:"symbol"`
		QuoteVolume string `json:"quoteVolume"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tickers); err != nil {
		return nil, fmt.Errorf("failed to decode volume data: %w", err)
	}

	volumeData := make(map[string]float64)
	for _, ticker := range tickers {
		volume, err := strconv.ParseFloat(ticker.QuoteVolume, 64)
		if err != nil {
			log.Printf("Warning: invalid volume for %s: %s", ticker.Symbol, ticker.QuoteVolume)
			continue
		}
		volumeData[strings.ToLower(ticker.Symbol)] = volume
	}

	return volumeData, nil
}

// StreamTrades starts streaming trades for the given symbols
func (c *Client) StreamTrades(ctx context.Context) error {
	symbols, err := c.GetSymbols(ctx)
	if err != nil {
		log.Printf("Error getting symbols: %v", err)
		// Don't return error, try again after delay
		time.Sleep(c.config.WebSocket.ReconnectDelay)
		return c.StreamTrades(ctx)
	}

	if len(symbols) == 0 {
		log.Println("No symbols to stream, retrying after delay...")
		time.Sleep(c.config.WebSocket.ReconnectDelay)
		return c.StreamTrades(ctx)
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
			log.Printf("Streaming error: %v, reconnecting...", err)
			time.Sleep(c.config.WebSocket.ReconnectDelay)
			return c.StreamTrades(ctx)
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (c *Client) handleSymbolGroup(ctx context.Context, symbols []string) error {
	url := c.buildStreamURL(symbols)
	if c.debug {
		log.Printf("Connecting to stream URL for %d symbols", len(symbols))
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := c.connectAndStream(ctx, url); err != nil {
				if c.debug {
					log.Printf("Stream error: %v, reconnecting...", err)
				}
				continue
			}
		}
	}
}

func (c *Client) buildStreamURL(symbols []string) string {
	streams := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		streams = append(streams, fmt.Sprintf("%s@trade", symbol))
	}
	return fmt.Sprintf("wss://stream.binance.com:9443/stream?streams=%s", strings.Join(streams, "/"))
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
	if c.debug {
		// Debug: Print raw message
		log.Printf("Raw WebSocket message: %s", string(message))
	}

	var event models.AggTradeEvent
	if err := json.Unmarshal(message, &event); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	if c.debug {
		// Debug: Print unmarshaled event
		log.Printf("Unmarshaled event: stream=%s, symbol=%s, IsBuyerMaker=%v",
			event.Stream, event.Data.Symbol, event.Data.IsBuyerMaker)
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

	// Only log in non-test mode and debug mode
	if !c.isTest && c.debug {
		log.Printf("Processed trade for %s: price=%s, quantity=%s, IsBuyerMaker=%v",
			trade.Symbol, trade.Price, trade.Quantity, trade.IsBuyerMaker)
	}

	return nil
}

// BuildStreamURL builds the WebSocket stream URL for the given symbols
func (c *Client) BuildStreamURL(symbols []string) string {
	streams := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		streams = append(streams, fmt.Sprintf("%s@trade", strings.ToLower(symbol)))
	}
	return fmt.Sprintf("wss://stream.binance.com:9443/stream?streams=%s", strings.Join(streams, "/"))
}
