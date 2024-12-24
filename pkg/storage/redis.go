package storage

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/config"
)

// RedisStore handles Redis storage operations
type RedisStore struct {
	client *redis.Client
	config *config.Config
}

// NewRedisStore creates a new Redis store
func NewRedisStore(cfg *config.Config) (*RedisStore, error) {
	log.Printf("Attempting to connect to Redis at URL: %s", cfg.Redis.URL)
	
	opt, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}
	log.Printf("Parsed Redis options: addr=%s db=%d", opt.Addr, opt.DB)

	client := redis.NewClient(opt)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	log.Printf("Successfully connected to Redis at %s", cfg.Redis.URL)

	return &RedisStore{
		client: client,
		config: cfg,
	}, nil
}

// GetRedisClient returns the underlying Redis client
func (s *RedisStore) GetRedisClient() *redis.Client {
	return s.client
}

// Close closes the Redis connection
func (s *RedisStore) Close() error {
	return s.client.Close()
}

// StoreTrade stores a trade in Redis
func (s *RedisStore) StoreTrade(ctx context.Context, trade *models.Trade) error {
	// Add symbol to set of tracked symbols
	symbolsKey := fmt.Sprintf("%ssymbols", s.config.Redis.KeyPrefix)
	if err := s.client.SAdd(ctx, symbolsKey, trade.Symbol).Err(); err != nil {
		return fmt.Errorf("failed to add symbol to set: %w", err)
	}

	// Store latest trade
	latestKey := fmt.Sprintf("%saggTrade:%s:latest", s.config.Redis.KeyPrefix, trade.Symbol)
	data, err := json.Marshal(trade)
	if err != nil {
		return fmt.Errorf("failed to marshal trade: %w", err)
	}

	if err := s.client.Set(ctx, latestKey, data, s.config.Redis.RetentionPeriod).Err(); err != nil {
		return fmt.Errorf("failed to store latest trade: %w", err)
	}

	return nil
}

// StoreRawTrade stores a raw trade event in Redis
func (s *RedisStore) StoreRawTrade(ctx context.Context, symbol string, data []byte) error {
	// Store in sorted set by timestamp
	historyKey := fmt.Sprintf("%saggTrade:%s:history", s.config.Redis.KeyPrefix, symbol)
	
	// Parse event to get timestamp for score
	var event struct {
		Data struct {
			TradeTime int64 `json:"T"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to parse trade time: %w", err)
	}

	// Add to sorted set with score as timestamp
	if err := s.client.ZAdd(ctx, historyKey, &redis.Z{
		Score:  float64(event.Data.TradeTime),
		Member: data,
	}).Err(); err != nil {
		return fmt.Errorf("failed to store trade history: %w", err)
	}

	// Trim old trades
	if err := s.trimHistory(ctx, historyKey); err != nil {
		log.Printf("Warning: failed to trim history: %v", err)
	}

	return nil
}

// trimHistory removes old trades from history
func (s *RedisStore) trimHistory(ctx context.Context, key string) error {
	// Remove trades older than retention period
	oldestTime := time.Now().Add(-s.config.Redis.RetentionPeriod).UnixNano()
	
	if err := s.client.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", oldestTime)).Err(); err != nil {
		return fmt.Errorf("failed to trim history: %w", err)
	}

	// Trim to max trades if configured
	if s.config.Redis.MaxTradesPerKey > 0 {
		if err := s.client.ZRemRangeByRank(ctx, key, 0, int64(-s.config.Redis.MaxTradesPerKey-1)).Err(); err != nil {
			return fmt.Errorf("failed to trim to max trades: %w", err)
		}
	}

	return nil
}

// GetLatestTrade gets the latest trade for a symbol
func (s *RedisStore) GetLatestTrade(ctx context.Context, symbol string) (*models.Trade, error) {
	key := fmt.Sprintf("%saggTrade:%s:latest", s.config.Redis.KeyPrefix, symbol)
	data, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest trade: %w", err)
	}

	// Check if data is compressed (starts with gzip magic number \x1f\x8b)
	if len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b {
		reader, err := gzip.NewReader(strings.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.Close()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress data: %w", err)
		}
		data = string(decompressed)
	}

	var trade models.Trade
	if err := json.Unmarshal([]byte(data), &trade); err != nil {
		return nil, fmt.Errorf("failed to unmarshal trade data: %w", err)
	}

	return &trade, nil
}

// GetTradeHistory gets historical trades for a symbol within a time range
func (s *RedisStore) GetTradeHistory(ctx context.Context, symbol string, start, end time.Time) ([]models.AggTradeEvent, error) {
	key := fmt.Sprintf("%saggTrade:%s:history", s.config.Redis.KeyPrefix, symbol)
	
	// Get trades from Redis sorted set
	trades, err := s.client.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", start.UnixNano()),
		Max: fmt.Sprintf("%d", end.UnixNano()),
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to get trade history: %w", err)
	}

	events := make([]models.AggTradeEvent, 0, len(trades))
	for _, trade := range trades {
		// Check if data is compressed
		if len(trade) > 2 && trade[0] == 0x1f && trade[1] == 0x8b {
			reader, err := gzip.NewReader(strings.NewReader(trade))
			if err != nil {
				log.Printf("Failed to create gzip reader for trade: %v", err)
				continue
			}
			decompressed, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				log.Printf("Failed to decompress trade data: %v", err)
				continue
			}
			trade = string(decompressed)
		}

		var event models.AggTradeEvent
		if err := json.Unmarshal([]byte(trade), &event); err != nil {
			log.Printf("Failed to unmarshal trade data: %v", err)
			continue
		}
		events = append(events, event)
	}

	return events, nil
} 