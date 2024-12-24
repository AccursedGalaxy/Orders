package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"binance-redis-streamer/internal/models"
)

// SQLiteStore handles historical trade data storage
type SQLiteStore struct {
	db *sql.DB
	dataDir string
}

// NewSQLiteStore creates a new SQLite store
func NewSQLiteStore(dataDir string) (*SQLiteStore, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	dbPath := filepath.Join(dataDir, "trades.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create tables if they don't exist
	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteStore{
		db: db,
		dataDir: dataDir,
	}, nil
}

func createTables(db *sql.DB) error {
	// Create trades table with 1-minute aggregation
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS trade_candles (
			symbol TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			open_price TEXT NOT NULL,
			high_price TEXT NOT NULL,
			low_price TEXT NOT NULL,
			close_price TEXT NOT NULL,
			volume TEXT NOT NULL,
			trade_count INTEGER NOT NULL,
			PRIMARY KEY (symbol, timestamp)
		);
		CREATE INDEX IF NOT EXISTS idx_trade_candles_time 
			ON trade_candles(timestamp);
	`)
	return err
}

// StoreCandleData stores 1-minute aggregated trade data
func (s *SQLiteStore) StoreCandleData(ctx context.Context, symbol string, candle *models.Candle) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO trade_candles (
			symbol, timestamp, open_price, high_price, low_price, 
			close_price, volume, trade_count
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		symbol, candle.Timestamp.Unix(), candle.OpenPrice,
		candle.HighPrice, candle.LowPrice, candle.ClosePrice,
		candle.Volume, candle.TradeCount,
	)
	return err
}

// GetHistoricalCandles retrieves historical candle data
func (s *SQLiteStore) GetHistoricalCandles(ctx context.Context, symbol string, start, end time.Time) ([]*models.Candle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT timestamp, open_price, high_price, low_price, 
			   close_price, volume, trade_count
		FROM trade_candles
		WHERE symbol = ? AND timestamp BETWEEN ? AND ?
		ORDER BY timestamp ASC`,
		symbol, start.Unix(), end.Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candles []*models.Candle
	for rows.Next() {
		var timestamp int64
		candle := &models.Candle{}
		err := rows.Scan(
			&timestamp, &candle.OpenPrice, &candle.HighPrice,
			&candle.LowPrice, &candle.ClosePrice, &candle.Volume,
			&candle.TradeCount,
		)
		if err != nil {
			return nil, err
		}
		candle.Timestamp = time.Unix(timestamp, 0)
		candles = append(candles, candle)
	}

	return candles, rows.Err()
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Vacuum optimizes the database file size
func (s *SQLiteStore) Vacuum() error {
	_, err := s.db.Exec("VACUUM")
	return err
} 