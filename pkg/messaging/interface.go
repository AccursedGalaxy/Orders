package messaging

import (
	"context"

	"binance-redis-streamer/internal/models"
)

// MessageBus defines the interface for message passing
type MessageBus interface {
	// Publish publishes a trade event
	Publish(ctx context.Context, trade *models.AggTradeEvent) error
	// Subscribe subscribes to trade events
	Subscribe(ctx context.Context, handler func(trade *models.AggTradeEvent) error) error
	// Close closes the message bus connection
	Close() error
}
