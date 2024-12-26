package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/config"
)

// Metrics represents collected metrics
type Metrics struct {
	Prices map[string]string // Symbol -> Price mapping
}

// MetricsExporter handles metrics collection and export
type MetricsExporter struct {
	config *config.Config
	client *redis.Client
	stopCh chan struct{}
}

// NewMetricsExporter creates a new metrics exporter
func NewMetricsExporter(cfg *config.Config, client *redis.Client) *MetricsExporter {
	return &MetricsExporter{
		config: cfg,
		client: client,
		stopCh: make(chan struct{}),
	}
}

// Start starts metrics collection
func (e *MetricsExporter) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			metrics, err := e.CollectMetrics(ctx)
			if err != nil {
				log.Printf("Error collecting metrics: %v", err)
				continue
			}
			e.exportMetrics(metrics)
		}
	}
}

// Stop stops the metrics collection
func (e *MetricsExporter) Stop() {
	close(e.stopCh)
}

// CollectMetrics collects current metrics from Redis
func (e *MetricsExporter) CollectMetrics(ctx context.Context) (*Metrics, error) {
	metrics := &Metrics{
		Prices: make(map[string]string),
	}

	// Get all symbols
	symbolsKey := fmt.Sprintf("%ssymbols", e.config.Redis.KeyPrefix)
	symbols, err := e.client.SMembers(ctx, symbolsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get symbols: %w", err)
	}

	// Get latest trade for each symbol
	for _, symbol := range symbols {
		key := fmt.Sprintf("%saggTrade:%s:latest", e.config.Redis.KeyPrefix, symbol)
		data, err := e.client.Get(ctx, key).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			log.Printf("Error getting latest trade for %s: %v", symbol, err)
			continue
		}

		var trade models.Trade
		if err := json.Unmarshal([]byte(data), &trade); err != nil {
			log.Printf("Error unmarshaling trade data for %s: %v", symbol, err)
			continue
		}

		metrics.Prices[symbol] = trade.Price
	}

	return metrics, nil
}

// exportMetrics exports the collected metrics
func (e *MetricsExporter) exportMetrics(metrics *Metrics) {
	for symbol, price := range metrics.Prices {
		log.Printf("Price for %s: %s", symbol, price)
	}
}
