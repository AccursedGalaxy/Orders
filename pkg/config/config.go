package config

import "time"

// Config holds all configuration for the application
type Config struct {
	Redis     RedisConfig
	Binance   BinanceConfig
	WebSocket WebSocketConfig
}

// RedisConfig holds Redis-specific configuration
type RedisConfig struct {
	Addr            string
	Password        string
	DB              int
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
			Addr:            "localhost:6379",
			Password:        "",
			DB:              0,
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