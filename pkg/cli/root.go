package cli

import (
	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "binance-cli",
		Short: "Binance trade data CLI interface",
		Long: `A command line interface for interacting with Binance trade data.
Provides real-time data viewing, historical data analysis, and visualization capabilities.`,
	}

	// Add subcommands
	cmd.AddCommand(
		newWatchCmd(),
		newStatsCmd(),
		newChartCmd(),
		newHistoryCmd(),
		newSymbolsCmd(),
	)

	return cmd
}
