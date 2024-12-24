package storage

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/go-redis/redis/v8"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/config"
)

// TradeStore defines the interface for trade storage
type TradeStore interface {
	StoreTrade(ctx context.Context, trade *models.Trade) error
	StoreRawTrade(ctx context.Context, symbol string, data []byte) error
	GetTradeHistory(ctx context.Context, symbol string, start, end time.Time) ([]models.AggTradeEvent, error)
	GetLatestTrade(ctx context.Context, symbol string) (*models.Trade, error)
	GetRedisClient() *redis.Client
	Close() error
}

// RedisStore implements TradeStore using Redis
type RedisStore struct {
	client *redis.Client
	config *config.Config
	stopCh chan struct{}
}

// NewRedisStore creates a new Redis store
func NewRedisStore(cfg *config.Config) (*RedisStore, error) {
	if cfg.Redis.URL == "" {
		return nil, fmt.Errorf("Redis URL is empty")
	}
	
	log.Printf("Attempting to connect to Redis at URL: %s", cfg.Redis.URL)
	
	opt, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}

	// Configure TLS for Heroku Redis
	if opt.TLSConfig != nil {
		opt.TLSConfig.InsecureSkipVerify = true
	}

	log.Printf("Parsed Redis options: addr=%s db=%d", opt.Addr, opt.DB)
	
	client := redis.NewClient(opt)

	// Test connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection error: %w", err)
	}

	log.Printf("Successfully connected to Redis at %s", cfg.Redis.URL)

	store := &RedisStore{
		client: client,
		config: cfg,
		stopCh: make(chan struct{}),
	}

	// Start cleanup routine
	go store.cleanupRoutine(ctx)

	return store, nil
}

