package cli

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/cobra"

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

			// Setup router
			r := mux.NewRouter()

			// Serve static files
			r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				tmpl, err := template.ParseFS(templateFS, "templates/chart.html")
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				data := struct {
					Symbol string
					Period string
				}{
					Symbol: strings.ToUpper(symbol),
					Period: period,
				}
				tmpl.Execute(w, data)
			})

			// API endpoint for chart data
			r.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
				end := time.Now()
				start := end.Add(-duration)

				candles, err := postgresStore.GetHistoricalCandles(r.Context(), symbol, start, end)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				data := ChartData{
					Symbol: strings.ToUpper(symbol),
					Time:   make([]string, len(candles)),
					Open:   make([]string, len(candles)),
					High:   make([]string, len(candles)),
					Low:    make([]string, len(candles)),
					Close:  make([]string, len(candles)),
					Volume: make([]float64, len(candles)),
				}

				for i, candle := range candles {
					data.Time[i] = candle.Timestamp.Format(time.RFC3339)
					data.Open[i] = candle.OpenPrice
					data.High[i] = candle.HighPrice
					data.Low[i] = candle.LowPrice
					data.Close[i] = candle.ClosePrice
					vol, _ := strconv.ParseFloat(candle.Volume, 64)
					data.Volume[i] = vol
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(data)
			})

			// Start server
			srv := &http.Server{
				Addr:    fmt.Sprintf(":%d", port),
				Handler: r,
			}

			// Handle graceful shutdown
			go func() {
				sigint := make(chan os.Signal, 1)
				signal.Notify(sigint, os.Interrupt)
				<-sigint

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				srv.Shutdown(ctx)
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
