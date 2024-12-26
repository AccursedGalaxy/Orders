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

	"github.com/go-redis/redis/v8"
	"github.com/spf13/cobra"

	"binance-redis-streamer/pkg/config"
	"binance-redis-streamer/pkg/storage"
)

type symbolMetrics struct {
	lastPrice     float64
	prevPrice     float64
	high24h       float64
	low24h        float64
	vwap          float64
	lastTradeTime time.Time
	tradesPerMin  float64
	initialized   bool

	// Price action metrics
	priceRange    float64 // 24h range as percentage
	rangePosition float64 // Where in the range current price sits (0-100%)
	volatility    float64 // Standard deviation of returns
	vwapDev       float64 // Deviation from VWAP as percentage

	// Volume metrics
	volMomentum float64 // Volume trend (positive = increasing)

	// Trade metrics
	avgTradeSize float64 // Average trade size
	tradeAccel   float64 // Trade frequency acceleration

	// Market microstructure
	orderImbalance float64 // Buy volume - Sell volume / Total volume
	marketImpact   float64 // Price movement per unit of volume

	// Historical data for calculations
	recentPrices  []float64
	recentVolumes []float64
	recentTrades  []float64
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
			cfg.Debug = debug

			// First check for custom Redis URL (highest priority for local development)
			if redisURL := os.Getenv("CUSTOM_REDIS_URL"); redisURL != "" {
				if debug {
					log.Printf("Using custom Redis URL from CUSTOM_REDIS_URL")
				}
				cfg.Redis.URL = redisURL
			} else if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
				// Then check for Heroku Redis URL
				if debug {
					log.Printf("Using Heroku Redis URL from REDIS_URL")
				}
				cfg.Redis.URL = redisURL
			} else if debug {
				log.Printf("Warning: Neither CUSTOM_REDIS_URL nor REDIS_URL set, using default: %s", cfg.Redis.URL)
			}

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

			// Convert symbols to uppercase
			for i := range symbols {
				symbols[i] = strings.ToUpper(symbols[i])
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
	fmt.Println()
}

func formatFloat(f float64, decimals int) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "-"
	}
	format := fmt.Sprintf("%%.%df", decimals)
	return fmt.Sprintf(format, f)
}

// formatVolume formats volume with K/M/B suffixes
func formatVolume(volume float64) string {
	if volume >= 1_000_000_000 {
		return fmt.Sprintf("%.2fB", volume/1_000_000_000)
	}
	if volume >= 1_000_000 {
		return fmt.Sprintf("%.2fM", volume/1_000_000)
	}
	if volume >= 1_000 {
		return fmt.Sprintf("%.2fK", volume/1_000)
	}
	return fmt.Sprintf("%.2f", volume)
}

func updateAndDisplayMetrics(ctx context.Context, store *storage.RedisStore, symbol string, m *symbolMetrics, cfg *config.Config) error {
	// Create a context with timeout for Redis operations
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Get latest trade
	trade, err := store.GetLatestTrade(timeoutCtx, symbol)
	if err != nil {
		if cfg.Debug {
			log.Printf("Error getting latest trade for %s: %v", symbol, err)
		}
		return fmt.Errorf("no trade data: %w", err)
	}
	if trade == nil {
		if cfg.Debug {
			log.Printf("No latest trade found for %s in Redis", symbol)
		}
		return fmt.Errorf("no trade data available for %s", symbol)
	}

	// Update basic metrics from latest trade
	price, _ := strconv.ParseFloat(trade.Price, 64)
	if !m.initialized {
		m.prevPrice = price
		m.high24h = price
		m.low24h = price
		m.initialized = true
	} else {
		m.prevPrice = m.lastPrice
		// Update high/low only if we don't have history
		if m.high24h == 0 {
			m.high24h = price
		} else if price > m.high24h {
			m.high24h = price
		}
		if m.low24h == 0 {
			m.low24h = price
		} else if price < m.low24h {
			m.low24h = price
		}
	}
	m.lastPrice = price
	m.lastTradeTime = trade.Time

	// Try to get recent history (last 15 minutes for display)
	end := time.Now()
	start := end.Add(-15 * time.Minute)
	history, err := store.GetTradeHistory(timeoutCtx, symbol, start, end)
	if err != nil {
		if cfg.Debug {
			log.Printf("Failed to get history for %s: %v", symbol, err)
		}
		// Continue with partial data
	} else if cfg.Debug {
		log.Printf("Got %d historical trades for %s", len(history), symbol)
	}

	// Calculate metrics from available history
	var volumePrice float64
	var totalQuantity float64
	var buyVol, sellVol float64
	tradeCount := len(history)

	// Get running volume for 2h window
	volumeKey := fmt.Sprintf("%s%s:volume:running", cfg.Redis.KeyPrefix, strings.ToUpper(symbol))
	totalVolumeStr, err := store.GetRedisClient().Get(timeoutCtx, volumeKey).Result()
	if err != nil && err != redis.Nil {
		if cfg.Debug {
			log.Printf("Failed to get running volume: %v", err)
		}
	}
	totalVolume := 0.0
	if totalVolumeStr != "" {
		totalVolume, _ = strconv.ParseFloat(totalVolumeStr, 64)
	}

	// Calculate metrics from recent trades
	for _, t := range history {
		p, err := strconv.ParseFloat(t.Data.Price, 64)
		if err != nil {
			continue
		}
		q, err := strconv.ParseFloat(t.Data.Quantity, 64)
		if err != nil {
			continue
		}

		quoteVolume := p * q
		volumePrice += p * q // For VWAP: Σ(price * quantity)
		totalQuantity += q   // For VWAP: Σ(quantity)

		if t.Data.IsBuyerMaker {
			sellVol += quoteVolume
		} else {
			buyVol += quoteVolume
		}

		// Update high/low prices
		if p > m.high24h {
			m.high24h = p
		}
		if p < m.low24h || m.low24h == 0 {
			m.low24h = p
		}
	}

	// Calculate metrics with available data
	recentVolume := buyVol + sellVol
	if recentVolume > 0 {
		m.orderImbalance = (buyVol - sellVol) / recentVolume
		m.avgTradeSize = recentVolume / float64(tradeCount)
		m.tradesPerMin = float64(tradeCount) / 15 // trades per minute over 15 minutes
	}

	// If we don't have running volume, use recent volume
	if totalVolume == 0 {
		totalVolume = recentVolume
	}

	// Display metrics
	fmt.Printf("─── %s %s%s %s ───\n",
		symbol,
		formatFloat(m.lastPrice, 2),
		formatPriceChange(((m.lastPrice-m.prevPrice)/m.prevPrice)*100),
		m.lastTradeTime.Format("15:04:05"))

	vwap := "-"
	if totalQuantity > 0 {
		vwap = formatFloat(volumePrice/totalQuantity, 2) // VWAP = Σ(price * quantity) / Σ(quantity)
	}

	fmt.Printf("Range: %s - %s    VWAP: %s\n",
		formatFloat(m.low24h, 2),
		formatFloat(m.high24h, 2),
		vwap)

	fmt.Println()

	fmt.Printf("Volume (2h):      %s USDT\n", formatVolume(totalVolume))
	buyPercent := 0.0
	if recentVolume > 0 {
		buyPercent = (buyVol / recentVolume) * 100
	}
	fmt.Printf("Buy Volume:       %.1f%%\n", buyPercent)
	fmt.Printf("Avg Trade Size:   %s USDT\n", formatVolume(m.avgTradeSize))
	fmt.Printf("Trades/min:       %.1f\n", m.tradesPerMin)

	fmt.Println()

	if m.high24h > m.low24h {
		m.priceRange = ((m.high24h - m.low24h) / m.low24h) * 100
		m.rangePosition = ((m.lastPrice - m.low24h) / (m.high24h - m.low24h)) * 100
	}

	fmt.Printf("Price Range:      %.2f%%\n", m.priceRange)
	fmt.Printf("Range Position:   %.1f%%\n", m.rangePosition)
	fmt.Printf("Order Imbalance:  %.1f%%\n", m.orderImbalance*100)

	fmt.Printf("%s\n\n", strings.Repeat("─", 50))

	return nil
}

