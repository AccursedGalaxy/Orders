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

// RedisPubSub implements MessageBus using Redis Pub/Sub
type RedisPubSub struct {
	client *redis.Client
}

// NewRedisPubSub creates a new Redis Pub/Sub message bus
func NewRedisPubSub(client *redis.Client) *RedisPubSub {
	return &RedisPubSub{
		client: client,
	}
}

// Publish publishes a trade event to Redis
func (r *RedisPubSub) Publish(ctx context.Context, trade *models.AggTradeEvent) error {
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
func (r *RedisPubSub) Subscribe(ctx context.Context, handler func(trade *models.AggTradeEvent) error) error {
	pubsub := r.client.Subscribe(ctx, tradeChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
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
}

// Close closes the Redis Pub/Sub connection
func (r *RedisPubSub) Close() error {
	return nil
}
