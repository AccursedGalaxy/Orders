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
	var totalQuantity float64

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

		quoteVolume := p * q // Calculate quote volume (in USDT)
		totalVolume += quoteVolume
		volumePrice += p * q // For VWAP calculation
		totalQuantity += q   // Track total quantity for VWAP

		if p > m.high24h {
			m.high24h = p
		}
		if p < m.low24h {
			m.low24h = p
		}

		// If IsBuyerMaker is true:
		// - Buyer was the maker (placed a limit order)
		// - Seller was the taker (placed a market order)
		// If IsBuyerMaker is false:
		// - Seller was the maker (placed a limit order)
		// - Buyer was the taker (placed a market order)
		if t.Data.IsBuyerMaker {
			sellVol += quoteVolume // Seller was the taker (market sell)
			buyerMakerCount++
		} else {
			buyVol += quoteVolume // Buyer was the taker (market buy)
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
	if totalQuantity > 0 {
		m.vwap = volumePrice / totalQuantity // VWAP = Σ(price * quantity) / Σ(quantity)
	} else {
		m.vwap = price
	}
	m.tradesPerMin = float64(tradeCount) / 1440 // trades per minute

	// Calculate advanced metrics
	if totalVolume > 0 {
		// Price action metrics
		m.priceRange = ((m.high24h - m.low24h) / m.low24h) * 100
		m.rangePosition = ((m.lastPrice - m.low24h) / (m.high24h - m.low24h)) * 100
		m.vwapDev = ((m.lastPrice - m.vwap) / m.vwap) * 100 // This calculation is correct, the input values were wrong

		// Calculate volatility (standard deviation of returns)
		var returns []float64
		for i := 1; i < len(m.recentPrices); i++ {
			ret := (m.recentPrices[i] - m.recentPrices[i-1]) / m.recentPrices[i-1]
			returns = append(returns, ret)
		}
		m.volatility = calculateStdDev(returns) * 100

		// Volume metrics
		avgVolume := totalVolume / float64(tradeCount)
		m.avgTradeSize = avgVolume
		m.volMomentum = calculateVolumeMomentum(m.recentVolumes)

		// Market microstructure
		m.orderImbalance = (buyVol - sellVol) / totalVolume
		m.marketImpact = math.Abs(m.lastPrice-m.prevPrice) / totalVolume
		m.tradeAccel = calculateTradeAcceleration(m.recentTrades)

		// Update historical data
		m.recentPrices = append(m.recentPrices, m.lastPrice)
		if len(m.recentPrices) > 100 {
			m.recentPrices = m.recentPrices[1:]
		}
	}

	// Calculate price change
	priceChange := 0.0
	if m.prevPrice > 0 {
		priceChange = ((m.lastPrice - m.prevPrice) / m.prevPrice) * 100
	}

	// Calculate buy percentage
	buyPercent := 0.0
	if totalVolume > 0 {
		buyPercent = (buyVol / totalVolume) * 100
	}

	// Header with symbol and timestamp
	fmt.Printf("─── %s %s%s %s ───\n",
		symbol,
		formatFloat(m.lastPrice, 2),
		formatPriceChange(priceChange),
		m.lastTradeTime.Format("15:04:05"))

	// Price information
	fmt.Printf("24h Range: %s - %s    VWAP: %s\n",
		formatFloat(m.low24h, 2),
		formatFloat(m.high24h, 2),
		formatFloat(m.vwap, 2))

	fmt.Println()

	// Left column
	fmt.Printf("Volume (24h):     %s USDT\n", formatVolume(totalVolume))
	fmt.Printf("Buy Volume:       %.1f%%\n", buyPercent)
	fmt.Printf("Avg Trade Size:   %s USDT\n", formatVolume(m.avgTradeSize))
	fmt.Printf("Trades/min:       %.1f\n", m.tradesPerMin)

	fmt.Println()

	// Right column
	fmt.Printf("Price Range:      %.2f%%\n", m.priceRange)
	fmt.Printf("Range Position:   %.1f%%\n", m.rangePosition)
	fmt.Printf("Volatility:       %.2f%%\n", m.volatility)
	fmt.Printf("VWAP Deviation:   %.2f%%\n", m.vwapDev)

	fmt.Println()

	// Market metrics
	fmt.Printf("Order Imbalance:  %.1f%%\n", m.orderImbalance*100)
	fmt.Printf("Market Impact:    %.6f\n", m.marketImpact)
	fmt.Printf("Volume Momentum:  %.2f%%\n", m.volMomentum)
	fmt.Printf("Trade Accel:      %.1f%%\n", m.tradeAccel)

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
