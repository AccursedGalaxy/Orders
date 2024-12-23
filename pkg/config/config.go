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
			BaseURL:            "https://fapi.binance.com",
			MaxStreamsPerConn:  200,
			HistorySize:        1000,
		},
		WebSocket: WebSocketConfig{
			ReconnectDelay: 5 * time.Second,
			PingInterval:   5 * time.Second,
		},
	}
}

// getRedisURL returns the Redis URL from environment
func getRedisURL() string {
	// Heroku Redis sets REDIS_URL environment variable
	url := os.Getenv("REDIS_URL")
	if url == "" {
		log.Fatal("REDIS_URL environment variable is required but not set")
	}
	log.Printf("Using Redis URL from environment: %s", url)
	return url
} 