package config

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Test Redis defaults
	if cfg.Redis.RetentionPeriod != 2*time.Hour {
		t.Errorf("Expected RetentionPeriod to be 2h, got %v", cfg.Redis.RetentionPeriod)
	}

	if cfg.Redis.CleanupInterval != 5*time.Minute {
		t.Errorf("Expected CleanupInterval to be 5m, got %v", cfg.Redis.CleanupInterval)
	}

	if cfg.Redis.KeyPrefix != "binance:" {
		t.Errorf("Expected KeyPrefix to be 'binance:', got %v", cfg.Redis.KeyPrefix)
	}

	if cfg.Redis.MaxTradesPerKey != 100000 {
		t.Errorf("Expected MaxTradesPerKey to be 100000, got %d", cfg.Redis.MaxTradesPerKey)
	}

	// Test Binance defaults
	if cfg.Binance.MaxSymbols != 10 {
		t.Errorf("Expected MaxSymbols to be 10, got %v", cfg.Binance.MaxSymbols)
	}

	expectedSymbols := []string{"BTCUSDT", "ETHUSDT"}
	if len(cfg.Binance.MainSymbols) != len(expectedSymbols) {
		t.Errorf("Expected MainSymbols to be %v, got %v", expectedSymbols, cfg.Binance.MainSymbols)
	}
	for i, symbol := range expectedSymbols {
		if cfg.Binance.MainSymbols[i] != symbol {
			t.Errorf("Expected symbol %s at position %d, got %s", symbol, i, cfg.Binance.MainSymbols[i])
		}
	}

	if cfg.Binance.MinDailyVolume != 1000000.0 {
		t.Errorf("Expected MinDailyVolume to be 1000000.0, got %v", cfg.Binance.MinDailyVolume)
	}

	// Test WebSocket defaults
	if cfg.WebSocket.PingInterval != time.Minute {
		t.Errorf("Expected PingInterval to be 1m, got %v", cfg.WebSocket.PingInterval)
	}

	if cfg.WebSocket.ReconnectDelay != 5*time.Second {
		t.Errorf("Expected ReconnectDelay to be 5s, got %v", cfg.WebSocket.ReconnectDelay)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name         string
		modifyConfig func(*Config)
		expectError  bool
	}{
		{
			name:         "valid default config",
			modifyConfig: func(c *Config) {},
			expectError:  false,
		},
		{
			name: "invalid retention period",
			modifyConfig: func(c *Config) {
				c.Redis.RetentionPeriod = -1 * time.Hour
			},
			expectError: true,
		},
		{
			name: "invalid cleanup interval",
			modifyConfig: func(c *Config) {
				c.Redis.CleanupInterval = -1 * time.Hour
			},
			expectError: true,
		},
		{
			name: "invalid max trades per key",
			modifyConfig: func(c *Config) {
				c.Redis.MaxTradesPerKey = -1
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modifyConfig(cfg)
			err := cfg.Validate()

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
