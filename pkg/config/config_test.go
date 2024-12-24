package config

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Test Redis defaults
	if cfg.Redis.RetentionPeriod != 24*time.Hour {
		t.Errorf("Expected RetentionPeriod to be 24h, got %v", cfg.Redis.RetentionPeriod)
	}

	if cfg.Redis.CleanupInterval != time.Hour {
		t.Errorf("Expected CleanupInterval to be 1h, got %v", cfg.Redis.CleanupInterval)
	}

	if cfg.Redis.KeyPrefix != "binance:" {
		t.Errorf("Expected KeyPrefix to be 'binance:', got %v", cfg.Redis.KeyPrefix)
	}

	// Test Binance defaults
	if cfg.Binance.MaxSymbols != 5 {
		t.Errorf("Expected MaxSymbols to be 5, got %v", cfg.Binance.MaxSymbols)
	}

	expectedSymbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT"}
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
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		modifyConfig func(*Config)
		expectError bool
	}{
		{
			name: "valid default config",
			modifyConfig: func(c *Config) {},
			expectError: false,
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