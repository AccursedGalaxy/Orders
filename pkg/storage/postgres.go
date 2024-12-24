package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"binance-redis-streamer/internal/models"
)

// PostgresStore handles historical trade data storage
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a new PostgreSQL store
func NewPostgresStore() (*PostgresStore, error) {
	// Get DATABASE_URL from environment (Heroku sets this automatically)
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("database URL environment variable is not set")
	}

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Set reasonable defaults
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	store := &PostgresStore{db: db}

	// Create tables if they don't exist
	if err := store.createTables(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *PostgresStore) createTables() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS trade_candles (
			symbol TEXT NOT NULL,
			timestamp TIMESTAMPTZ NOT NULL,
			open_price NUMERIC NOT NULL,
			high_price NUMERIC NOT NULL,
			low_price NUMERIC NOT NULL,
			close_price NUMERIC NOT NULL,
			volume NUMERIC NOT NULL,
			trade_count BIGINT NOT NULL,
			PRIMARY KEY (symbol, timestamp)
		);
		
		CREATE INDEX IF NOT EXISTS idx_trade_candles_time 
			ON trade_candles(timestamp);
	`)
	
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}
	
	log.Println("Successfully created/verified PostgreSQL tables")
	return nil
}

// StoreCandleData stores 1-minute aggregated trade data
func (s *PostgresStore) StoreCandleData(ctx context.Context, symbol string, candle *models.Candle) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO trade_candles (
			symbol, timestamp, open_price, high_price, low_price, 
			close_price, volume, trade_count
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (symbol, timestamp) DO UPDATE SET
			open_price = EXCLUDED.open_price,
			high_price = GREATEST(trade_candles.high_price, EXCLUDED.high_price),
			low_price = LEAST(trade_candles.low_price, EXCLUDED.low_price),
			close_price = EXCLUDED.close_price,
			volume = trade_candles.volume + EXCLUDED.volume,
			trade_count = trade_candles.trade_count + EXCLUDED.trade_count`,
		symbol, candle.Timestamp, candle.OpenPrice,
		candle.HighPrice, candle.LowPrice, candle.ClosePrice,
		candle.Volume, candle.TradeCount,
	)
	return err
}

// GetHistoricalCandles retrieves historical candle data
func (s *PostgresStore) GetHistoricalCandles(ctx context.Context, symbol string, start, end time.Time) ([]*models.Candle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT timestamp, open_price, high_price, low_price, 
			   close_price, volume, trade_count
		FROM trade_candles
		WHERE symbol = $1 AND timestamp BETWEEN $2 AND $3
		ORDER BY timestamp ASC`,
		symbol, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candles []*models.Candle
	for rows.Next() {
		candle := &models.Candle{}
		err := rows.Scan(
			&candle.Timestamp, &candle.OpenPrice, &candle.HighPrice,
			&candle.LowPrice, &candle.ClosePrice, &candle.Volume,
			&candle.TradeCount,
		)
		if err != nil {
			return nil, err
		}
		candles = append(candles, candle)
	}

	return candles, rows.Err()
}

// GetAggregatedCandles retrieves candles with custom time buckets
func (s *PostgresStore) GetAggregatedCandles(ctx context.Context, symbol string, start, end time.Time, interval string) ([]*models.Candle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT 
			time_bucket($4, timestamp) as bucket,
			first(open_price, timestamp) as open_price,
			max(high_price) as high_price,
			min(low_price) as low_price,
			last(close_price, timestamp) as close_price,
			sum(volume) as volume,
			sum(trade_count) as trade_count
		FROM trade_candles
		WHERE symbol = $1 AND timestamp BETWEEN $2 AND $3
		GROUP BY bucket
		ORDER BY bucket ASC`,
		symbol, start, end, interval, // interval can be '5 minutes', '1 hour', etc.
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candles []*models.Candle
	for rows.Next() {
		candle := &models.Candle{}
		err := rows.Scan(
			&candle.Timestamp, &candle.OpenPrice, &candle.HighPrice,
			&candle.LowPrice, &candle.ClosePrice, &candle.Volume,
			&candle.TradeCount,
		)
		if err != nil {
			return nil, err
		}
		candles = append(candles, candle)
	}

	return candles, rows.Err()
}

// Close closes the database connection
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

// Vacuum optimizes the database (not needed for PostgreSQL/TimescaleDB)
func (s *PostgresStore) Vacuum() error {
	return nil
} 