// compressData compresses data using gzip
func (s *RedisStore) compressData(data []byte) ([]byte, error) {
	if !s.config.Redis.UseCompression {
		return data, nil
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decompressData decompresses gzipped data
func (s *RedisStore) decompressData(data []byte) ([]byte, error) {
	if !s.config.Redis.UseCompression {
		return data, nil
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	return io.ReadAll(gz)
}

// StoreTrade stores a trade in Redis with expiration
func (s *RedisStore) StoreTrade(ctx context.Context, trade *models.Trade) error {
	key := fmt.Sprintf("%saggTrade:%s:latest", s.config.Redis.KeyPrefix, trade.Symbol)
	
	// Convert trade to JSON
	tradeJSON, err := json.Marshal(trade)
	if err != nil {
		return fmt.Errorf("failed to marshal trade: %w", err)
	}

	// Compress if enabled
	if compressed, err := s.compressData(tradeJSON); err == nil {
		tradeJSON = compressed
	} else {
		log.Printf("Warning: compression failed: %v", err)
	}

	pipe := s.client.Pipeline()
	
	// Store trade data
	pipe.Set(ctx, key, tradeJSON, s.config.Redis.RetentionPeriod)

	// Store symbol in a set for easy retrieval of all symbols
	symbolsKey := fmt.Sprintf("%ssymbols", s.config.Redis.KeyPrefix)
	pipe.SAdd(ctx, symbolsKey, trade.Symbol)
	pipe.Expire(ctx, symbolsKey, s.config.Redis.RetentionPeriod)

	_, err = pipe.Exec(ctx)
	return err
}

// StoreRawTrade stores the raw trade data in a sorted set with timestamp
func (s *RedisStore) StoreRawTrade(ctx context.Context, symbol string, data []byte) error {
	timestamp := time.Now().UnixNano()
	historyKey := fmt.Sprintf("%saggTrade:%s:history", s.config.Redis.KeyPrefix, symbol)
	
	// Compress if enabled
	if compressed, err := s.compressData(data); err == nil {
		data = compressed
	} else {
		log.Printf("Warning: compression failed: %v", err)
	}

	pipe := s.client.Pipeline()

	// Store trade data in a sorted set with timestamp as score
	pipe.ZAdd(ctx, historyKey, &redis.Z{
		Score:  float64(timestamp),
		Member: string(data),
	})

	// Set expiration for the sorted set
	pipe.Expire(ctx, historyKey, s.config.Redis.RetentionPeriod)

	// Trim to maintain size limit
	if s.config.Redis.MaxTradesPerKey > 0 {
		pipe.ZRemRangeByRank(ctx, historyKey, 0, int64(-(s.config.Redis.MaxTradesPerKey + 1)))
	}

	// Remove old data
	cutoff := float64(time.Now().Add(-s.config.Redis.RetentionPeriod).UnixNano())
	pipe.ZRemRangeByScore(ctx, historyKey, "-inf", fmt.Sprintf("%f", cutoff))

	_, err := pipe.Exec(ctx)
	return err
}

// GetLatestTrade retrieves the latest trade for a symbol
func (s *RedisStore) GetLatestTrade(ctx context.Context, symbol string) (*models.Trade, error) {
	key := fmt.Sprintf("%saggTrade:%s:latest", s.config.Redis.KeyPrefix, symbol)
	
	data, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("no trade found for symbol %s", symbol)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest trade: %w", err)
	}

	var trade models.Trade
	if err := json.Unmarshal([]byte(data), &trade); err != nil {
		return nil, fmt.Errorf("failed to unmarshal trade data: %w", err)
	}

	return &trade, nil
}

// GetTradeHistory retrieves trade history for a symbol within a time range
func (s *RedisStore) GetTradeHistory(ctx context.Context, symbol string, start, end time.Time) ([]models.AggTradeEvent, error) {
	historyKey := fmt.Sprintf("%saggTrade:%s:history", s.config.Redis.KeyPrefix, symbol)
	
	trades, err := s.client.ZRangeByScore(ctx, historyKey, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", start.UnixNano()),
		Max: fmt.Sprintf("%d", end.UnixNano()),
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to get trade history: %w", err)
	}

	var events []models.AggTradeEvent
	for _, tradeData := range trades {
		// Decompress if needed
		data := []byte(tradeData)
		if decompressed, err := s.decompressData(data); err == nil {
			data = decompressed
		} else {
			log.Printf("Warning: decompression failed: %v", err)
			continue
		}

		var event models.AggTradeEvent
		if err := json.Unmarshal(data, &event); err != nil {
			log.Printf("Warning: failed to unmarshal trade data: %v", err)
			continue
		}
		events = append(events, event)
	}

	return events, nil
}

// cleanupRoutine periodically removes old data
func (s *RedisStore) cleanupRoutine(ctx context.Context) {
	ticker := time.NewTicker(s.config.Redis.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanup(ctx)
		}
	}
}

// cleanup removes data older than retention period
func (s *RedisStore) cleanup(ctx context.Context) {
	log.Println("Starting cleanup routine...")
	
	// Get all symbols
	symbolsKey := fmt.Sprintf("%ssymbols", s.config.Redis.KeyPrefix)
	symbols, err := s.client.SMembers(ctx, symbolsKey).Result()
	if err != nil {
		log.Printf("Error getting symbols: %v", err)
		return
	}

	for _, symbol := range symbols {
		historyKey := fmt.Sprintf("%saggTrade:%s:history", s.config.Redis.KeyPrefix, symbol)
		cutoff := float64(time.Now().Add(-s.config.Redis.RetentionPeriod).UnixNano())
		
		// Remove old entries
		if err := s.client.ZRemRangeByScore(ctx, historyKey, "-inf", fmt.Sprintf("%f", cutoff)).Err(); err != nil {
			log.Printf("Error cleaning up old data for %s: %v", symbol, err)
		}
	}

	log.Println("Cleanup routine completed")
}

// Close closes the Redis connection and stops the cleanup routine
func (s *RedisStore) Close() error {
	close(s.stopCh)
	return s.client.Close()
}

// GetRedisClient returns the Redis client instance
func (s *RedisStore) GetRedisClient() *redis.Client {
	return s.client
} 