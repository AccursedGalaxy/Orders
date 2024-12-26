package cli

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/storage"
)

type symbolMetrics struct {
	lastPrice     float64
	prevPrice     float64
	volume24h     float64
	trades24h     int64
	high24h       float64
	low24h        float64
	vwap          float64
	buyVolume     float64
	sellVolume    float64
	lastTradeTime time.Time
	tradesPerMin  float64
	initialized   bool
}

func newWatchCmd() *cobra.Command {
	var interval int
	var symbols []string
	var debug bool

	cmd := &cobra.Command{
		Use:   "watch [symbols...]",
		Short: "Watch real-time trade data",
		Long: `Watch real-time trade data for specified symbols.
Example: binance-cli watch BTCUSDT ETHUSDT`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				symbols = args
			}

			cfg := config.DefaultConfig()
			store, err := storage.NewRedisStore(cfg)
			if err != nil {
				return fmt.Errorf("failed to connect to Redis: %w", err)
			}
			defer store.Close()

			// Setup signal handling
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt)
			go func() {
				<-sigCh
				cancel()
			}()

			// If no symbols provided, get all available symbols
			if len(symbols) == 0 {
				symbolsKey := fmt.Sprintf("%ssymbols", cfg.Redis.KeyPrefix)
				if debug {
					log.Printf("Looking for symbols in Redis key: %s", symbolsKey)
				}
				symbols, err = store.GetRedisClient().SMembers(ctx, symbolsKey).Result()
				if err != nil {
					return fmt.Errorf("failed to get symbols: %w", err)
				}
				if debug {
					log.Printf("Found symbols: %v", symbols)
				}
			}

			// Convert symbols to lowercase
			for i := range symbols {
				symbols[i] = strings.ToLower(symbols[i])
			}

			if len(symbols) == 0 {
				return fmt.Errorf("no symbols found to watch")
			}

			// Initialize metrics for each symbol
			metrics := make(map[string]*symbolMetrics)
			for _, symbol := range symbols {
				metrics[symbol] = &symbolMetrics{}
			}

			// Clear screen and hide cursor
			fmt.Print("\033[2J\033[H\033[?25l")
			defer fmt.Print("\033[?25h") // Show cursor on exit

			ticker := time.NewTicker(time.Duration(interval) * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return nil
				case <-ticker.C:
					fmt.Print("\033[H") // Move cursor to top
					printHeader()

					for _, symbol := range symbols {
						if err := updateAndDisplayMetrics(ctx, store, symbol, metrics[symbol], cfg); err != nil {
							if debug {
								log.Printf("Error updating metrics for %s: %v", symbol, err)
							}
							continue
						}
					}
				}
			}
		},
	}

	cmd.Flags().IntVarP(&interval, "interval", "i", 1, "Update interval in seconds")
	cmd.Flags().BoolVarP(&debug, "debug", "d", false, "Enable debug logging")
	return cmd
}

func printHeader() {
	fmt.Println("Press Ctrl+C to exit")
	fmt.Println(strings.Repeat("─", 140))
	fmt.Printf("%-10s %-12s %-10s %-10s %-10s %-10s %-12s %-12s %-12s %-15s %-10s\n",
		"Symbol", "Price", "Change%", "High", "Low", "VWAP", "Volume", "Buy Vol%", "Trades/min", "Updated", "Trend")
	fmt.Println(strings.Repeat("─", 140))
}

func formatFloat(f float64, decimals int) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "-"
	}
	format := fmt.Sprintf("%%.%df", decimals)
	return fmt.Sprintf(format, f)
}

