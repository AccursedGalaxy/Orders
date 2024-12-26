package processor

import (
	"context"
	"fmt"
	"log"
	"sync"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/messaging"
	"binance-redis-streamer/pkg/storage"
)

// Service handles the processing of trade data
type Service struct {
	config     *config.Config
	messageBus messaging.MessageBus
	redisStore *storage.RedisStore
	aggregator *storage.TradeAggregator
	workerPool chan struct{}
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// NewService creates a new processor service
func NewService(
	cfg *config.Config,
	store *storage.RedisStore,
	aggregator *storage.TradeAggregator,
) *Service {
	return &Service{
		config:     cfg,
		messageBus: messaging.NewRedisPubSub(store.GetRedisClient()),
		redisStore: store,
		aggregator: aggregator,
		workerPool: make(chan struct{}, 100), // Limit concurrent processing
		stopCh:     make(chan struct{}),
	}
}

// Start starts the processor service
func (s *Service) Start(ctx context.Context) error {
	// Subscribe to trade events
	if err := s.messageBus.Subscribe(ctx, s.handleTrade); err != nil {
		return fmt.Errorf("failed to subscribe to trades: %w", err)
	}

	// Wait for context cancellation
	<-ctx.Done()
	return ctx.Err()
}

// handleTrade processes a single trade event
func (s *Service) handleTrade(trade *models.AggTradeEvent) error {
	// Acquire worker from pool
	select {
	case s.workerPool <- struct{}{}:
		defer func() { <-s.workerPool }()
	case <-s.stopCh:
		return fmt.Errorf("service is stopping")
	}

	log.Printf("Received trade event for %s: price=%s, quantity=%s",
		trade.Data.Symbol, trade.Data.Price, trade.Data.Quantity)

	// Convert to trade model
	processedTrade := trade.ToTrade()

	// Store in Redis
	if err := s.redisStore.StoreTrade(context.Background(), processedTrade); err != nil {
		log.Printf("Failed to store trade in Redis: %v", err)
	}

	// Store raw trade data
	if err := s.redisStore.StoreRawTrade(context.Background(), processedTrade.Symbol, trade.Raw); err != nil {
		log.Printf("Failed to store raw trade: %v", err)
	}

	// Process through aggregator
	if err := s.aggregator.ProcessTrade(context.Background(), processedTrade); err != nil {
		log.Printf("Failed to process trade through aggregator: %v", err)
	} else {
		log.Printf("Successfully processed trade through aggregator for %s", processedTrade.Symbol)
	}

	return nil
}

// Stop gracefully stops the processor service
func (s *Service) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}
