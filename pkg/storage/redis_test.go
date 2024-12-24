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

func BenchmarkStoreTrade(b *testing.B) {
	store, mr, err := setupTestRedis()
	if err != nil {
		b.Fatal(err)
	}
	defer mr.Close()
	defer store.Close()

	trade := &models.Trade{
			Symbol:    "BTCUSDT",
			Price:     "50000.00",
			Quantity:  "1.5",
			TradeID:   12345,
			Time:      time.Now().UnixNano(),
			EventTime: time.Now().UnixNano(),
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

	rawData := []byte(`{"stream":"btcusdt@aggTrade","data":{"e":"aggTrade","E":1625232862,"s":"BTCUSDT","p":"50000.00","q":"1.5"}}`)

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
	rawData := []byte(`{"stream":"btcusdt@aggTrade","data":{"e":"aggTrade","E":1625232862,"s":"BTCUSDT","p":"50000.00","q":"1.5"}}`)
	for i := 0; i < 100; i++ {
		if err := store.StoreRawTrade(ctx, "BTCUSDT", rawData); err != nil {
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