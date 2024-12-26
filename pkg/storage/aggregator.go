package storage

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"binance-redis-streamer/internal/models"
)

// TradeAggregator handles trade aggregation and storage
type TradeAggregator struct {
	redisStore    *RedisStore
	postgresStore *PostgresStore
	candles       map[string]*models.Candle
	candleMu      sync.RWMutex
	stopCh        chan struct{}
}

// NewTradeAggregator creates a new trade aggregator
func NewTradeAggregator(redisStore *RedisStore, postgresStore *PostgresStore) *TradeAggregator {
	return &TradeAggregator{
		redisStore:    redisStore,
		postgresStore: postgresStore,
		candles:       make(map[string]*models.Candle),
		stopCh:        make(chan struct{}),
	}
}

// Start starts the aggregation process
func (a *TradeAggregator) Start(ctx context.Context) {
	// Flush candles every 10 seconds instead of every minute
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	log.Printf("Starting trade aggregator with 10-second flush interval")

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-a.stopCh:
				return
			case <-ticker.C:
				if err := a.flushCandles(ctx); err != nil {
					log.Printf("Error flushing candles: %v", err)
				}
			}
		}
	}()

	// Start historical data migration
	go a.migrateHistoricalData(ctx)
}

// ProcessTrade processes a new trade and updates the current candle
func (a *TradeAggregator) ProcessTrade(ctx context.Context, trade *models.Trade) error {
	a.candleMu.Lock()
	defer a.candleMu.Unlock()

	// Truncate to minute for candle
	candleTime := trade.Time.Truncate(time.Minute)
	key := fmt.Sprintf("%s:%s", trade.Symbol, candleTime.Format(time.RFC3339))

	log.Printf("Processing trade for %s at %s: price=%s, quantity=%s, trade_time=%s",
		trade.Symbol, candleTime.Format(time.RFC3339), trade.Price, trade.Quantity, trade.Time.Format(time.RFC3339))

	// Get or create candle
	candle, exists := a.candles[key]
	if !exists {
		candle = models.NewCandle(candleTime)
		a.candles[key] = candle
		log.Printf("Created new candle for %s at %s", trade.Symbol, candleTime.Format(time.RFC3339))
	}
	candle.UpdateFromTrade(trade)

	log.Printf("Updated candle for %s at %s: open=%s, high=%s, low=%s, close=%s, volume=%s, trades=%d",
		trade.Symbol, candleTime.Format(time.RFC3339),
		candle.OpenPrice, candle.HighPrice, candle.LowPrice, candle.ClosePrice,
		candle.Volume, candle.TradeCount)

	return nil
}

// flushCandles writes completed candles to PostgreSQL
func (a *TradeAggregator) flushCandles(ctx context.Context) error {
	a.candleMu.Lock()
	defer a.candleMu.Unlock()

	log.Printf("Starting candle flush, current count: %d", len(a.candles))
	currentMinute := time.Now().Truncate(time.Minute)
	flushedCount := 0

	for key, candle := range a.candles {
		// Only flush candles that are complete (from previous minutes)
		if candle.Timestamp.Before(currentMinute) {
			symbol := strings.Split(key, ":")[0]
			log.Printf("Flushing candle for %s at %s: open=%s, high=%s, low=%s, close=%s, volume=%s, trades=%d",
				symbol, candle.Timestamp.Format(time.RFC3339),
				candle.OpenPrice, candle.HighPrice, candle.LowPrice, candle.ClosePrice,
				candle.Volume, candle.TradeCount)

			if err := a.postgresStore.StoreCandleData(ctx, symbol, candle); err != nil {
				log.Printf("Failed to store candle data: %v", err)
				continue
			}
			delete(a.candles, key)
			flushedCount++
			log.Printf("Successfully flushed candle for %s at %s", symbol, candle.Timestamp.Format(time.RFC3339))
		} else {
			log.Printf("Skipping current candle for %s at %s (not complete yet)",
				strings.Split(key, ":")[0], candle.Timestamp.Format(time.RFC3339))
		}
	}

	log.Printf("Flush complete: flushed %d candles, %d remaining in memory",
		flushedCount, len(a.candles))

	return nil
}

// migrateHistoricalData moves old data from Redis to PostgreSQL
func (a *TradeAggregator) migrateHistoricalData(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-ticker.C:
			if err := a.performMigration(ctx); err != nil {
				log.Printf("Error migrating historical data: %v", err)
			}
		}
	}
}

// performMigration performs the actual data migration
func (a *TradeAggregator) performMigration(ctx context.Context) error {
	// Get symbols from Redis
	symbolsKey := fmt.Sprintf("%ssymbols", a.redisStore.config.Redis.KeyPrefix)
	symbols, err := a.redisStore.client.SMembers(ctx, symbolsKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get symbols: %w", err)
	}

	for _, symbol := range symbols {
		// Get trades older than 2 hours for migration to PostgreSQL
		end := time.Now().Add(-2 * time.Hour)
		start := end.Add(-22 * time.Hour) // Get the remaining 22 hours to complete 24h in PostgreSQL

		trades, err := a.redisStore.GetTradeHistory(ctx, symbol, start, end)
		if err != nil {
			log.Printf("Error getting trade history for %s: %v", symbol, err)
			continue
		}

		// Group trades by minute
		candleMap := make(map[time.Time]*models.Candle)
		for _, trade := range trades {
			// Convert milliseconds to time
			tradeTime := time.UnixMilli(trade.Data.TradeTime).Truncate(time.Minute)

			if candle, exists := candleMap[tradeTime]; exists {
				candle.UpdateFromTrade(trade.Data.ToTrade())
			} else {
				candle = models.NewCandle(tradeTime)
				candle.UpdateFromTrade(trade.Data.ToTrade())
				candleMap[tradeTime] = candle
			}
		}

		// Store candles in PostgreSQL
		for _, candle := range candleMap {
			if err := a.postgresStore.StoreCandleData(ctx, symbol, candle); err != nil {
				log.Printf("Error storing candle data for %s: %v", symbol, err)
				continue
			}
		}

		// After successful migration, clean up Redis data older than retention period
		if err := a.redisStore.trimHistory(ctx, fmt.Sprintf("%strade:%s:history",
			a.redisStore.config.Redis.KeyPrefix, strings.ToUpper(symbol))); err != nil {
			log.Printf("Warning: failed to trim Redis history for %s: %v", symbol, err)
		}
	}

	return nil
}

// Stop stops the aggregator
func (a *TradeAggregator) Stop() {
	close(a.stopCh)
}
