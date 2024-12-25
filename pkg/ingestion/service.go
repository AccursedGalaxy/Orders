package ingestion

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/binance"
	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/messaging"
)

// Service handles the ingestion of trade data from Binance
type Service struct {
	config     *config.Config
	client     *binance.Client
	messageBus messaging.MessageBus
	mu         sync.RWMutex
	wsConns    map[string]*websocket.Conn
}

// NewService creates a new ingestion service
func NewService(cfg *config.Config, client *binance.Client, messageBus messaging.MessageBus) *Service {
	return &Service{
		config:     cfg,
		client:     client,
		messageBus: messageBus,
		wsConns:    make(map[string]*websocket.Conn),
	}
}

// Start starts the ingestion service
func (s *Service) Start(ctx context.Context) error {
	symbols, err := s.client.GetSymbols(ctx)
	if err != nil {
		return fmt.Errorf("failed to get symbols: %w", err)
	}

	// Create symbol groups for parallel processing
	symbolGroups := s.createSymbolGroups(symbols)

	// Start processing each group
	var wg sync.WaitGroup
	errChan := make(chan error, len(symbolGroups))

	for _, group := range symbolGroups {
		wg.Add(1)
		go func(symbols []string) {
			defer wg.Done()
			if err := s.processSymbolGroup(ctx, symbols); err != nil {
				select {
				case errChan <- err:
				default:
				}
			}
		}(group)
	}

	// Wait for error or context cancellation
	go func() {
		wg.Wait()
		close(errChan)
	}()

	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("streaming error: %w", err)
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// createSymbolGroups splits symbols into groups based on MaxStreamsPerConn
func (s *Service) createSymbolGroups(symbols []string) [][]string {
	var groups [][]string
	for i := 0; i < len(symbols); i += s.config.Binance.MaxStreamsPerConn {
		end := i + s.config.Binance.MaxStreamsPerConn
		if end > len(symbols) {
			end = len(symbols)
		}
		groups = append(groups, symbols[i:end])
	}
	return groups
}

// processSymbolGroup handles WebSocket connection for a group of symbols
func (s *Service) processSymbolGroup(ctx context.Context, symbols []string) error {
	url := s.client.BuildStreamURL(symbols)
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := s.connectAndStream(ctx, url, symbols); err != nil {
				log.Printf("Stream error for symbols %v: %v, reconnecting...", symbols, err)
				time.Sleep(s.config.WebSocket.ReconnectDelay)
				continue
			}
		}
	}
}

// connectAndStream establishes WebSocket connection and processes messages
func (s *Service) connectAndStream(ctx context.Context, url string, symbols []string) error {
	wsConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial error: %w", err)
	}
	defer wsConn.Close()

	// Store connection
	connKey := fmt.Sprintf("%v", symbols)
	s.mu.Lock()
	s.wsConns[connKey] = wsConn
	s.mu.Unlock()

	// Remove connection on exit
	defer func() {
		s.mu.Lock()
		delete(s.wsConns, connKey)
		s.mu.Unlock()
	}()

	// Set up ping handler
	go s.handlePing(ctx, wsConn)

	// Process messages
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, message, err := wsConn.ReadMessage()
			if err != nil {
				return fmt.Errorf("websocket read error: %w", err)
			}

			if err := s.processMessage(ctx, message); err != nil {
				log.Printf("Failed to process message: %v", err)
			}
		}
	}
}

// handlePing sends periodic ping messages to keep the connection alive
func (s *Service) handlePing(ctx context.Context, conn *websocket.Conn) {
	ticker := time.NewTicker(s.config.WebSocket.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("Failed to send ping: %v", err)
				return
			}
		}
	}
}

// processMessage processes a WebSocket message and publishes it to NATS
func (s *Service) processMessage(ctx context.Context, message []byte) error {
	var event models.AggTradeEvent
	if err := event.UnmarshalJSON(message); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Publish to message bus
	if err := s.messageBus.Publish(ctx, &event); err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	return nil
}

// Stop gracefully stops all WebSocket connections
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, conn := range s.wsConns {
		conn.Close()
	}
	s.wsConns = make(map[string]*websocket.Conn)
} 