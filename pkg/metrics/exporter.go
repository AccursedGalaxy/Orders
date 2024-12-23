package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"

	"binance-redis-streamer/pkg/config"
)

// TradeMetrics holds aggregated trade metrics
type TradeMetrics struct {
	Symbol        string
	Price         float64
	Volume24h     float64
	TradeCount24h int64
	LastUpdate    time.Time
	HighPrice24h  float64
	LowPrice24h   float64
}

// MetricsExporter handles metrics calculation and export
type MetricsExporter struct {
	redis  *redis.Client
	config *config.Config
	stopCh chan struct{}
	mutex  sync.RWMutex
	metrics map[string]*TradeMetrics
}

// NewMetricsExporter creates a new metrics exporter
func NewMetricsExporter(cfg *config.Config, redisClient *redis.Client) *MetricsExporter {
	return &MetricsExporter{
		redis:   redisClient,
		config:  cfg,
		stopCh:  make(chan struct{}),
		metrics: make(map[string]*TradeMetrics),
	}
}

// Start begins metrics collection
func (m *MetricsExporter) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Initial collection
	m.collectMetrics(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.collectMetrics(ctx)
		}
	}
}

// collectMetrics gathers metrics from Redis
func (m *MetricsExporter) collectMetrics(ctx context.Context) {
	// Get all symbols
	symbolsKey := fmt.Sprintf("%ssymbols", m.config.Redis.KeyPrefix)
	symbols, err := m.redis.SMembers(ctx, symbolsKey).Result()
	if err != nil {
		log.Printf("Error getting symbols: %v", err)
		return
	}

	for _, symbol := range symbols {
		metrics, err := m.calculateSymbolMetrics(ctx, symbol)
		if err != nil {
			log.Printf("Error calculating metrics for %s: %v", symbol, err)
			continue
		}

		m.mutex.Lock()
		m.metrics[symbol] = metrics
		m.mutex.Unlock()
	}
}

// calculateSymbolMetrics calculates metrics for a single symbol
func (m *MetricsExporter) calculateSymbolMetrics(ctx context.Context, symbol string) (*TradeMetrics, error) {
	historyKey := fmt.Sprintf("%saggTrade:%s:history", m.config.Redis.KeyPrefix, symbol)
	now := time.Now()
	start := now.Add(-24 * time.Hour)

	// Get trades from last 24 hours
	trades, err := m.redis.ZRangeByScore(ctx, historyKey, &redis.ZRangeBy{
		Min: fmt.Sprintf("%d", start.UnixNano()),
		Max: fmt.Sprintf("%d", now.UnixNano()),
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to get trades: %w", err)
	}

	metrics := &TradeMetrics{
		Symbol:     symbol,
		LastUpdate: now,
	}

	var volume float64
	for _, trade := range trades {
		var event struct {
			Data struct {
				Price    string `json:"p"`
				Quantity string `json:"q"`
			} `json:"data"`
		}

		if err := json.Unmarshal([]byte(trade), &event); err != nil {
			continue
		}

		price, _ := strconv.ParseFloat(event.Data.Price, 64)
		quantity, _ := strconv.ParseFloat(event.Data.Quantity, 64)

		if price > metrics.HighPrice24h {
			metrics.HighPrice24h = price
		}
		if metrics.LowPrice24h == 0 || price < metrics.LowPrice24h {
			metrics.LowPrice24h = price
		}

		volume += price * quantity
		metrics.Price = price // Last price
	}

	metrics.Volume24h = volume
	metrics.TradeCount24h = int64(len(trades))

	return metrics, nil
}

// GetMetrics returns current metrics for a symbol
func (m *MetricsExporter) GetMetrics(symbol string) (*TradeMetrics, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	metrics, ok := m.metrics[symbol]
	return metrics, ok
}

// GetAllMetrics returns metrics for all symbols
func (m *MetricsExporter) GetAllMetrics() map[string]*TradeMetrics {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	
	// Create a copy to avoid map mutations while reading
	metrics := make(map[string]*TradeMetrics, len(m.metrics))
	for k, v := range m.metrics {
		metrics[k] = v
	}
	return metrics
}

// Stop stops the metrics collection
func (m *MetricsExporter) Stop() {
	close(m.stopCh)
} 