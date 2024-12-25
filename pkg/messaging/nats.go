package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"

	"binance-redis-streamer/internal/models"
)

const (
	defaultReconnectWait = 2 * time.Second
	defaultMaxReconnects = 60
	tradeSubject        = "trades"
)

// NATSConfig holds NATS connection configuration
type NATSConfig struct {
	URL            string
	MaxReconnects  int
	ReconnectWait  time.Duration
	ConnectTimeout time.Duration
}

// MessageBus represents a message bus interface
type MessageBus interface {
	Publish(ctx context.Context, trade *models.AggTradeEvent) error
	Subscribe(ctx context.Context, handler func(trade *models.AggTradeEvent) error) error
	Close() error
}

// NATSBus implements MessageBus using NATS
type NATSBus struct {
	conn      *nats.Conn
	config    *NATSConfig
	closeOnce chan struct{}
}

// NewNATSBus creates a new NATS message bus
func NewNATSBus(config *NATSConfig) (*NATSBus, error) {
	if config == nil {
		config = &NATSConfig{
			URL:            nats.DefaultURL,
			MaxReconnects:  defaultMaxReconnects,
			ReconnectWait:  defaultReconnectWait,
			ConnectTimeout: 10 * time.Second,
		}
	}

	opts := []nats.Option{
		nats.MaxReconnects(config.MaxReconnects),
		nats.ReconnectWait(config.ReconnectWait),
		nats.Timeout(config.ConnectTimeout),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Printf("NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("NATS reconnected to %s", nc.ConnectedUrl())
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			log.Printf("NATS error: %v", err)
		}),
	}

	conn, err := nats.Connect(config.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	return &NATSBus{
		conn:      conn,
		config:    config,
		closeOnce: make(chan struct{}),
	}, nil
}

// Publish publishes a trade event to NATS
func (n *NATSBus) Publish(ctx context.Context, trade *models.AggTradeEvent) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-n.closeOnce:
		return fmt.Errorf("message bus is closed")
	default:
		data, err := json.Marshal(trade)
		if err != nil {
			return fmt.Errorf("failed to marshal trade: %w", err)
		}

		if err := n.conn.Publish(tradeSubject, data); err != nil {
			return fmt.Errorf("failed to publish trade: %w", err)
		}

		return nil
	}
}

// Subscribe subscribes to trade events
func (n *NATSBus) Subscribe(ctx context.Context, handler func(trade *models.AggTradeEvent) error) error {
	sub, err := n.conn.Subscribe(tradeSubject, func(msg *nats.Msg) {
		var trade models.AggTradeEvent
		if err := json.Unmarshal(msg.Data, &trade); err != nil {
			log.Printf("Failed to unmarshal trade: %v", err)
			return
		}

		if err := handler(&trade); err != nil {
			log.Printf("Failed to handle trade: %v", err)
		}
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	go func() {
		<-ctx.Done()
		if err := sub.Unsubscribe(); err != nil {
			log.Printf("Failed to unsubscribe: %v", err)
		}
	}()

	return nil
}

// Close closes the NATS connection
func (n *NATSBus) Close() error {
	close(n.closeOnce)
	n.conn.Close()
	return nil
} 