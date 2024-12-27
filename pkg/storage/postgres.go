package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"binance-redis-streamer/internal/models"
)

// PostgresStore handles historical trade data storage
type PostgresStore struct {
	db    *sql.DB
	debug bool
}

// SetDebug sets the debug flag
func (s *PostgresStore) SetDebug(debug bool) {
	s.debug = debug
}

// NewPostgresStore creates a new PostgreSQL store
func NewPostgresStore() (*PostgresStore, error) {
	// Get DATABASE_URL from environment (Heroku sets this automatically)
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Printf("Warning: DATABASE_URL environment variable is not set, using default configuration")
		dbURL = "postgres://postgres:postgres@localhost:5432/binance_trades?sslmode=disable"
	}

	log.Printf("Attempting to connect to PostgreSQL at: %s", maskPassword(dbURL))

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

	store := &PostgresStore{
		db:    db,
		debug: true,
	}

	// Create tables if they don't exist
	if err := store.createTables(); err != nil {
		db.Close()
		return nil, err
	}

	log.Printf("Successfully connected to PostgreSQL")
	return store, nil
}

// maskPassword masks the password in the database URL for logging
func maskPassword(dbURL string) string {
	if strings.Contains(dbURL, "@") {
		parts := strings.Split(dbURL, "@")
		if len(parts) > 1 {
			credentials := strings.Split(parts[0], ":")
			if len(credentials) > 2 {
				return fmt.Sprintf("%s:****@%s", credentials[0], parts[1])
			}
		}
	}
	return "postgres://****:****@host:5432/database"
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
	if s.debug {
		log.Printf("[DEBUG] Attempting to store candle data for %s at %s", symbol, candle.Timestamp.Format(time.RFC3339))
		log.Printf("[DEBUG] Candle data: open=%s, high=%s, low=%s, close=%s, volume=%s, trades=%d",
			candle.OpenPrice, candle.HighPrice, candle.LowPrice, candle.ClosePrice,
			candle.Volume, candle.TradeCount)
	}

	// Ensure timestamp is in UTC
	timestamp := candle.Timestamp.UTC()
	if timestamp.IsZero() {
		return fmt.Errorf("invalid timestamp: zero value")
	}

	if s.debug {
		log.Printf("[DEBUG] Using UTC timestamp: %s for candle data", timestamp.Format(time.RFC3339))
	}

	result, err := s.db.ExecContext(ctx, `
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
			trade_count = trade_candles.trade_count + EXCLUDED.trade_count
		RETURNING (xmax = 0) as inserted`,
		symbol, timestamp, candle.OpenPrice,
		candle.HighPrice, candle.LowPrice, candle.ClosePrice,
		candle.Volume, candle.TradeCount,
	)

	if err != nil {
		if s.debug {
			log.Printf("[ERROR] Failed to store candle data: %v", err)
		}
		return fmt.Errorf("failed to store candle data: %w", err)
	}

	if s.debug {
		rowsAffected, _ := result.RowsAffected()
		log.Printf("[DEBUG] Successfully stored candle data. Rows affected: %d", rowsAffected)
	}

	return nil
}

