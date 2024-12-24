package config

import (
	"log"
	"os"
	"time"
)

// Config holds all configuration for the application
type Config struct {
	Redis     RedisConfig
	Binance   BinanceConfig
	WebSocket WebSocketConfig
}

// RedisConfig holds Redis-specific configuration
type RedisConfig struct {
	URL             string
	RetentionPeriod time.Duration
	CleanupInterval time.Duration
	KeyPrefix       string
}

// BinanceConfig holds Binance-specific configuration
type BinanceConfig struct {
	BaseURL            string
	MaxStreamsPerConn  int
	HistorySize        int64
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
			URL:             getRedisURL(),
			RetentionPeriod: 24 * time.Hour,
			CleanupInterval: 1 * time.Hour,
			KeyPrefix:       "binance:",
		},
		Binance: BinanceConfig{
			BaseURL:            "https://api.binance.com",
			MaxStreamsPerConn:  200,
			HistorySize:        1000,
		},
		WebSocket: WebSocketConfig{
			ReconnectDelay: 5 * time.Second,
			PingInterval:   5 * time.Second,
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