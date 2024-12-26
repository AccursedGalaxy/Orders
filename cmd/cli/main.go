package main

import (
	"fmt"
	"os"

	"binance-redis-streamer/pkg/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
