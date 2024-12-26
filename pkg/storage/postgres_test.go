package storage

import (
	"context"
	"os"
	"testing"
	"time"

	"binance-redis-streamer/internal/models"

	_ "github.com/lib/pq"
)

func setupTestPostgres(t *testing.T) (*PostgresStore, func()) {
	// Use test database URL from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("TEST_DATABASE_URL")
	}
	if dbURL == "" {
		t.Skip("Neither DATABASE_URL nor TEST_DATABASE_URL is set, skipping PostgreSQL tests")
		return nil, nil
	}

	// Temporarily set the environment variable for the store
	oldURL := os.Getenv("DATABASE_URL")
	os.Setenv("DATABASE_URL", dbURL)

	store, err := NewPostgresStore()

	// Restore the original environment variable
	if oldURL != "" {
		os.Setenv("DATABASE_URL", oldURL)
	} else {
		os.Unsetenv("DATABASE_URL")
	}

	if err != nil {
		t.Skipf("Failed to create PostgreSQL store: %v", err)
		return nil, nil
	}

	cleanup := func() {
		if store != nil {
			// Clean up test data
			_, err := store.db.Exec("DELETE FROM trade_candles")
			if err != nil {
				t.Errorf("Failed to clean up test data: %v", err)
			}
			store.Close()
		}
	}

	return store, cleanup
}

func TestPostgresStore_StoreCandleData(t *testing.T) {
	store, cleanup := setupTestPostgres(t)
	defer cleanup()

	ctx := context.Background()
	timestamp := time.Now().Truncate(time.Minute)

	candle := &models.Candle{
		Timestamp:  timestamp,
		OpenPrice:  "50000.00",
		HighPrice:  "51000.00",
		LowPrice:   "49000.00",
		ClosePrice: "50500.00",
		Volume:     "10.5",
		TradeCount: 100,
	}

	// Test storing new candle
	err := store.StoreCandleData(ctx, "BTCUSDT", candle)
	if err != nil {
		t.Fatalf("Failed to store candle data: %v", err)
	}

	// Test updating existing candle
	updatedCandle := &models.Candle{
		Timestamp:  timestamp,
		OpenPrice:  "50000.00",
		HighPrice:  "52000.00",
		LowPrice:   "48000.00",
		ClosePrice: "51500.00",
		Volume:     "15.5",
		TradeCount: 150,
	}

	err = store.StoreCandleData(ctx, "BTCUSDT", updatedCandle)
	if err != nil {
		t.Fatalf("Failed to update candle data: %v", err)
	}

	// Verify the data
	var result struct {
		highPrice  string
		lowPrice   string
		volume     string
		tradeCount int64
	}

	err = store.db.QueryRowContext(ctx, `
		SELECT high_price, low_price, volume, trade_count 
		FROM trade_candles 
		WHERE symbol = $1 AND timestamp = $2`,
		"BTCUSDT", timestamp,
	).Scan(&result.highPrice, &result.lowPrice, &result.volume, &result.tradeCount)

	if err != nil {
		t.Fatalf("Failed to query candle data: %v", err)
	}

	if result.highPrice != "52000.00" {
		t.Errorf("Expected high price 52000.00, got %s", result.highPrice)
	}
	if result.lowPrice != "48000.00" {
		t.Errorf("Expected low price 48000.00, got %s", result.lowPrice)
	}
	if result.volume != "26.00" {
		t.Errorf("Expected volume 26.00, got %s", result.volume)
	}
	if result.tradeCount != 250 {
		t.Errorf("Expected trade count 250, got %d", result.tradeCount)
	}
}
