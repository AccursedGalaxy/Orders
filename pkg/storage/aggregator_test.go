package storage

import (
	"context"
	"testing"
	"time"

	"binance-redis-streamer/internal/models"
)

func setupTestAggregator(t *testing.T) (*TradeAggregator, func()) {
	redisStore, mr, err := setupTestRedis()
	if err != nil {
		t.Fatalf("Failed to create Redis store: %v", err)
	}

	postgresStore, cleanup := setupTestPostgres(t)
	if postgresStore == nil {
		mr.Close()
		redisStore.Close()
		t.Skip("PostgreSQL store not available, skipping aggregator tests")
		return nil, nil
	}

	aggregator := NewTradeAggregator(redisStore, postgresStore)

	return aggregator, func() {
		cleanup()
		mr.Close()
		redisStore.Close()
	}
}

func TestTradeAggregator_ProcessTrade(t *testing.T) {
	aggregator, cleanup := setupTestAggregator(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	
	trade := &models.Trade{
		Symbol:    "BTCUSDT",
		Price:     "50000.00",
		Quantity:  "1.5",
		TradeID:   12345,
		Time:      now.UnixNano(),
		EventTime: now.UnixNano(),
	}

	// Process the trade
	err := aggregator.ProcessTrade(ctx, trade)
	if err != nil {
		t.Fatalf("Failed to process trade: %v", err)
	}

	// Verify candle was created
	key := "BTCUSDT:" + now.Truncate(time.Minute).Format(time.RFC3339)
	aggregator.candleMu.RLock()
	candle, exists := aggregator.candles[key]
	aggregator.candleMu.RUnlock()

	if !exists {
		t.Fatal("Expected candle to exist")
	}

	if candle.OpenPrice != "50000.00" {
		t.Errorf("Expected open price 50000.00, got %s", candle.OpenPrice)
	}
	if candle.Volume != "1.5" {
		t.Errorf("Expected volume 1.5, got %s", candle.Volume)
	}
	if candle.TradeCount != 1 {
		t.Errorf("Expected trade count 1, got %d", candle.TradeCount)
	}
}

func TestTradeAggregator_FlushCandles(t *testing.T) {
	aggregator, cleanup := setupTestAggregator(t)
	defer cleanup()

	ctx := context.Background()
	pastTime := time.Now().Add(-2 * time.Minute)
	
	// Create a candle in the past
	trade := &models.Trade{
		Symbol:    "BTCUSDT",
		Price:     "50000.00",
		Quantity:  "1.5",
		TradeID:   12345,
		Time:      pastTime.UnixNano(),
		EventTime: pastTime.UnixNano(),
	}

	err := aggregator.ProcessTrade(ctx, trade)
	if err != nil {
		t.Fatalf("Failed to process trade: %v", err)
	}

	// Flush candles
	err = aggregator.flushCandles(ctx)
	if err != nil {
		t.Fatalf("Failed to flush candles: %v", err)
	}

	// Verify candle was flushed
	aggregator.candleMu.RLock()
	numCandles := len(aggregator.candles)
	aggregator.candleMu.RUnlock()

	if numCandles != 0 {
		t.Errorf("Expected 0 candles after flush, got %d", numCandles)
	}
} 