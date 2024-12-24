package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		w.Write([]byte(`{
			"symbols": [
				{"symbol": "BTCUSDT", "status": "TRADING"},
				{"symbol": "ETHUSDT", "status": "TRADING"}
			]
		}`))
	}))

	cfg := config.DefaultConfig()
	cfg.Binance.BaseURL = server.URL
	return server, cfg
}

type mockStore struct {
	storage.TradeStore
}

func (m *mockStore) StoreTrade(ctx context.Context, trade *models.Trade) error {
	return nil
}

func (m *mockStore) StoreRawTrade(ctx context.Context, symbol string, data []byte) error {
	return nil
}

func (m *mockStore) GetTradeHistory(ctx context.Context, symbol string, start, end time.Time) ([]models.AggTradeEvent, error) {
	return nil, nil
}

func (m *mockStore) GetLatestTrade(ctx context.Context, symbol string) (*models.Trade, error) {
	return nil, nil
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
	client := NewClient(cfg, &mockStore{})
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
	b.Run("ProcessMessage", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err := client.processMessage(ctx, message); err != nil {
				b.Fatal(err)
			}
		}
	})
} 