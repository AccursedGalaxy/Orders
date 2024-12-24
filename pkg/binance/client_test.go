package binance

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
		_, err := w.Write([]byte(`{
			"symbols": [
				{"symbol": "BTCUSDT", "status": "TRADING"},
				{"symbol": "ETHUSDT", "status": "TRADING"}
			]
		}`))
		if err != nil {
			panic(err) // In tests, we can panic on write error
		}
	}))

	cfg := config.DefaultConfig()
	cfg.Binance.BaseURL = server.URL
	return server, cfg
}

type mockStore struct {
	storage.TradeStore
	mu sync.RWMutex
	trades map[string]*models.Trade
	rawTrades map[string][]byte
}

func newMockStore() *mockStore {
	return &mockStore{
		trades: make(map[string]*models.Trade),
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
	server, cfg := setupTestServer()
	defer server.Close()

	client := NewClient(cfg, &mockStore{})
	ctx := context.Background()

	b.ResetTimer()
	b.Run("GetSymbols", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := client.GetSymbols(ctx)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkProcessMessage(b *testing.B) {
	cfg := config.DefaultConfig()
	client := NewTestClient(cfg, newMockStore())
	ctx := context.Background()

	message := []byte(`{
		"stream": "btcusdt@aggTrade",
		"data": {
			"e": "aggTrade",
			"E": 1625232862,
			"s": "BTCUSDT",
			"p": "50000.00",
			"q": "1.5",
			"f": 100,
			"l": 200,
			"T": 1625232862,
			"m": true
		}
	}`)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := client.processMessage(ctx, message); err != nil {
				b.Fatal(err)
			}
		}
	})
} 