package binance

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/storage"

	"github.com/go-redis/redis/v8"
)

func setupTestServer() (*httptest.Server, *config.Config) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if strings.Contains(r.URL.Path, "/api/v3/ticker/24hr") {
			// Mock 24hr ticker response
			_, err := w.Write([]byte(`[
				{"symbol":"BTCUSDT","quoteVolume":"1000000.00"},
				{"symbol":"ETHUSDT","quoteVolume":"500000.00"}
			]`))
			if err != nil {
				panic(err)
			}
			return
		}

		// Mock exchange info response
		_, err := w.Write([]byte(`{
			"symbols": [
				{"symbol": "BTCUSDT", "status": "TRADING"},
				{"symbol": "ETHUSDT", "status": "TRADING"}
			]
		}`))
		if err != nil {
			panic(err)
		}
	}))

	cfg := config.DefaultConfig()
	cfg.Binance.BaseURL = server.URL
	return server, cfg
}

type mockStore struct {
	storage.TradeStore
	mu        sync.RWMutex
	trades    map[string]*models.Trade
	rawTrades map[string][]byte
}

func newMockStore() *mockStore {
	return &mockStore{
		trades:    make(map[string]*models.Trade),
		rawTrades: make(map[string][]byte),
	}
}

func (m *mockStore) StoreTrade(ctx context.Context, trade *models.Trade) error {
	m.mu.Lock()
	m.trades[trade.Symbol] = trade
	m.mu.Unlock()
	return nil
}

func (m *mockStore) StoreRawTrade(ctx context.Context, symbol string, data []byte) error {
	m.mu.Lock()
	m.rawTrades[symbol] = data
	m.mu.Unlock()
	return nil
}

func (m *mockStore) GetTradeHistory(ctx context.Context, symbol string, start, end time.Time) ([]models.AggTradeEvent, error) {
	return nil, nil
}

func (m *mockStore) GetLatestTrade(ctx context.Context, symbol string) (*models.Trade, error) {
	m.mu.RLock()
	trade, ok := m.trades[symbol]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no trade found")
	}
	return trade, nil
}

func (m *mockStore) GetRedisClient() *redis.Client {
	return nil
}

func (m *mockStore) Close() error {
	return nil
}

func BenchmarkGetSymbols(b *testing.B) {
	cfg := config.DefaultConfig()
	cfg.Redis.URL = "redis://localhost:6379/0"

	store := &mockStore{}
	client := NewClient(cfg, store)

	// Setup mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/v3/ticker/24hr") {
			// Mock 24hr ticker response
			w.Write([]byte(`[
				{"symbol":"BTCUSDT","quoteVolume":"1000000.00"},
				{"symbol":"ETHUSDT","quoteVolume":"500000.00"}
			]`))
			return
		}
		// Mock exchange info response
		w.Write([]byte(`{
			"symbols": [
				{"symbol":"BTCUSDT","status":"TRADING"},
				{"symbol":"ETHUSDT","status":"TRADING"},
				{"symbol":"BNBUSDT","status":"TRADING"}
			]
		}`))
	}))
	defer mockServer.Close()

	// Override base URL to use mock server
	client.baseURL = mockServer.URL

	b.ResetTimer()
	b.Run("GetSymbols", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := client.GetSymbols(context.Background())
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkProcessMessage(b *testing.B) {
	cfg := config.DefaultConfig()
	cfg.Redis.URL = "redis://localhost:6379/0"

	store := &mockStore{}
	client := NewClient(cfg, store)

	msg := []byte(`{"stream":"btcusdt@aggTrade","data":{"e":"aggTrade","E":1625232862,"s":"BTCUSDT","p":"50000.00","q":"1.5"}}`)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := client.processMessage(context.Background(), msg)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestGetSymbols(t *testing.T) {
	server, cfg := setupTestServer()
	defer server.Close()

	store := newMockStore()
	client := NewClient(cfg, store)

	symbols, err := client.GetSymbols(context.Background())
	if err != nil {
		t.Fatalf("Failed to get symbols: %v", err)
	}

	expectedSymbols := []string{"btcusdt", "ethusdt"}
	if len(symbols) != len(expectedSymbols) {
		t.Errorf("Expected %d symbols, got %d", len(expectedSymbols), len(symbols))
	}

	for i, symbol := range symbols {
		if symbol != expectedSymbols[i] {
			t.Errorf("Expected symbol %s, got %s", expectedSymbols[i], symbol)
		}
	}
}

func TestProcessMessage(t *testing.T) {
	_, cfg := setupTestServer()
	store := newMockStore()
	client := NewClient(cfg, store)

	msg := []byte(`{"stream":"btcusdt@aggTrade","data":{"e":"aggTrade","E":1625232862,"s":"BTCUSDT","p":"50000.00","q":"1.5","T":1625232862,"m":true}}`)

	err := client.processMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("Failed to process message: %v", err)
	}

	// Verify the trade was stored
	trade, err := store.GetLatestTrade(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("Failed to get stored trade: %v", err)
	}

	if trade.Price != "50000.00" || trade.Quantity != "1.5" {
		t.Errorf("Trade data mismatch: got price=%s, quantity=%s", trade.Price, trade.Quantity)
	}
}
