package cli

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"github.com/spf13/cobra"

	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/storage"
)

func newStatsCmd() *cobra.Command {
	var period string
	var symbols []string
	var debug bool

	cmd := &cobra.Command{
		Use:   "stats [symbols...]",
		Short: "View trade statistics",
		Long: `View trade statistics for specified symbols.
Example: binance-cli stats --period 1h BTCUSDT ETHUSDT`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				symbols = args
			}

			// Parse time period
			duration, err := parseDuration(period)
			if err != nil {
				return fmt.Errorf("invalid period format: %w", err)
			}

			cfg := config.DefaultConfig()
			redisStore, err := storage.NewRedisStore(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect to Redis: %w", err)
			}
			defer redisStore.Close()

			postgresStore, err := storage.NewPostgresStore()
			if err != nil {
				return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
			}
			defer postgresStore.Close()

			ctx := context.Background()

			// If no symbols provided, get all available symbols
			if len(symbols) == 0 {
				symbolsKey := fmt.Sprintf("%ssymbols", cfg.Redis.KeyPrefix)
				symbols, err = redisStore.GetRedisClient().SMembers(ctx, symbolsKey).Result()
				if err != nil {
					return fmt.Errorf("failed to get symbols: %w", err)
				}
			}

			// Convert all symbols to uppercase
			for i := range symbols {
				symbols[i] = strings.ToUpper(symbols[i])
			}

			end := time.Now()
			start := end.Add(-duration)

			fmt.Printf("Statistics for the last %s\n", period)
			fmt.Println(strings.Repeat("-", 100))
			fmt.Printf("%-10s %-12s %-12s %-12s %-12s %-15s %-10s\n",
				"Symbol", "Open", "High", "Low", "Close", "Volume", "Trades")
			fmt.Println(strings.Repeat("-", 100))

			noDataFound := true
			for _, symbol := range symbols {
				if debug {
					log.Printf("Fetching historical candles for %s from %s to %s", symbol, start.Format(time.RFC3339), end.Format(time.RFC3339))
				}

				candles, err := postgresStore.GetHistoricalCandles(ctx, symbol, start, end)
				if err != nil {
					if debug {
						log.Printf("Error getting data for %s: %v", symbol, err)
					}
					continue
				}

				if len(candles) == 0 {
					if debug {
						log.Printf("No data found for %s in the specified period", symbol)
					}
					continue
				}

				noDataFound = false

				// Calculate aggregated statistics
				first := candles[0]
				last := candles[len(candles)-1]
				high := first.HighPrice
				low := first.LowPrice
				volume := 0.0
				trades := int64(0)

				for _, candle := range candles {
					if debug {
						log.Printf("Processing candle: symbol=%s, time=%s, open=%s, close=%s, volume=%s, trades=%d",
							symbol, candle.Timestamp.Format(time.RFC3339), candle.OpenPrice, candle.ClosePrice, candle.Volume, candle.TradeCount)
					}

					if candle.HighPrice > high {
						high = candle.HighPrice
					}
					if candle.LowPrice < low {
						low = candle.LowPrice
					}
					v, _ := strconv.ParseFloat(candle.Volume, 64)
					volume += v
					trades += candle.TradeCount
				}

				fmt.Printf("%-10s %-12s %-12s %-12s %-12s %-15.2f %-10d\n",
					symbol,
					first.OpenPrice,
					high,
					low,
					last.ClosePrice,
					volume,
					trades,
				)
			}

			if noDataFound {
				fmt.Printf("\nNo data found for any symbols in the last %s\n", period)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&period, "period", "p", "1h", "Time period (e.g., 1h, 24h, 7d)")
	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")
	return cmd
}

func parseDuration(period string) (time.Duration, error) {
	// Convert common formats to Go duration format
	period = strings.ToLower(period)
	if strings.HasSuffix(period, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(period, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(period)
}
