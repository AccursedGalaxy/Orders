package storage

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
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
	Update24hVolume(ctx context.Context, symbol string) error
}

// RedisStore handles Redis storage operations
type RedisStore struct {
	client *redis.Client
	config *config.Config
}

// NewRedisStore creates a new Redis store
func NewRedisStore(cfg *config.Config) (*RedisStore, error) {
	if cfg.Debug {
		log.Printf("Attempting to connect to Redis at URL: %s", cfg.Redis.URL)
	}

	opt, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
	}
	if cfg.Debug {
		log.Printf("Parsed Redis options: addr=%s db=%d", opt.Addr, opt.DB)
	}

	// Configure TLS for Heroku Redis
	if opt.TLSConfig != nil {
		if cfg.Debug {
			log.Printf("Configuring TLS for Redis connection")
		}
		opt.TLSConfig.InsecureSkipVerify = true
	}

	client := redis.NewClient(opt)
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	if cfg.Debug {
		log.Printf("Successfully connected to Redis at %s", cfg.Redis.URL)
	}

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
	// Add symbol to tracked symbols set
	symbolsKey := fmt.Sprintf("%ssymbols", s.config.Redis.KeyPrefix)
	if err := s.client.SAdd(ctx, symbolsKey, strings.ToUpper(trade.Symbol)).Err(); err != nil {
		return fmt.Errorf("failed to add symbol to set: %w", err)
	}

	// Store latest trade
	latestKey := fmt.Sprintf("%strade:%s:latest", s.config.Redis.KeyPrefix, strings.ToUpper(trade.Symbol))
	data, err := json.Marshal(trade)
	if err != nil {
		return fmt.Errorf("failed to marshal trade: %w", err)
	}

	if err := s.client.Set(ctx, latestKey, data, s.config.Redis.RetentionPeriod).Err(); err != nil {
		return fmt.Errorf("failed to store latest trade: %w", err)
	}

	// Store in history
	historyKey := fmt.Sprintf("%strade:%s:history", s.config.Redis.KeyPrefix, strings.ToUpper(trade.Symbol))

	// Create AggTradeEvent from Trade
	event := models.AggTradeEvent{
		Stream: fmt.Sprintf("%s@trade", strings.ToLower(trade.Symbol)),
		Data: models.TradeData{
			EventType: "trade",
			EventTime: trade.EventTime.UnixMilli(),
			Symbol:    trade.Symbol,
			TradeID:   trade.TradeID,
			Price:     trade.Price,
			Quantity:  trade.Quantity,
			TradeTime: trade.Time.UnixMilli(),
		},
	}

	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal trade event: %w", err)
	}

	// Add to sorted set with score as timestamp in milliseconds
	if err := s.client.ZAdd(ctx, historyKey, &redis.Z{
		Score:  float64(trade.Time.UnixMilli()),
		Member: string(eventData),
	}).Err(); err != nil {
		return fmt.Errorf("failed to store trade history: %w", err)
	}

	// Trim old trades
	if err := s.trimHistory(ctx, historyKey); err != nil {
		if s.config.Debug {
			log.Printf("Warning: failed to trim history: %v", err)
		}
	}

	// Update running volume in Redis
	volumeKey := fmt.Sprintf("%s%s:volume:running", s.config.Redis.KeyPrefix, strings.ToUpper(trade.Symbol))
	price, _ := strconv.ParseFloat(trade.Price, 64)
	quantity, _ := strconv.ParseFloat(trade.Quantity, 64)
	tradeVolume := price * quantity

	// Atomically update running volume
	pipe := s.client.Pipeline()
	pipe.IncrByFloat(ctx, volumeKey, tradeVolume)
	pipe.Expire(ctx, volumeKey, 24*time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("Warning: failed to update running volume: %v", err)
	}

	// Trigger volume update less frequently (every 5 minutes per symbol)
	updateKey := fmt.Sprintf("%s%s:volume:last_update", s.config.Redis.KeyPrefix, strings.ToUpper(trade.Symbol))
	needsUpdate, err := s.client.SetNX(ctx, updateKey, time.Now().Unix(), 5*time.Minute).Result()
	if err != nil {
		log.Printf("Warning: failed to check last update time: %v", err)
	} else if needsUpdate {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.Update24hVolume(ctx, trade.Symbol); err != nil && s.config.Debug {
				log.Printf("Warning: failed to update 24h volume for %s: %v", trade.Symbol, err)
			}
		}()
	}

	return nil
}

