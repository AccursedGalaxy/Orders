package cli

import (
	"context"
	"fmt"
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

			end := time.Now()
			start := end.Add(-duration)

			fmt.Printf("Statistics for the last %s\n", period)
			fmt.Println(strings.Repeat("-", 100))
			fmt.Printf("%-10s %-12s %-12s %-12s %-12s %-15s %-10s\n",
				"Symbol", "Open", "High", "Low", "Close", "Volume", "Trades")
			fmt.Println(strings.Repeat("-", 100))

			for _, symbol := range symbols {
				candles, err := postgresStore.GetHistoricalCandles(ctx, strings.ToLower(symbol), start, end)
				if err != nil {
					fmt.Printf("Error getting data for %s: %v\n", symbol, err)
					continue
				}

				if len(candles) == 0 {
					continue
				}

				// Calculate aggregated statistics
				first := candles[0]
				last := candles[len(candles)-1]
				high := first.HighPrice
				low := first.LowPrice
				volume := 0.0
				trades := int64(0)

				for _, candle := range candles {
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

			return nil
		},
	}

	cmd.Flags().StringVarP(&period, "period", "p", "1h", "Time period (e.g., 1h, 24h, 7d)")
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