func updateAndDisplayMetrics(ctx context.Context, store *storage.RedisStore, symbol string, m *symbolMetrics, cfg *config.Config) error {
	// Get latest trade
	trade, err := store.GetLatestTrade(ctx, symbol)
	if err != nil || trade == nil {
		return fmt.Errorf("no trade data")
	}

	// Get 24h history - use millisecond timestamps
	end := time.Now()
	start := end.Add(-24 * time.Hour)
	history, err := store.GetTradeHistory(ctx, symbol, start, end)
	if err != nil {
		if cfg.Debug {
			log.Printf("Failed to get history for %s: %v", symbol, err)
		}
		return fmt.Errorf("failed to get history: %w", err)
	}

	if cfg.Debug {
		log.Printf("Got %d historical trades for %s", len(history), symbol)
	}

	// Update metrics
	price, _ := strconv.ParseFloat(trade.Price, 64)
	if !m.initialized {
		m.prevPrice = price
		m.initialized = true
	} else {
		m.prevPrice = m.lastPrice
	}
	m.lastPrice = price
	m.lastTradeTime = trade.Time

	// Calculate metrics from history
	var totalVolume, volumePrice float64
	m.high24h = price // Initialize with current price
	m.low24h = price  // Initialize with current price
	var buyVol, sellVol float64
	tradeCount := 0

	// Debug counters
	buyerMakerCount := 0
	nonBuyerMakerCount := 0
	var lastTrades []string

	for _, t := range history {
		p, err := strconv.ParseFloat(t.Data.Price, 64)
		if err != nil {
			if cfg.Debug {
				log.Printf("Failed to parse price: %v", err)
			}
			continue
		}
		q, err := strconv.ParseFloat(t.Data.Quantity, 64)
		if err != nil {
			if cfg.Debug {
				log.Printf("Failed to parse quantity: %v", err)
			}
			continue
		}

		volume := q          // Use quantity for volume
		volumePrice += p * q // For VWAP calculation

		if p > m.high24h {
			m.high24h = p
		}
		if p < m.low24h {
			m.low24h = p
		}

		totalVolume += volume
		// If IsBuyerMaker is true:
		// - Buyer was the maker (placed a limit order)
		// - Seller was the taker (placed a market order)
		// If IsBuyerMaker is false:
		// - Seller was the maker (placed a limit order)
		// - Buyer was the taker (placed a market order)
		if t.Data.IsBuyerMaker {
			sellVol += volume // Seller was the taker (market sell)
			buyerMakerCount++
		} else {
			buyVol += volume // Buyer was the taker (market buy)
			nonBuyerMakerCount++
		}
		tradeCount++

		// Store last few trades for debugging
		if cfg.Debug && len(lastTrades) < 5 {
			lastTrades = append(lastTrades, fmt.Sprintf(
				"Time: %s, Price: %.2f, Quantity: %.8f, IsBuyerMaker: %v (Interpretation: %s)",
				time.UnixMilli(t.Data.TradeTime).Format("15:04:05"),
				p, q, t.Data.IsBuyerMaker,
				interpretTrade(t.Data.IsBuyerMaker, p, q),
			))
		}
	}

	if cfg.Debug {
		// Debug output
		log.Printf("\nTrade analysis for %s:", symbol)
		log.Printf("Total trades: %d", tradeCount)
		log.Printf("BuyerMaker trades: %d (%.1f%%) - Trades where buyers placed limit orders and sellers executed market orders",
			buyerMakerCount, float64(buyerMakerCount)/float64(tradeCount)*100)
		log.Printf("NonBuyerMaker trades: %d (%.1f%%) - Trades where sellers placed limit orders and buyers executed market orders",
			nonBuyerMakerCount, float64(nonBuyerMakerCount)/float64(tradeCount)*100)
		log.Printf("Total volume: %.8f", totalVolume)
		log.Printf("Market Buy volume: %.8f (%.1f%%) - Volume from market buy orders",
			buyVol, buyVol/totalVolume*100)
		log.Printf("Market Sell volume: %.8f (%.1f%%) - Volume from market sell orders",
			sellVol, sellVol/totalVolume*100)
		log.Printf("\nLast 5 trades (most recent first):")
		for _, t := range lastTrades {
			log.Printf("  %s", t)
		}
	}

	// Calculate VWAP and trades per minute
	if totalVolume > 0 {
		m.vwap = volumePrice / totalVolume
	} else {
		m.vwap = price
	}
	m.tradesPerMin = float64(tradeCount) / 1440 // trades per minute

	// Display metrics
	priceChange := 0.0
	if m.prevPrice > 0 {
		priceChange = ((m.lastPrice - m.prevPrice) / m.prevPrice) * 100
	}

	// Calculate market buy percentage
	buyPercent := 0.0
	if totalVolume > 0 {
		buyPercent = (buyVol / totalVolume) * 100
		if cfg.Debug {
			log.Printf("\nMarket Buy percentage: %.8f / %.8f * 100 = %.8f%% (percentage of volume from market buy orders)",
				buyVol, totalVolume, buyPercent)
		}
	}

	// Color coding
	priceColor := color.New(color.FgWhite)
	if priceChange > 0 {
		priceColor = color.New(color.FgGreen)
	} else if priceChange < 0 {
		priceColor = color.New(color.FgRed)
	}

	// Trend indicator
	trend := "─"
	if priceChange > 0.1 {
		trend = "↗"
	} else if priceChange < -0.1 {
		trend = "↘"
	}

	fmt.Printf("%-10s ", strings.ToUpper(symbol))
	priceColor.Printf("%-12s ", formatFloat(m.lastPrice, 2))
	priceColor.Printf("%-10s ", formatFloat(priceChange, 2))
	fmt.Printf("%-10s %-10s %-10s %-12s %-12s %-12s %-15s %s\n",
		formatFloat(m.high24h, 2),
		formatFloat(m.low24h, 2),
		formatFloat(m.vwap, 2),
		formatFloat(totalVolume, 4),
		formatFloat(buyPercent, 1), // Show market buy percentage
		formatFloat(m.tradesPerMin, 1),
		m.lastTradeTime.Format("15:04:05"),
		trend,
	)

	return nil
}

// interpretTrade provides a human-readable interpretation of a trade
func interpretTrade(isBuyerMaker bool, price, quantity float64) string {
	if isBuyerMaker {
		return fmt.Sprintf("Market SELL %.8f BTC at %.2f (seller hit buyer's limit order)", quantity, price)
	}
	return fmt.Sprintf("Market BUY %.8f BTC at %.2f (buyer hit seller's limit order)", quantity, price)
}

// displayTrade formats and displays a trade
func displayTrade(trade *models.Trade) {
	fmt.Printf("%-10s %-12s %-12s %-15s\n",
		trade.Symbol,
		trade.Price,
		trade.Quantity,
		trade.Time.Format("15:04:05"),
	)
}
