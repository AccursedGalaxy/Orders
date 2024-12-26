package storage

import (
	"context"
	"testing"
	"time"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/config"

	"github.com/alicebob/miniredis/v2"
)

func setupTestRedis() (*RedisStore, *miniredis.Miniredis, error) {
	mr, err := miniredis.Run()
	if err != nil {
		return nil, nil, err
	}

	cfg := &config.Config{
		Redis: config.RedisConfig{
			URL:             "redis://" + mr.Addr(),
			RetentionPeriod: 24 * time.Hour,
			CleanupInterval: 1 * time.Hour,
			KeyPrefix:       "test:",
		},
	}

	store, err := NewRedisStore(cfg)
	if err != nil {
		mr.Close()
		return nil, nil, err
	}

	return store, mr, nil
}

func TestRedisStore_StoreTrade(t *testing.T) {
	store, mr, err := setupTestRedis()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	defer store.Close()

	now := time.Now()
	trade := &models.Trade{
		Symbol:    "BTCUSDT",
		Price:     "50000.00",
		Quantity:  "1.5",
		TradeID:   12345,
		Time:      now,
		EventTime: now,
	}

	ctx := context.Background()
	err = store.StoreTrade(ctx, trade)
	if err != nil {
		t.Fatalf("Failed to store trade: %v", err)
	}

	// Verify trade was stored
	trades, err := store.GetTradeHistory(ctx, "BTCUSDT", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Failed to get trade history: %v", err)
	}

	if len(trades) != 1 {
		t.Fatalf("Expected 1 trade, got %d", len(trades))
	}

	storedTrade := trades[0].ToTrade()
	if storedTrade.Price != trade.Price {
		t.Errorf("Expected price %s, got %s", trade.Price, storedTrade.Price)
	}
	if storedTrade.Quantity != trade.Quantity {
		t.Errorf("Expected quantity %s, got %s", trade.Quantity, storedTrade.Quantity)
	}
}

func BenchmarkStoreTrade(b *testing.B) {
	store, mr, err := setupTestRedis()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()
	defer store.Close()

	now := time.Now()
	trade := &models.Trade{
		Symbol:    "BTCUSDT",
		Price:     "50000.00",
		Quantity:  "1.5",
		TradeID:   12345,
		Time:      now,
		EventTime: now,
	}

	ctx := context.Background()
	b.ResetTimer()

	b.Run("StoreTrade", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err := store.StoreTrade(ctx, trade); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkStoreRawTrade(b *testing.B) {
	store, mr, err := setupTestRedis()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()
	defer store.Close()

	rawData := []byte(`{"stream":"btcusdt@aggTrade","data":{"e":"aggTrade","E":1625232862,"s":"BTCUSDT","p":"50000.00","q":"1.5","T":1625232862,"t":12345,"m":true}}`)

	ctx := context.Background()
	b.ResetTimer()

	b.Run("StoreRawTrade", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err := store.StoreRawTrade(ctx, "BTCUSDT", rawData); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkGetTradeHistory(b *testing.B) {
	store, mr, err := setupTestRedis()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()
	defer store.Close()

	// Populate some test data
	ctx := context.Background()
	now := time.Now()
	trade := &models.Trade{
		Symbol:    "BTCUSDT",
		Price:     "50000.00",
		Quantity:  "1.5",
		TradeID:   12345,
		Time:      now,
		EventTime: now,
	}

	for i := 0; i < 100; i++ {
		if err := store.StoreTrade(ctx, trade); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()

	end := time.Now()
	start := end.Add(-24 * time.Hour)

	b.Run("GetTradeHistory", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := store.GetTradeHistory(ctx, "BTCUSDT", start, end)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
} 