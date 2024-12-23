package storage

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/config"
)

// TradeStore defines the interface for trade storage
type TradeStore interface {
	StoreTrade(ctx context.Context, trade *models.Trade) error
	StoreRawTrade(ctx context.Context, symbol string, data []byte) error
	Close() error
}

// RedisStore implements TradeStore using Redis
type RedisStore struct {
	client *redis.Client
	config *config.Config
}

// NewRedisStore creates a new Redis store
func NewRedisStore(cfg *config.Config) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection error: %w", err)
	}

	return &RedisStore{
		client: client,
		config: cfg,
	}, nil
}

// StoreTrade stores a trade in Redis
func (s *RedisStore) StoreTrade(ctx context.Context, trade *models.Trade) error {
	key := fmt.Sprintf("binance:aggTrade:%s", trade.Symbol)
	
	err := s.client.HSet(ctx, key, map[string]interface{}{
		"price":     trade.Price,
		"quantity":  trade.Quantity,
		"tradeId":   trade.TradeID,
		"time":      trade.Time,
		"eventTime": trade.EventTime,
	}).Err()

	if err != nil {
		return fmt.Errorf("failed to store trade: %w", err)
	}

	return nil
}

// StoreRawTrade stores the raw trade data in a time-series list
func (s *RedisStore) StoreRawTrade(ctx context.Context, symbol string, data []byte) error {
	listKey := fmt.Sprintf("binance:aggTrade:history:%s", symbol)
	
	pipe := s.client.Pipeline()
	pipe.LPush(ctx, listKey, string(data))
	pipe.LTrim(ctx, listKey, 0, s.config.Binance.HistorySize-1)
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to store raw trade: %w", err)
	}

	return nil
}

// Close closes the Redis connection
func (s *RedisStore) Close() error {
	return s.client.Close()
} 