package config

import (
	"fmt"
	"os"
	"time"
)

// Config represents the application configuration
type Config struct {
	Redis     RedisConfig
	Binance   BinanceConfig
	WebSocket WebSocketConfig
	Debug     bool
}

// RedisConfig holds Redis-specific configuration
type RedisConfig struct {
	URL             string
	RetentionPeriod time.Duration
	CleanupInterval time.Duration
	KeyPrefix       string
	// New fields for optimization
	UseCompression  bool
	MaxTradesPerKey int // Limit number of trades stored per symbol
}

// BinanceConfig holds Binance-specific configuration
type BinanceConfig struct {
	BaseURL           string
	MaxStreamsPerConn int
	HistorySize       int64
	// New fields for symbol filtering
	MainSymbols    []string // Priority symbols to track (e.g., ["BTCUSDT", "ETHUSDT"])
	MaxSymbols     int      // Maximum number of symbols to track (0 for unlimited)
	MinDailyVolume float64  // Minimum 24h volume to track a symbol (0 for unlimited)
}

// WebSocketConfig holds WebSocket-specific configuration
type WebSocketConfig struct {
	ReconnectDelay time.Duration
	PingInterval   time.Duration
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Redis: RedisConfig{
			URL:             getEnvOrDefault("REDIS_URL", "redis://localhost:6379"),
			RetentionPeriod: 2 * time.Hour,
			CleanupInterval: 5 * time.Minute,
			KeyPrefix:       "binance:",
			MaxTradesPerKey: 100000,
		},
		Binance: BinanceConfig{
			BaseURL:           "https://api.binance.com",
			MaxSymbols:        10,
			MaxStreamsPerConn: 1000,
			MinDailyVolume:    1000000, // 1M USDT
			MainSymbols:       []string{"BTCUSDT", "ETHUSDT"},
		},
		WebSocket: WebSocketConfig{
			PingInterval:   time.Minute,
			ReconnectDelay: 5 * time.Second,
		},
		Debug: false,
	}
}

// getEnvOrDefault returns environment variable value or default if not set
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Redis.RetentionPeriod <= 0 {
		return fmt.Errorf("retention period must be positive")
	}
	if c.Redis.CleanupInterval <= 0 {
		return fmt.Errorf("cleanup interval must be positive")
	}
	if c.Redis.MaxTradesPerKey < 0 {
		return fmt.Errorf("max trades per key must be non-negative")
	}
	return nil
}
