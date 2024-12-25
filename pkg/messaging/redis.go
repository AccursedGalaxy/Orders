package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/go-redis/redis/v8"

	"binance-redis-streamer/internal/models"
)

const tradeChannel = "trades"

// RedisBus implements MessageBus using Redis Pub/Sub
type RedisBus struct {
	client *redis.Client
}

// NewRedisBus creates a new Redis message bus
func NewRedisBus(client *redis.Client) *RedisBus {
	return &RedisBus{
		client: client,
	}
}

// Publish publishes a trade event to Redis
func (r *RedisBus) Publish(ctx context.Context, trade *models.AggTradeEvent) error {
	data, err := json.Marshal(trade)
	if err != nil {
		return fmt.Errorf("failed to marshal trade: %w", err)
	}

	if err := r.client.Publish(ctx, tradeChannel, data).Err(); err != nil {
		return fmt.Errorf("failed to publish trade: %w", err)
	}

	return nil
}

// Subscribe subscribes to trade events
func (r *RedisBus) Subscribe(ctx context.Context, handler func(trade *models.AggTradeEvent) error) error {
	pubsub := r.client.Subscribe(ctx, tradeChannel)

	// Start subscription in a goroutine
	go func() {
		defer pubsub.Close()
		ch := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-ch:
				if msg == nil {
					continue
				}
				var trade models.AggTradeEvent
				if err := json.Unmarshal([]byte(msg.Payload), &trade); err != nil {
					log.Printf("Failed to unmarshal trade: %v", err)
					continue
				}

				if err := handler(&trade); err != nil {
					log.Printf("Failed to handle trade: %v", err)
				}
			}
		}
	}()

	return nil
}

// Close closes the Redis Pub/Sub connection
func (r *RedisBus) Close() error {
	return nil
} 