// formatPriceChange formats the price change with color and direction
func formatPriceChange(change float64) string {
	if change > 0 {
		return fmt.Sprintf(" +%.2f%%", change)
	}
	return fmt.Sprintf(" %.2f%%", change)
}

// interpretTrade provides a human-readable interpretation of a trade
func interpretTrade(isBuyerMaker bool, price, quantity float64) string {
	if isBuyerMaker {
		return fmt.Sprintf("Market SELL %.8f BTC at %.2f (seller hit buyer's limit order)", quantity, price)
	}
	return fmt.Sprintf("Market BUY %.8f BTC at %.2f (buyer hit seller's limit order)", quantity, price)
}

// displayTrade formats and displays a trade
func calculateStdDev(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Calculate mean
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))

	// Calculate variance
	var variance float64
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	variance /= float64(len(values))

	// Annualize volatility (√252 for trading days per year)
	// We multiply by 100 to convert to percentage
	return math.Sqrt(variance) * math.Sqrt(252) * 100
}

func calculateVolumeMomentum(volumes []float64) float64 {
	if len(volumes) < 2 {
		return 0
	}

	// Use exponential weighting
	const lambda = 0.94 // decay factor (higher = more weight to recent data)
	var recentSum, recentWeight float64
	var earlierSum, earlierWeight float64

	midPoint := len(volumes) / 2

	// Calculate weighted averages for recent and earlier periods
	for i := len(volumes) - 1; i >= midPoint; i-- {
		weight := math.Pow(lambda, float64(len(volumes)-1-i))
		recentSum += volumes[i] * weight
		recentWeight += weight
	}

	for i := midPoint - 1; i >= 0; i-- {
		weight := math.Pow(lambda, float64(midPoint-1-i))
		earlierSum += volumes[i] * weight
		earlierWeight += weight
	}

	if earlierWeight == 0 || recentWeight == 0 {
		return 0
	}

	recentAvg := recentSum / recentWeight
	earlierAvg := earlierSum / earlierWeight

	if earlierAvg == 0 {
		return 0
	}

	return (recentAvg - earlierAvg) / earlierAvg * 100
}

func calculateTradeAcceleration(trades []float64) float64 {
	if len(trades) < 3 {
		return 0
	}

	// Use exponential weighting
	const lambda = 0.94 // decay factor (higher = more weight to recent data)
	var recentSum, recentWeight float64
	var earlierSum, earlierWeight float64

	midPoint := len(trades) / 2

	// Calculate weighted averages for recent and earlier periods
	for i := len(trades) - 1; i >= midPoint; i-- {
		weight := math.Pow(lambda, float64(len(trades)-1-i))
		recentSum += trades[i] * weight
		recentWeight += weight
	}

	for i := midPoint - 1; i >= 0; i-- {
		weight := math.Pow(lambda, float64(midPoint-1-i))
		earlierSum += trades[i] * weight
		earlierWeight += weight
	}

	if earlierWeight == 0 || recentWeight == 0 {
		return 0
	}

	recentAvg := recentSum / recentWeight
	earlierAvg := earlierSum / earlierWeight

	if earlierAvg == 0 {
		return 0
	}

	return (recentAvg - earlierAvg) / earlierAvg * 100
}