// StoreRawTrade stores a raw trade event in Redis
func (s *RedisStore) StoreRawTrade(ctx context.Context, symbol string, data []byte) error {
	historyKey := fmt.Sprintf("%strade:%s:history", s.config.Redis.KeyPrefix, strings.ToUpper(symbol))

	if s.config.Debug {
		// Debug: Print raw trade data being stored
		log.Printf("Storing raw trade data for %s: %s", symbol, string(data))
	}

	// Parse event to get timestamp for score
	var event struct {
		Data struct {
			TradeTime int64 `json:"T"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return fmt.Errorf("failed to parse trade time: %w", err)
	}

	// Add to sorted set with score as timestamp in milliseconds
	if err := s.client.ZAdd(ctx, historyKey, &redis.Z{
		Score:  float64(event.Data.TradeTime), // TradeTime is already in milliseconds
		Member: data,
	}).Err(); err != nil {
		return fmt.Errorf("failed to store trade history: %w", err)
	}

	if s.config.Debug {
		// Debug: Print stored trade data
		log.Printf("Successfully stored trade data for %s with timestamp %d", symbol, event.Data.TradeTime)
	}

	// Trim old trades
	if err := s.trimHistory(ctx, historyKey); err != nil {
		if s.config.Debug {
			log.Printf("Warning: failed to trim history: %v", err)
		}
	}

	return nil
}

// trimHistory removes old trades from history
func (s *RedisStore) trimHistory(ctx context.Context, key string) error {
	// Remove trades older than retention period (convert to milliseconds)
	oldestTime := time.Now().Add(-s.config.Redis.RetentionPeriod).UnixMilli()

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
	key := fmt.Sprintf("%strade:%s:latest", s.config.Redis.KeyPrefix, strings.ToUpper(symbol))
	data, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
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

// GetTradeHistory gets historical trades for a symbol within a time range
func (s *RedisStore) GetTradeHistory(ctx context.Context, symbol string, start, end time.Time) ([]models.AggTradeEvent, error) {
	key := fmt.Sprintf("%strade:%s:history", s.config.Redis.KeyPrefix, strings.ToUpper(symbol))

	// Convert timestamps to milliseconds for Redis score
	startMs := start.UnixMilli()
	endMs := end.UnixMilli()

	if s.config.Debug {
		log.Printf("Fetching trade history for %s from %s to %s (key: %s)",
			symbol, start.Format(time.RFC3339), end.Format(time.RFC3339), key)
	}

	// Get trades from Redis sorted set
	trades, err := s.client.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", startMs),
		Max: fmt.Sprintf("%d", endMs),
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to get trade history: %w", err)
	}

	if s.config.Debug {
		log.Printf("Found %d trades in Redis for %s", len(trades), symbol)
	}

	events := make([]models.AggTradeEvent, 0, len(trades))
	for _, trade := range trades {
		// Check if data is compressed
		if len(trade) > 2 && trade[0] == 0x1f && trade[1] == 0x8b {
			reader, err := gzip.NewReader(strings.NewReader(trade))
			if err != nil {
				if s.config.Debug {
					log.Printf("Failed to create gzip reader for trade: %v", err)
				}
				continue
			}
			decompressed, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				if s.config.Debug {
					log.Printf("Failed to decompress trade data: %v", err)
				}
				continue
			}
			trade = string(decompressed)
		}

		if s.config.Debug {
			// Debug: Print raw trade data
			log.Printf("Raw trade data: %s", trade)
		}

		var event models.AggTradeEvent
		event.SetDebug(s.config.Debug)
		if err := json.Unmarshal([]byte(trade), &event); err != nil {
			if s.config.Debug {
				log.Printf("Failed to unmarshal trade data: %v", err)
			}
			continue
		}

		events = append(events, event)
	}

	if s.config.Debug {
		log.Printf("Successfully parsed %d trades for %s", len(events), symbol)
	}
	return events, nil
}

// Update24hVolume calculates and stores the 24-hour volume for a symbol
func (s *RedisStore) Update24hVolume(ctx context.Context, symbol string) error {
	volumeKey := fmt.Sprintf("%s%s:volume:24h", s.config.Redis.KeyPrefix, strings.ToUpper(symbol))

	// Use Redis lock to prevent concurrent updates
	lockKey := fmt.Sprintf("%s%s:volume:lock", s.config.Redis.KeyPrefix, strings.ToUpper(symbol))
	locked, err := s.client.SetNX(ctx, lockKey, "1", 30*time.Second).Result()
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	if !locked {
		return nil // Another process is updating the volume
	}
	defer s.client.Del(ctx, lockKey)

	// Check if we need to update (volume key doesn't exist or is about to expire)
	ttl, err := s.client.TTL(ctx, volumeKey).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to get volume TTL: %w", err)
	}
	if ttl > 30*time.Second {
		return nil // Volume is fresh enough
	}

	// Get trades from the last 24 hours
	end := time.Now()
	start := end.Add(-24 * time.Hour)

	trades, err := s.GetTradeHistory(ctx, symbol, start, end)
	if err != nil {
		return fmt.Errorf("failed to get trade history: %w", err)
	}

	// Calculate total volume
	var totalVolume float64
	for _, trade := range trades {
		quantity, err := strconv.ParseFloat(trade.Data.Quantity, 64)
		if err != nil {
			continue
		}
		price, err := strconv.ParseFloat(trade.Data.Price, 64)
		if err != nil {
			continue
		}
		totalVolume += quantity * price
	}

	// Store the volume with 5-minute expiry
	err = s.client.Set(ctx, volumeKey, fmt.Sprintf("%.2f", totalVolume), 5*time.Minute).Err()
	if err != nil {
		return fmt.Errorf("failed to store 24h volume: %w", err)
	}

	if s.config.Debug {
		log.Printf("Updated 24h volume for %s: %.2f", symbol, totalVolume)
	}

	return nil
}
