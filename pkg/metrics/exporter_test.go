package metrics

import (
	"context"
	"testing"
	"time"

	"binance-redis-streamer/pkg/config"

	"github.com/go-redis/redis/v8"
)

func setupTestExporter(t *testing.T) (*MetricsExporter, *redis.Client) {
	cfg := config.DefaultConfig()
	
	// Use test Redis URL
	cfg.Redis.URL = "redis://localhost:6379/0"
	
	opt, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		t.Fatalf("Failed to parse Redis URL: %v", err)
	}

	client := redis.NewClient(opt)

	// Clean up any existing data
	client.FlushAll(context.Background())

	exporter := NewMetricsExporter(cfg, client)

	return exporter, client
}

func TestMetricsExporter_CollectMetrics(t *testing.T) {
	exporter, client := setupTestExporter(t)
	defer client.Close()

	ctx := context.Background()

	// Add test data
	pipe := client.Pipeline()
	pipe.Set(ctx, "binance:aggTrade:BTCUSDT:latest", `{"symbol":"BTCUSDT","price":"50000.00","quantity":"1.0"}`, time.Hour)
	pipe.Set(ctx, "binance:aggTrade:ETHUSDT:latest", `{"symbol":"ETHUSDT","price":"3000.00","quantity":"2.0"}`, time.Hour)
	pipe.SAdd(ctx, "binance:symbols", "BTCUSDT", "ETHUSDT")
	_, err := pipe.Exec(ctx)
	if err != nil {
		t.Fatalf("Failed to set test data: %v", err)
	}

	// Collect metrics
	metrics, err := exporter.CollectMetrics(ctx)
	if err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	// Verify metrics
	if len(metrics.Prices) != 2 {
		t.Errorf("Expected 2 price metrics, got %d", len(metrics.Prices))
	}

	if metrics.Prices["BTCUSDT"] != "50000.00" {
		t.Errorf("Expected BTCUSDT price 50000.00, got %s", metrics.Prices["BTCUSDT"])
	}

	if metrics.Prices["ETHUSDT"] != "3000.00" {
		t.Errorf("Expected ETHUSDT price 3000.00, got %s", metrics.Prices["ETHUSDT"])
	}
}

func TestMetricsExporter_Start(t *testing.T) {
	exporter, client := setupTestExporter(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start metrics collection
	go exporter.Start(ctx)

	// Add test data
	pipe := client.Pipeline()
	pipe.Set(ctx, "binance:aggTrade:BTCUSDT:latest", `{"symbol":"BTCUSDT","price":"50000.00","quantity":"1.0"}`, time.Hour)
	pipe.SAdd(ctx, "binance:symbols", "BTCUSDT")
	_, err := pipe.Exec(ctx)
	if err != nil {
		t.Fatalf("Failed to set test data: %v", err)
	}

	// Wait for metrics to be collected
	time.Sleep(1100 * time.Millisecond)

	// Verify metrics were collected
	metrics, err := exporter.CollectMetrics(ctx)
	if err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	if len(metrics.Prices) != 1 {
		t.Errorf("Expected 1 price metric, got %d", len(metrics.Prices))
	}

	if metrics.Prices["BTCUSDT"] != "50000.00" {
		t.Errorf("Expected BTCUSDT price 50000.00, got %s", metrics.Prices["BTCUSDT"])
	}
} 