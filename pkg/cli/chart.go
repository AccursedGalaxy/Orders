package cli

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"

	"binance-redis-streamer/internal/models"
	"binance-redis-streamer/pkg/storage"
)

//go:embed templates
var templateFS embed.FS

type ChartData struct {
	Symbol string    `json:"symbol"`
	Time   []string  `json:"time"`
	Open   []string  `json:"open"`
	High   []string  `json:"high"`
	Low    []string  `json:"low"`
	Close  []string  `json:"close"`
	Volume []float64 `json:"volume"`
}

func newChartCmd() *cobra.Command {
	var port int
	var period string

	cmd := &cobra.Command{
		Use:   "chart [symbol]",
		Short: "View interactive price charts",
		Long: `View interactive price charts in your web browser.
Example: binance-cli chart BTCUSDT --period 24h`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			symbol := strings.ToUpper(args[0])

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

			// Fetch candles for the time period
			end := time.Now()
			start := end.Add(-duration)

			log.Printf("Fetching candles for %s from %s to %s", symbol, start.Format(time.RFC3339), end.Format(time.RFC3339))

			dbCandles, err := postgresStore.GetHistoricalCandles(context.Background(), symbol, start, end)
			if err != nil {
				log.Printf("Error fetching candles: %v", err)
				return fmt.Errorf("failed to fetch candles: %w", err)
			}

			log.Printf("Retrieved %d candles from PostgreSQL", len(dbCandles))
			if len(dbCandles) > 0 {
				log.Printf("First candle: Time=%s, Open=%s, High=%s, Low=%s, Close=%s, Volume=%s",
					dbCandles[0].Timestamp.Format(time.RFC3339),
					dbCandles[0].OpenPrice,
					dbCandles[0].HighPrice,
					dbCandles[0].LowPrice,
					dbCandles[0].ClosePrice,
					dbCandles[0].Volume)
			}

			// Convert to chart data format
			data := ChartData{
				Symbol: symbol,
				Time:   make([]string, len(dbCandles)),
				Open:   make([]string, len(dbCandles)),
				High:   make([]string, len(dbCandles)),
				Low:    make([]string, len(dbCandles)),
				Close:  make([]string, len(dbCandles)),
				Volume: make([]float64, len(dbCandles)),
			}

			for i, candle := range dbCandles {
				// Convert timestamp to Unix timestamp in seconds
				data.Time[i] = fmt.Sprintf("%d", candle.Timestamp.Unix())

				// Convert string prices to float64 for proper JSON encoding
				open, _ := strconv.ParseFloat(candle.OpenPrice, 64)
				high, _ := strconv.ParseFloat(candle.HighPrice, 64)
				low, _ := strconv.ParseFloat(candle.LowPrice, 64)
				close, _ := strconv.ParseFloat(candle.ClosePrice, 64)

				data.Open[i] = fmt.Sprintf("%.8f", open)
				data.High[i] = fmt.Sprintf("%.8f", high)
				data.Low[i] = fmt.Sprintf("%.8f", low)
				data.Close[i] = fmt.Sprintf("%.8f", close)

				vol, _ := strconv.ParseFloat(candle.Volume, 64)
				data.Volume[i] = vol
			}

			// Setup router
			r := mux.NewRouter()

			// Serve static files
			r.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
				tmpl, err := template.ParseFS(templateFS, "templates/chart.html")
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				data := struct {
					Symbol string
					Period string
					Data   []*models.Candle
				}{
					Symbol: symbol,
					Period: period,
					Data:   dbCandles,
				}

				if err := tmpl.Execute(w, data); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			})

			// API endpoint for chart data
			r.HandleFunc("/api/data", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				// Log the data being sent for debugging
				if len(data.Time) > 0 {
					log.Printf("Sending %d candles. First candle: Time=%s, Open=%s, High=%s, Low=%s, Close=%s, Volume=%.2f",
						len(data.Time), data.Time[0], data.Open[0], data.High[0], data.Low[0], data.Close[0], data.Volume[0])
				} else {
					log.Printf("Warning: No candle data available")
				}

				if err := json.NewEncoder(w).Encode(data); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			})

			// Start server
			srv := &http.Server{
				Addr:              fmt.Sprintf(":%d", port),
				Handler:           r,
				ReadHeaderTimeout: 10 * time.Second,
			}

			// Handle graceful shutdown
			go func() {
				sigint := make(chan os.Signal, 1)
				signal.Notify(sigint, os.Interrupt)
				<-sigint

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				if err := srv.Shutdown(ctx); err != nil {
					log.Printf("Error shutting down server: %v", err)
				}
			}()

			fmt.Printf("Opening chart for %s in your browser at http://localhost:%d\n", strings.ToUpper(symbol), port)
			fmt.Println("Press Ctrl+C to stop")

			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				return err
			}

			return nil
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8080, "Port to serve the web interface")
	cmd.Flags().StringVarP(&period, "period", "t", "24h", "Time period (e.g., 1h, 24h, 7d)")
	return cmd
}