// GetHistoricalCandles retrieves historical candle data
func (s *PostgresStore) GetHistoricalCandles(ctx context.Context, symbol string, start, end time.Time) ([]*models.Candle, error) {
	if s.debug {
		log.Printf("Fetching historical candles for %s from %s to %s",
			symbol, start.Format(time.RFC3339), end.Format(time.RFC3339))
	}

	// First check if any data exists for this symbol and get the time range
	var count int
	var minTime, maxTime sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), MIN(timestamp), MAX(timestamp) 
		FROM trade_candles 
		WHERE symbol = $1`,
		symbol,
	).Scan(&count, &minTime, &maxTime)

	if err != nil {
		if s.debug {
			log.Printf("Error checking candle data for %s: %v", symbol, err)
		}
	} else if s.debug {
		log.Printf("Found %d total candles for %s", count, symbol)
		if count > 0 {
			log.Printf("Data time range for %s: %s to %s",
				symbol,
				minTime.Time.Format(time.RFC3339),
				maxTime.Time.Format(time.RFC3339))
		}
	}

	// Get candles for the specified time range
	query := `
		SELECT timestamp, open_price, high_price, low_price, 
			   close_price, volume, trade_count
		FROM trade_candles
		WHERE symbol = $1 AND timestamp BETWEEN $2 AND $3
		ORDER BY timestamp ASC`

	if s.debug {
		log.Printf("Executing query: %s with params: symbol=%s, start=%s, end=%s",
			query, symbol, start.Format(time.RFC3339), end.Format(time.RFC3339))
	}

	rows, err := s.db.QueryContext(ctx, query, symbol, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to query historical candles: %w", err)
	}
	defer rows.Close()

	// Pre-allocate the slice with the expected capacity
	candles := make([]*models.Candle, 0, count)

	for rows.Next() {
		candle := &models.Candle{}
		err := rows.Scan(
			&candle.Timestamp, &candle.OpenPrice, &candle.HighPrice,
			&candle.LowPrice, &candle.ClosePrice, &candle.Volume,
			&candle.TradeCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan candle data: %w", err)
		}
		candles = append(candles, candle)

		if s.debug {
			log.Printf("Retrieved candle for %s at %s: open=%s, close=%s, volume=%s",
				symbol, candle.Timestamp.Format(time.RFC3339),
				candle.OpenPrice, candle.ClosePrice, candle.Volume)
		}
	}

	if s.debug {
		log.Printf("Found %d historical candles for %s in the specified time range", len(candles), symbol)
	}

	return candles, rows.Err()
}

// GetAggregatedCandles retrieves candles with custom time buckets
func (s *PostgresStore) GetAggregatedCandles(ctx context.Context, symbol string, start, end time.Time, interval string) ([]*models.Candle, error) {
	// Convert interval string to PostgreSQL interval (e.g., '1m' to 'minute')
	pgInterval := "minute"
	if strings.HasSuffix(interval, "m") {
		if val := strings.TrimSuffix(interval, "m"); val != "1" {
			pgInterval = fmt.Sprintf("%s minutes", val)
		}
	} else if strings.HasSuffix(interval, "h") {
		if val := strings.TrimSuffix(interval, "h"); val == "1" {
			pgInterval = "hour"
		} else {
			pgInterval = fmt.Sprintf("%s hours", val)
		}
	}

	if s.debug {
		log.Printf("[DEBUG] Converting interval %s to PostgreSQL interval: %s", interval, pgInterval)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT 
			date_trunc($4, timestamp) as bucket,
			FIRST_VALUE(open_price) OVER (PARTITION BY date_trunc($4, timestamp) ORDER BY timestamp) as open_price,
			MAX(high_price) as high_price,
			MIN(low_price) as low_price,
			LAST_VALUE(close_price) OVER (PARTITION BY date_trunc($4, timestamp) ORDER BY timestamp) as close_price,
			SUM(volume) as volume,
			SUM(trade_count) as trade_count
		FROM trade_candles
		WHERE symbol = $1 AND timestamp BETWEEN $2 AND $3
		GROUP BY bucket, open_price, close_price
		ORDER BY bucket ASC`,
		symbol, start, end, pgInterval,
	)
	if err != nil {
		if s.debug {
			log.Printf("[ERROR] Failed to query aggregated candles: %v", err)
		}
		return nil, fmt.Errorf("failed to query aggregated candles: %w", err)
	}
	defer rows.Close()

	// Pre-allocate the slice with a reasonable initial capacity
	candles := make([]*models.Candle, 0, 1000)

	for rows.Next() {
		candle := &models.Candle{}
		err := rows.Scan(
			&candle.Timestamp, &candle.OpenPrice, &candle.HighPrice,
			&candle.LowPrice, &candle.ClosePrice, &candle.Volume,
			&candle.TradeCount,
		)
		if err != nil {
			if s.debug {
				log.Printf("[ERROR] Failed to scan candle data: %v", err)
			}
			return nil, fmt.Errorf("failed to scan candle data: %w", err)
		}
		candles = append(candles, candle)

		if s.debug {
			log.Printf("[DEBUG] Retrieved aggregated candle for %s at %s: open=%s, close=%s, volume=%s",
				symbol, candle.Timestamp.Format(time.RFC3339),
				candle.OpenPrice, candle.ClosePrice, candle.Volume)
		}
	}

	if s.debug {
		log.Printf("[DEBUG] Found %d aggregated candles for %s", len(candles), symbol)
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
