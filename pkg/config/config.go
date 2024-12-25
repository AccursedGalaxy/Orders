package config

import (
	"fmt"
	"log"
	"os"
	"time"
)

// Config holds all configuration for the application
type Config struct {
	Redis     RedisConfig
	Binance   BinanceConfig
	WebSocket WebSocketConfig
	NATS      NATSConfig
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
	BaseURL            string
	MaxStreamsPerConn  int
	HistorySize        int64
	// New fields for symbol filtering
	MainSymbols        []string // Priority symbols to track (e.g., ["BTCUSDT", "ETHUSDT"])
	MaxSymbols         int      // Maximum number of symbols to track (0 for unlimited)
	MinDailyVolume     float64  // Minimum 24h volume to track a symbol (0 for unlimited)
}

// WebSocketConfig holds WebSocket-specific configuration
type WebSocketConfig struct {
	ReconnectDelay time.Duration
	PingInterval   time.Duration
}

// NATSConfig holds NATS-specific configuration
type NATSConfig struct {
	URL            string
	MaxReconnects  int
	ReconnectWait  time.Duration
	ConnectTimeout time.Duration
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Redis: RedisConfig{
			URL:             getRedisURL(),
			RetentionPeriod: 24 * time.Hour,
			CleanupInterval: 1 * time.Hour,
			KeyPrefix:       "binance:",
			UseCompression:  true,
			MaxTradesPerKey: 1000000, // Limit history per symbol
		},
		Binance: BinanceConfig{
			BaseURL:           "https://api.binance.com",
			MaxStreamsPerConn: 200,
			HistorySize:       10000,
			MainSymbols:       []string{"BTCUSDT", "ETHUSDT", "BNBUSDT"},
			MaxSymbols:        5,                                                     // Limit total symbols
			MinDailyVolume:    1000000.0,                                             // $1M daily volume minimum
		},
		WebSocket: WebSocketConfig{
			ReconnectDelay: 5 * time.Second,
			PingInterval:   5 * time.Second,
		},
		NATS: NATSConfig{
			URL:            getNATSURL(),
			MaxReconnects:  60,
			ReconnectWait:  2 * time.Second,
			ConnectTimeout: 10 * time.Second,
		},
	}
}

// getRedisURL returns the Redis URL based on the environment
func getRedisURL() string {
	// First check for custom Redis URL (highest priority for local development)
	url := os.Getenv("CUSTOM_REDIS_URL")
	if url != "" {
		log.Printf("Using custom Redis URL from CUSTOM_REDIS_URL environment variable")
		return url
	}

	// Then check for Heroku Redis URL (used in Heroku environment)
	url = os.Getenv("REDIS_URL")
	if url != "" {
		log.Printf("Using Heroku Redis URL from REDIS_URL environment variable")
		return url
	}

	// Default to local Redis if no environment variables are set
	defaultURL := "redis://localhost:6379/0"
	log.Printf("No Redis URL found in environment variables (CUSTOM_REDIS_URL or REDIS_URL), using default local URL: %s", defaultURL)
	return defaultURL
}

// getNATSURL returns the NATS URL based on the environment
func getNATSURL() string {
	url := os.Getenv("NATS_URL")
	if url != "" {
		log.Printf("Using NATS URL from environment: %s", url)
		return url
	}

	defaultURL := "nats://localhost:4222"
	log.Printf("No NATS URL found in environment, using default: %s", defaultURL)
	return defaultURL
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