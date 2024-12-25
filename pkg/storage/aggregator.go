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
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

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

	// Create candle key (symbol + minute timestamp)
	candleTime := time.Unix(0, trade.Time).Truncate(time.Minute)
	key := fmt.Sprintf("%s:%d", trade.Symbol, candleTime.Unix())

	// Get or create candle
	candle, exists := a.candles[key]
	if !exists {
		candle = models.NewCandle(candleTime)
		a.candles[key] = candle
	}
	candle.UpdateFromTrade(trade)

	return nil
}

// flushCandles writes completed candles to PostgreSQL
func (a *TradeAggregator) flushCandles(ctx context.Context) error {
	a.candleMu.Lock()
	defer a.candleMu.Unlock()

	currentMinute := time.Now().Truncate(time.Minute)
	for key, candle := range a.candles {
		// Only flush candles from previous minutes
		if candle.Timestamp.Before(currentMinute) {
			symbol := key[:strings.Index(key, ":")]
			if err := a.postgresStore.StoreCandleData(ctx, symbol, candle); err != nil {
				return fmt.Errorf("failed to store candle data: %w", err)
			}
			delete(a.candles, key)
		}
	}

	return nil
}

// migrateHistoricalData moves old data from Redis to PostgreSQL
func (a *TradeAggregator) migrateHistoricalData(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
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
		// Get trades older than 2 hours
		end := time.Now().Add(-2 * time.Hour)
		start := end.Add(-22 * time.Hour) // Get the previous 22 hours of data
		
		trades, err := a.redisStore.GetTradeHistory(ctx, symbol, start, end)
		if err != nil {
			log.Printf("Error getting trade history for %s: %v", symbol, err)
			continue
		}

		// Group trades by minute
		candleMap := make(map[time.Time]*models.Candle)
		for _, trade := range trades {
			tradeTime := time.Unix(0, trade.Data.TradeTime).Truncate(time.Minute)
			
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
	}

	return nil
}

// Stop stops the aggregator
func (a *TradeAggregator) Stop() {
	close(a.stopCh)
}