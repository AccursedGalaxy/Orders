package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/storage"
)

func newSymbolsCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "symbols",
		Short: "List available trading pairs",
		Long: `List all available trading pairs that are being tracked.
Example: binance-cli symbols --format table`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.DefaultConfig()
			store, err := storage.NewRedisStore(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect to Redis: %w", err)
			}
			defer store.Close()

			// Get all symbols
			symbolsKey := fmt.Sprintf("%ssymbols", cfg.Redis.KeyPrefix)
			symbols, err := store.GetRedisClient().SMembers(context.Background(), symbolsKey).Result()
			if err != nil {
				return fmt.Errorf("failed to get symbols: %w", err)
			}

			if len(symbols) == 0 {
				return fmt.Errorf("no trading pairs found")
			}

			// Sort symbols for consistent output
			sort.Strings(symbols)

			// Get latest trades for all symbols
			trades := make(map[string]struct {
				Price     string
				Volume24h string
			})

			for _, symbol := range symbols {
				trade, err := store.GetLatestTrade(context.Background(), symbol)
				if err != nil {
					continue
				}
				if trade == nil {
					continue
				}

				// Get 24h volume from Redis
				volumeKey := fmt.Sprintf("%s%s:volume:24h", cfg.Redis.KeyPrefix, symbol)
				volume, _ := store.GetRedisClient().Get(context.Background(), volumeKey).Result()

				trades[symbol] = struct {
					Price     string
					Volume24h string
				}{
					Price:     trade.Price,
					Volume24h: volume,
				}
			}

			switch format {
			case "table":
				fmt.Printf("%-10s %-15s %-15s\n", "Symbol", "Price", "24h Volume")
				fmt.Println(strings.Repeat("-", 42))

				for _, symbol := range symbols {
					if trade, ok := trades[symbol]; ok {
						fmt.Printf("%-10s %-15s %-15s\n",
							strings.ToUpper(symbol),
							trade.Price,
							trade.Volume24h,
						)
					}
				}

			case "simple":
				for _, symbol := range symbols {
					fmt.Println(strings.ToUpper(symbol))
				}

			case "json":
				fmt.Println("{")
				for i, symbol := range symbols {
					if trade, ok := trades[symbol]; ok {
						fmt.Printf("  %q: {\"price\": %q, \"volume_24h\": %q}",
							strings.ToUpper(symbol),
							trade.Price,
							trade.Volume24h,
						)
						if i < len(symbols)-1 {
							fmt.Println(",")
						} else {
							fmt.Println("")
						}
					}
				}
				fmt.Println("}")

			default:
				return fmt.Errorf("unsupported format: %s", format)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format (table, simple, or json)")
	return cmd
}
