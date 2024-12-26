package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"binance-redis-streamer/pkg/storage"
)

func newHistoryCmd() *cobra.Command {
	var (
		period   string
		interval string
		limit    int
		format   string
	)

	cmd := &cobra.Command{
		Use:   "history [symbol]",
		Short: "View historical trade data",
		Long: `View historical trade data for a symbol with custom time intervals.
Example: binance-cli history BTCUSDT --period 24h --interval 5m`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			symbol := strings.ToLower(args[0])

			// Parse time period
			duration, err := parseDuration(period)
			if err != nil {
				return fmt.Errorf("invalid period format: %w", err)
			}

			postgresStore, err := storage.NewPostgresStore()
			if err != nil {
				return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
			}
			defer postgresStore.Close()

			end := time.Now()
			start := end.Add(-duration)

			candles, err := postgresStore.GetAggregatedCandles(context.Background(), symbol, start, end, interval)
			if err != nil {
				return fmt.Errorf("failed to get historical data: %w", err)
			}

			if len(candles) == 0 {
				return fmt.Errorf("no data found for %s in the specified period", strings.ToUpper(symbol))
			}

			// Limit the number of results if specified
			if limit > 0 && limit < len(candles) {
				candles = candles[len(candles)-limit:]
			}

			// Print header
			fmt.Printf("Historical data for %s (%s intervals)\n", strings.ToUpper(symbol), interval)
			fmt.Println(strings.Repeat("-", 100))

			switch format {
			case "table":
				fmt.Printf("%-20s %-12s %-12s %-12s %-12s %-15s %-10s\n",
					"Time", "Open", "High", "Low", "Close", "Volume", "Trades")
				fmt.Println(strings.Repeat("-", 100))

				for _, candle := range candles {
					fmt.Printf("%-20s %-12s %-12s %-12s %-12s %-15s %-10d\n",
						candle.Timestamp.Format("2006-01-02 15:04:05"),
						candle.OpenPrice,
						candle.HighPrice,
						candle.LowPrice,
						candle.ClosePrice,
						candle.Volume,
						candle.TradeCount,
					)
				}

			case "csv":
				fmt.Println("timestamp,open,high,low,close,volume,trades")
				for _, candle := range candles {
					fmt.Printf("%s,%s,%s,%s,%s,%s,%d\n",
						candle.Timestamp.Format("2006-01-02 15:04:05"),
						candle.OpenPrice,
						candle.HighPrice,
						candle.LowPrice,
						candle.ClosePrice,
						candle.Volume,
						candle.TradeCount,
					)
				}

			default:
				return fmt.Errorf("unsupported format: %s", format)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&period, "period", "p", "24h", "Time period (e.g., 1h, 24h, 7d)")
	cmd.Flags().StringVarP(&interval, "interval", "i", "1m", "Time interval (e.g., 1m, 5m, 1h)")
	cmd.Flags().IntVarP(&limit, "limit", "l", 0, "Limit the number of results (0 for all)")
	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format (table or csv)")

	return cmd
}
